#![allow(clippy::items_after_test_module)]

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use async_stream::stream;
use domain::{
    default_parent_builtin_tool_specs, default_subagent_builtin_tool_specs, AgentDefinition,
    ConfigStore, ModelConfig, SafetyConfig, SessionId, SystemPromptSegment, ToolSpec,
};
use futures::{stream::BoxStream, StreamExt};
use mcp::McpRegistry;
use outbound::OutboundEmitter;
use safety::error_tracker::ToolErrorTracker;
use safety::thinking_guard::ThinkingStreamFilter;
use safety::{overthinking_feedback, xml_repair_reminder, SafetyHarness, TurnSafety};
use storage::SubagentTaskRepo;
use tools::{JsonTool, ToolBuildContext};
use tracing::warn;

use crate::compaction;
use crate::history::{
    append_model_message, load_model_history, load_session_context, persist_session_context,
    seed_model_history_from_session_history,
};
use crate::model_client::{ChatModelClient, ModelClientConfig};
use crate::primitives::{
    AgentMessage, FinishReason, MessagePart, ModelRequest, ModelStreamEvent, ToolCall,
};
use crate::rig_tool_registry::{
    build_agent_tools, emit_tool_error, emit_tool_invoked, DynamicTool, ToolContext,
};
use crate::tool_executor::ToolExecutor;
use crate::{AgentEvent, AgentRunner, Result, TurnInput};
use crate::{PlanUpdater, QuestionRequester};

const CREDITS_EXHAUSTED_MESSAGE: &str =
    "Sorry, it seems your organisation ran out of credits. Please visit https://usehivy.com/w to resolve this and you'll be back up and running immediately.";

/// Maximum number of consecutive model failures (request failure, stream
/// failure, cut-off, thinking-only, or empty response) before the turn aborts.
/// Without this cap a persistently failing model endpoint would spin the turn
/// forever and queue every subsequent session message behind it.
const MAX_CONSECUTIVE_MODEL_FAILURES: u32 = 3;

#[derive(Clone)]
enum ToolSettlement {
    Immediate {
        call: ToolCall,
        result: serde_json::Value,
    },
    Execute(ToolCall),
}

pub struct RigAgentRunner {
    config: ConfigStore,
    tool_context: ToolBuildContext,
    outbound_emitter: Option<Arc<OutboundEmitter>>,
    subagent_task_repo: Option<Arc<dyn SubagentTaskRepo>>,
    question_requester: Option<Arc<dyn QuestionRequester>>,
    plan_updater: Option<Arc<dyn PlanUpdater>>,
    event_repo: Option<Arc<dyn storage::EventRepo>>,
    mcp_registry: Option<Arc<McpRegistry>>,
    safety: SafetyHarness,
}

impl RigAgentRunner {
    pub fn new(config: ConfigStore, workspace_root: PathBuf) -> Self {
        Self {
            config,
            tool_context: ToolBuildContext::new(workspace_root),
            outbound_emitter: None,
            subagent_task_repo: None,
            question_requester: None,
            plan_updater: None,
            event_repo: None,
            mcp_registry: None,
            safety: SafetyHarness::new(SafetyConfig::default()),
        }
    }

    pub fn with_outbound_emitter(mut self, emitter: Arc<OutboundEmitter>) -> Self {
        self.outbound_emitter = Some(emitter);
        self
    }

    pub fn with_subagent_task_repo(mut self, repo: Arc<dyn SubagentTaskRepo>) -> Self {
        self.subagent_task_repo = Some(repo);
        self
    }

    pub fn with_question_requester(mut self, requester: Arc<dyn QuestionRequester>) -> Self {
        self.question_requester = Some(requester);
        self
    }

    pub fn with_plan_updater(mut self, updater: Arc<dyn PlanUpdater>) -> Self {
        self.plan_updater = Some(updater);
        self
    }

    pub fn with_event_repo(mut self, event_repo: Arc<dyn storage::EventRepo>) -> Self {
        self.event_repo = Some(event_repo);
        self
    }

    pub fn with_mcp_registry(mut self, registry: Arc<McpRegistry>) -> Self {
        self.mcp_registry = Some(registry);
        self
    }

    pub fn with_safety_config(mut self, config: SafetyConfig) -> Self {
        self.safety = SafetyHarness::new(config);
        self
    }
}

#[async_trait::async_trait]
impl AgentRunner for RigAgentRunner {
    #[allow(unused_assignments)]
    async fn run_turn(
        &self,
        session_id: &SessionId,
        user_input: TurnInput,
        definition_override: Option<Arc<AgentDefinition>>,
    ) -> Result<BoxStream<'static, AgentEvent>> {
        let is_subagent_definition = definition_override.is_some();
        let snapshot = definition_override.unwrap_or_else(|| self.config.snapshot());
        let model_config = pick_model_for_turn(&snapshot, &user_input);
        let runtime_env = self.config.runtime_env();
        let safety_config = snapshot.safety.clone();
        let session_stream_id = user_input.session_stream_id.clone();
        let provided_trace_id = user_input.trace_id.clone();
        let provided_turn_id = user_input.turn_id.clone();
        let ModelClientConfig {
            client,
            model_id,
            cache_policy,
            profile,
            provider_options,
            reasoning_effort,
            temperature,
            max_output_tokens,
        } = build_model_client(&model_config, &runtime_env)?;

        let mut messages = build_initial_messages(
            &snapshot,
            &self.tool_context.workspace_root,
            session_id,
            user_input,
            self.event_repo.as_deref(),
            self.mcp_registry.as_deref(),
        )
        .await?;
        let compaction_config = snapshot.context.compaction.clone();
        if let Some(ref config) = compaction_config {
            if config.enabled {
                let ctx = compaction::CompactContext::from_messages(&messages);
                if compaction::should_compact(&ctx, config) {
                    compaction::compact(&mut messages, config);
                }
            }
        }
        let mut tool_context = self.tool_context.clone();
        tool_context.runtime_env = runtime_env;
        let subagent_task_repo = self.subagent_task_repo.clone();
        let event_repo_for_tools = self.event_repo.clone();
        let process_registry = self.tool_context.process_registry.clone();
        let mcp_registry = self.mcp_registry.clone();

        let tool_specs = effective_tool_specs(&snapshot, is_subagent_definition);
        let mut available_tools = build_all_tools(
            &tool_specs,
            session_id,
            &tool_context,
            &ToolContext {
                subagent_task_repo: subagent_task_repo.clone(),
                question_requester: self.question_requester.clone(),
                plan_updater: self.plan_updater.clone(),
                event_repo: event_repo_for_tools.clone(),
                process_registry: Some(process_registry.clone()),
                mcp_registry: mcp_registry.clone(),
                workspace_root: tool_context.workspace_root.clone(),
                outbound_emitter: self.outbound_emitter.clone(),
                agent_registry: self.config.agent_registry(),
                session_stream_id,
            },
            mcp_registry.clone(),
        );
        available_tools.sort_by(|a, b| a.definition().name.cmp(&b.definition().name));
        let tool_executor = ToolExecutor::new(
            available_tools.clone(),
            snapshot.limits.tool_call_timeout_seconds,
        );

        let max_turns = snapshot.limits.max_turns_per_session.max(1);
        let session_id = session_id.clone();
        let event_repo = self.event_repo.clone();
        let emitter = self.outbound_emitter.clone();
        let safety = SafetyHarness::new(safety_config);
        Ok(Box::pin(stream! {
            let mut final_text = String::new();
            let turn_id = provided_turn_id
                .clone()
                .unwrap_or_else(|| format!("turn-{}", chrono::Utc::now().timestamp_millis()));
            let trace_id = provided_trace_id.clone();
            yield AgentEvent::RunEvent {
                event: "turn_started".to_string(),
                payload: serde_json::json!({
                    "session_id": session_id.as_str(),
                    "turn_id": turn_id,
                    "trace_id": trace_id,
                    "model": model_id,
                }),
            };
            let mut completed_with_final = false;
            let mut effective_turn = 0u32;
            let mut consecutive_empty_responses = 0u32;
            let mut consecutive_model_failures = 0u32;
            let mut cumulative_completion_tokens: u64 = 0;
            let mut turn_safety = TurnSafety::new(&safety);
            let mut error_tracker = ToolErrorTracker::new(profile.max_consecutive_tool_errors);
            let mut consecutive_repeat_rejections = 0u32;
            let mut total_tool_calls = 0u32;
            // Latch so the output-budget warning is injected at most once per
            // turn. Without it the instruction was re-appended on every iteration
            // past 80%, bloating the prompt and wasting tokens it was meant to save.
            let mut budget_warned = false;
            while effective_turn < max_turns {
                // Compaction: check actual conversation size before each request
                if let Some(ref config) = compaction_config {
                    if config.enabled {
                        let current_tokens = compaction::estimate_tokens_static(&messages);
                        let threshold = compaction::effective_token_threshold(config);
                        if current_tokens >= threshold {
                            let tokens_before = current_tokens;
                            compaction::compact(&mut messages, config);
                            let tokens_after = compaction::estimate_tokens_static(&messages);
                            yield AgentEvent::RunEvent {
                                event: "compaction_applied".to_string(),
                                payload: serde_json::json!({
                                    "session_id": session_id.as_str(),
                                    "turn_id": turn_id,
                                    "tokens_before": tokens_before,
                                    "tokens_after": tokens_after,
                                }),
                            };
                        }
                    }
                }

                let definitions = available_tools.iter().map(|tool| tool.definition()).collect();
                let request = ModelRequest {
                    model: model_id.clone(),
                    messages: messages.clone(),
                    tools: definitions,
                    temperature,
                    max_output_tokens,
                    reasoning_effort: reasoning_effort.clone(),
                    cache_policy,
                    profile: profile.clone(),
                    provider_options: provider_options.clone(),
                };

                yield AgentEvent::RunEvent {
                    event: "model_request_started".to_string(),
                    payload: serde_json::json!({
                        "session_id": session_id.as_str(),
                        "turn_id": turn_id,
                        "model": model_id,
                        "messages": messages.len(),
                        "tools": available_tools.len(),
                    }),
                };

                let mut model_stream = match client.stream(request).await {
                    Ok(stream) => stream,
                    Err(error) => {
                        if is_billing_model_error(&error) {
                            yield AgentEvent::RunEvent {
                                event: "model_billing_exhausted".to_string(),
                                payload: serde_json::json!({
                                    "session_id": session_id.as_str(),
                                    "turn_id": turn_id,
                                    "model": model_id,
                                }),
                            };
                            yield AgentEvent::TokenChunk {
                                text: CREDITS_EXHAUSTED_MESSAGE.to_string(),
                            };
                            let assistant =
                                AgentMessage::assistant(CREDITS_EXHAUSTED_MESSAGE.to_string());
                            if let Err(error) =
                                append_model_message(event_repo.as_deref(), &session_id, &assistant)
                                    .await
                            {
                                yield AgentEvent::Error {
                                    message: error.to_string(),
                                };
                                return;
                            }
                            final_text = CREDITS_EXHAUSTED_MESSAGE.to_string();
                            completed_with_final = true;
                            break;
                        }
                        consecutive_model_failures += 1;
                        yield AgentEvent::RunEvent {
                            event: "model_request_failed".to_string(),
                            payload: serde_json::json!({
                                "session_id": session_id.as_str(),
                                "turn_id": turn_id,
                                "model": model_id,
                                "error": error.to_string(),
                                "consecutive_failures": consecutive_model_failures,
                            }),
                        };
                        if consecutive_model_failures >= MAX_CONSECUTIVE_MODEL_FAILURES {
                            yield AgentEvent::Error {
                                message: format!(
                                    "model request failed {consecutive_model_failures} times consecutively: {error}"
                                ),
                            };
                            return;
                        }
                        if consecutive_model_failures % 3 == 0 {
                            messages.push(AgentMessage::user(
                                "[system instruction] The model request failed multiple times. \
                                 This may be a temporary network issue. Please continue where \
                                 you left off — your previous work and the conversation history \
                                 are preserved."
                                    .to_string(),
                            ));
                        }
                        let backoff = std::time::Duration::from_millis(
                            500 * 2u64.saturating_pow(consecutive_model_failures.saturating_sub(1)),
                        ).min(std::time::Duration::from_secs(30));
                        tokio::time::sleep(backoff).await;
                        continue;
                    }
                };

                let mut turn_text = String::new();
                let mut tool_calls: Vec<ToolCall> = Vec::new();
                let mut killed_by_overthinking = false;
                let mut killed_by_stream_failure = false;
                let mut had_thinking = false;
                let mut last_finish_reason: Option<FinishReason> = None;
                // Stateful across the deltas of this model response so a
                // `<think>` tag split across deltas does not leak reasoning into
                // the visible answer.
                let mut thinking_filter = ThinkingStreamFilter::new();
                while let Some(event) = model_stream.next().await {
                    match event {
                        Ok(ModelStreamEvent::TextDelta(text)) => {
                            if safety.config().thinking_strip {
                                let split = thinking_filter.push(&text);
                                if split.had_thinking {
                                    had_thinking = true;
                                }
                                if !split.thinking.is_empty() {
                                    yield AgentEvent::ThinkingChunk { text: split.thinking };
                                }
                                if !split.visible.is_empty() {
                                    turn_text.push_str(&split.visible);
                                    yield AgentEvent::TokenChunk { text: split.visible };
                                }
                            } else {
                                turn_text.push_str(&text);
                                yield AgentEvent::TokenChunk { text };
                            }
                        }
                        Ok(ModelStreamEvent::ThinkingDelta(text)) => {
                            had_thinking = true;
                            yield AgentEvent::ThinkingChunk { text: text.clone() };
                            if safety.config().overthinking.enabled {
                                let status = turn_safety.overthinking.feed(&text);
                                if status.is_overthinking() {
                                    let reason = status.reason();
                                    yield AgentEvent::RunEvent {
                                        event: "overthinking_detected".to_string(),
                                        payload: serde_json::json!({
                                            "session_id": session_id.as_str(),
                                            "turn_id": turn_id,
                                            "model": model_id,
                                            "reason": reason,
                                        }),
                                    };
                                    let feedback = overthinking_feedback(&status);
                                    messages.push(AgentMessage::user(format!(
                                        "[system instruction] {feedback}"
                                    )));
                                    turn_safety.overthinking.reset();
                                    killed_by_overthinking = true;
                                    break;
                                }
                            }
                        }
                        Ok(ModelStreamEvent::ToolCalls(calls)) => tool_calls.extend(calls),
                        Ok(ModelStreamEvent::Usage(usage)) => {
                            cumulative_completion_tokens += usage.completion_tokens.max(0) as u64;
                            yield AgentEvent::RunEvent {
                                event: "model_usage".to_string(),
                                payload: serde_json::json!({
                                    "session_id": session_id.as_str(),
                                    "turn_id": turn_id,
                                    "model": model_id,
                                    "usage": {
                                        "prompt_tokens": usage.prompt_tokens,
                                        "completion_tokens": usage.completion_tokens,
                                        "total_tokens": usage.total_tokens,
                                        "cached_tokens": usage.cached_tokens,
                                        "cache_write_tokens": usage.cache_write_tokens,
                                        "reasoning_tokens": usage.reasoning_tokens,
                                        "cost": usage.cost,
                                    }
                                }),
                            };
                            tracing::info!(
                                prompt_tokens = usage.prompt_tokens,
                                completion_tokens = usage.completion_tokens,
                                total_tokens = usage.total_tokens,
                                cached_tokens = usage.cached_tokens,
                                cache_write_tokens = usage.cache_write_tokens,
                                reasoning_tokens = usage.reasoning_tokens,
                                cost = usage.cost,
                                "model usage"
                            );
                        }
                        Ok(ModelStreamEvent::Done(reason)) => {
                            last_finish_reason = Some(reason);
                        }
                        Err(error) => {
                            consecutive_model_failures += 1;
                            yield AgentEvent::RunEvent {
                                event: "model_stream_failed".to_string(),
                                payload: serde_json::json!({
                                    "session_id": session_id.as_str(),
                                    "turn_id": turn_id,
                                    "model": model_id,
                                    "error": error.to_string(),
                                    "consecutive_failures": consecutive_model_failures,
                                }),
                            };
                            if consecutive_model_failures >= MAX_CONSECUTIVE_MODEL_FAILURES {
                                yield AgentEvent::Error {
                                    message: format!(
                                        "model stream failed {consecutive_model_failures} times consecutively: {error}"
                                    ),
                                };
                                return;
                            }
                            messages.push(AgentMessage::user(
                                "[system instruction] The model stream was interrupted. \
                                 This may be a temporary network issue. Please continue \
                                 where you left off."
                                    .to_string(),
                            ));
                            killed_by_stream_failure = true;
                            break;
                        }
                    }
                }

                // Flush any partial tag the streaming filter was holding back at
                // the end of the response so no content is silently dropped.
                if safety.config().thinking_strip {
                    let tail = thinking_filter.finish();
                    if tail.had_thinking {
                        had_thinking = true;
                    }
                    if !tail.thinking.is_empty() {
                        yield AgentEvent::ThinkingChunk { text: tail.thinking };
                    }
                    if !tail.visible.is_empty() {
                        turn_text.push_str(&tail.visible);
                        yield AgentEvent::TokenChunk { text: tail.visible };
                    }
                }

                // XML tool call repair: detect and extract tool calls from text content.
                // Some providers emit native tool call ids with empty arguments while also
                // streaming the real call as XML text. In that case, keep the provider ids
                // but replace the empty arguments from the XML payload.
                if safety.config().xml_tool_repair && !turn_text.is_empty() {
                    let known_names: Vec<String> = available_tools
                        .iter()
                        .map(|t| t.definition().name.clone())
                        .collect();
                    let (cleaned, xml_calls) = safety
                        .xml_repair()
                        .try_extract_tool_calls(&turn_text, &known_names);
                    if !xml_calls.is_empty() {
                        let repaired = if tool_calls.is_empty() {
                            for xml_call in xml_calls {
                                tool_calls.push(ToolCall {
                                    id: xml_call.id,
                                    name: xml_call.name,
                                    arguments: xml_call.arguments,
                                });
                            }
                            true
                        } else {
                            replace_empty_tool_call_arguments_from_xml(&mut tool_calls, &xml_calls)
                        };
                        if repaired {
                            turn_text = cleaned;
                            let reminder = xml_repair_reminder();
                            yield AgentEvent::ThinkingChunk {
                                text: reminder.clone(),
                            };
                            messages.push(AgentMessage::user(format!(
                                "[system instruction] {reminder}"
                            )));
                        }
                    }
                }

                if killed_by_overthinking {
                    consecutive_empty_responses = 0;
                    continue;
                }

                if killed_by_stream_failure {
                    continue;
                }

                if tool_calls.is_empty() {
                    let reason = last_finish_reason.as_ref();

                    let is_cut_off = reason.is_some_and(|r| r.is_cut_off());
                    // Stream interrupted mid-content (model was producing text, stream died)
                    let is_stream_interrupted = reason.is_none() && !turn_text.is_empty();
                    // Model only produced thinking, no content at all
                    let is_thinking_only = !reason.is_some_and(|r| r.is_complete())
                        && had_thinking
                        && turn_text.is_empty();

                    if is_cut_off || is_stream_interrupted {
                        // Model hit token limit or stream was interrupted mid-content — reprompt aggressively
                        consecutive_empty_responses += 1;
                        yield AgentEvent::RunEvent {
                            event: "model_cut_off".to_string(),
                            payload: serde_json::json!({
                                "session_id": session_id.as_str(),
                                "turn_id": turn_id,
                                "model": model_id,
                                "finish_reason": reason.map(|r| format!("{:?}", r)),
                                "had_thinking": had_thinking,
                                "consecutive": consecutive_empty_responses,
                            }),
                        };
                        if consecutive_empty_responses >= MAX_CONSECUTIVE_MODEL_FAILURES {
                            yield AgentEvent::Error {
                                message: format!(
                                    "model produced no usable response {consecutive_empty_responses} times consecutively (cut off)"
                                ),
                            };
                            return;
                        }
                        messages.push(AgentMessage::user(
                            "[system instruction] Your response was interrupted. \
                             You must act immediately — call a tool, write code, or provide your \
                             final answer. Do not think further; take action now."
                                .to_string(),
                        ));
                        let backoff = std::time::Duration::from_millis(
                            500 * 2u64.saturating_pow(consecutive_empty_responses.saturating_sub(1)),
                        ).min(std::time::Duration::from_secs(30));
                        tokio::time::sleep(backoff).await;
                        continue;
                    }

                    if is_thinking_only {
                        consecutive_empty_responses += 1;
                        yield AgentEvent::RunEvent {
                            event: "model_thinking_only".to_string(),
                            payload: serde_json::json!({
                                "session_id": session_id.as_str(),
                                "turn_id": turn_id,
                                "model": model_id,
                                "consecutive": consecutive_empty_responses,
                            }),
                        };
                        had_thinking = false;
                        if consecutive_empty_responses >= MAX_CONSECUTIVE_MODEL_FAILURES {
                            yield AgentEvent::Error {
                                message: format!(
                                    "model produced only internal reasoning {consecutive_empty_responses} times consecutively"
                                ),
                            };
                            return;
                        }
                        // Per Forgecode's approach: do NOT inject anything into the conversation
                        // for these early retries. Retry with the exact same context — the model
                        // has no knowledge of the failed attempt. This prevents the model from
                        // restarting thinking from scratch in response to injected instructions.
                        // Persistent thinking-only loops are bounded by the cap above.
                        let backoff = std::time::Duration::from_millis(
                            500 * 2u64.saturating_pow(consecutive_empty_responses.saturating_sub(1)),
                        ).min(std::time::Duration::from_secs(30));
                        tokio::time::sleep(backoff).await;
                        continue;
                    }

                    if reason.is_some_and(|r| r.is_complete()) && !turn_text.is_empty() {
                        // Model finished normally with text — this is a real completion
                        consecutive_empty_responses = 0;
                        consecutive_model_failures = 0;
                        had_thinking = false;
                        final_text = turn_text.clone();
                        completed_with_final = true;
                        let assistant = AgentMessage::assistant(turn_text);
                        if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &assistant).await {
                            yield AgentEvent::Error { message: error.to_string() };
                            return;
                        }
                        messages.push(assistant);
                        break;
                    }

                    // Model finished with Stop but no text, or no finish_reason — reprompt
                    consecutive_empty_responses += 1;
                    yield AgentEvent::RunEvent {
                        event: "model_empty_response".to_string(),
                        payload: serde_json::json!({
                            "session_id": session_id.as_str(),
                            "turn_id": turn_id,
                            "model": model_id,
                            "finish_reason": reason.map(|r| format!("{:?}", r)),
                            "had_thinking": had_thinking,
                            "consecutive_empty": consecutive_empty_responses,
                        }),
                    };
                    if consecutive_empty_responses >= MAX_CONSECUTIVE_MODEL_FAILURES {
                        yield AgentEvent::Error {
                            message: format!(
                                "model produced an empty response {consecutive_empty_responses} times consecutively"
                            ),
                        };
                        return;
                    }
                    messages.push(AgentMessage::user(
                        "[system instruction] You produced an empty response. \
                         Please continue working on the task. Call a tool, write code, \
                         or provide your final answer."
                            .to_string(),
                    ));
                    let backoff = std::time::Duration::from_millis(
                        500 * 2u64.saturating_pow(consecutive_empty_responses.saturating_sub(1)),
                    ).min(std::time::Duration::from_secs(30));
                    tokio::time::sleep(backoff).await;
                    continue;
                }

                let assistant_tool_calls = AgentMessage::assistant_tool_calls(tool_calls.clone());
                if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &assistant_tool_calls).await {
                    yield AgentEvent::Error { message: error.to_string() };
                    return;
                }
                messages.push(assistant_tool_calls);
                consecutive_empty_responses = 0;
                consecutive_model_failures = 0;
                had_thinking = false;
                let mut stop_after_tool_calls: Option<String> = None;
                let mut stop_after_immediate_tool_result: Option<String> = None;
                let mut settlements: Vec<ToolSettlement> = Vec::new();
                for call in tool_calls {
                    total_tool_calls = total_tool_calls.saturating_add(1);
                    if total_tool_calls > profile.max_tool_calls_per_turn {
                        stop_after_tool_calls = Some(format!(
                            "I stopped because the model exceeded the {} tool-call limit for the {} model profile.",
                            profile.max_tool_calls_per_turn,
                            profile.id.as_str()
                        ));
                        yield AgentEvent::RunEvent {
                            event: "tool_call_limit_exceeded".to_string(),
                            payload: serde_json::json!({
                                "session_id": session_id.as_str(),
                                "turn_id": turn_id,
                                "model": model_id,
                                "model_profile": profile.id.as_str(),
                                "limit": profile.max_tool_calls_per_turn,
                            }),
                        };
                        break;
                    }
                    yield AgentEvent::ToolCall { id: call.id.clone(), tool: call.name.clone(), args: call.arguments.clone() };

                    if safety.config().repeat_detection.enabled {
                        if let Some(error_msg) = turn_safety.repeat_detector.check(&call.name, &call.arguments) {
                            consecutive_repeat_rejections += 1;
                            yield AgentEvent::RunEvent {
                                event: "repeat_tool_call_rejected".to_string(),
                                payload: serde_json::json!({
                                    "session_id": session_id.as_str(),
                                    "turn_id": turn_id,
                                    "tool": call.name.clone(),
                                    "reason": error_msg,
                                }),
                            };
                            let result = json_safe_error(&error_msg);
                            settlements.push(ToolSettlement::Immediate { call: call.clone(), result });
                            if consecutive_repeat_rejections >= profile.max_consecutive_tool_errors {
                                stop_after_immediate_tool_result = Some(call.id.clone());
                                stop_after_tool_calls = Some(format!(
                                    "I stopped because the model repeatedly issued the same invalid `{}` tool call. Please retry with a different model or request.",
                                    call.name
                                ));
                                break;
                            }
                            continue;
                        }
                    }
                    consecutive_repeat_rejections = 0;

                    settlements.push(ToolSettlement::Execute(call));
                }

                let mut settlement_index = 0usize;
                while settlement_index < settlements.len() {
                    let executions = match &settlements[settlement_index] {
                        ToolSettlement::Immediate { call, result } => {
                            yield AgentEvent::ToolResult { id: call.id.clone(), result: result.clone() };
                            let message = AgentMessage::tool_result(call.id.clone(), result.to_string());
                            if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &message).await {
                                yield AgentEvent::Error { message: error.to_string() };
                                return;
                            }
                            messages.push(message);
                            if stop_after_immediate_tool_result.as_deref() == Some(call.id.as_str()) {
                                break;
                            }
                            settlement_index += 1;
                            continue;
                        }
                        ToolSettlement::Execute(call) => {
                            if call.name == "subagent_task" {
                                let start = settlement_index;
                                let mut end = start;
                                while end < settlements.len() {
                                    match &settlements[end] {
                                        ToolSettlement::Execute(call) if call.name == "subagent_task" => {
                                            end += 1;
                                        }
                                        _ => break,
                                    }
                                }
                                let calls: Vec<ToolCall> = settlements[start..end]
                                    .iter()
                                    .filter_map(|settlement| match settlement {
                                        ToolSettlement::Execute(call) => Some(call.clone()),
                                        ToolSettlement::Immediate { .. } => None,
                                    })
                                    .collect();
                                settlement_index = end;
                                futures::future::join_all(calls.iter().map(|call| {
                                    let tool_executor = &tool_executor;
                                    async move { (call.clone(), tool_executor.execute(call).await) }
                                }))
                                .await
                            } else {
                                let call = call.clone();
                                settlement_index += 1;
                                vec![(call.clone(), tool_executor.execute(&call).await)]
                            }
                        }
                    };

                    for (call, execution) in executions {
                        match execution {
                            Ok(result) => {
                                error_tracker.reset(&call.name);
                                emit_tool_invoked(emitter.clone(), &session_id, &call.name, &call.arguments, &result).await;
                                yield AgentEvent::ToolResult { id: call.id.clone(), result: result.clone() };
                                let message = AgentMessage::tool_result(call.id, result.to_string());
                                if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &message).await {
                                    yield AgentEvent::Error { message: error.to_string() };
                                    return;
                                }
                                messages.push(message);
                            }
                            Err(error) => {
                                let tool_failure_count = error_tracker.record_failure(&call.name);
                                let raw_error = error.raw_message();
                                capture_tool_error(&session_id, &call.name, &call.arguments, &raw_error);
                                let error_msg = error_tracker.format_retry_hint(&call.name, &raw_error);
                                emit_tool_error(emitter.clone(), &session_id, &call.name, &call.arguments, &error_msg).await;
                                let result = if error.is_safe_argument_error() {
                                    json_safe_error(&error_msg)
                                } else {
                                    json_error(&error_msg)
                                };
                                yield AgentEvent::ToolResult { id: call.id.clone(), result: result.clone() };
                                let message = AgentMessage::tool_result(call.id, result.to_string());
                                if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &message).await {
                                    yield AgentEvent::Error { message: error.to_string() };
                                    return;
                                }
                                messages.push(message);
                                if tool_failure_count >= profile.max_consecutive_tool_errors {
                                    stop_after_tool_calls = Some(format!(
                                        "I stopped because `{}` failed {} times in a row.",
                                        call.name,
                                        profile.max_consecutive_tool_errors
                                    ));
                                    yield AgentEvent::RunEvent {
                                        event: "tool_error_limit_exceeded".to_string(),
                                        payload: serde_json::json!({
                                            "session_id": session_id.as_str(),
                                            "turn_id": turn_id,
                                            "tool": call.name,
                                            "limit": profile.max_consecutive_tool_errors,
                                        }),
                                    };
                                    break;
                                }
                            }
                        }
                    }
                    if stop_after_tool_calls.is_some() {
                        break;
                    }
                }
                if let Some(message) = stop_after_tool_calls {
                    final_text = message.clone();
                    completed_with_final = true;
                    let assistant = AgentMessage::assistant(message);
                    if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &assistant).await {
                        yield AgentEvent::Error { message: error.to_string() };
                        return;
                    }
                    messages.push(assistant);
                    break;
                }
                if !budget_warned
                    && cumulative_completion_tokens
                        >= snapshot.limits.output_token_budget as u64 * 80 / 100
                {
                    budget_warned = true;
                    yield AgentEvent::RunEvent {
                        event: "output_budget_warning".to_string(),
                        payload: serde_json::json!({
                            "session_id": session_id.as_str(),
                            "turn_id": turn_id,
                            "completion_tokens": cumulative_completion_tokens,
                            "output_token_budget": snapshot.limits.output_token_budget,
                        }),
                    };
                    messages.push(AgentMessage::user(format!(
                        "[system instruction] Approaching output token budget ({} of {} used). \
                         Be concise and prioritize completing the task now.",
                        cumulative_completion_tokens, snapshot.limits.output_token_budget
                    )));
                }
                effective_turn += 1;
            }

            if !completed_with_final {
                yield AgentEvent::RunEvent {
                    event: "max_turns_exhausted".to_string(),
                    payload: serde_json::json!({
                        "session_id": session_id.as_str(),
                        "turn_id": turn_id,
                        "limit": max_turns,
                        "model": model_id,
                    }),
                };
                if final_text.is_empty() {
                    final_text = "[Agent reached the turn limit. The task may be partially complete.]".to_string();
                }
            }

            yield AgentEvent::RunEvent {
                event: "turn_completed".to_string(),
                payload: serde_json::json!({
                    "session_id": session_id.as_str(),
                    "turn_id": turn_id,
                    "text_len": final_text.len(),
                }),
            };
            yield AgentEvent::FinalMessage { text: final_text };
        }))
    }

    fn active_background_processes(&self, session_id: &SessionId) -> usize {
        self.tool_context
            .process_registry
            .running_for_session(session_id.as_str())
    }
}

fn is_billing_model_error(error: &crate::AgentError) -> bool {
    let crate::AgentError::Model(message) = error else {
        return false;
    };
    let message = message.to_ascii_lowercase();
    message.contains("(billing)")
        || message.contains("http 402")
        || message.contains("insufficient credit")
        || message.contains("insufficient balance")
        || message.contains("payment required")
}

async fn build_initial_messages(
    snapshot: &AgentDefinition,
    workspace_root: &std::path::Path,
    session_id: &SessionId,
    input: TurnInput,
    event_repo: Option<&dyn storage::EventRepo>,
    mcp_registry: Option<&McpRegistry>,
) -> Result<Vec<AgentMessage>> {
    let session_context = if input.session_context.is_empty() {
        match load_session_context(event_repo, session_id).await {
            Ok(context) => context,
            Err(error) => {
                capture_preloaded_context_error(session_id, "load", &error.to_string());
                Vec::new()
            }
        }
    } else {
        if let Err(error) =
            persist_session_context(event_repo, session_id, &input.session_context).await
        {
            capture_preloaded_context_error(session_id, "persist", &error.to_string());
        }
        input.session_context.clone()
    };
    let mut messages = vec![
        AgentMessage::system(render_cacheable_system_prompt(snapshot)),
        AgentMessage::system(
            render_dynamic_system_prompt(snapshot, workspace_root, mcp_registry, &session_context)
                .await,
        ),
    ];
    let mut history = load_model_history(event_repo, session_id, 1000).await?;
    if history.is_empty() && !input.prior_history.is_empty() {
        history =
            seed_model_history_from_session_history(event_repo, session_id, &input.prior_history)
                .await?;
    }
    messages.extend(history);
    let mut user = AgentMessage::user(input.text);
    for image in input.images {
        user.push_part(MessagePart::InlineData {
            mime_type: image.mime_type,
            data: image.data,
        });
    }
    append_model_message(event_repo, session_id, &user).await?;
    messages.push(user);
    Ok(messages)
}

fn render_cacheable_system_prompt(snapshot: &AgentDefinition) -> String {
    let mut prompt = String::new();
    for segment in &snapshot.system_prompt.cacheable_segments {
        append_rendered_segment(&mut prompt, render_static_segment(segment));
    }
    prompt
}

async fn render_dynamic_system_prompt(
    snapshot: &AgentDefinition,
    workspace_root: &std::path::Path,
    mcp_registry: Option<&McpRegistry>,
    session_context: &[String],
) -> String {
    let mut prompt = String::new();
    let skill_store = skills::SkillStore::new(workspace_root);
    let skill_summaries = skill_store.summaries(None);
    let mcp_tools = match mcp_registry {
        Some(registry) => registry.available_tool_names(),
        None => Vec::new(),
    };
    let renders_legacy_context_segment = snapshot
        .system_prompt
        .dynamic_segments
        .iter()
        .any(|segment| matches!(segment, SystemPromptSegment::DynamicContext(_)));
    if !renders_legacy_context_segment {
        append_rendered_segment(&mut prompt, render_session_context(session_context));
    }
    for segment in &snapshot.system_prompt.dynamic_segments {
        let rendered = match segment {
            SystemPromptSegment::StaticText(_) => render_static_segment(segment),
            SystemPromptSegment::DynamicContext(config) => {
                render_dynamic_context_segment(config, session_context)
            }
            SystemPromptSegment::SkillCatalog(config) => {
                render_skill_catalog_segment(config, &skill_summaries)
            }
            SystemPromptSegment::McpTools(config) => render_tool_list_segment(config, &mcp_tools),
        };
        append_rendered_segment(&mut prompt, rendered);
    }

    tracing::info!(
        mcp_tool_count = mcp_tools.len(),
        skill_count = skill_summaries.len(),
        prompt_len = prompt.len(),
        "system prompt augmented with skill and MCP tool catalog"
    );
    prompt
}

fn render_session_context(session_context: &[String]) -> Option<String> {
    let sections = session_context
        .iter()
        .map(|context| context.trim())
        .filter(|context| !context.is_empty())
        .collect::<Vec<_>>();
    if sections.is_empty() {
        None
    } else {
        Some(sections.join("\n\n"))
    }
}

fn append_rendered_segment(prompt: &mut String, rendered: Option<String>) {
    let Some(rendered) = rendered else {
        return;
    };
    let rendered = rendered.trim();
    if rendered.is_empty() {
        return;
    }
    if !prompt.is_empty() {
        prompt.push_str("\n\n");
    }
    prompt.push_str(rendered);
}

fn render_static_segment(segment: &SystemPromptSegment) -> Option<String> {
    let SystemPromptSegment::StaticText(config) = segment else {
        return None;
    };
    Some(render_section(&config.title, &config.content))
}

fn render_dynamic_context_segment(
    config: &domain::DynamicContextPromptSegment,
    dynamic_context: &[String],
) -> Option<String> {
    let mut items = Vec::new();
    for context in dynamic_context {
        let content = context.trim();
        if content.is_empty() {
            continue;
        }
        items.push(apply_template(
            &config.item_template,
            &[("content", content)],
        ));
    }
    if items.is_empty() {
        let content = config.preamble.trim();
        if !config.title.trim().is_empty() || !content.is_empty() {
            return Some(render_section(&config.title, content));
        }
        return None;
    }
    render_item_section(&config.title, &config.preamble, &[], &items, &[])
}

fn render_skill_catalog_segment(
    config: &domain::ListPromptSegment,
    skills: &[skills::SkillSummary],
) -> Option<String> {
    let items = skills
        .iter()
        .map(|skill| {
            apply_template(
                &config.item_template,
                &[
                    ("name", skill.name.as_str()),
                    ("description", skill.description.as_str()),
                ],
            )
        })
        .collect::<Vec<_>>();
    render_item_section(&config.title, &config.preamble, &[], &items, &[])
}

fn render_tool_list_segment(
    config: &domain::ListPromptSegment,
    tools: &[String],
) -> Option<String> {
    let items = tools
        .iter()
        .map(|name| apply_template(&config.item_template, &[("name", name.as_str())]))
        .collect::<Vec<_>>();
    render_item_section(&config.title, &config.preamble, &[], &items, &[])
}

fn render_item_section(
    title: &str,
    preamble: &str,
    before_items: &[String],
    items: &[String],
    after_items: &[String],
) -> Option<String> {
    if items.is_empty() {
        return None;
    }
    let mut lines = Vec::new();
    let preamble = preamble.trim();
    if !preamble.is_empty() {
        lines.push(preamble.to_string());
    }
    lines.extend(before_items.iter().filter_map(|line| nonempty_line(line)));
    lines.extend(items.iter().filter_map(|line| nonempty_line(line)));
    lines.extend(after_items.iter().filter_map(|line| nonempty_line(line)));
    if lines.is_empty() {
        return None;
    }
    Some(render_section(title, &lines.join("\n")))
}

fn render_section(title: &str, content: &str) -> String {
    let title = title.trim();
    let content = content.trim();
    if title.is_empty() {
        content.to_string()
    } else if content.is_empty() {
        format!("## {title}")
    } else {
        format!("## {title}\n{content}")
    }
}

fn nonempty_line(value: &str) -> Option<String> {
    let value = value.trim();
    if value.is_empty() {
        None
    } else {
        Some(value.to_string())
    }
}

fn apply_template(template: &str, replacements: &[(&str, &str)]) -> String {
    let mut output = template.to_string();
    for (key, value) in replacements {
        output = output.replace(&format!("{{{key}}}"), value.trim());
    }
    output
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;

    use axum::{http::StatusCode, response::IntoResponse, routing::post, Json, Router};
    use domain::{
        AgentMeta, ContextConfig, Limits, ListPromptSegment, ModelConfig, StaticPromptSegment,
        SystemPromptConfig,
    };
    use futures::StreamExt;
    use tokio::net::TcpListener;

    use crate::tool_executor::{is_safe_tool_argument_error, missing_required_argument_message};

    use super::*;

    fn test_definition() -> AgentDefinition {
        AgentDefinition {
            agent: AgentMeta {
                name: "Ari".to_string(),
                description: "Engineering teammate".to_string(),
            },
            system_prompt: test_system_prompt(),
            model: ModelConfig::OpenaiCompatible {
                base_url: "http://localhost".to_string(),
                model_id: "test".to_string(),
                canonical_model_id: Some("test".to_string()),
                provider_id: Some("openrouter".to_string()),
                upstream_model_id: Some("test".to_string()),
                model_profile: None,
                provider_options: HashMap::new(),
                capabilities: None,
                api_key_env: "TEST_API_KEY".to_string(),
                temperature: None,
                max_output_tokens: None,
                reasoning_effort: None,
                extra_headers: HashMap::new(),
                fallback: None,
            },
            multimodal_model: None,
            limits: Limits::default(),
            context: ContextConfig::default(),
            tools: Some(Vec::new()),
            mcp_servers: Vec::new(),
            skills: Vec::new(),
            outbound_channels: Vec::new(),
            sub_agents: Default::default(),
            safety: Default::default(),
        }
    }

    fn tool_kind(spec: &ToolSpec) -> &'static str {
        match spec {
            ToolSpec::Bash(_) => "bash",
            ToolSpec::ReadFile(_) => "read_file",
            ToolSpec::WriteFile(_) => "write_file",
            ToolSpec::FileSearch(_) => "file_search",
            ToolSpec::Glob(_) => "glob",
            ToolSpec::Grep(_) => "grep",
            ToolSpec::MultiGrep(_) => "multi_grep",
            ToolSpec::ApplyPatch(_) => "apply_patch",
            ToolSpec::Lsp(_) => "lsp",
            ToolSpec::SubagentTask(_) => "subagent_task",
            ToolSpec::CheckBashStatus => "check_bash_status",
            ToolSpec::SkillsList => "skills_list",
            ToolSpec::SkillView => "skill_view",
            ToolSpec::SkillManage => "skill_manage",
            ToolSpec::SearchSessions => "search_sessions",
            ToolSpec::RequestUserInput => "request_user_input",
            ToolSpec::UpdatePlan => "update_plan",
        }
    }

    fn has_tool(specs: &[ToolSpec], kind: &str) -> bool {
        specs.iter().any(|spec| tool_kind(spec) == kind)
    }

    #[test]
    fn missing_parent_tools_default_to_all_builtin_tools() {
        let mut definition = test_definition();
        definition.tools = None;

        let specs = effective_tool_specs(&definition, false);

        for kind in [
            "bash",
            "read_file",
            "write_file",
            "subagent_task",
            "check_bash_status",
            "skills_list",
            "skill_view",
            "skill_manage",
            "search_sessions",
            "request_user_input",
            "update_plan",
        ] {
            assert!(has_tool(&specs, kind), "missing {kind}");
        }
    }

    #[test]
    fn missing_subagent_tools_default_without_root_orchestration_tools() {
        let mut definition = test_definition();
        definition.tools = None;

        let specs = effective_tool_specs(&definition, true);

        for kind in [
            "bash",
            "read_file",
            "write_file",
            "check_bash_status",
            "skills_list",
            "skill_view",
            "skill_manage",
            "search_sessions",
            "update_plan",
        ] {
            assert!(has_tool(&specs, kind), "missing {kind}");
        }
        for kind in ["subagent_task", "request_user_input"] {
            assert!(!has_tool(&specs, kind), "unexpected {kind}");
        }
    }

    #[test]
    fn explicit_subagent_tools_are_filtered_from_child_agents() {
        let mut definition = test_definition();
        definition.tools = Some(vec![
            ToolSpec::SkillsList,
            ToolSpec::SubagentTask(Default::default()),
            ToolSpec::RequestUserInput,
            ToolSpec::UpdatePlan,
        ]);

        let specs = effective_tool_specs(&definition, true);

        assert!(has_tool(&specs, "skills_list"));
        assert!(has_tool(&specs, "update_plan"));
        for kind in ["subagent_task", "request_user_input"] {
            assert!(!has_tool(&specs, kind), "unexpected {kind}");
        }
    }

    #[test]
    fn explicit_empty_tools_remain_empty() {
        let definition = test_definition();

        let specs = effective_tool_specs(&definition, false);

        assert!(specs.is_empty());
    }

    #[test]
    fn tool_argument_summary_keeps_keys_without_values() {
        let summary = summarize_tool_arguments(&serde_json::json!({
            "query": "secret customer question",
            "api_key": "should not be sent",
        }));

        assert!(summary.contains("api_key"));
        assert!(summary.contains("query"));
        assert!(!summary.contains("secret customer question"));
        assert!(!summary.contains("should not be sent"));
    }

    #[test]
    fn xml_repair_replaces_empty_native_tool_arguments() {
        let repair = safety::xml_tool_repair::XmlToolCallRepair::new();
        let content = r#"<tool_call>
<function=update_plan>
<parameter=plan>[{"status":"in_progress","step":"Inspect"}]</parameter>
</function>
</tool_call>"#;
        let known_tools = vec!["update_plan".to_string()];
        let (_cleaned, xml_calls) = repair.try_extract_tool_calls(content, &known_tools);
        let mut native_calls = vec![ToolCall {
            id: "call_native".to_string(),
            name: "update_plan".to_string(),
            arguments: serde_json::json!({}),
        }];

        assert!(replace_empty_tool_call_arguments_from_xml(
            &mut native_calls,
            &xml_calls
        ));
        assert_eq!(native_calls[0].id, "call_native");
        assert!(native_calls[0].arguments["plan"].is_array());
        assert_eq!(native_calls[0].arguments["plan"][0]["step"], "Inspect");
    }

    #[test]
    fn xml_repair_handles_exact_mimo_production_tool_call_samples() {
        let repair = safety::xml_tool_repair::XmlToolCallRepair::new();
        let samples = vec![
            (
                r#"<tool_call>
<function=read_file>
<parameter=path>/workspace/repos/hivy/Makefile</parameter>
<parameter=limit>100</parameter>
</function>
</tool_call>"#,
                "read_file",
                "path",
            ),
            (
                r#"<tool_call>
<function=update_plan>
<parameter=explanation>Setting up the Hivy repository for local development following the AGENTS.md instructions</parameter>
<parameter=plan>[{"status": "in_progress", "step": "Copy .env.example to .env"}, {"status": "pending", "step": "Check Docker availability"}, {"status": "pending", "step": "Start the stack with make up"}, {"status": "pending", "step": "Verify all services are running"}]</parameter>
</function>
</tool_call>"#,
                "update_plan",
                "plan",
            ),
            (
                r#"<tool_call>
<function=update_plan>
<parameter=explanation>Setting up the hivy repo requires: .env file, Go toolchain, and Docker Compose to bring up the full local stack (API, worker, web, Postgres, Redis, Nango, Qdrant, MinIO).</parameter>
<parameter=plan>[{"status": "in_progress", "step": "Copy .env.example to .env"}, {"status": "pending", "step": "Check prerequisites: Docker, Go, Node"}, {"status": "pending", "step": "Generate real local secrets for .env"}, {"status": "pending", "step": "Pull Docker images and start the stack with make up"}, {"status": "pending", "step": "Verify all services are healthy (API, worker, web, DBs)"}]</parameter>
</function>
</tool_call>"#,
                "update_plan",
                "plan",
            ),
            (
                r#"<tool_call>
<function=update_plan>
<parameter=explanation>Set up the hivy local development stack per AGENTS.md: copy env, verify prereqs, run make up, and verify services.</parameter>
<parameter=plan>[{"status": "in_progress", "step": "Copy .env.example to .env"}, {"status": "pending", "step": "Check prerequisites (Docker, Go, make)"}, {"status": "pending", "step": "Pull Docker images and start the stack with make up"}, {"status": "pending", "step": "Verify all services are healthy (API, Worker, Web, Postgres, Redis, etc.)"}]</parameter>
</function>
</tool_call>"#,
                "update_plan",
                "plan",
            ),
            (
                r#"<tool_call>
<function=update_plan>
<parameter=explanation>Setting up the hivy repo for local development per AGENTS.md instructions: copy .env.example to .env, verify Docker is running, then `make up`.</parameter>
<parameter=plan>[{"step": "Check prerequisites (Docker, Go, Make)", "status": "in_progress"}, {"step": "Copy .env.example to .env", "status": "pending"}, {"step": "Pull latest changes (git pull)", "status": "pending"}, {"step": "Run make up to start the full local stack", "status": "pending"}, {"step": "Verify services are healthy (API, worker, web, etc.)", "status": "pending"}]</parameter>
</function>
</tool_call>"#,
                "update_plan",
                "plan",
            ),
        ];
        let known_tools = vec!["read_file".to_string(), "update_plan".to_string()];

        for (content, tool_name, expected_key) in samples {
            let (_cleaned, xml_calls) = repair.try_extract_tool_calls(content, &known_tools);
            let mut native_calls = vec![ToolCall {
                id: format!("call_{tool_name}"),
                name: tool_name.to_string(),
                arguments: serde_json::json!({}),
            }];

            assert!(
                replace_empty_tool_call_arguments_from_xml(&mut native_calls, &xml_calls),
                "expected repair for {tool_name}"
            );
            assert_eq!(native_calls[0].id, format!("call_{tool_name}"));
            assert!(
                native_calls[0].arguments.get(expected_key).is_some(),
                "missing expected key {expected_key} in repaired {tool_name} args"
            );
            if tool_name == "update_plan" {
                assert!(
                    native_calls[0].arguments["plan"].is_array(),
                    "MiMo update_plan sample must repair plan as an array"
                );
            }
        }
    }

    #[test]
    fn missing_required_argument_message_is_user_safe() {
        let definition = tools::ToolDefinition {
            name: "read_file".to_string(),
            description: "Read a file".to_string(),
            parameters: serde_json::json!({
                "type": "object",
                "required": ["path"],
                "properties": {
                    "path": {"type": "string"}
                }
            }),
        };

        let message = missing_required_argument_message(&definition, &serde_json::json!({}))
            .expect("missing path should be detected");
        assert!(message.contains("read_file"));
        assert!(message.contains("path"));
    }

    #[test]
    fn invalid_tool_argument_errors_are_safe_to_surface() {
        assert!(is_safe_tool_argument_error(
            "invalid update_plan arguments: invalid type: string, expected a sequence"
        ));
        assert!(!is_safe_tool_argument_error(
            "process exited with status 1: secret command output"
        ));
    }

    #[test]
    fn cacheable_prompt_uses_control_plane_segments_and_ignores_deprecated_fields() {
        let prompt = render_cacheable_system_prompt(&test_definition());

        assert!(prompt.contains("Runtime-owned base prompt from control plane."));
        assert!(prompt.contains("## Control-plane identity"));
        assert!(prompt.contains("Use the injected identity segment."));
        assert!(!prompt.contains("<@U123ABC> can you confirm the deploy window?"));
        assert!(!prompt.contains("Company name: ExampleCo"));
        assert!(!prompt.contains("Prefer source-grounded answers."));
    }

    #[tokio::test]
    async fn dynamic_prompt_contains_session_context_without_placeholder() {
        let definition = test_definition();
        let prompt = render_dynamic_system_prompt(
            &definition,
            std::path::Path::new("/tmp"),
            None,
            &["## Channel-specific instruction\nKeep replies short.".to_string()],
        )
        .await;

        assert!(prompt.contains("Keep replies short."));
        assert!(!prompt.contains("## Runtime Context"));
        assert!(!prompt.contains("Company name: ExampleCo"));
    }

    #[tokio::test]
    async fn billing_model_failure_completes_with_user_facing_credit_message() {
        async fn billing_handler() -> impl IntoResponse {
            (
                StatusCode::PAYMENT_REQUIRED,
                Json(serde_json::json!({
                    "error": {
                        "message": "insufficient credits"
                    }
                })),
            )
        }

        let listener = TcpListener::bind("127.0.0.1:0").await.expect("bind");
        let base_url = format!("http://{}", listener.local_addr().expect("local addr"));
        let app = Router::new().route("/chat/completions", post(billing_handler));
        tokio::spawn(async move {
            axum::serve(listener, app).await.expect("model server");
        });

        let mut definition = test_definition();
        let ModelConfig::OpenaiCompatible { base_url: url, .. } = &mut definition.model;
        *url = base_url;
        let config = ConfigStore::with_runtime_env(
            definition,
            HashMap::from([("TEST_API_KEY".to_string(), "test-key".to_string())]),
        );
        let runner = RigAgentRunner::new(config, std::env::temp_dir());

        let mut stream = runner
            .run_turn(
                &SessionId::from("billing-session"),
                TurnInput::text("hello"),
                None,
            )
            .await
            .expect("run turn");
        let mut events = Vec::new();
        while let Some(event) = stream.next().await {
            events.push(event);
        }

        assert!(events.iter().any(|event| matches!(
            event,
            AgentEvent::RunEvent { event, .. } if event == "model_billing_exhausted"
        )));
        assert!(events.iter().any(|event| matches!(
            event,
            AgentEvent::TokenChunk { text } if text == CREDITS_EXHAUSTED_MESSAGE
        )));
        assert!(events.iter().any(|event| matches!(
            event,
            AgentEvent::FinalMessage { text } if text == CREDITS_EXHAUSTED_MESSAGE
        )));
        assert!(!events.iter().any(|event| {
            matches!(event, AgentEvent::Error { .. })
                || matches!(
                    event,
                    AgentEvent::RunEvent { event, .. } if event == "model_request_failed"
                )
        }));
    }

    #[tokio::test]
    async fn empty_responses_abort_turn_instead_of_looping_forever() {
        use std::sync::atomic::{AtomicUsize, Ordering};
        // Server always returns a well-formed SSE stream that finishes with `stop`
        // but no content — the `model_empty_response` path. Without a cap the
        // runner would reprompt forever; it must abort after the cap.
        let hits = Arc::new(AtomicUsize::new(0));
        let hits_handler = hits.clone();
        async fn empty_handler(
            axum::extract::State(hits): axum::extract::State<Arc<AtomicUsize>>,
        ) -> impl IntoResponse {
            hits.fetch_add(1, Ordering::SeqCst);
            (
                [(axum::http::header::CONTENT_TYPE, "text/event-stream")],
                "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n\
                 data: [DONE]\n\n"
                    .to_string(),
            )
        }

        let listener = TcpListener::bind("127.0.0.1:0").await.expect("bind");
        let base_url = format!("http://{}", listener.local_addr().expect("local addr"));
        let app = Router::new()
            .route("/chat/completions", post(empty_handler))
            .with_state(hits_handler);
        tokio::spawn(async move {
            axum::serve(listener, app).await.expect("model server");
        });

        let mut definition = test_definition();
        let ModelConfig::OpenaiCompatible { base_url: url, .. } = &mut definition.model;
        *url = base_url;
        let config = ConfigStore::with_runtime_env(
            definition,
            HashMap::from([("TEST_API_KEY".to_string(), "test-key".to_string())]),
        );
        let runner = RigAgentRunner::new(config, std::env::temp_dir());

        let mut stream = runner
            .run_turn(
                &SessionId::from("empty-session"),
                TurnInput::text("hello"),
                None,
            )
            .await
            .expect("run turn");

        // The turn must terminate (not hang) and emit an Error.
        let events = tokio::time::timeout(std::time::Duration::from_secs(10), async {
            let mut events = Vec::new();
            while let Some(event) = stream.next().await {
                events.push(event);
            }
            events
        })
        .await
        .expect("turn must terminate, not loop forever");

        assert!(
            events
                .iter()
                .any(|event| matches!(event, AgentEvent::Error { .. })),
            "expected an Error event after the empty-response cap, got: {events:?}"
        );
        // The model must have been called a bounded number of times, not forever.
        assert!(
            hits.load(Ordering::SeqCst) <= 4,
            "empty-response retries must be capped, got {} calls",
            hits.load(Ordering::SeqCst)
        );
    }

    #[tokio::test]
    async fn request_failures_abort_turn_instead_of_looping_forever() {
        // Server always returns 500 (retryable Server class). The model client
        // exhausts its internal retries and surfaces a request failure to the
        // runner on every turn; the runner must cap consecutive request failures
        // and abort rather than spinning forever.
        async fn error_handler() -> impl IntoResponse {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(serde_json::json!({"error": {"message": "boom"}})),
            )
        }

        let listener = TcpListener::bind("127.0.0.1:0").await.expect("bind");
        let base_url = format!("http://{}", listener.local_addr().expect("local addr"));
        let app = Router::new().route("/chat/completions", post(error_handler));
        tokio::spawn(async move {
            axum::serve(listener, app).await.expect("model server");
        });

        let mut definition = test_definition();
        let ModelConfig::OpenaiCompatible { base_url: url, .. } = &mut definition.model;
        *url = base_url;
        let config = ConfigStore::with_runtime_env(
            definition,
            HashMap::from([("TEST_API_KEY".to_string(), "test-key".to_string())]),
        );
        let runner = RigAgentRunner::new(config, std::env::temp_dir());

        let mut stream = runner
            .run_turn(
                &SessionId::from("error-session"),
                TurnInput::text("hello"),
                None,
            )
            .await
            .expect("run turn");

        let events = tokio::time::timeout(std::time::Duration::from_secs(30), async {
            let mut events = Vec::new();
            while let Some(event) = stream.next().await {
                events.push(event);
            }
            events
        })
        .await
        .expect("turn must terminate, not loop forever");

        assert!(
            events
                .iter()
                .any(|event| matches!(event, AgentEvent::Error { .. })),
            "expected an Error event after the request-failure cap, got: {events:?}"
        );
    }

    #[tokio::test]
    async fn output_budget_warning_is_latched_to_a_single_emission() {
        // Every model response is a tool call (so the loop iterates) plus a usage
        // event whose completion tokens alone exceed 80% of the budget. The
        // warning instruction must be injected at most once across all
        // iterations, not re-appended every turn past the threshold.
        async fn tool_call_handler() -> impl IntoResponse {
            (
                [(axum::http::header::CONTENT_TYPE, "text/event-stream")],
                // usage.completion_tokens = 7000 ≥ 80% of an 8000 budget.
                "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"does_not_exist\",\"arguments\":\"{}\"}}]}}]}\n\n\
                 data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":7000,\"total_tokens\":7001}}\n\n\
                 data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
                 data: [DONE]\n\n"
                    .to_string(),
            )
        }

        let listener = TcpListener::bind("127.0.0.1:0").await.expect("bind");
        let base_url = format!("http://{}", listener.local_addr().expect("local addr"));
        let app = Router::new().route("/chat/completions", post(tool_call_handler));
        tokio::spawn(async move {
            axum::serve(listener, app).await.expect("model server");
        });

        let mut definition = test_definition();
        // Keep the loop short and the budget small so the threshold trips early.
        definition.limits.max_turns_per_session = 4;
        definition.limits.output_token_budget = 8_000;
        let ModelConfig::OpenaiCompatible { base_url: url, .. } = &mut definition.model;
        *url = base_url;
        let config = ConfigStore::with_runtime_env(
            definition,
            HashMap::from([("TEST_API_KEY".to_string(), "test-key".to_string())]),
        );
        let runner = RigAgentRunner::new(config, std::env::temp_dir());

        let mut stream = runner
            .run_turn(
                &SessionId::from("budget-session"),
                TurnInput::text("hi"),
                None,
            )
            .await
            .expect("run turn");

        let events = tokio::time::timeout(std::time::Duration::from_secs(15), async {
            let mut events = Vec::new();
            while let Some(event) = stream.next().await {
                events.push(event);
            }
            events
        })
        .await
        .expect("turn must terminate");

        let warnings = events
            .iter()
            .filter(|event| {
                matches!(
                    event,
                    AgentEvent::RunEvent { event, .. } if event == "output_budget_warning"
                )
            })
            .count();
        assert_eq!(
            warnings, 1,
            "budget warning must be latched to exactly one emission, got {warnings}"
        );
    }

    fn test_system_prompt() -> SystemPromptConfig {
        SystemPromptConfig {
            cacheable_segments: vec![
                SystemPromptSegment::StaticText(StaticPromptSegment {
                    title: String::new(),
                    content: "Runtime-owned base prompt from control plane.".to_string(),
                }),
                SystemPromptSegment::StaticText(StaticPromptSegment {
                    title: "Control-plane identity".to_string(),
                    content: "Use the injected identity segment.".to_string(),
                }),
            ],
            dynamic_segments: vec![
                SystemPromptSegment::SkillCatalog(ListPromptSegment {
                    title: "Available skills (load when relevant)".to_string(),
                    preamble: "Before using tools for a task, check this list and call skill_view(name) when a skill matches the user's request. Do not load unrelated skills.".to_string(),
                    item_template: "- {name}: {description}".to_string(),
                }),
                SystemPromptSegment::McpTools(ListPromptSegment {
                    title: "Available MCP tools (use directly)".to_string(),
                    preamble: String::new(),
                    item_template: "- {name}".to_string(),
                }),
            ],
        }
    }
}

fn pick_model_for_turn(snapshot: &domain::AgentDefinition, input: &TurnInput) -> ModelConfig {
    if let Some(model) = input.model_override.as_ref() {
        return model.clone();
    }
    if !input.images.is_empty() {
        if let Some(model) = snapshot.multimodal_model.as_ref() {
            return model.clone();
        }
    }
    snapshot.model.clone()
}

fn build_model_client(
    model: &ModelConfig,
    runtime_env: &HashMap<String, String>,
) -> Result<ModelClientConfig> {
    ChatModelClient::from_model_config(model, runtime_env)
}

fn build_all_tools(
    specs: &[domain::ToolSpec],
    session_id: &SessionId,
    context: &ToolBuildContext,
    tool_context: &ToolContext,
    mcp_registry: Option<Arc<McpRegistry>>,
) -> Vec<Arc<dyn JsonTool>> {
    let mut tools = tools::build_builtin_tools(specs, context, session_id);
    tools.extend(build_agent_tools(specs, session_id, tool_context));
    if let Some(registry) = mcp_registry {
        for def in registry.loaded_tools() {
            let registry = registry.clone();
            let prefixed = def.prefixed_name.clone();
            let session_id = session_id.clone();
            let definition = tools::ToolDefinition {
                name: prefixed.clone(),
                description: def.description,
                parameters: def.parameters,
            };
            tools.push(Arc::new(DynamicTool::new(definition, move |args| {
                let registry = registry.clone();
                let prefixed = prefixed.clone();
                let session_id = session_id.clone();
                Box::pin(async move {
                    registry
                        .call_tool_for_session(session_id.as_str(), &prefixed, args)
                        .await
                })
            })));
        }
    }
    let mut by_name: HashMap<String, Arc<dyn JsonTool>> = HashMap::new();
    for tool in tools {
        by_name.insert(tool.definition().name, tool);
    }
    let mut tools: Vec<_> = by_name.into_values().collect();
    tools.sort_by(|a, b| a.definition().name.cmp(&b.definition().name));
    tools
}

fn effective_tool_specs(snapshot: &AgentDefinition, is_subagent_definition: bool) -> Vec<ToolSpec> {
    let specs = match snapshot.tools.as_ref() {
        Some(tools) => tools.clone(),
        None if is_subagent_definition => default_subagent_builtin_tool_specs(),
        None => default_parent_builtin_tool_specs(),
    };

    if is_subagent_definition {
        return specs
            .into_iter()
            .filter(|spec| !matches!(spec, ToolSpec::SubagentTask(_) | ToolSpec::RequestUserInput))
            .collect();
    }

    specs
}

fn capture_tool_error(
    session_id: &SessionId,
    tool_name: &str,
    arguments: &serde_json::Value,
    error: &str,
) {
    let (channel, thread_ts) = derive_channel_and_thread(session_id);
    let argument_summary = summarize_tool_arguments(arguments);
    warn!(
        session_id = %session_id.as_str(),
        tool = tool_name,
        error = %error,
        "agent tool error captured"
    );
    sentry::with_scope(
        |scope| {
            scope.set_tag("runtime.agent_event", "tool_result");
            scope.set_tag("runtime.session_id", session_id.as_str());
            scope.set_tag("runtime.tool_name", tool_name);
            if !channel.is_empty() {
                scope.set_tag("runtime.channel", channel.as_str());
            }
            if !thread_ts.is_empty() {
                scope.set_tag("runtime.thread_ts", thread_ts.as_str());
            }
            scope.set_extra("tool_arguments", argument_summary.into());
            scope.set_extra("internal_error", error.to_string().into());
        },
        || {
            sentry::capture_message("agent tool error", sentry::Level::Error);
        },
    );
}

fn capture_preloaded_context_error(session_id: &SessionId, operation: &str, error: &str) {
    let (channel, thread_ts) = derive_channel_and_thread(session_id);
    warn!(
        session_id = %session_id.as_str(),
        operation,
        error = %error,
        "agent preloaded context error captured"
    );
    sentry::with_scope(
        |scope| {
            scope.set_tag("runtime.agent_event", "agent.preloaded_context.error");
            scope.set_tag("runtime.session_id", session_id.as_str());
            scope.set_tag("runtime.preloaded_context.operation", operation);
            if !channel.is_empty() {
                scope.set_tag("runtime.channel", channel.as_str());
            }
            if !thread_ts.is_empty() {
                scope.set_tag("runtime.thread_ts", thread_ts.as_str());
            }
            scope.set_extra("internal_error", error.to_string().into());
        },
        || {
            sentry::capture_message("agent preloaded context error", sentry::Level::Error);
        },
    );
}

fn derive_channel_and_thread(session_id: &SessionId) -> (String, String) {
    let raw = session_id.as_str();
    match raw.split_once('-') {
        Some((channel, thread_ts)) => (channel.to_string(), thread_ts.to_string()),
        None => (raw.to_string(), String::new()),
    }
}

fn summarize_tool_arguments(arguments: &serde_json::Value) -> String {
    match arguments {
        serde_json::Value::Object(map) => {
            let mut keys: Vec<&str> = map.keys().map(String::as_str).collect();
            keys.sort_unstable();
            serde_json::json!({
                "kind": "object",
                "keys": keys,
            })
            .to_string()
        }
        serde_json::Value::Array(items) => serde_json::json!({
            "kind": "array",
            "length": items.len(),
        })
        .to_string(),
        serde_json::Value::Null => serde_json::json!({"kind": "null"}).to_string(),
        serde_json::Value::Bool(_) => serde_json::json!({"kind": "bool"}).to_string(),
        serde_json::Value::Number(_) => serde_json::json!({"kind": "number"}).to_string(),
        serde_json::Value::String(value) => serde_json::json!({
            "kind": "string",
            "length": value.len(),
        })
        .to_string(),
    }
}

fn replace_empty_tool_call_arguments_from_xml(
    tool_calls: &mut [ToolCall],
    xml_calls: &[safety::xml_tool_repair::ExtractedToolCall],
) -> bool {
    let mut used = vec![false; xml_calls.len()];
    let mut repaired = false;
    for call in tool_calls {
        if !is_empty_json_object(&call.arguments) {
            continue;
        }
        let Some(index) = xml_calls.iter().enumerate().find_map(|(index, xml_call)| {
            if !used[index] && xml_call.name == call.name {
                Some(index)
            } else {
                None
            }
        }) else {
            continue;
        };
        used[index] = true;
        call.arguments = xml_calls[index].arguments.clone();
        repaired = true;
    }
    repaired
}

fn is_empty_json_object(value: &serde_json::Value) -> bool {
    value.as_object().is_some_and(|object| object.is_empty())
}

fn json_error(message: &str) -> serde_json::Value {
    serde_json::json!({"error": message})
}

fn json_safe_error(message: &str) -> serde_json::Value {
    serde_json::json!({"error": message, "safe_error": true})
}
