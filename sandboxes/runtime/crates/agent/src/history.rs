use domain::{EventKind, SessionEvent, SessionId};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeSet, HashSet};
use storage::EventRepo;

use crate::primitives::{AgentMessage, AgentMessageRole};
use crate::{HistoryEntry, HistoryRole, Result};

const HISTORY_PAYLOAD_VERSION: u32 = 1;
const MODEL_HISTORY_REDACTION_VERSION: u32 = 1;
const SESSION_CONTEXT_PAYLOAD_VERSION: u32 = 1;
const SESSION_CONTEXT_IDEMPOTENCY_KEY_PREFIX: &str = "session-context";
const MODEL_HISTORY_CONTROL_REDACTION: &str = "redaction";
const MODEL_TURN_PAYLOAD_VERSION: u32 = 1;
const MODEL_HISTORY_CONTROL_TURN: &str = "turn";
const MODEL_TURN_IDEMPOTENCY_KEY_PREFIX: &str = "model-turn";
const AUTO_LOAD_SKILLS_TURN_PAYLOAD_VERSION: u32 = 1;
const AUTO_LOAD_SKILLS_TURN_CONTROL: &str = "auto_load_skills_turn";
const AUTO_LOAD_SKILLS_TURN_IDEMPOTENCY_KEY_PREFIX: &str = "auto-load-skills-turn";
pub const SKILL_VIEW_TOOL_NAME: &str = "hivy_skill_view";
pub const LOAD_TOOLS_TOOL_NAME: &str = "load_tools";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModelHistoryPayload {
    pub version: u32,
    pub message: AgentMessage,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct SessionContextPayload {
    version: u32,
    session_context: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct ModelHistoryRedactionPayload {
    model_history_control: String,
    version: u32,
    start_tool_call_id: String,
    reason: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct ModelTurnPayload {
    model_history_control: String,
    version: u32,
    turn_id: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct AutoLoadSkillsTurnPayload {
    model_history_control: String,
    version: u32,
    turn_id: String,
}

pub async fn load_model_history(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    limit: u32,
) -> Result<Vec<AgentMessage>> {
    load_model_history_for_turn(repo, session_id, limit, None).await
}

pub async fn load_model_history_for_turn(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    limit: u32,
    current_turn_id: Option<&str>,
) -> Result<Vec<AgentMessage>> {
    let Some(repo) = repo else {
        return Ok(Vec::new());
    };
    let events = repo
        .list_chronological(session_id, limit)
        .await
        .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    let current_turn_start = current_turn_id.and_then(|turn_id| {
        events.iter().rposition(|event| {
            model_turn_from_event(event).is_some_and(|payload| payload.turn_id == turn_id)
        })
    });
    let mut messages = Vec::new();
    let mut current_turn_skill_call_ids = HashSet::new();
    let mut current_turn_tool_load_call_ids = HashSet::new();
    for (event_index, event) in events.into_iter().enumerate() {
        if let Some(redaction) = redaction_from_event(&event) {
            apply_redaction(&mut messages, &redaction.start_tool_call_id);
            continue;
        }
        if let Some(message) = message_from_event(&event) {
            if current_turn_start.is_some_and(|start| event_index > start) {
                current_turn_skill_call_ids.extend(
                    message
                        .tool_calls
                        .iter()
                        .filter(|call| call.name == SKILL_VIEW_TOOL_NAME)
                        .map(|call| call.id.clone()),
                );
                current_turn_tool_load_call_ids.extend(
                    message
                        .tool_calls
                        .iter()
                        .filter(|call| call.name == LOAD_TOOLS_TOOL_NAME)
                        .map(|call| call.id.clone()),
                );
            }
            messages.push(message);
        }
    }
    let messages = prune_prior_skill_loads(messages, &current_turn_skill_call_ids);
    Ok(prune_prior_tool_loads(
        messages,
        &current_turn_tool_load_call_ids,
    ))
}

pub async fn append_model_message(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    message: &AgentMessage,
) -> Result<()> {
    let Some(repo) = repo else {
        return Ok(());
    };
    let payload = serde_json::to_value(ModelHistoryPayload {
        version: HISTORY_PAYLOAD_VERSION,
        message: message.clone(),
    })
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    repo.append(session_id, event_kind_for_message(message), payload)
        .await
        .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(())
}

pub async fn append_model_history_redaction(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    start_tool_call_id: &str,
    reason: &str,
) -> Result<()> {
    let Some(repo) = repo else {
        return Ok(());
    };
    let payload = serde_json::to_value(ModelHistoryRedactionPayload {
        model_history_control: MODEL_HISTORY_CONTROL_REDACTION.to_string(),
        version: MODEL_HISTORY_REDACTION_VERSION,
        start_tool_call_id: start_tool_call_id.to_string(),
        reason: reason.to_string(),
    })
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    repo.append(session_id, EventKind::RunEvent, payload)
        .await
        .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(())
}

pub async fn persist_session_context(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    session_context: &[String],
) -> Result<()> {
    let Some(repo) = repo else {
        return Ok(());
    };
    let session_context: Vec<String> = session_context
        .iter()
        .map(|item| item.trim())
        .filter(|item| !item.is_empty())
        .map(ToString::to_string)
        .collect();
    if session_context.is_empty() {
        return Ok(());
    }
    let payload = serde_json::to_value(SessionContextPayload {
        version: SESSION_CONTEXT_PAYLOAD_VERSION,
        session_context,
    })
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    repo.append_idempotent(
        session_id,
        EventKind::RunEvent,
        payload,
        &session_context_idempotency_key(session_id),
    )
    .await
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(())
}

pub async fn load_session_context(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
) -> Result<Vec<String>> {
    let Some(repo) = repo else {
        return Ok(Vec::new());
    };
    let events = repo
        .list_chronological(session_id, 1000)
        .await
        .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(events
        .into_iter()
        .filter_map(session_context_from_event)
        .next_back()
        .unwrap_or_default())
}

pub async fn record_model_turn_started(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    turn_id: &str,
) -> Result<()> {
    let Some(repo) = repo else {
        return Ok(());
    };
    let payload = serde_json::to_value(ModelTurnPayload {
        model_history_control: MODEL_HISTORY_CONTROL_TURN.to_string(),
        version: MODEL_TURN_PAYLOAD_VERSION,
        turn_id: turn_id.to_string(),
    })
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    repo.append_idempotent(
        session_id,
        EventKind::RunEvent,
        payload,
        &turn_scoped_idempotency_key(MODEL_TURN_IDEMPOTENCY_KEY_PREFIX, session_id, turn_id),
    )
    .await
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(())
}

pub async fn auto_load_skills_completed_for_turn(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    turn_id: &str,
) -> Result<bool> {
    let Some(repo) = repo else {
        return Ok(false);
    };
    let events = repo
        .list_chronological(session_id, 1000)
        .await
        .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(events.iter().any(|event| {
        auto_load_skills_turn_from_event(event).is_some_and(|payload| payload.turn_id == turn_id)
    }))
}

pub async fn record_auto_load_skills_completed_for_turn(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    turn_id: &str,
) -> Result<()> {
    let Some(repo) = repo else {
        return Ok(());
    };
    let payload = serde_json::to_value(AutoLoadSkillsTurnPayload {
        model_history_control: AUTO_LOAD_SKILLS_TURN_CONTROL.to_string(),
        version: AUTO_LOAD_SKILLS_TURN_PAYLOAD_VERSION,
        turn_id: turn_id.to_string(),
    })
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    repo.append_idempotent(
        session_id,
        EventKind::RunEvent,
        payload,
        &turn_scoped_idempotency_key(
            AUTO_LOAD_SKILLS_TURN_IDEMPOTENCY_KEY_PREFIX,
            session_id,
            turn_id,
        ),
    )
    .await
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(())
}

fn session_context_idempotency_key(session_id: &SessionId) -> String {
    format!(
        "{SESSION_CONTEXT_IDEMPOTENCY_KEY_PREFIX}:{}",
        session_id.as_str()
    )
}

/// Skill bodies are intentionally turn-scoped. Persisted tool messages remain
/// available for audit and debugging, but earlier `skill_view` calls are
/// projected into one compact reminder before a later model turn.
fn prune_prior_skill_loads(
    messages: Vec<AgentMessage>,
    current_turn_skill_call_ids: &HashSet<String>,
) -> Vec<AgentMessage> {
    let mut skill_call_ids = HashSet::new();
    let mut skill_names = BTreeSet::new();

    for message in &messages {
        for call in &message.tool_calls {
            if call.name != SKILL_VIEW_TOOL_NAME || current_turn_skill_call_ids.contains(&call.id) {
                continue;
            }
            skill_call_ids.insert(call.id.clone());
            if let Some(name) = safe_skill_name(
                call.arguments
                    .get("name")
                    .and_then(serde_json::Value::as_str),
            ) {
                skill_names.insert(name);
            }
        }
    }
    if skill_call_ids.is_empty() {
        return messages;
    }

    let mut projected = Vec::with_capacity(messages.len());
    for mut message in messages {
        if message.role == AgentMessageRole::Assistant {
            message
                .tool_calls
                .retain(|call| !skill_call_ids.contains(&call.id));
            if message.tool_calls.is_empty() && message.parts.is_empty() {
                continue;
            }
        }
        if message.role == AgentMessageRole::Tool
            && message
                .tool_call_id
                .as_ref()
                .is_some_and(|id| skill_call_ids.contains(id))
        {
            continue;
        }
        projected.push(message);
    }

    let loaded = if skill_names.is_empty() {
        "one or more skills".to_string()
    } else {
        skill_names
            .into_iter()
            .map(|name| format!("`{name}`"))
            .collect::<Vec<_>>()
            .join(", ")
    };
    let reminder = AgentMessage::system(format!(
        "[system instruction] You loaded {loaded} in an earlier turn. Their skill content was pruned because skill loads are turn-scoped. If any skill is needed for the current task, load it again with `{SKILL_VIEW_TOOL_NAME}` before relying on it."
    ));
    let reminder_index = projected
        .iter()
        .position(|message| {
            message
                .tool_calls
                .iter()
                .any(|call| current_turn_skill_call_ids.contains(&call.id))
        })
        .unwrap_or(projected.len());
    projected.insert(reminder_index, reminder);
    projected
}

/// MCP tool schemas expire at the turn boundary. Keep raw loader events for
/// audit, but remove earlier `load_tools` calls/results from model-visible
/// history so their success payload cannot be mistaken for current state.
fn prune_prior_tool_loads(
    messages: Vec<AgentMessage>,
    current_turn_tool_load_call_ids: &HashSet<String>,
) -> Vec<AgentMessage> {
    let mut prior_call_ids = HashSet::new();
    let mut tool_names = BTreeSet::new();
    for message in &messages {
        for call in &message.tool_calls {
            if call.name != LOAD_TOOLS_TOOL_NAME
                || current_turn_tool_load_call_ids.contains(&call.id)
            {
                continue;
            }
            prior_call_ids.insert(call.id.clone());
            if let Some(names) = call
                .arguments
                .get("tool_names")
                .and_then(serde_json::Value::as_array)
            {
                for name in names {
                    if let Some(name) = safe_skill_name(name.as_str()) {
                        tool_names.insert(name);
                    }
                }
            }
        }
    }
    if prior_call_ids.is_empty() {
        return messages;
    }

    let mut projected = Vec::with_capacity(messages.len());
    for mut message in messages {
        if message.role == AgentMessageRole::Assistant {
            message
                .tool_calls
                .retain(|call| !prior_call_ids.contains(&call.id));
            if message.tool_calls.is_empty() && message.parts.is_empty() {
                continue;
            }
        }
        if message.role == AgentMessageRole::Tool
            && message
                .tool_call_id
                .as_ref()
                .is_some_and(|id| prior_call_ids.contains(id))
        {
            continue;
        }
        projected.push(message);
    }

    let loaded = if tool_names.is_empty() {
        "MCP tools".to_string()
    } else {
        tool_names
            .into_iter()
            .map(|name| format!("`{name}`"))
            .collect::<Vec<_>>()
            .join(", ")
    };
    let reminder = AgentMessage::system(format!(
        "[system instruction] You loaded {loaded} in an earlier turn. Those MCP tool schemas expired at the turn boundary. If they are needed now, call `{LOAD_TOOLS_TOOL_NAME}` again for this turn."
    ));
    let reminder_index = projected
        .iter()
        .position(|message| {
            message
                .tool_calls
                .iter()
                .any(|call| current_turn_tool_load_call_ids.contains(&call.id))
        })
        .unwrap_or(projected.len());
    projected.insert(reminder_index, reminder);
    projected
}

fn safe_skill_name(name: Option<&str>) -> Option<String> {
    let name = name?.trim();
    if name.is_empty()
        || name.len() > 128
        || !name
            .chars()
            .all(|character| character.is_ascii_alphanumeric() || "._/-".contains(character))
    {
        return None;
    }
    Some(name.to_string())
}

fn model_turn_from_event(event: &SessionEvent) -> Option<ModelTurnPayload> {
    if event.kind != EventKind::RunEvent {
        return None;
    }
    let payload: ModelTurnPayload = serde_json::from_value(event.payload.clone()).ok()?;
    if payload.version != MODEL_TURN_PAYLOAD_VERSION
        || payload.model_history_control != MODEL_HISTORY_CONTROL_TURN
        || payload.turn_id.trim().is_empty()
    {
        return None;
    }
    Some(payload)
}

fn auto_load_skills_turn_from_event(event: &SessionEvent) -> Option<AutoLoadSkillsTurnPayload> {
    if event.kind != EventKind::RunEvent {
        return None;
    }
    let payload: AutoLoadSkillsTurnPayload = serde_json::from_value(event.payload.clone()).ok()?;
    if payload.version != AUTO_LOAD_SKILLS_TURN_PAYLOAD_VERSION
        || payload.model_history_control != AUTO_LOAD_SKILLS_TURN_CONTROL
        || payload.turn_id.trim().is_empty()
    {
        return None;
    }
    Some(payload)
}

fn turn_scoped_idempotency_key(prefix: &str, session_id: &SessionId, turn_id: &str) -> String {
    format!("{prefix}:{}:{turn_id}", session_id.as_str())
}

pub async fn seed_model_history_from_session_history(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    history: &[HistoryEntry],
) -> Result<Vec<AgentMessage>> {
    let mut messages = Vec::new();
    for entry in history {
        let message = match entry.role {
            HistoryRole::User => AgentMessage::user(format_history_user(
                entry.speaker_id.clone(),
                entry.speaker_display_name.clone(),
                entry.text.clone(),
            )),
            HistoryRole::Assistant => AgentMessage::assistant(entry.text.clone()),
        };
        append_model_message(repo, session_id, &message).await?;
        messages.push(message);
    }
    Ok(messages)
}

pub fn visible_messages_from_session_history(history: Vec<HistoryEntry>) -> Vec<AgentMessage> {
    history
        .into_iter()
        .map(|entry| match entry.role {
            HistoryRole::User => AgentMessage::user(format_history_user(
                entry.speaker_id,
                entry.speaker_display_name,
                entry.text,
            )),
            HistoryRole::Assistant => AgentMessage::assistant(entry.text),
        })
        .collect()
}

fn message_from_event(event: &SessionEvent) -> Option<AgentMessage> {
    let payload: ModelHistoryPayload = serde_json::from_value(event.payload.clone()).ok()?;
    if payload.version != HISTORY_PAYLOAD_VERSION {
        return None;
    }
    Some(payload.message)
}

fn redaction_from_event(event: &SessionEvent) -> Option<ModelHistoryRedactionPayload> {
    if event.kind != EventKind::RunEvent {
        return None;
    }
    let payload: ModelHistoryRedactionPayload =
        serde_json::from_value(event.payload.clone()).ok()?;
    if payload.version != MODEL_HISTORY_REDACTION_VERSION
        || payload.model_history_control != MODEL_HISTORY_CONTROL_REDACTION
        || payload.start_tool_call_id.trim().is_empty()
    {
        return None;
    }
    Some(payload)
}

fn apply_redaction(messages: &mut Vec<AgentMessage>, start_tool_call_id: &str) {
    let Some(index) = messages.iter().position(|message| {
        message.role == AgentMessageRole::Assistant
            && message
                .tool_calls
                .iter()
                .any(|call| call.id == start_tool_call_id)
    }) else {
        return;
    };
    messages.truncate(index);
}

fn session_context_from_event(event: SessionEvent) -> Option<Vec<String>> {
    if event.kind != EventKind::RunEvent {
        return None;
    }
    let payload: SessionContextPayload = serde_json::from_value(event.payload).ok()?;
    if payload.version != SESSION_CONTEXT_PAYLOAD_VERSION {
        return None;
    }
    let session_context: Vec<String> = payload
        .session_context
        .into_iter()
        .map(|item| item.trim().to_string())
        .filter(|item| !item.is_empty())
        .collect();
    if session_context.is_empty() {
        None
    } else {
        Some(session_context)
    }
}

fn event_kind_for_message(message: &AgentMessage) -> EventKind {
    match message.role {
        AgentMessageRole::User => EventKind::UserMessage,
        AgentMessageRole::Assistant if !message.tool_calls.is_empty() => EventKind::ToolCall,
        AgentMessageRole::Assistant => EventKind::AssistantMessage,
        AgentMessageRole::Tool => EventKind::ToolResult,
        AgentMessageRole::System => EventKind::AssistantMessage,
    }
}

fn format_history_user(user_id: String, name: Option<String>, text: String) -> String {
    let user_id = user_id.trim();
    match name.map(|name| name.trim().to_string()) {
        Some(name) if !name.is_empty() && !user_id.is_empty() => {
            format!("{name} ({user_id}): {text}")
        }
        Some(name) if !name.is_empty() => format!("{name}: {text}"),
        _ if !user_id.is_empty() && user_id != "cron" && user_id != "bot" => {
            format!("{user_id}: {text}")
        }
        _ => text,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::primitives::ToolCall;
    use chrono::Utc;
    use std::sync::Arc;
    use tokio::sync::Mutex;

    #[derive(Default)]
    struct MemoryEventRepo {
        events: Mutex<Vec<SessionEvent>>,
    }

    #[async_trait::async_trait]
    impl EventRepo for MemoryEventRepo {
        async fn append(
            &self,
            session_id: &SessionId,
            kind: EventKind,
            payload: serde_json::Value,
        ) -> storage::Result<i64> {
            let mut events = self.events.lock().await;
            let id = events.len() as i64 + 1;
            events.push(SessionEvent {
                id,
                session_id: session_id.clone(),
                seq: id,
                kind,
                payload,
                created_at: Utc::now(),
            });
            Ok(id)
        }

        async fn append_idempotent(
            &self,
            session_id: &SessionId,
            kind: EventKind,
            payload: serde_json::Value,
            idempotency_key: &str,
        ) -> storage::Result<Option<i64>> {
            let mut payload = payload;
            if let Some(object) = payload.as_object_mut() {
                object.insert(
                    "_idempotency_key".to_string(),
                    serde_json::Value::String(idempotency_key.to_string()),
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

    #[tokio::test]
    async fn history_round_trips_tool_messages() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s1");
        let user = AgentMessage::user("hello");
        let calls = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "call_1".into(),
            name: "lookup".into(),
            arguments: serde_json::json!({"q":"x"}),
        }]);
        let result = AgentMessage::tool_result("call_1", "{\"ok\":true}");
        let final_message = AgentMessage::assistant("done");

        for message in [&user, &calls, &result, &final_message] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        let loaded = load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap();
        assert_eq!(loaded.len(), 4);
        assert_eq!(loaded[0].role, AgentMessageRole::User);
        assert_eq!(loaded[1].role, AgentMessageRole::Assistant);
        assert_eq!(loaded[1].tool_calls[0].name, "lookup");
        assert_eq!(loaded[2].role, AgentMessageRole::Tool);
        assert_eq!(loaded[3].role, AgentMessageRole::Assistant);
    }

    #[tokio::test]
    async fn session_context_persists_per_session_without_becoming_model_history() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s-context");
        persist_session_context(
            Some(repo.as_ref()),
            &session_id,
            &[
                "## Recent sessions\n- Previous Slack thread".to_string(),
                "  ".to_string(),
            ],
        )
        .await
        .unwrap();
        persist_session_context(
            Some(repo.as_ref()),
            &session_id,
            &["## Recent sessions\n- Duplicate should be ignored".to_string()],
        )
        .await
        .unwrap();

        let loaded = load_session_context(Some(repo.as_ref()), &session_id)
            .await
            .unwrap();
        assert_eq!(loaded, vec!["## Recent sessions\n- Previous Slack thread"]);
        assert!(load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap()
            .is_empty());
    }

    #[tokio::test]
    async fn model_history_redaction_removes_loop_segment_from_projection() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s-redaction");
        let user = AgentMessage::user("check the repo");
        let first_call = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "call_1".into(),
            name: "bash".into(),
            arguments: serde_json::json!({"command":"pwd"}),
        }]);
        let first_result = AgentMessage::tool_result("call_1", "{\"ok\":true}");
        let second_call = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "call_2".into(),
            name: "bash".into(),
            arguments: serde_json::json!({"command":"pwd"}),
        }]);
        let second_result = AgentMessage::tool_result("call_2", "{\"ok\":true}");
        let instruction = AgentMessage::user(
            "[system instruction] I removed a repeated tool-call loop from the context.",
        );

        for message in [
            &user,
            &first_call,
            &first_result,
            &second_call,
            &second_result,
        ] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }
        append_model_history_redaction(Some(repo.as_ref()), &session_id, "call_1", "repeat")
            .await
            .unwrap();
        append_model_message(Some(repo.as_ref()), &session_id, &instruction)
            .await
            .unwrap();

        let loaded = load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap();
        assert_eq!(loaded.len(), 2);
        assert_eq!(loaded[0].role, AgentMessageRole::User);
        assert!(loaded[1].tool_calls.is_empty());
        let crate::primitives::MessagePart::Text { text } = &loaded[1].parts[0];
        assert!(text.contains("repeated tool-call loop"));
    }

    #[tokio::test]
    async fn session_context_idempotency_is_scoped_by_session() {
        let repo = Arc::new(MemoryEventRepo::default());
        let first = SessionId::from("s-context-one");
        let second = SessionId::from("s-context-two");

        persist_session_context(
            Some(repo.as_ref()),
            &first,
            &["## Recent sessions\n- First session".to_string()],
        )
        .await
        .unwrap();
        persist_session_context(
            Some(repo.as_ref()),
            &second,
            &["## Recent sessions\n- Second session".to_string()],
        )
        .await
        .unwrap();

        assert_eq!(
            load_session_context(Some(repo.as_ref()), &first)
                .await
                .unwrap(),
            vec!["## Recent sessions\n- First session"]
        );
        assert_eq!(
            load_session_context(Some(repo.as_ref()), &second)
                .await
                .unwrap(),
            vec!["## Recent sessions\n- Second session"]
        );
    }

    #[tokio::test]
    async fn session_history_seed_only_contains_visible_roles() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s2");
        let seeded = seed_model_history_from_session_history(
            Some(repo.as_ref()),
            &session_id,
            &[
                HistoryEntry {
                    role: HistoryRole::User,
                    speaker_id: "U123".into(),
                    speaker_display_name: Some("Kim".into()),
                    text: "hi".into(),
                },
                HistoryEntry {
                    role: HistoryRole::Assistant,
                    speaker_id: "bot".into(),
                    speaker_display_name: None,
                    text: "hello".into(),
                },
            ],
        )
        .await
        .unwrap();

        assert_eq!(seeded.len(), 2);
        let crate::primitives::MessagePart::Text { text: seeded_text } = &seeded[0].parts[0];
        assert_eq!(seeded_text, "Kim (U123): hi");
        assert_eq!(
            load_model_history(Some(repo.as_ref()), &session_id, 100)
                .await
                .unwrap()
                .len(),
            2
        );
    }

    #[tokio::test]
    async fn next_turn_preserves_tool_prefix_before_new_user_message() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s3");
        let first_user = AgentMessage::user("find issues");
        let calls = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "tool_1".into(),
            name: "linear_list_issues".into(),
            arguments: serde_json::json!({"team":"PSL"}),
        }]);
        let tool_result = AgentMessage::tool_result("tool_1", "{\"issues\":[\"A\"]}");
        let final_reply = AgentMessage::assistant("Found one issue.");

        for message in [&first_user, &calls, &tool_result, &final_reply] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        let mut second_turn = load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap();
        second_turn.push(AgentMessage::user("tell me more"));

        assert_eq!(second_turn[0].role, AgentMessageRole::User);
        assert_eq!(second_turn[1].role, AgentMessageRole::Assistant);
        assert_eq!(second_turn[1].tool_calls[0].name, "linear_list_issues");
        assert_eq!(second_turn[2].role, AgentMessageRole::Tool);
        assert_eq!(second_turn[4].role, AgentMessageRole::User);
    }

    #[tokio::test]
    async fn prior_skill_load_content_is_replaced_with_one_reload_reminder() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s-skill-pruning");
        let first_user = AgentMessage::user("use the browser skill");
        let root_call = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "skill-root".into(),
            name: SKILL_VIEW_TOOL_NAME.into(),
            arguments: serde_json::json!({"name":"browser"}),
        }]);
        let root_result =
            AgentMessage::tool_result("skill-root", "TURN_ONE_ROOT_SKILL_BODY_SENTINEL");
        let file_call = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "skill-file".into(),
            name: SKILL_VIEW_TOOL_NAME.into(),
            arguments: serde_json::json!({
                "name":"browser",
                "file_path":"references/commands.md"
            }),
        }]);
        let file_result =
            AgentMessage::tool_result("skill-file", "TURN_ONE_FILE_SKILL_BODY_SENTINEL");
        let reply = AgentMessage::assistant("Finished the browser task.");

        for message in [
            &first_user,
            &root_call,
            &root_result,
            &file_call,
            &file_result,
            &reply,
        ] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        let projected = load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap();
        let projected_json = serde_json::to_string(&projected).unwrap();
        assert!(!projected_json.contains("TURN_ONE_ROOT_SKILL_BODY_SENTINEL"));
        assert!(!projected_json.contains("TURN_ONE_FILE_SKILL_BODY_SENTINEL"));
        assert!(!projected_json.contains("skill-root"));
        assert!(!projected_json.contains("skill-file"));
        assert_eq!(
            projected
                .iter()
                .filter(|message| {
                    serde_json::to_string(message)
                        .unwrap()
                        .contains("skill loads are turn-scoped")
                })
                .count(),
            1
        );
        assert!(projected_json.contains("`browser`"));
        assert!(projected_json.contains("load it again"));
        assert!(projected_json.contains("Finished the browser task."));

        // Projection is non-destructive: the raw event log retains the original
        // result for audit/debugging even though the model no longer receives it.
        let raw_events = repo.events.lock().await;
        let raw_json = serde_json::to_string(&*raw_events).unwrap();
        assert!(raw_json.contains("TURN_ONE_ROOT_SKILL_BODY_SENTINEL"));
        assert!(raw_json.contains("TURN_ONE_FILE_SKILL_BODY_SENTINEL"));
    }

    #[tokio::test]
    async fn prior_turn_tool_load_is_pruned_while_current_turn_reload_is_retained() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s-tool-load-pruning");
        record_model_turn_started(Some(repo.as_ref()), &session_id, "turn-one")
            .await
            .unwrap();
        let prior_calls = AgentMessage::assistant_tool_calls(vec![
            ToolCall {
                id: "load-prior".into(),
                name: LOAD_TOOLS_TOOL_NAME.into(),
                arguments: serde_json::json!({
                    "tool_names": ["github_list_issues", "slack_post_message"]
                }),
            },
            ToolCall {
                id: "bash-prior".into(),
                name: "bash".into(),
                arguments: serde_json::json!({"command":"pwd"}),
            },
        ]);
        let prior_load_result =
            AgentMessage::tool_result("load-prior", "PRIOR_LOAD_RESULT_SENTINEL");
        let prior_bash_result =
            AgentMessage::tool_result("bash-prior", "PRIOR_BASH_RESULT_SENTINEL");
        for message in [&prior_calls, &prior_load_result, &prior_bash_result] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        record_model_turn_started(Some(repo.as_ref()), &session_id, "turn-two")
            .await
            .unwrap();
        let current_call = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "load-current".into(),
            name: LOAD_TOOLS_TOOL_NAME.into(),
            arguments: serde_json::json!({"tool_names":["github_list_issues"]}),
        }]);
        let current_result =
            AgentMessage::tool_result("load-current", "CURRENT_LOAD_RESULT_SENTINEL");
        for message in [&current_call, &current_result] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        let projected =
            load_model_history_for_turn(Some(repo.as_ref()), &session_id, 100, Some("turn-two"))
                .await
                .unwrap();
        let projected_json = serde_json::to_string(&projected).unwrap();
        assert!(!projected_json.contains("load-prior"));
        assert!(!projected_json.contains("PRIOR_LOAD_RESULT_SENTINEL"));
        assert!(projected_json.contains("bash-prior"));
        assert!(projected_json.contains("PRIOR_BASH_RESULT_SENTINEL"));
        assert!(projected_json.contains("load-current"));
        assert!(projected_json.contains("CURRENT_LOAD_RESULT_SENTINEL"));
        assert!(projected_json.contains("expired at the turn boundary"));
        assert!(projected_json.contains("`github_list_issues`"));
        assert!(projected_json.contains("`slack_post_message`"));

        let raw_json = serde_json::to_string(&*repo.events.lock().await).unwrap();
        assert!(raw_json.contains("PRIOR_LOAD_RESULT_SENTINEL"));
    }

    #[tokio::test]
    async fn skill_pruning_preserves_parallel_non_skill_calls_and_results() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s-skill-parallel");
        let calls = AgentMessage::assistant_tool_calls(vec![
            ToolCall {
                id: "skill-1".into(),
                name: SKILL_VIEW_TOOL_NAME.into(),
                arguments: serde_json::json!({"name":"browser"}),
            },
            ToolCall {
                id: "bash-1".into(),
                name: "bash".into(),
                arguments: serde_json::json!({"command":"pwd"}),
            },
        ]);
        let skill_result = AgentMessage::tool_result("skill-1", "SKILL_BODY_SENTINEL");
        let bash_result = AgentMessage::tool_result("bash-1", r#"{"output":"/workspace"}"#);

        for message in [&calls, &skill_result, &bash_result] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        let projected = load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap();
        let bash_call = projected
            .iter()
            .find(|message| message.tool_calls.iter().any(|call| call.id == "bash-1"))
            .expect("parallel bash call remains");
        assert_eq!(bash_call.tool_calls.len(), 1);
        assert_eq!(bash_call.tool_calls[0].id, "bash-1");
        assert!(projected.iter().any(|message| {
            message.role == AgentMessageRole::Tool
                && message.tool_call_id.as_deref() == Some("bash-1")
        }));
        assert!(!projected.iter().any(|message| {
            message.tool_call_id.as_deref() == Some("skill-1")
                || message.tool_calls.iter().any(|call| call.id == "skill-1")
        }));
        assert!(!serde_json::to_string(&projected)
            .unwrap()
            .contains("SKILL_BODY_SENTINEL"));
    }

    #[test]
    fn visible_session_history_keeps_user_id_with_name() {
        let messages = visible_messages_from_session_history(vec![HistoryEntry {
            role: HistoryRole::User,
            speaker_id: "U08P1G9EDNG".into(),
            speaker_display_name: Some("Nora".into()),
            text: "Loop me in on invoice failures.".into(),
        }]);
        let crate::primitives::MessagePart::Text { text } = &messages[0].parts[0];
        assert_eq!(text, "Nora (U08P1G9EDNG): Loop me in on invoice failures.");
    }
}
