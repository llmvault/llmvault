use std::future::Future;
use std::path::PathBuf;
use std::pin::Pin;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::agent_registry::AgentDefinitionRegistry;
use domain::{
    validate_request_user_input_payload, validate_update_plan_payload, RequestUserInputPayload,
    SessionId, SubagentTask, SubagentTaskConfig, SubagentTaskState, ToolSpec, UpdatePlanPayload,
};
use mcp::McpRegistry;
use outbound::OutboundEmitter;
use serde_json::{json, Value};
use storage::{EventRepo, SubagentTaskRepo};
use tools::{JsonTool, ToolDefinition};

use crate::{PlanUpdater, QuestionRequester};

pub type ToolFuture = Pin<Box<dyn Future<Output = Result<Value>> + Send>>;

static SUBAGENT_TASK_SEQUENCE: AtomicU64 = AtomicU64::new(1);
const SUBAGENT_TASK_WAIT_INTERVAL: Duration = Duration::from_millis(250);
const SUBAGENT_TASK_FOREGROUND_TIMEOUT: Duration = Duration::from_secs(15 * 60);

pub struct DynamicTool {
    definition: ToolDefinition,
    executor: Arc<dyn Fn(Value) -> ToolFuture + Send + Sync>,
}

impl DynamicTool {
    pub fn new(
        definition: ToolDefinition,
        executor: impl Fn(Value) -> ToolFuture + Send + Sync + 'static,
    ) -> Self {
        Self {
            definition,
            executor: Arc::new(executor),
        }
    }
}

#[async_trait]
impl JsonTool for DynamicTool {
    fn definition(&self) -> ToolDefinition {
        self.definition.clone()
    }

    async fn call(&self, args: Value) -> Result<Value> {
        (self.executor)(args).await
    }
}

pub struct ToolContext {
    pub subagent_task_repo: Option<Arc<dyn SubagentTaskRepo>>,
    pub event_repo: Option<Arc<dyn EventRepo>>,
    pub question_requester: Option<Arc<dyn QuestionRequester>>,
    pub plan_updater: Option<Arc<dyn PlanUpdater>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
    pub workspace_root: PathBuf,
    pub outbound_emitter: Option<Arc<OutboundEmitter>>,
    pub agent_registry: Arc<AgentDefinitionRegistry>,
    pub session_stream_id: Option<String>,
}

pub fn build_agent_tools(
    specs: &[ToolSpec],
    session_id: &SessionId,
    ctx: &ToolContext,
) -> Vec<Arc<dyn JsonTool>> {
    let mut tools: Vec<Arc<dyn JsonTool>> = Vec::new();

    for spec in specs {
        match spec {
            ToolSpec::SearchSessions => {
                if let Some(repo) = &ctx.event_repo {
                    tools.push(search_sessions_tool(repo.clone()));
                }
            }
            ToolSpec::SubagentTask(config) => {
                if let Some(repo) = &ctx.subagent_task_repo {
                    tools.push(subagent_task_tool(
                        repo.clone(),
                        session_id.clone(),
                        ctx.agent_registry.clone(),
                        config.clone(),
                        ctx.session_stream_id.clone(),
                    ));
                }
            }
            ToolSpec::RequestUserInput => {
                if let Some(requester) = &ctx.question_requester {
                    tools.push(request_user_input_tool(
                        requester.clone(),
                        session_id.clone(),
                    ));
                }
            }
            ToolSpec::UpdatePlan => {
                if let Some(updater) = &ctx.plan_updater {
                    tools.push(update_plan_tool(updater.clone(), session_id.clone()));
                }
            }
            _ => {}
        }
    }

    tools
}

pub async fn emit_tool_invoked(
    _emitter: Option<Arc<OutboundEmitter>>,
    _session_id: &SessionId,
    _tool: &str,
    _args: &Value,
    _result: &Value,
) {
}

pub async fn emit_tool_error(
    _emitter: Option<Arc<OutboundEmitter>>,
    _session_id: &SessionId,
    _tool: &str,
    _args: &Value,
    _error: &str,
) {
}

fn request_user_input_tool(
    requester: Arc<dyn QuestionRequester>,
    session_id: SessionId,
) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "request_user_input".into(),
            description:
                "Ask the user one to three structured questions and wait until they answer.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "questions": {
                        "type": "array",
                        "minItems": 1,
                        "maxItems": 3,
                        "items": {
                            "type": "object",
                            "properties": {
                                "id": { "type": "string" },
                                "header": {
                                    "type": "string",
                                    "description": "Short UI header, 12 or fewer characters."
                                },
                                "question": { "type": "string" },
                                "options": {
                                    "type": "array",
                                    "minItems": 2,
                                    "maxItems": 3,
                                    "items": {
                                        "type": "object",
                                        "properties": {
                                            "label": { "type": "string" },
                                            "description": { "type": "string" }
                                        },
                                        "required": ["label", "description"]
                                    }
                                }
                            },
                            "required": ["id", "header", "question", "options"]
                        }
                    }
                },
                "required": ["questions"]
            }),
        },
        move |args| {
            let requester = requester.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let payload: RequestUserInputPayload = serde_json::from_value(args)
                    .map_err(|error| anyhow!("invalid request_user_input arguments: {error}"))?;
                validate_request_user_input_payload(&payload)
                    .map_err(|error| anyhow!("invalid request_user_input arguments: {error}"))?;
                let (question_request_id, answer) =
                    requester.request_user_input(&session_id, payload).await?;
                Ok(json!({
                    "question_request_id": question_request_id,
                    "answers": answer.answers,
                }))
            })
        },
    ))
}

fn update_plan_tool(updater: Arc<dyn PlanUpdater>, session_id: SessionId) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "update_plan".into(),
            description: "Replace the current visible task plan with a concise checklist of steps."
                .into(),
            parameters: json!({
                "type": "object",
                "additionalProperties": false,
                "properties": {
                    "explanation": {
                        "type": ["string", "null"],
                        "description": "Optional short reason or context for this plan update."
                    },
                    "plan": {
                        "type": "array",
                        "minItems": 1,
                        "items": {
                            "type": "object",
                            "additionalProperties": false,
                            "properties": {
                                "step": { "type": "string" },
                                "status": {
                                    "type": "string",
                                    "enum": ["pending", "in_progress", "completed"]
                                }
                            },
                            "required": ["step", "status"]
                        }
                    }
                },
                "required": ["plan"]
            }),
        },
        move |args| {
            let updater = updater.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let payload: UpdatePlanPayload = serde_json::from_value(args)
                    .map_err(|error| anyhow!("invalid update_plan arguments: {error}"))?;
                validate_update_plan_payload(&payload)
                    .map_err(|error| anyhow!("invalid update_plan arguments: {error}"))?;
                let result = updater.update_plan(&session_id, payload).await?;
                serde_json::to_value(result)
                    .map_err(|error| anyhow!("serialize plan result: {error}"))
            })
        },
    ))
}

fn search_sessions_tool(repo: Arc<dyn EventRepo>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "search_sessions".into(),
            description: "Search recent local conversation history from this sandbox. Use it to find prior user messages, agent replies, and compact tool summaries before relying on older context.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search terms for recent conversations"
                    },
                    "session_id": {
                        "type": "string",
                        "description": "Optional exact session id to search within"
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Maximum matches to return, default 8, max 20"
                    }
                },
                "required": ["query"]
            }),
        },
        move |args| {
            let repo = repo.clone();
            Box::pin(async move {
                let query = args
                    .get("query")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .ok_or_else(|| anyhow!("query required"))?;
                let limit = args
                    .get("limit")
                    .and_then(Value::as_u64)
                    .unwrap_or(8)
                    .clamp(1, 20) as u32;
                let session_id = args
                    .get("session_id")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .map(SessionId::from);
                let matches = repo
                    .search_sessions(query, session_id.as_ref(), limit)
                    .await?;
                let items: Vec<Value> = matches
                    .into_iter()
                    .map(|item| {
                        json!({
                            "session_id": item.session_id,
                            "event_id": item.event_id,
                            "event_kind": item.kind,
                            "created_at": item.created_at.to_rfc3339(),
                            "score": item.score,
                            "snippet": item.snippet,
                            "text": truncate_search_text(&item.content, 700),
                        })
                    })
                    .collect();
                Ok(json!({ "matches": items }))
            })
        },
    ))
}

fn truncate_search_text(value: &str, max_chars: usize) -> String {
    let mut out = String::new();
    for ch in value.chars().take(max_chars) {
        out.push(ch);
    }
    out
}

fn build_agent_list_description(
    registry: &AgentDefinitionRegistry,
    allowlist: &[String],
) -> String {
    let agents = if allowlist.is_empty() {
        registry.available_agents()
    } else {
        allowlist.to_vec()
    };
    let mut parts = Vec::new();
    for name in &agents {
        let desc = registry.agent_description(name);
        parts.push(format!("{} - {}", name, desc));
    }
    format!("Sub-agent name. Available: {}", parts.join(", "))
}

fn subagent_task_tool(
    repo: Arc<dyn SubagentTaskRepo>,
    session_id: SessionId,
    agent_registry: Arc<AgentDefinitionRegistry>,
    config: SubagentTaskConfig,
    session_stream_id: Option<String>,
) -> Arc<dyn JsonTool> {
    let agent_desc = build_agent_list_description(&agent_registry, &config.agents);
    let agent_names: Vec<String> = if config.agents.is_empty() {
        agent_registry.available_agents()
    } else {
        config.agents.clone()
    };
    let agent_names_clone = agent_names.clone();
    let config_agents = config.agents.clone();

    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "subagent_task".into(),
            description: "Run a task with a configured subagent in an isolated foreground session."
                .into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "goal": {
                        "type": "string",
                        "description": "The task for the subagent."
                    },
                    "agent": {
                        "type": "string",
                        "description": agent_desc
                    }
                },
                "required": ["goal", "agent"]
            }),
        },
        move |args| {
            let repo = repo.clone();
            let session_id = session_id.clone();
            let agent_registry = agent_registry.clone();
            let agent_names = agent_names_clone.clone();
            let config_agents = config_agents.clone();
            let session_stream_id = session_stream_id.clone();
            Box::pin(async move {
                let goal = args
                    .get("goal")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("goal required"))?
                    .to_string();
                let agent_name = args
                    .get("agent")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("agent required"))?
                    .to_string();

                if !config_agents.is_empty() && !config_agents.contains(&agent_name) {
                    return Err(anyhow!(
                        "agent '{}' not in allowlist. Allowed: {:?}",
                        agent_name,
                        config_agents
                    ));
                }
                if agent_registry.resolve(&agent_name).is_none() {
                    return Err(anyhow!(
                        "unknown agent '{}'. Available: {:?}",
                        agent_name,
                        agent_names
                    ));
                }

                let now = Utc::now();
                let id = next_subagent_task_id(now);
                let child_session_id = SessionId::from(format!("subagent-{}", id));

                let task = SubagentTask {
                    id: id.clone(),
                    parent_session_id: session_id.clone(),
                    child_session_id: child_session_id.clone(),
                    agent_name: agent_name.clone(),
                    goal,
                    stream_id: session_stream_id.clone(),
                    state: SubagentTaskState::Queued,
                    result: None,
                    error: None,
                    created_at: now,
                    started_at: None,
                    completed_at: None,
                    updated_at: now,
                };
                repo.create(&task).await?;
                let completed =
                    wait_for_subagent_task_completion(repo.clone(), &id, &agent_name).await?;
                let result = completed.result.clone().unwrap_or_default();
                Ok(json!({
                    "job_id": id,
                    "state": subagent_task_state_string(completed.state),
                    "session_id": child_session_id.as_str(),
                    "agent": agent_name,
                    "result": result,
                    "output": result,
                    "message": format!("Subagent '{}' completed.", agent_name),
                    "system_reminder": "Subagent tasks run in isolated sessions and may not share this session's local filesystem. Use Drive or another explicit shared location for artifacts that must be inspected or shared."
                }))
            })
        },
    ))
}

fn next_subagent_task_id(now: DateTime<Utc>) -> String {
    let sequence = SUBAGENT_TASK_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    format!("subagent-task-{}-{}", now.timestamp_millis(), sequence)
}

async fn wait_for_subagent_task_completion(
    repo: Arc<dyn SubagentTaskRepo>,
    id: &str,
    agent_name: &str,
) -> Result<SubagentTask> {
    let start = Instant::now();
    loop {
        let task = repo
            .get(id)
            .await?
            .ok_or_else(|| anyhow!("subagent task '{}' not found", id))?;
        match task.state {
            SubagentTaskState::Completed => return Ok(task),
            SubagentTaskState::Failed => {
                let error = task
                    .error
                    .clone()
                    .unwrap_or_else(|| "subagent failed without an error message".to_string());
                return Err(anyhow!("subagent '{}' failed: {}", agent_name, error));
            }
            SubagentTaskState::Queued | SubagentTaskState::Running => {}
        }

        if start.elapsed() >= SUBAGENT_TASK_FOREGROUND_TIMEOUT {
            let message = format!(
                "subagent '{}' timed out after {} seconds",
                agent_name,
                SUBAGENT_TASK_FOREGROUND_TIMEOUT.as_secs()
            );
            let _ = repo
                .complete(
                    id,
                    SubagentTaskState::Failed,
                    Utc::now(),
                    "",
                    Some(&message),
                )
                .await;
            return Err(anyhow!(message));
        }
        tokio::time::sleep(SUBAGENT_TASK_WAIT_INTERVAL).await;
    }
}

fn subagent_task_state_string(state: SubagentTaskState) -> &'static str {
    match state {
        SubagentTaskState::Queued => "queued",
        SubagentTaskState::Running => "running",
        SubagentTaskState::Completed => "completed",
        SubagentTaskState::Failed => "failed",
    }
}

#[cfg(test)]
#[path = "rig_tool_registry_tests.rs"]
mod tests;
