use domain::{AgentDefinition, SessionId, SystemPromptSegment, ToolFilter};
use mcp::McpRegistry;
use tracing::warn;

use crate::history::{
    append_model_message, auto_load_skills_completed, load_model_history, load_session_context,
    persist_session_context, record_auto_load_skills_completed,
    seed_model_history_from_session_history,
};
use crate::primitives::{AgentMessage, ToolCall};
use crate::{Result, TurnInput};

/// The runtime advertises the trusted control-plane MCP server under the
/// `hivy` prefix, so `skill_view` is reachable as `hivy_skill_view`. Calling it
/// through the registry triggers the same `materialize` side effect a
/// model-initiated call would (see `mcp::McpRegistry::call_tool_for_session`).
const SKILL_VIEW_TOOL: &str = "hivy_skill_view";

/// Seam over the single `skill_view` invocation used by the auto-load
/// bootstrap. Production is backed by the real MCP registry (below), so the
/// call takes the exact same path — including the trusted-server `materialize`
/// side effect — as a model-initiated call. Tests supply a fake to exercise the
/// bootstrap without standing up an MCP server.
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
        self.call_tool_for_session(session_id, actor_user_id, SKILL_VIEW_TOOL, args)
            .await
    }
}

use super::capture_preloaded_context_error;

pub(super) struct InitialMessagePromptSources<'a> {
    pub(super) mcp_registry: Option<&'a McpRegistry>,
    pub(super) mcp_tool_filter: Option<&'a ToolFilter>,
}

pub(super) async fn build_initial_messages(
    snapshot: &AgentDefinition,
    session_id: &SessionId,
    input: TurnInput,
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
    let mut messages = vec![
        AgentMessage::system(render_cacheable_system_prompt(snapshot)),
        AgentMessage::system(
            render_dynamic_system_prompt(
                snapshot,
                prompt_sources.mcp_registry,
                prompt_sources.mcp_tool_filter,
                &session_context,
            )
            .await,
        ),
    ];
    let mut history = load_model_history(event_repo, session_id, 1000).await?;
    // "First turn" == no prior model history existed for this session before
    // this turn. Captured before any prior-history seeding so a freshly seeded
    // sub-agent (which had no model history yet) still bootstraps its skills.
    let is_first_turn = history.is_empty();
    if history.is_empty() && !input.prior_history.is_empty() {
        history =
            seed_model_history_from_session_history(event_repo, session_id, &input.prior_history)
                .await?;
    }
    messages.extend(history);
    // Skill auto-load bootstrap: on the first turn only, ahead of the first user
    // message, inject the resolved definition's declared skills so the agent
    // starts with them in context and their files materialized. Runs on the same
    // shared path for main sessions and sub-agent child sessions (both flow
    // through `build_initial_messages`), and always uses `snapshot` — the turn's
    // resolved definition — so a sub-agent auto-loads its own skills.
    if is_first_turn {
        let caller = prompt_sources
            .mcp_registry
            .map(|registry| registry as &dyn SkillViewCaller);
        let synthetic = inject_auto_load_skills(
            snapshot,
            session_id,
            event_repo,
            caller,
            input.actor_user_id.as_deref(),
        )
        .await;
        messages.extend(synthetic);
    }
    let user = AgentMessage::user(input.text);
    append_model_message(event_repo, session_id, &user).await?;
    messages.push(user);
    Ok(messages)
}

/// On the first turn, load the definition's `auto_load_skills` by invoking
/// `skill_view` through the MCP registry — the exact path a model-initiated call
/// takes, so the `materialize` side effect writes `.skills/<slug>/…` identically
/// — and return synthetic assistant-tool-call + tool-result messages mirroring
/// what the model would have produced. Persists the messages and a one-shot
/// idempotence marker so resume / later turns never re-inject. Any failure
/// (already loaded, no registry, `skill_view` error) logs a warning and skips;
/// the agent can still load the skill manually. Never returns an error.
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
    event_repo: Option<&dyn storage::EventRepo>,
    caller: Option<&dyn SkillViewCaller>,
    actor_user_id: Option<&str>,
) -> Vec<AgentMessage> {
    if snapshot.auto_load_skills.is_empty() {
        return Vec::new();
    }
    match auto_load_skills_completed(event_repo, session_id).await {
        Ok(true) => return Vec::new(),
        Ok(false) => {}
        Err(error) => {
            // Can't confirm idempotence — skip rather than risk a duplicate
            // injection. The agent can still load its skills manually.
            warn!(
                session_id = session_id.as_str(),
                %error, "skill auto-load skipped: idempotence check failed"
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
                    let tool_call_id = format!("autoload_{counter}");
                    counter += 1;
                    let assistant = AgentMessage::assistant_tool_calls(vec![ToolCall {
                        id: tool_call_id.clone(),
                        name: SKILL_VIEW_TOOL.to_string(),
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

    // One-shot: record the marker even if some/all entries failed so resume and
    // subsequent turns never re-inject.
    if let Err(error) = record_auto_load_skills_completed(event_repo, session_id).await {
        warn!(
            session_id = session_id.as_str(),
            %error, "skill auto-load: failed to persist idempotence marker"
        );
    }

    synthetic
}

pub(super) fn render_cacheable_system_prompt(snapshot: &AgentDefinition) -> String {
    let mut prompt = String::new();
    for segment in &snapshot.system_prompt.cacheable_segments {
        append_rendered_segment(&mut prompt, render_static_segment(segment));
    }
    prompt
}

pub(super) async fn render_dynamic_system_prompt(
    snapshot: &AgentDefinition,
    mcp_registry: Option<&McpRegistry>,
    mcp_tool_filter: Option<&ToolFilter>,
    session_context: &[String],
) -> String {
    let mut prompt = String::new();
    let mcp_tools = match mcp_registry {
        Some(registry) => registry.available_tool_names_filtered(mcp_tool_filter),
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
    if !mcp_tools.is_empty() {
        append_rendered_segment(
            &mut prompt,
            Some(
                "## MCP progressive discovery\nThe names in the MCP catalog are available capabilities, but their full schemas are intentionally hidden to preserve context. Use `search_tools` to find relevant names, then call `get_tool_details` with an exact name to inspect and activate that tool. The activated definition becomes directly callable on the next model request."
                    .to_string(),
            ),
        );
    }
    let has_mcp_tool_segment = snapshot
        .system_prompt
        .dynamic_segments
        .iter()
        .any(|segment| matches!(segment, SystemPromptSegment::McpTools(_)));
    for segment in &snapshot.system_prompt.dynamic_segments {
        let rendered = match segment {
            SystemPromptSegment::StaticText(_) => render_static_segment(segment),
            SystemPromptSegment::DynamicContext(config) => {
                render_dynamic_context_segment(config, session_context)
            }
            SystemPromptSegment::McpTools(config) => render_tool_list_segment(config, &mcp_tools),
        };
        append_rendered_segment(&mut prompt, rendered);
    }
    if !has_mcp_tool_segment && !mcp_tools.is_empty() {
        append_rendered_segment(
            &mut prompt,
            render_tool_list_segment(
                &domain::ListPromptSegment {
                    title: "Available MCP tool names".to_string(),
                    preamble: "Complete exact-name catalog:".to_string(),
                    item_template: "- {name}".to_string(),
                },
                &mcp_tools,
            ),
        );
    }

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
mod auto_load_tests {
    use super::{inject_auto_load_skills, SkillViewCaller};
    use crate::history::{auto_load_skills_completed, load_model_history};
    use crate::primitives::{AgentMessage, AgentMessageRole, MessagePart};
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
            self.calls.lock().await.push(args.clone());
            if self.fail {
                anyhow::bail!("skill_view boom");
            }
            let name = args.get("name").and_then(Value::as_str).unwrap_or_default();
            Ok(json!({
                "content": [{ "type": "text", "text": format!("loaded {name}") }],
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

    fn message_text(message: &AgentMessage) -> &str {
        match message.parts.first() {
            Some(MessagePart::Text { text }) => text.as_str(),
            None => "",
        }
    }

    #[tokio::test]
    async fn first_turn_injects_synthetic_pairs_then_second_turn_does_not() {
        let repo = Arc::new(MemoryEventRepo::default());
        let caller = FakeSkillViewCaller::ok();
        let session_id = SessionId::from("s-autoload-first");
        let definition =
            definition_with_auto_load(r#"[{"name":"browser","files":["references/commands.md"]}]"#);

        let synthetic = inject_auto_load_skills(
            &definition,
            &session_id,
            Some(repo.as_ref()),
            Some(&caller),
            None,
        )
        .await;

        // Skill root + one linked file = two skill_view calls => two pairs.
        assert_eq!(synthetic.len(), 4, "expected two assistant/tool pairs");

        // First call: the skill root, with clean model-visible arguments (no
        // hidden _hivy_ fields — those are injected inside the registry only).
        assert_eq!(synthetic[0].role, AgentMessageRole::Assistant);
        assert_eq!(synthetic[0].tool_calls.len(), 1);
        assert_eq!(synthetic[0].tool_calls[0].name, "hivy_skill_view");
        assert_eq!(synthetic[0].tool_calls[0].id, "autoload_0");
        assert_eq!(
            synthetic[0].tool_calls[0].arguments,
            json!({ "name": "browser" })
        );
        assert_eq!(synthetic[1].role, AgentMessageRole::Tool);
        assert_eq!(synthetic[1].tool_call_id.as_deref(), Some("autoload_0"));
        assert!(message_text(&synthetic[1]).contains("loaded browser"));

        // Second call: the linked file.
        assert_eq!(
            synthetic[2].tool_calls[0].arguments,
            json!({ "name": "browser", "file_path": "references/commands.md" })
        );
        assert_eq!(synthetic[2].tool_calls[0].id, "autoload_1");

        // The caller received exactly the two expected requests.
        assert_eq!(caller.calls.lock().await.len(), 2);

        // Marker recorded; the synthetic pairs are persisted as model history.
        assert!(auto_load_skills_completed(Some(repo.as_ref()), &session_id)
            .await
            .unwrap());
        assert_eq!(
            load_model_history(Some(repo.as_ref()), &session_id, 100)
                .await
                .unwrap()
                .len(),
            4
        );

        // Second turn: marker present => no re-injection, no further calls.
        let second = inject_auto_load_skills(
            &definition,
            &session_id,
            Some(repo.as_ref()),
            Some(&caller),
            None,
        )
        .await;
        assert!(second.is_empty());
        assert_eq!(
            caller.calls.lock().await.len(),
            2,
            "must not re-call skill_view"
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
            Some(repo.as_ref()),
            Some(&caller),
            None,
        )
        .await;

        // Failure => no synthetic messages, but the turn is not failed and the
        // marker is recorded so we never retry-loop on resume.
        assert!(synthetic.is_empty());
        assert_eq!(caller.calls.lock().await.len(), 1);
        assert!(auto_load_skills_completed(Some(repo.as_ref()), &session_id)
            .await
            .unwrap());
        assert!(load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap()
            .is_empty());
    }

    #[tokio::test]
    async fn no_registry_skips_without_marking_done() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s-autoload-noreg");
        let definition = definition_with_auto_load(r#"[{"name":"browser"}]"#);

        let synthetic =
            inject_auto_load_skills(&definition, &session_id, Some(repo.as_ref()), None, None)
                .await;

        assert!(synthetic.is_empty());
        // No registry is environmental; leave the marker unset so a later,
        // correctly-provisioned turn can still bootstrap.
        assert!(
            !auto_load_skills_completed(Some(repo.as_ref()), &session_id)
                .await
                .unwrap()
        );
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
            Some(repo.as_ref()),
            Some(&caller),
            None,
        )
        .await;

        assert!(synthetic.is_empty());
        assert_eq!(caller.calls.lock().await.len(), 0);
        assert!(
            !auto_load_skills_completed(Some(repo.as_ref()), &session_id)
                .await
                .unwrap()
        );
    }
}
