use domain::{AgentDefinition, SessionId, SystemPromptSegment, ToolFilter};
use mcp::McpRegistry;
use std::hash::{Hash, Hasher};
use tracing::warn;

use crate::history::{
    append_model_message, auto_load_skills_completed_for_turn, load_model_history_for_turn,
    load_session_context, persist_session_context, record_auto_load_skills_completed_for_turn,
    record_model_turn_started, seed_model_history_from_session_history, SKILL_VIEW_TOOL_NAME,
};
use crate::primitives::{AgentMessage, ToolCall};
use crate::{Result, TurnInput};

/// The runtime advertises the trusted control-plane MCP server under the
/// `hivy` prefix, so `skill_view` is reachable as `hivy_skill_view`. Calling it
/// through the registry triggers the same `materialize` side effect a
/// model-initiated call would (see `mcp::McpRegistry::call_tool_for_session`).
/// Seam over the `skill_view` invocations used by the auto-load bootstrap.
/// Production is backed by the real MCP registry (below), so each call takes
/// the exact same path — including the trusted-server `materialize` side effect
/// — as a model-initiated call. Tests supply a fake to exercise the bootstrap
/// without standing up an MCP server.
#[async_trait::async_trait]
pub(super) trait SkillViewCaller: Send + Sync {
    async fn skill_view(
        &self,
        session_id: &str,
        actor_user_id: Option<&str>,
        args: serde_json::Value,
    ) -> anyhow::Result<serde_json::Value>;
}

#[async_trait::async_trait]
impl SkillViewCaller for McpRegistry {
    async fn skill_view(
        &self,
        session_id: &str,
        actor_user_id: Option<&str>,
        args: serde_json::Value,
    ) -> anyhow::Result<serde_json::Value> {
        self.call_tool_for_session(session_id, actor_user_id, SKILL_VIEW_TOOL_NAME, args)
            .await
    }
}

use super::capture_preloaded_context_error;

pub(super) struct InitialMessagePromptSources<'a> {
    pub(super) mcp_registry: Option<&'a McpRegistry>,
    pub(super) mcp_tool_filter: Option<&'a ToolFilter>,
    /// Exact names of every native tool definition exposed to the model for
    /// this turn. This includes `load_tools`; MCP tools belong in the registry
    /// catalog below and must not be included here.
    pub(super) native_tool_names: &'a [String],
    pub(super) skill_view_caller: Option<&'a dyn SkillViewCaller>,
}

pub(super) async fn build_initial_messages(
    snapshot: &AgentDefinition,
    session_id: &SessionId,
    input: TurnInput,
    turn_id: &str,
    event_repo: Option<&dyn storage::EventRepo>,
    prompt_sources: InitialMessagePromptSources<'_>,
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
    let loadable_mcp_tool_names = prompt_sources
        .mcp_registry
        .map(|registry| registry.available_tool_names_filtered(prompt_sources.mcp_tool_filter))
        .unwrap_or_default();
    let mut messages = vec![
        AgentMessage::system(render_cacheable_system_prompt(
            snapshot,
            prompt_sources.native_tool_names,
            &loadable_mcp_tool_names,
        )),
        AgentMessage::system(render_dynamic_system_prompt(snapshot, &session_context)),
    ];
    record_model_turn_started(event_repo, session_id, turn_id).await?;
    let mut history =
        load_model_history_for_turn(event_repo, session_id, 1000, Some(turn_id)).await?;
    if history.is_empty() && !input.prior_history.is_empty() {
        history =
            seed_model_history_from_session_history(event_repo, session_id, &input.prior_history)
                .await?;
    }
    messages.extend(history);
    // Auto-loaded skill bodies are turn-scoped. Reload them before every model
    // turn; the history projection replaces earlier skill payloads with a
    // compact reminder so only the current turn's content remains in context.
    let synthetic = inject_auto_load_skills(
        snapshot,
        session_id,
        turn_id,
        event_repo,
        prompt_sources.skill_view_caller,
        input.actor_user_id.as_deref(),
    )
    .await;
    messages.extend(synthetic);
    let user = AgentMessage::user(input.text);
    append_model_message(event_repo, session_id, &user).await?;
    messages.push(user);
    Ok(messages)
}

/// On every turn, load the definition's `auto_load_skills` by invoking
/// `skill_view` through the MCP registry — the exact path a model-initiated call
/// takes, so the `materialize` side effect writes `.skills/<slug>/…` identically
/// — and return synthetic assistant-tool-call + tool-result messages mirroring
/// what the model would have produced. Persists the messages for audit and
/// same-turn continuity. Any failure logs a warning and skips; the agent can
/// still load the skill manually. Never returns an error.
///
/// Mechanism note: every provider adapter here is OpenAI-compatible and
/// serializes assistant `tool_calls` + `tool` results identically regardless of
/// `strict_tool_schema` (which only sanitizes tool *definition* schemas, not
/// messages), and the runner already persists this exact pair for real calls.
/// So the synthetic-pair form is valid for all profiles — no user-message
/// fallback is needed.
async fn inject_auto_load_skills(
    snapshot: &AgentDefinition,
    session_id: &SessionId,
    turn_id: &str,
    event_repo: Option<&dyn storage::EventRepo>,
    caller: Option<&dyn SkillViewCaller>,
    actor_user_id: Option<&str>,
) -> Vec<AgentMessage> {
    if snapshot.auto_load_skills.is_empty() {
        return Vec::new();
    }
    match auto_load_skills_completed_for_turn(event_repo, session_id, turn_id).await {
        Ok(true) => return Vec::new(),
        Ok(false) => {}
        Err(error) => {
            warn!(
                session_id = session_id.as_str(),
                %error,
                "skill auto-load skipped: per-turn idempotence check failed"
            );
            return Vec::new();
        }
    }

    let Some(caller) = caller else {
        warn!(
            session_id = session_id.as_str(),
            "skill auto-load skipped: no MCP registry available"
        );
        return Vec::new();
    };

    let mut synthetic = Vec::new();
    let mut counter = 0usize;
    for skill in &snapshot.auto_load_skills {
        // Skill root, then each declared linked file.
        let mut requests = vec![serde_json::json!({ "name": skill.name })];
        for file in &skill.files {
            requests.push(serde_json::json!({ "name": skill.name, "file_path": file }));
        }
        for args in requests {
            match caller
                .skill_view(session_id.as_str(), actor_user_id, args.clone())
                .await
            {
                Ok(result) => {
                    let tool_call_id = auto_load_tool_call_id(turn_id, counter);
                    counter += 1;
                    let assistant = AgentMessage::assistant_tool_calls(vec![ToolCall {
                        id: tool_call_id.clone(),
                        name: SKILL_VIEW_TOOL_NAME.to_string(),
                        arguments: args,
                    }]);
                    let tool_result = AgentMessage::tool_result(tool_call_id, result.to_string());
                    // Persist so resume rebuilds an identical transcript.
                    if let Err(error) =
                        append_model_message(event_repo, session_id, &assistant).await
                    {
                        warn!(
                            session_id = session_id.as_str(),
                            %error, "skill auto-load: failed to persist synthetic tool call"
                        );
                    }
                    if let Err(error) =
                        append_model_message(event_repo, session_id, &tool_result).await
                    {
                        warn!(
                            session_id = session_id.as_str(),
                            %error, "skill auto-load: failed to persist synthetic tool result"
                        );
                    }
                    synthetic.push(assistant);
                    synthetic.push(tool_result);
                }
                Err(error) => {
                    warn!(
                        session_id = session_id.as_str(),
                        skill = %skill.name,
                        %error,
                        "skill auto-load: skill_view failed; skipping entry"
                    );
                }
            }
        }
    }

    if let Err(error) =
        record_auto_load_skills_completed_for_turn(event_repo, session_id, turn_id).await
    {
        warn!(
            session_id = session_id.as_str(),
            %error,
            "skill auto-load: failed to persist per-turn completion marker"
        );
    }

    synthetic
}

fn auto_load_tool_call_id(turn_id: &str, counter: usize) -> String {
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    turn_id.hash(&mut hasher);
    format!("autoload_{:016x}_{counter}", hasher.finish())
}

pub(super) fn render_cacheable_system_prompt(
    snapshot: &AgentDefinition,
    native_tool_names: &[String],
    loadable_mcp_tool_names: &[String],
) -> String {
    // Tool usage is deliberately the first cacheable content the model sees on
    // every turn. Keep this catalog stable and complete: the model has no
    // search tool with which to discover omitted MCP names.
    let mut prompt = render_system_tool_usage(native_tool_names, loadable_mcp_tool_names);
    for segment in &snapshot.system_prompt.cacheable_segments {
        append_rendered_segment(&mut prompt, render_static_segment(segment));
    }
    prompt
}

pub(super) fn render_dynamic_system_prompt(
    snapshot: &AgentDefinition,
    session_context: &[String],
) -> String {
    let mut prompt = String::new();
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
            // The complete, permission-filtered MCP catalog is permanently
            // rendered in the leading cacheable <system-tool-usage> block.
            // Rendering this legacy segment would duplicate it in a dynamic
            // (and therefore non-cacheable) system message.
            SystemPromptSegment::McpTools(_) => None,
        };
        append_rendered_segment(&mut prompt, rendered);
    }

    prompt
}

fn render_system_tool_usage(
    native_tool_names: &[String],
    loadable_mcp_tool_names: &[String],
) -> String {
    fn stable_exact_names(names: &[String]) -> Vec<&str> {
        let mut names = names.iter().map(String::as_str).collect::<Vec<_>>();
        names.sort_unstable();
        names.dedup();
        names
    }

    let native_tool_names = stable_exact_names(native_tool_names);
    let loadable_mcp_tool_names = stable_exact_names(loadable_mcp_tool_names);
    let native_json =
        serde_json::to_string(&native_tool_names).expect("tool names must serialize as JSON");
    let mcp_json =
        serde_json::to_string(&loadable_mcp_tool_names).expect("tool names must serialize as JSON");

    format!(
        "<system-tool-usage>\n\
The following catalogs are the complete set of tools available to you for this turn.\n\
Native tools are already available. Call them directly; never pass native tool names to `load_tools`.\n\
Native tool exact names: {native_json}\n\
Loadable MCP tool exact names: {mcp_json}\n\
MCP tool schemas are not available until loaded. At the beginning of each turn, identify every MCP tool needed for the current task and call the native `load_tools` tool once with all required exact names in one batch.\n\
Loaded MCP tool schemas are available only for the current turn and expire before the next turn. Load them again on every later turn that needs them.\n\
There is no tool search or tool-details lookup. The catalog above is complete; select exact MCP tool names from it.\n\
</system-tool-usage>"
    )
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
mod system_tool_usage_tests {
    use super::{
        build_initial_messages, render_cacheable_system_prompt, render_dynamic_system_prompt,
        InitialMessagePromptSources,
    };
    use crate::primitives::{AgentMessageRole, MessagePart};
    use crate::TurnInput;
    use domain::{
        AgentDefinition, ListPromptSegment, SessionId, StaticPromptSegment, SystemPromptSegment,
    };

    fn definition() -> AgentDefinition {
        serde_json::from_str(
            r#"{"agent":{"name":"prompt-test"},"model":{"provider":"openai_compatible","base_url":"http://x","model_id":"m","api_key_env":"K"}}"#,
        )
        .expect("definition should deserialize")
    }

    #[test]
    fn system_tool_usage_is_first_cacheable_content_and_contains_complete_exact_catalogs() {
        let mut definition = definition();
        definition
            .system_prompt
            .cacheable_segments
            .push(SystemPromptSegment::StaticText(StaticPromptSegment {
                title: "Identity".to_string(),
                content: "You are the test agent.".to_string(),
            }));

        let prompt = render_cacheable_system_prompt(
            &definition,
            &[
                "read_file".to_string(),
                "bash".to_string(),
                "load_tools".to_string(),
            ],
            &[
                "slack_post_message".to_string(),
                "github_list_issues".to_string(),
            ],
        );

        assert!(prompt.starts_with("<system-tool-usage>"));
        assert!(prompt.contains(r#"Native tool exact names: ["bash","load_tools","read_file"]"#));
        assert!(prompt.contains(
            r#"Loadable MCP tool exact names: ["github_list_issues","slack_post_message"]"#
        ));
        assert!(prompt.contains("call the native `load_tools` tool once"));
        assert!(prompt.contains("all required exact names in one batch"));
        assert!(prompt.contains("available only for the current turn"));
        assert!(prompt.contains("expire before the next turn"));
        assert!(prompt.contains("There is no tool search or tool-details lookup"));
        assert!(
            prompt.find("</system-tool-usage>").unwrap() < prompt.find("## Identity").unwrap(),
            "tool instructions must precede configured cacheable prompt segments"
        );
    }

    #[test]
    fn legacy_dynamic_mcp_catalog_is_not_rendered() {
        let mut definition = definition();
        definition
            .system_prompt
            .dynamic_segments
            .push(SystemPromptSegment::McpTools(ListPromptSegment {
                title: "Legacy MCP catalog".to_string(),
                preamble: "This must not render.".to_string(),
                item_template: "- {name}".to_string(),
            }));

        let prompt = render_dynamic_system_prompt(
            &definition,
            &["## Current context\nKeep this context.".to_string()],
        );

        assert!(prompt.contains("Keep this context."));
        assert!(!prompt.contains("Legacy MCP catalog"));
        assert!(!prompt.contains("This must not render."));
        assert!(!prompt.contains("Loading MCP tools"));
    }

    #[tokio::test]
    async fn first_system_message_carries_tool_usage_on_every_turn() {
        let definition = definition();
        let native_tool_names = vec!["bash".to_string(), "load_tools".to_string()];
        let messages = build_initial_messages(
            &definition,
            &SessionId::from("prompt-tools-turn"),
            TurnInput::text("do the task"),
            "turn-one",
            None,
            InitialMessagePromptSources {
                mcp_registry: None,
                mcp_tool_filter: None,
                native_tool_names: &native_tool_names,
                skill_view_caller: None,
            },
        )
        .await
        .expect("initial messages");

        assert_eq!(messages[0].role, AgentMessageRole::System);
        let MessagePart::Text { text } = &messages[0].parts[0];
        assert!(text.starts_with("<system-tool-usage>"));
        assert!(text.contains(r#"Native tool exact names: ["bash","load_tools"]"#));
        assert!(text.contains("Loadable MCP tool exact names: []"));
    }
}

#[cfg(test)]
mod auto_load_tests {
    use super::{
        build_initial_messages, inject_auto_load_skills, InitialMessagePromptSources,
        SkillViewCaller,
    };
    use crate::history::load_model_history;
    use crate::primitives::AgentMessageRole;
    use crate::TurnInput;
    use domain::{AgentDefinition, EventKind, SessionEvent, SessionId};
    use serde_json::{json, Value};
    use std::sync::Arc;
    use tokio::sync::Mutex;

    #[derive(Default)]
    struct MemoryEventRepo {
        events: Mutex<Vec<SessionEvent>>,
    }

    #[async_trait::async_trait]
    impl storage::EventRepo for MemoryEventRepo {
        async fn append(
            &self,
            session_id: &SessionId,
            kind: EventKind,
            payload: Value,
        ) -> storage::Result<i64> {
            let mut events = self.events.lock().await;
            let id = events.len() as i64 + 1;
            events.push(SessionEvent {
                id,
                session_id: session_id.clone(),
                seq: id,
                kind,
                payload,
                created_at: chrono::Utc::now(),
            });
            Ok(id)
        }

        async fn append_idempotent(
            &self,
            session_id: &SessionId,
            kind: EventKind,
            payload: Value,
            idempotency_key: &str,
        ) -> storage::Result<Option<i64>> {
            let mut payload = payload;
            if let Some(object) = payload.as_object_mut() {
                object.insert(
                    "_idempotency_key".to_string(),
                    Value::String(idempotency_key.to_string()),
                );
            }
            if self.events.lock().await.iter().any(|event| {
                event.session_id == *session_id
                    && event
                        .payload
                        .get("_idempotency_key")
                        .and_then(|v| v.as_str())
                        == Some(idempotency_key)
            }) {
                return Ok(None);
            }
            self.append(session_id, kind, payload).await.map(Some)
        }

        async fn list_recent(
            &self,
            session_id: &SessionId,
            _limit: u32,
        ) -> storage::Result<Vec<SessionEvent>> {
            let mut events: Vec<_> = self
                .events
                .lock()
                .await
                .iter()
                .filter(|event| &event.session_id == session_id)
                .cloned()
                .collect();
            events.reverse();
            Ok(events)
        }

        async fn list_chronological(
            &self,
            session_id: &SessionId,
            limit: u32,
        ) -> storage::Result<Vec<SessionEvent>> {
            let mut events = self.list_recent(session_id, limit).await?;
            events.reverse();
            Ok(events)
        }

        async fn search_sessions(
            &self,
            _query: &str,
            _session_id: Option<&SessionId>,
            _limit: u32,
        ) -> storage::Result<Vec<storage::SessionSearchResult>> {
            Ok(Vec::new())
        }
    }

    /// Records every `skill_view` invocation and returns a canned result (or an
    /// error) so the bootstrap can be exercised without an MCP server.
    struct FakeSkillViewCaller {
        calls: Mutex<Vec<Value>>,
        fail: bool,
    }

    impl FakeSkillViewCaller {
        fn ok() -> Self {
            Self {
                calls: Mutex::new(Vec::new()),
                fail: false,
            }
        }

        fn failing() -> Self {
            Self {
                calls: Mutex::new(Vec::new()),
                fail: true,
            }
        }
    }

    #[async_trait::async_trait]
    impl SkillViewCaller for FakeSkillViewCaller {
        async fn skill_view(
            &self,
            _session_id: &str,
            _actor_user_id: Option<&str>,
            args: Value,
        ) -> anyhow::Result<Value> {
            let call_number = {
                let mut calls = self.calls.lock().await;
                calls.push(args.clone());
                calls.len()
            };
            if self.fail {
                anyhow::bail!("skill_view boom");
            }
            let name = args.get("name").and_then(Value::as_str).unwrap_or_default();
            Ok(json!({
                "content": [{
                    "type": "text",
                    "text": format!("loaded {name} call {call_number}")
                }],
                "structuredContent": { "materialized": { "ok": true } }
            }))
        }
    }

    fn definition_with_auto_load(skills_json: &str) -> AgentDefinition {
        let json = format!(
            r#"{{"agent":{{"name":"a"}},"model":{{"provider":"openai_compatible","base_url":"http://x","model_id":"m","api_key_env":"K"}},"auto_load_skills":{skills_json}}}"#
        );
        serde_json::from_str(&json).expect("definition should deserialize")
    }

    #[tokio::test]
    async fn auto_loaded_skill_content_is_fresh_and_turn_scoped() {
        let repo = Arc::new(MemoryEventRepo::default());
        let caller = FakeSkillViewCaller::ok();
        let session_id = SessionId::from("s-autoload-first");
        let definition =
            definition_with_auto_load(r#"[{"name":"browser","files":["references/commands.md"]}]"#);

        let first = build_initial_messages(
            &definition,
            &session_id,
            TurnInput::text("first task"),
            "turn-one",
            Some(repo.as_ref()),
            InitialMessagePromptSources {
                mcp_registry: None,
                mcp_tool_filter: None,
                native_tool_names: &[],
                skill_view_caller: Some(&caller),
            },
        )
        .await;
        let first = first.unwrap();
        let first_json = serde_json::to_string(&first).unwrap();

        // Skill root + one linked file = two skill_view calls => two pairs.
        let first_skill_messages: Vec<_> = first
            .iter()
            .filter(|message| {
                message
                    .tool_calls
                    .iter()
                    .any(|call| call.name == "hivy_skill_view")
                    || message
                        .tool_call_id
                        .as_deref()
                        .is_some_and(|id| id.starts_with("autoload_"))
            })
            .collect();
        assert_eq!(
            first_skill_messages.len(),
            4,
            "expected two assistant/tool pairs"
        );
        assert!(first_json.contains("loaded browser call 1"));
        assert!(first_json.contains("loaded browser call 2"));
        let first_ids: Vec<_> = first_skill_messages
            .iter()
            .flat_map(|message| &message.tool_calls)
            .map(|call| call.id.clone())
            .collect();

        // First call: the skill root, with clean model-visible arguments (no
        // hidden _hivy_ fields — those are injected inside the registry only).
        let first_call = first_skill_messages[0];
        assert_eq!(first_call.role, AgentMessageRole::Assistant);
        assert_eq!(first_call.tool_calls.len(), 1);
        assert_eq!(first_call.tool_calls[0].name, "hivy_skill_view");
        assert!(first_call.tool_calls[0].id.starts_with("autoload_"));
        assert_eq!(
            first_call.tool_calls[0].arguments,
            json!({ "name": "browser" })
        );
        assert_eq!(caller.calls.lock().await.len(), 2);

        let retry = build_initial_messages(
            &definition,
            &session_id,
            TurnInput::text("retry same turn"),
            "turn-one",
            Some(repo.as_ref()),
            InitialMessagePromptSources {
                mcp_registry: None,
                mcp_tool_filter: None,
                native_tool_names: &[],
                skill_view_caller: Some(&caller),
            },
        )
        .await
        .unwrap();
        let retry_json = serde_json::to_string(&retry).unwrap();
        assert_eq!(
            caller.calls.lock().await.len(),
            2,
            "the same turn_id must not reload auto-loaded skills"
        );
        assert!(retry_json.contains("loaded browser call 1"));
        assert!(retry_json.contains("loaded browser call 2"));
        assert!(!retry_json.contains("skill loads are turn-scoped"));

        let second = build_initial_messages(
            &definition,
            &session_id,
            TurnInput::text("second task"),
            "turn-two",
            Some(repo.as_ref()),
            InitialMessagePromptSources {
                mcp_registry: None,
                mcp_tool_filter: None,
                native_tool_names: &[],
                skill_view_caller: Some(&caller),
            },
        )
        .await
        .unwrap();
        let second_json = serde_json::to_string(&second).unwrap();
        assert_eq!(caller.calls.lock().await.len(), 4);
        assert!(!second_json.contains("loaded browser call 1"));
        assert!(!second_json.contains("loaded browser call 2"));
        assert!(second_json.contains("loaded browser call 3"));
        assert!(second_json.contains("loaded browser call 4"));
        assert!(second_json.contains("skill loads are turn-scoped"));
        assert!(second_json.contains("load it again"));
        assert!(second_json.contains("first task"));
        assert!(second_json.contains("second task"));

        let second_ids: Vec<_> = second
            .iter()
            .flat_map(|message| &message.tool_calls)
            .filter(|call| call.name == "hivy_skill_view")
            .map(|call| call.id.clone())
            .collect();
        assert_eq!(second_ids.len(), 2);
        assert!(
            second_ids.iter().all(|id| !first_ids.contains(id)),
            "auto-load tool-call ids must be unique across turns"
        );

        let second_retry = build_initial_messages(
            &definition,
            &session_id,
            TurnInput::text("retry second turn"),
            "turn-two",
            Some(repo.as_ref()),
            InitialMessagePromptSources {
                mcp_registry: None,
                mcp_tool_filter: None,
                native_tool_names: &[],
                skill_view_caller: Some(&caller),
            },
        )
        .await
        .unwrap();
        let second_retry_json = serde_json::to_string(&second_retry).unwrap();
        assert_eq!(
            caller.calls.lock().await.len(),
            4,
            "retrying turn-two must reuse its current skill payload"
        );
        assert!(!second_retry_json.contains("loaded browser call 1"));
        assert!(!second_retry_json.contains("loaded browser call 2"));
        assert!(second_retry_json.contains("loaded browser call 3"));
        assert!(second_retry_json.contains("loaded browser call 4"));
        let reminder_index = second_retry
            .iter()
            .position(|message| {
                serde_json::to_string(message)
                    .unwrap()
                    .contains("skill loads are turn-scoped")
            })
            .expect("prior-turn reload reminder");
        let current_skill_index = second_retry
            .iter()
            .position(|message| {
                message
                    .tool_calls
                    .iter()
                    .any(|call| second_ids.contains(&call.id))
            })
            .expect("turn-two skill call");
        assert!(
            reminder_index < current_skill_index,
            "the prior-turn reminder must precede the retained current-turn skill content"
        );
    }

    #[tokio::test]
    async fn subagent_definition_auto_loads_its_own_skills() {
        // The bootstrap always reads `snapshot.auto_load_skills`; run_turn passes
        // the sub-agent's own resolved definition as the snapshot, so this same
        // path loads the sub-agent's declared skills (not the parent's).
        let repo = Arc::new(MemoryEventRepo::default());
        let caller = FakeSkillViewCaller::ok();
        let session_id = SessionId::from("s-autoload-subagent");
        let definition = definition_with_auto_load(r#"[{"name":"qa-execution"}]"#);

        let synthetic = inject_auto_load_skills(
            &definition,
            &session_id,
            "subagent-turn",
            Some(repo.as_ref()),
            Some(&caller),
            None,
        )
        .await;

        assert_eq!(synthetic.len(), 2);
        assert_eq!(
            synthetic[0].tool_calls[0].arguments,
            json!({ "name": "qa-execution" })
        );
        let calls = caller.calls.lock().await;
        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0], json!({ "name": "qa-execution" }));
    }

    #[tokio::test]
    async fn skill_view_failure_degrades_gracefully() {
        let repo = Arc::new(MemoryEventRepo::default());
        let caller = FakeSkillViewCaller::failing();
        let session_id = SessionId::from("s-autoload-fail");
        let definition = definition_with_auto_load(r#"[{"name":"browser"}]"#);

        let synthetic = inject_auto_load_skills(
            &definition,
            &session_id,
            "failing-turn",
            Some(repo.as_ref()),
            Some(&caller),
            None,
        )
        .await;

        // Failure => no synthetic messages and the turn is not failed. A later
        // turn may retry because auto-loaded skills are turn-scoped.
        assert!(synthetic.is_empty());
        assert_eq!(caller.calls.lock().await.len(), 1);
        assert!(load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap()
            .is_empty());
    }

    #[tokio::test]
    async fn no_registry_skips_the_current_turn() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s-autoload-noreg");
        let definition = definition_with_auto_load(r#"[{"name":"browser"}]"#);

        let synthetic = inject_auto_load_skills(
            &definition,
            &session_id,
            "no-registry-turn",
            Some(repo.as_ref()),
            None,
            None,
        )
        .await;

        assert!(synthetic.is_empty());
    }

    #[tokio::test]
    async fn empty_auto_load_skills_is_a_noop() {
        let repo = Arc::new(MemoryEventRepo::default());
        let caller = FakeSkillViewCaller::ok();
        let session_id = SessionId::from("s-autoload-empty");
        let definition = definition_with_auto_load("[]");

        let synthetic = inject_auto_load_skills(
            &definition,
            &session_id,
            "empty-turn",
            Some(repo.as_ref()),
            Some(&caller),
            None,
        )
        .await;

        assert!(synthetic.is_empty());
        assert_eq!(caller.calls.lock().await.len(), 0);
    }
}
