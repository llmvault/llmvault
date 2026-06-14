#![allow(clippy::items_after_test_module)]

mod composition;
mod media;
mod session;

use std::sync::Arc;

use agent::{AgentEvent, AgentRunner, TurnInput};
use anyhow::Result;
use async_trait::async_trait;
use chrono::Utc;
use domain::{
    event_types, ConfigStore, InboundEvent, OutboundEvent, SessionId, SessionStatus,
    SubagentTaskState,
};
use futures::StreamExt;
use outbound::OutboundEmitter;
use serde_json::Value;
use storage::{CronJobRepo, SessionRepo, SubagentTaskRepo};
use tokio::sync::mpsc;
use tracing::{info, warn};

use composition::compose_annotated_text;
use media::{collect_media_for_turn, DownloadResults};
pub use media::{AttachmentDownloader, HttpAttachmentDownloader};
use session::ensure_session_persisted;

use crate::session_coordinator::{SessionCoordinator, Submission};

const USER_RETRY_MESSAGE: &str =
    "Something went wrong while generating that response. Please retry. The Hivy team has been notified of this error.";

#[derive(Clone, Debug)]
pub struct StreamEventMetadata {
    pub subagent: SubagentStreamMetadata,
}

#[derive(Clone, Debug)]
pub struct SubagentStreamMetadata {
    pub job_id: String,
    pub agent_name: String,
    pub parent_session_id: String,
    pub child_session_id: String,
}

#[async_trait]
pub trait TurnEventSink: Send + Sync + 'static {
    async fn activate_session_stream(&self, _session_id: &SessionId, _stream_id: &str) {}

    async fn clear_active_session_stream(&self, _session_id: &SessionId, _stream_id: &str) {}

    async fn publish_final(
        &self,
        _stream_id: &str,
        _session_id: &SessionId,
        _text: &str,
        _metadata: Option<&StreamEventMetadata>,
    ) {
    }

    async fn publish_done(&self, _stream_id: &str, _session_id: &SessionId) {}

    async fn publish_waiting(&self, _stream_id: &str, _session_id: &SessionId, _reason: &str) {}

    async fn publish_agent_event(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        event: &AgentEvent,
        metadata: Option<&StreamEventMetadata>,
    );

    /// Emit on the PARENT stream when a subagent task starts.
    async fn publish_subagent_started(
        &self,
        _parent_session_id: &str,
        _parent_stream_id: Option<&str>,
        _job_id: &str,
        _agent_name: &str,
        _child_session_id: &str,
    ) {
    }

    /// Emit on the PARENT stream when a subagent task completes.
    async fn publish_subagent_completed(
        &self,
        _parent_session_id: &str,
        _parent_stream_id: Option<&str>,
        _job_id: &str,
        _agent_name: &str,
        _child_session_id: &str,
    ) {
    }

    /// Emit on the PARENT stream when a subagent task errors.
    async fn publish_subagent_errored(
        &self,
        _parent_session_id: &str,
        _parent_stream_id: Option<&str>,
        _job_id: &str,
        _agent_name: &str,
        _child_session_id: &str,
        _error: &str,
    ) {
    }
}

#[async_trait]
impl TurnEventSink for api::SessionStreamBroker {
    async fn activate_session_stream(&self, session_id: &SessionId, stream_id: &str) {
        self.activate_session_stream(session_id.as_str(), stream_id)
            .await;
    }

    async fn clear_active_session_stream(&self, session_id: &SessionId, stream_id: &str) {
        self.clear_active_session_stream(session_id.as_str(), stream_id)
            .await;
    }

    async fn publish_final(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        text: &str,
        metadata: Option<&StreamEventMetadata>,
    ) {
        self.publish(
            stream_id,
            "final",
            stream_payload(
                serde_json::json!({
                "session_id": session_id.as_str(),
                "text": text,
                }),
                metadata,
            ),
        )
        .await;
    }

    async fn publish_done(&self, stream_id: &str, session_id: &SessionId) {
        self.publish(
            stream_id,
            "done",
            serde_json::json!({
                "session_id": session_id.as_str(),
            }),
        )
        .await;
    }

    async fn publish_waiting(&self, stream_id: &str, session_id: &SessionId, reason: &str) {
        self.publish(
            stream_id,
            "session_waiting",
            serde_json::json!({
                "session_id": session_id.as_str(),
                "reason": reason,
            }),
        )
        .await;
    }

    async fn publish_agent_event(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        event: &AgentEvent,
        metadata: Option<&StreamEventMetadata>,
    ) {
        match event {
            AgentEvent::ThinkingChunk { text } => {
                self.publish(
                    stream_id,
                    "thinking",
                    stream_payload(
                        serde_json::json!({
                            "session_id": session_id.as_str(),
                            "text": text,
                        }),
                        metadata,
                    ),
                )
                .await;
            }
            AgentEvent::TokenChunk { text } => {
                self.publish(
                    stream_id,
                    "token",
                    stream_payload(
                        serde_json::json!({
                            "session_id": session_id.as_str(),
                            "text": text,
                        }),
                        metadata,
                    ),
                )
                .await;
            }
            AgentEvent::ToolCall { id, tool, args } => {
                self.publish(
                    stream_id,
                    "tool_call",
                    stream_payload(
                        serde_json::json!({
                            "session_id": session_id.as_str(),
                            "id": id,
                            "tool": tool,
                            "args": args,
                        }),
                        metadata,
                    ),
                )
                .await;
            }
            AgentEvent::ToolResult { id, result } => {
                self.publish(
                    stream_id,
                    "tool_result",
                    stream_payload(
                        serde_json::json!({
                            "session_id": session_id.as_str(),
                            "id": id,
                            "result": result,
                        }),
                        metadata,
                    ),
                )
                .await;
            }
            AgentEvent::RunEvent { event, payload } => {
                self.publish(stream_id, event, stream_payload(payload.clone(), metadata))
                    .await;
            }
            AgentEvent::Error { message } => {
                self.publish(
                    stream_id,
                    "error",
                    stream_payload(
                        serde_json::json!({
                            "session_id": session_id.as_str(),
                            "message": message,
                        }),
                        metadata,
                    ),
                )
                .await;
            }
            AgentEvent::FinalMessage { .. } => {}
        }
    }

    async fn publish_subagent_started(
        &self,
        parent_session_id: &str,
        parent_stream_id: Option<&str>,
        job_id: &str,
        agent_name: &str,
        child_session_id: &str,
    ) {
        let parent_stream_id = match parent_stream_id {
            Some(stream_id) => Some(stream_id.to_string()),
            None => self.stream_id_for_session(parent_session_id).await,
        };
        if let Some(parent_stream_id) = parent_stream_id {
            self.publish(
                &parent_stream_id,
                "subagent_started",
                subagent_lifecycle_payload(parent_session_id, job_id, agent_name, child_session_id),
            )
            .await;
        }
    }

    async fn publish_subagent_completed(
        &self,
        parent_session_id: &str,
        parent_stream_id: Option<&str>,
        job_id: &str,
        agent_name: &str,
        child_session_id: &str,
    ) {
        let parent_stream_id = match parent_stream_id {
            Some(stream_id) => Some(stream_id.to_string()),
            None => self.stream_id_for_session(parent_session_id).await,
        };
        if let Some(parent_stream_id) = parent_stream_id {
            self.publish(
                &parent_stream_id,
                "subagent_completed",
                subagent_lifecycle_payload(parent_session_id, job_id, agent_name, child_session_id),
            )
            .await;
        }
    }

    async fn publish_subagent_errored(
        &self,
        parent_session_id: &str,
        parent_stream_id: Option<&str>,
        job_id: &str,
        agent_name: &str,
        child_session_id: &str,
        error: &str,
    ) {
        let parent_stream_id = match parent_stream_id {
            Some(stream_id) => Some(stream_id.to_string()),
            None => self.stream_id_for_session(parent_session_id).await,
        };
        if let Some(parent_stream_id) = parent_stream_id {
            self.publish(&parent_stream_id, "subagent_errored", {
                let mut payload = subagent_lifecycle_payload(
                    parent_session_id,
                    job_id,
                    agent_name,
                    child_session_id,
                );
                if let Some(map) = payload.as_object_mut() {
                    map.insert("error".to_string(), serde_json::json!(error));
                }
                payload
            })
            .await;
        }
    }
}

fn stream_payload(mut payload: Value, metadata: Option<&StreamEventMetadata>) -> Value {
    let Some(metadata) = metadata else {
        return payload;
    };
    let Some(map) = payload.as_object_mut() else {
        return payload;
    };
    map.insert("scope".to_string(), serde_json::json!("subagent"));
    map.insert(
        "subagent".to_string(),
        serde_json::json!({
            "job_id": metadata.subagent.job_id.as_str(),
            "agent_name": metadata.subagent.agent_name.as_str(),
            "parent_session_id": metadata.subagent.parent_session_id.as_str(),
            "child_session_id": metadata.subagent.child_session_id.as_str(),
        }),
    );
    payload
}

fn subagent_lifecycle_payload(
    parent_session_id: &str,
    job_id: &str,
    agent_name: &str,
    child_session_id: &str,
) -> Value {
    stream_payload(
        serde_json::json!({
            "session_id": parent_session_id,
            "job_id": job_id,
            "agent_name": agent_name,
        }),
        Some(&StreamEventMetadata {
            subagent: SubagentStreamMetadata {
                job_id: job_id.to_string(),
                agent_name: agent_name.to_string(),
                parent_session_id: parent_session_id.to_string(),
                child_session_id: child_session_id.to_string(),
            },
        }),
    )
}

#[allow(clippy::too_many_arguments)]
pub async fn handle_inbound(
    runner: Arc<dyn AgentRunner>,
    attachment_downloader: Arc<dyn AttachmentDownloader>,
    config: ConfigStore,
    emitter: Arc<OutboundEmitter>,
    session_repo: Arc<dyn SessionRepo>,
    coordinator: Arc<SessionCoordinator>,
    turn_event_sink: Arc<dyn TurnEventSink>,
    cron_repo: Arc<dyn CronJobRepo>,
    subagent_task_repo: Arc<dyn SubagentTaskRepo>,
    inbound_sink: mpsc::Sender<InboundEvent>,
    inbound: InboundEvent,
) -> Result<()> {
    let submission = coordinator.submit_or_queue(inbound.clone());
    if matches!(submission, Submission::Queued) {
        let event_source = inbound_event_source(&inbound);
        emit_user_message_received(&emitter, &inbound, event_source, true).await;
        // This follow-up was queued behind an in-flight turn and will be merged
        // into it. The merged turn streams its tokens to the ACTIVE turn's
        // stream, not to this message's stream, so emitting a bare `done` here
        // leaves the client (which opened a fresh stream for the follow-up and
        // aborted the old one) showing nothing. Instead emit a `session_waiting`
        // event so the stream stays open, and bridge the merged turn's terminal
        // output back to it (see merge_queued_inbound + process_single_turn).
        if let Some(stream_id) = session_stream_id(&inbound) {
            turn_event_sink
                .publish_waiting(&stream_id, &inbound.session_id, "merged_into_active_turn")
                .await;
        }
        return Ok(());
    }

    let mut current_inbound = inbound;
    let session_stream_id = session_stream_id(&current_inbound);
    'turns: loop {
        process_single_turn(
            runner.clone(),
            attachment_downloader.clone(),
            config.clone(),
            emitter.clone(),
            session_repo.clone(),
            &current_inbound,
            turn_event_sink.clone(),
            cron_repo.clone(),
            subagent_task_repo.clone(),
            coordinator.clone(),
            inbound_sink.clone(),
        )
        .await?;

        let mut published_waiting = false;
        let mut subagent_task_wait_deadline: Option<std::time::Instant> = None;
        loop {
            let follow_ups = coordinator.drain_queued(&current_inbound.session_id);
            if !follow_ups.is_empty() {
                current_inbound = merge_queued_inbound(&current_inbound, follow_ups);
                continue 'turns;
            }

            if session_has_active_subagent_tasks(
                subagent_task_repo.as_ref(),
                &current_inbound.session_id,
            )
            .await
            {
                // Bound how long the parent turn waits on its subagent tasks. A
                // subagent task whose complete_subagent_task_result write never lands (or
                // a sub-agent that never reports back) would otherwise leave the
                // job Active forever and spin this loop indefinitely.
                let deadline = *subagent_task_wait_deadline
                    .get_or_insert_with(|| std::time::Instant::now() + MAX_SUBAGENT_TASK_WAIT);
                if std::time::Instant::now() >= deadline {
                    warn!(
                        session = %current_inbound.session_id,
                        "subagent task wait exceeded maximum; force-failing stuck tasks"
                    );
                    force_fail_active_subagent_tasks(
                        subagent_task_repo.as_ref(),
                        &current_inbound.session_id,
                    )
                    .await;
                    continue;
                }
                if !published_waiting {
                    if let Some(stream_id) = session_stream_id.as_deref() {
                        turn_event_sink
                            .publish_waiting(
                                stream_id,
                                &current_inbound.session_id,
                                "subagent_tasks",
                            )
                            .await;
                    }
                    published_waiting = true;
                }
                tokio::time::sleep(std::time::Duration::from_millis(500)).await;
                continue;
            }
            subagent_task_wait_deadline = None;

            if runner.active_background_processes(&current_inbound.session_id) > 0 {
                if !published_waiting {
                    if let Some(stream_id) = session_stream_id.as_deref() {
                        turn_event_sink
                            .publish_waiting(
                                stream_id,
                                &current_inbound.session_id,
                                "background_processes",
                            )
                            .await;
                    }
                    published_waiting = true;
                }
                tokio::time::sleep(std::time::Duration::from_millis(500)).await;
                continue;
            }

            if published_waiting {
                tokio::time::sleep(std::time::Duration::from_millis(100)).await;
                let follow_ups = coordinator.drain_queued(&current_inbound.session_id);
                if !follow_ups.is_empty() {
                    current_inbound = merge_queued_inbound(&current_inbound, follow_ups);
                    continue 'turns;
                }
            }

            let follow_ups = coordinator.finish_turn(&current_inbound.session_id);
            if follow_ups.is_empty() {
                if !is_subagent_task_inbound(&current_inbound) {
                    if let Some(stream_id) = session_stream_id.as_deref() {
                        turn_event_sink
                            .publish_done(stream_id, &current_inbound.session_id)
                            .await;
                    }
                }
                break 'turns;
            }

            current_inbound = merge_queued_inbound(&current_inbound, follow_ups);
            coordinator.reserve(&current_inbound.session_id);
            continue 'turns;
        }
    }

    Ok(())
}

/// Maximum time the parent turn will wait for its subagent tasks to report back
/// before force-failing them. Without this bound a task whose result write
/// never lands keeps the job Active and spins the parent loop forever.
const MAX_SUBAGENT_TASK_WAIT: std::time::Duration = std::time::Duration::from_secs(15 * 60);

async fn session_has_active_subagent_tasks(
    repo: &dyn SubagentTaskRepo,
    session_id: &SessionId,
) -> bool {
    let Ok(tasks) = repo.list_active_by_parent(session_id).await else {
        return false;
    };
    !tasks.is_empty()
}

/// Force every still-Active subagent task for this session into a terminal state
/// so the parent turn's busy-poll loop can exit. Used only after the parent has
/// waited past MAX_SUBAGENT_TASK_WAIT.
async fn force_fail_active_subagent_tasks(repo: &dyn SubagentTaskRepo, session_id: &SessionId) {
    if let Err(e) = repo
        .fail_active_for_parent(
            session_id,
            Utc::now(),
            "subagent task did not report back within the maximum wait",
        )
        .await
    {
        warn!(error = %e, session = %session_id, "failed to force-fail subagent tasks");
    }
}

fn merge_queued_inbound(current: &InboundEvent, queued: Vec<InboundEvent>) -> InboundEvent {
    let mut merged = current.clone();
    let mut text =
        String::from("[Additional request(s) received while working on the previous task]\n");
    let mut attachments = Vec::new();
    let mut raw_events = Vec::new();
    // Collect the stream ids of every queued follow-up so the merged turn's
    // terminal output can be bridged back to the streams those clients are
    // watching. Carry any already-bridged ids forward across repeated merges
    // so nothing is dropped over multiple follow-ups.
    let mut bridged_stream_ids: Vec<String> = bridged_stream_ids_from_raw(&current.raw);

    for (index, event) in queued.into_iter().enumerate() {
        if let Some(id) = session_stream_id(&event) {
            bridged_stream_ids.push(id);
        }
        let number = index + 1;
        let source = inbound_event_source(&event);
        let display_name = event
            .user_display_name
            .clone()
            .unwrap_or_else(|| event.user.clone());
        text.push_str(&format!(
            "\n{number}. Source: {source}; User: {display_name} ({})\n",
            event.user
        ));
        if !event.text.trim().is_empty() {
            text.push_str("   Text:\n");
            for line in event.text.lines() {
                text.push_str("   ");
                text.push_str(line);
                text.push('\n');
            }
        }
        if !event.attachments.is_empty() {
            text.push_str("   Attachments:\n");
            for attachment in &event.attachments {
                text.push_str(&format!(
                    "   - {} ({}, {} bytes)\n",
                    attachment.name,
                    attachment.mime_type,
                    attachment.size_bytes.unwrap_or_default()
                ));
            }
        }
        attachments.extend(event.attachments.clone());
        raw_events.push(serde_json::json!({
            "envelope_id": event.envelope_id,
            "source": source,
            "user": event.user,
            "user_display_name": event.user_display_name,
            "raw": event.raw,
        }));
    }

    merged.envelope_id = format!("{}-queued", current.envelope_id);
    merged.user = "queued-inbound".to_string();
    merged.user_display_name = Some("Queued inbound messages".to_string());
    merged.text = text;
    merged.attachments = attachments;
    let mut raw = serde_json::json!({
        "source": "queued_batch",
        "events": raw_events,
    });
    // Preserve the running turn's primary stream ids (replacing merged.raw would
    // otherwise drop them, so the merged turn would stop streaming entirely) and
    // record the follow-ups' streams so terminal output can be bridged to them.
    if let Some(map) = raw.as_object_mut() {
        if let Some(id) = session_stream_id(current) {
            map.insert("session_stream_id".to_string(), Value::String(id));
        }
        if !bridged_stream_ids.is_empty() {
            map.insert(
                "bridged_stream_ids".to_string(),
                Value::Array(bridged_stream_ids.into_iter().map(Value::String).collect()),
            );
        }
    }
    merged.raw = raw;
    merged
}

fn bridged_stream_ids_from_raw(raw: &Value) -> Vec<String> {
    raw.get("bridged_stream_ids")
        .and_then(Value::as_array)
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(ToString::to_string))
                .collect()
        })
        .unwrap_or_default()
}

fn bridged_stream_ids(inbound: &InboundEvent) -> Vec<String> {
    bridged_stream_ids_from_raw(&inbound.raw)
}

/// Publish the merged turn's final answer plus a terminal `done` to every queued
/// follow-up's stream so those clients render the response instead of hanging
/// on a stream that was only told `session_waiting`.
async fn bridge_terminal_to_streams(
    sink: &dyn TurnEventSink,
    stream_ids: &[String],
    session_id: &SessionId,
    final_text: &str,
) {
    for stream_id in stream_ids {
        sink.publish_final(stream_id, session_id, final_text, None)
            .await;
        sink.publish_done(stream_id, session_id).await;
    }
}

#[cfg(test)]
mod queue_tests {
    use super::*;
    use domain::Attachment;

    fn inbound(session: &str, envelope: &str, text: &str, raw: serde_json::Value) -> InboundEvent {
        InboundEvent {
            envelope_id: envelope.to_string(),
            session_id: SessionId::from(session.to_string()),
            user: "U123".to_string(),
            user_display_name: Some("Ada".to_string()),
            text: text.to_string(),
            attachments: vec![Attachment {
                url: "https://example.test/evidence.txt".to_string(),
                mime_type: "text/plain".to_string(),
                name: "evidence.txt".to_string(),
                size_bytes: Some(128),
            }],
            dynamic_context: Vec::new(),
            raw,
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        }
    }

    #[test]
    fn merged_queued_turn_preserves_event_metadata_and_attachments() {
        let current = inbound(
            "C123-T1",
            "E1",
            "working",
            serde_json::json!({"source": "http"}),
        );
        let queued = vec![
            inbound(
                "C123-T1",
                "E2",
                "first follow-up",
                serde_json::json!({
                    "source": "trigger",
                    "refs": {"issue": "123"},
                    "summary_refs": {"title": "Bug"}
                }),
            ),
            inbound(
                "C123-T1",
                "E3",
                "second follow-up",
                serde_json::json!({"source": "http"}),
            ),
        ];

        let merged = merge_queued_inbound(&current, queued);

        assert_eq!(merged.session_id.as_str(), "C123-T1");
        assert_eq!(merged.user, "queued-inbound");
        assert_eq!(merged.attachments.len(), 2);
        assert!(merged.text.contains("1. Source: trigger; User: Ada (U123)"));
        assert!(merged.text.contains("first follow-up"));
        assert!(merged.text.contains("evidence.txt (text/plain, 128 bytes)"));
        assert_eq!(merged.raw["source"], "queued_batch");
        assert_eq!(merged.raw["events"][0]["raw"]["refs"]["issue"], "123");
        assert_eq!(
            merged.raw["events"][0]["raw"]["summary_refs"]["title"],
            "Bug"
        );
    }

    #[test]
    fn merged_turn_preserves_primary_stream_and_bridges_followup_streams() {
        // Regression: a queued follow-up opens its own SSE stream and the client
        // aborts the previous one. The merged turn must keep streaming to the
        // running turn's primary stream AND record the follow-ups' stream ids so
        // their terminal output can be bridged, instead of those clients hanging
        // on a stream that only ever received a bare `done`.
        let current = inbound(
            "C123-T1",
            "E1",
            "working",
            serde_json::json!({
                "source": "session",
                "session_stream_id": "stream-primary"
            }),
        );
        let queued = vec![
            inbound(
                "C123-T1",
                "E2",
                "follow-up",
                serde_json::json!({
                    "source": "session",
                    "session_stream_id": "stream-followup-1"
                }),
            ),
            inbound(
                "C123-T1",
                "E3",
                "another follow-up",
                serde_json::json!({
                    "source": "session",
                    "session_stream_id": "stream-followup-2"
                }),
            ),
        ];

        let merged = merge_queued_inbound(&current, queued);

        // Primary stream ids survive the merge (without this the merged turn
        // would stop streaming entirely).
        assert_eq!(
            session_stream_id(&merged).as_deref(),
            Some("stream-primary")
        );

        // Every follow-up's stream id is captured for bridging.
        let bridged = super::bridged_stream_ids(&merged);
        assert!(bridged.contains(&"stream-followup-1".to_string()));
        assert!(bridged.contains(&"stream-followup-2".to_string()));
    }

    #[test]
    fn repeated_merges_accumulate_bridged_streams() {
        // Multiple rounds of follow-ups must not drop earlier bridged stream ids.
        let current = inbound(
            "C123-T1",
            "E1",
            "working",
            serde_json::json!({
                "source": "session",
                "session_stream_id": "stream-primary"
            }),
        );
        let first = merge_queued_inbound(
            &current,
            vec![inbound(
                "C123-T1",
                "E2",
                "follow-up 1",
                serde_json::json!({"source": "session", "session_stream_id": "stream-a"}),
            )],
        );
        let second = merge_queued_inbound(
            &first,
            vec![inbound(
                "C123-T1",
                "E3",
                "follow-up 2",
                serde_json::json!({"source": "session", "session_stream_id": "stream-b"}),
            )],
        );

        let bridged = super::bridged_stream_ids(&second);
        assert!(bridged.contains(&"stream-a".to_string()));
        assert!(bridged.contains(&"stream-b".to_string()));
        // The primary stream is still preserved after repeated merges.
        assert_eq!(
            session_stream_id(&second).as_deref(),
            Some("stream-primary")
        );
    }

    #[test]
    fn session_inbound_source_and_final_metadata_are_preserved() {
        let inbound = inbound(
            "session-1",
            "E1",
            "hello",
            serde_json::json!({
                "source": "session",
                "session_stream_id": "stream-1",
                "trace_id": "trace-1",
                "turn_id": "turn-1",
                "raw": {
                    "provider": "fake-provider",
                    "route_id": "route-1"
                }
            }),
        );
        let mut payload = serde_json::json!({
            "session_id": inbound.session_id.as_str(),
            "source": inbound_event_source(&inbound),
            "text": "done"
        });

        copy_inbound_metadata(&mut payload, &inbound);

        assert_eq!(payload["source"], "session");
        assert_eq!(payload["trace_id"], "trace-1");
        assert_eq!(payload["turn_id"], "turn-1");
        assert_eq!(payload["provider"], "fake-provider");
        assert_eq!(payload["route_id"], "route-1");
    }

    #[test]
    fn wake_inbound_is_not_treated_as_subagent_task_completion() {
        let inbound = inbound(
            "C123-T1",
            "wake-1",
            "Check subagent task status",
            serde_json::json!({
                "source": "wake",
                "job_kind": "wake",
                "job_id": "wake-1",
                "parent_session_id": "C123-T1"
            }),
        );

        assert_eq!(inbound_event_source(&inbound), "wake");
        assert!(!is_subagent_task_inbound(&inbound));
    }

    #[test]
    fn subagent_task_inbound_requires_explicit_subagent_task_kind() {
        let legacy_wake = inbound(
            "C123-T1",
            "wake-1",
            "Check subagent task status",
            serde_json::json!({
                "source": "cron",
                "job_id": "wake-1",
                "parent_session_id": "C123-T1"
            }),
        );
        let subagent_task = inbound(
            "subagent-task-session",
            "subagent-task-1",
            "do work",
            serde_json::json!({
                "source": "cron",
                "job_kind": "subagent_task",
                "job_id": "subagent-task-1",
                "parent_session_id": "C123-T1"
            }),
        );

        assert!(!is_subagent_task_inbound(&legacy_wake));
        assert!(is_subagent_task_inbound(&subagent_task));
    }
}

#[allow(clippy::too_many_arguments)]
async fn process_single_turn(
    runner: Arc<dyn AgentRunner>,
    attachment_downloader: Arc<dyn AttachmentDownloader>,
    config: ConfigStore,
    emitter: Arc<OutboundEmitter>,
    session_repo: Arc<dyn SessionRepo>,
    inbound: &InboundEvent,
    turn_event_sink: Arc<dyn TurnEventSink>,
    cron_repo: Arc<dyn CronJobRepo>,
    subagent_task_repo: Arc<dyn SubagentTaskRepo>,
    coordinator: Arc<SessionCoordinator>,
    inbound_sink: mpsc::Sender<InboundEvent>,
) -> Result<()> {
    if inbound.text.trim().is_empty() && inbound.attachments.is_empty() {
        return Ok(());
    }
    info!(
        session = %inbound.session_id,
        user = %inbound.user,
        text_len = inbound.text.len(),
        attachments = inbound.attachments.len(),
        "inbound message"
    );

    let snapshot = config.snapshot();
    let session_id = inbound.session_id.clone();

    let was_new_session = ensure_session_persisted(session_repo.as_ref(), inbound, &emitter).await;
    if !was_new_session {
        let _ = session_repo.touch(&session_id, Utc::now()).await;
    }
    let event_source = inbound_event_source(inbound);
    emit_user_message_received(&emitter, inbound, event_source, false).await;

    let multimodal_available = snapshot.multimodal_model.is_some();
    let media = collect_media_for_turn(
        attachment_downloader.as_ref(),
        &inbound.attachments,
        multimodal_available,
    )
    .await;
    let annotated_text = compose_annotated_text(inbound, &media);

    let DownloadResults { images, .. } = media;
    let mut turn_input = TurnInput::text(annotated_text);
    if let Some(stream_id) = session_stream_id(inbound) {
        turn_input = turn_input.with_session_stream_id(stream_id);
    }
    for context in &inbound.dynamic_context {
        turn_input = turn_input.with_dynamic_context(context.clone());
    }
    for (mime, bytes) in images {
        turn_input = turn_input.with_image(mime, bytes);
    }

    let session_stream_id = session_stream_id(inbound);
    let stream_metadata = stream_event_metadata(inbound);
    if let Some(stream_id) = session_stream_id.as_deref() {
        turn_event_sink
            .activate_session_stream(&session_id, stream_id)
            .await;
    }

    let stream_result = runner
        .run_turn(&session_id, turn_input, inbound.agent_definition.clone())
        .await;
    let outcome = match stream_result {
        Ok(stream) => {
            consume_agent_stream(
                stream,
                session_stream_id.clone(),
                &session_id,
                &emitter,
                event_source,
                turn_event_sink.as_ref(),
                stream_metadata.as_ref(),
            )
            .await
        }
        Err(e) => StreamOutcome {
            text: String::new(),
            final_message: None,
            error: Some(e.to_string()),
        },
    };

    let final_text = format_final_message(&outcome);
    let reply_text_for_event = final_text.clone();
    if let Some(stream_id) = session_stream_id.as_deref() {
        turn_event_sink
            .publish_final(
                stream_id,
                &session_id,
                &final_text,
                stream_metadata.as_ref(),
            )
            .await;
    }
    // Bridge the merged turn's answer to every queued follow-up's stream.
    // Those clients opened fresh streams that were told `session_waiting`; emit
    // the real answer plus a terminal `done` so they render the response instead
    // of hanging.
    bridge_terminal_to_streams(
        turn_event_sink.as_ref(),
        &bridged_stream_ids(inbound),
        &session_id,
        &final_text,
    )
    .await;

    if let Some(error_message) = outcome.error.as_ref() {
        emitter
            .emit(OutboundEvent::new(
                event_types::ERROR_MODEL,
                serde_json::json!({
                    "session_id": session_id.as_str(),
                    "source": event_source,
                    "error": error_message,
                }),
            ))
            .await;
        let _ = session_repo
            .set_status(&session_id, SessionStatus::Errored)
            .await;
    } else {
        let mut payload = serde_json::json!({
            "session_id": session_id.as_str(),
            "source": event_source,
            "text": reply_text_for_event,
        });
        copy_inbound_metadata(&mut payload, inbound);
        emitter
            .emit(OutboundEvent::new(event_types::AGENT_MESSAGE_SENT, payload))
            .await;
    }
    emitter.flush_database().await;

    if let Some(stream_id) = session_stream_id.as_deref() {
        turn_event_sink
            .clear_active_session_stream(&session_id, stream_id)
            .await;
    }

    info!(session = %session_id, len = outcome.text.len(), "turn complete");

    // If this is a subagent task session, notify the parent and record the result.
    if is_subagent_task_inbound(inbound) {
        if let (Some(parent_session_id), Some(job_id)) = (
            inbound.raw.get("parent_session_id").and_then(Value::as_str),
            inbound.raw.get("job_id").and_then(Value::as_str),
        ) {
            let subagent_error = outcome.error.as_deref();
            let result_text = if let Some(err) = subagent_error {
                format!("[Sub-agent error: {}]", err)
            } else {
                outcome.text.clone()
            };
            let agent_name = inbound
                .raw
                .get("agent_name")
                .and_then(Value::as_str)
                .unwrap_or("sub-agent");
            if subagent_error.is_some() {
                turn_event_sink
                    .publish_subagent_errored(
                        parent_session_id,
                        session_stream_id.as_deref(),
                        job_id,
                        agent_name,
                        session_id.as_str(),
                        subagent_error.unwrap_or("unknown"),
                    )
                    .await;
            } else {
                turn_event_sink
                    .publish_subagent_completed(
                        parent_session_id,
                        session_stream_id.as_deref(),
                        job_id,
                        agent_name,
                        session_id.as_str(),
                    )
                    .await;
            }

            // Send notification to parent BEFORE marking job as completed.
            // This prevents a race where the parent's polling loop sees the
            // subagent task as no longer active before the notification arrives.
            let notification = InboundEvent {
                envelope_id: format!("subagent-task-result-{}", Utc::now().timestamp_millis()),
                session_id: SessionId::from(parent_session_id),
                user: "system".to_string(),
                user_display_name: Some("Sub-agent".to_string()),
                text: format!(
                    "Sub-agent '{}' completed task (job: {}):\n\n{}",
                    agent_name, job_id, result_text
                ),
                attachments: Vec::new(),
                dynamic_context: Vec::new(),
                raw: serde_json::json!({
                    "source": "subagent_task_result",
                    "job_id": job_id,
                    "agent_name": agent_name,
                }),
                is_direct_message: false,
                is_directly_addressed: true,
                link_previews: Vec::new(),
                agent_definition: None,
            };

            // Queue via coordinator so the parent's polling loop picks it up
            // in drain_queued() before it exits. If the parent's turn has
            // already finished (no coordinator entry), submit_or_queue inserts
            // a *fresh* reservation that no running task owns — leaving the
            // parent session permanently reserved and the subagent task result
            // undelivered. In that case, dispatch the notification through the
            // real inbound path so a handler runs it and calls finish_turn.
            match coordinator.submit_or_queue(notification.clone()) {
                Submission::Queued => {}
                Submission::RunNow => {
                    // Release the reservation we just took; handle_inbound's own
                    // submit_or_queue will re-reserve and own the lifecycle.
                    let _ = coordinator.finish_turn(&notification.session_id);
                    if let Err(e) = inbound_sink.send(notification).await {
                        warn!(
                            error = %e,
                            job_id = %job_id,
                            "failed to re-dispatch subagent task result to parent session"
                        );
                    }
                }
            }

            // NOW mark the job as completed (after notification is queued).
            // This MUST eventually move the job out of the `Active` state: the
            // parent turn busy-polls session_has_active_subagent_tasks(), so a job
            // stuck Active leaves the parent spinning forever. Retry the result
            // write, and if it keeps failing force the job to a terminal state
            // so the parent's poll loop can exit.
            complete_subagent_task_result_or_force_fail(
                subagent_task_repo.as_ref(),
                job_id,
                subagent_error,
                &result_text,
            )
            .await;
        }
    } else if let Some(run) = ScheduledRunContext::from_inbound(inbound) {
        // A scheduled (cron/wake) run only reaches a terminal status now that
        // the turn has actually executed — not when the scheduler enqueued it.
        // Emit SCHEDULE_RUN_COMPLETED/FAILED, record the run, delete
        // one-shot/wake jobs, and advance repeat counts here.
        complete_scheduled_run(
            cron_repo.as_ref(),
            emitter.clone(),
            &run,
            &session_id,
            outcome.error.as_deref(),
        )
        .await;
    }

    Ok(())
}

/// Run-lifecycle context the scheduler embeds in a scheduled job's inbound `raw`
/// so the turn handler can complete the run after the turn executes.
struct ScheduledRunContext {
    job_id: String,
    run_key: String,
    scheduled_at: chrono::DateTime<Utc>,
    started_at: chrono::DateTime<Utc>,
    is_one_shot: bool,
    is_wake: bool,
}

impl ScheduledRunContext {
    fn from_inbound(inbound: &InboundEvent) -> Option<Self> {
        let raw = inbound.raw.as_object()?;
        // Only scheduler-dispatched runs carry these keys; ordinary inbound
        // messages (and subagent task results, handled above) do not.
        let run_key = raw.get("schedule_run_key")?.as_str()?.to_string();
        let job_id = raw.get("job_id")?.as_str()?.to_string();
        let scheduled_at = raw
            .get("schedule_scheduled_at")
            .and_then(Value::as_str)
            .and_then(|s| s.parse().ok())?;
        let started_at = raw
            .get("schedule_started_at")
            .and_then(Value::as_str)
            .and_then(|s| s.parse().ok())?;
        Some(Self {
            job_id,
            run_key,
            scheduled_at,
            started_at,
            is_one_shot: raw
                .get("schedule_is_one_shot")
                .and_then(Value::as_bool)
                .unwrap_or(false),
            is_wake: raw
                .get("schedule_is_wake")
                .and_then(Value::as_bool)
                .unwrap_or(false),
        })
    }
}

/// Complete a scheduled run after its turn executed: record the run status, emit
/// the terminal schedule event, and apply lifecycle changes (delete one-shot/wake
/// jobs, advance repeat counts). Keyed to the real turn outcome rather than the
/// scheduler's enqueue time.
async fn complete_scheduled_run(
    cron_repo: &dyn CronJobRepo,
    emitter: Arc<OutboundEmitter>,
    run: &ScheduledRunContext,
    session_id: &SessionId,
    error: Option<&str>,
) {
    use agent::rig_tool_registry::{emit_schedule_event, ScheduleRunPayload};

    let completed_at = Utc::now();
    let status = if error.is_some() {
        "error"
    } else {
        "completed"
    };
    if let Err(e) = cron_repo
        .record_run(&run.job_id, completed_at, status, error)
        .await
    {
        warn!(job_id = %run.job_id, error = %e, "cron: failed to record run completion");
    }

    // Re-fetch the latest job row for the event payload; fall back to skipping the
    // event if the job is already gone (e.g. cancelled mid-turn).
    let job = cron_repo.get(&run.job_id).await.ok().flatten();
    if let Some(job) = job.as_ref() {
        let event_type = if error.is_some() {
            event_types::SCHEDULE_RUN_FAILED
        } else {
            event_types::SCHEDULE_RUN_COMPLETED
        };
        emit_schedule_event(
            Some(emitter),
            event_type,
            job,
            session_id,
            "scheduler",
            Some(ScheduleRunPayload {
                run_key: run.run_key.clone(),
                scheduled_at: run.scheduled_at,
                started_at: Some(run.started_at),
                completed_at: Some(completed_at),
                duration_ms: Some((completed_at - run.started_at).num_milliseconds()),
                error: error.map(ToString::to_string),
            }),
        )
        .await;
    }

    // One-shot and wake jobs are single-use: remove them now that the turn ran.
    if run.is_one_shot || run.is_wake {
        let _ = cron_repo
            .set_state(&run.job_id, domain::cron::CronJobState::Completed)
            .await;
        let _ = cron_repo.delete(&run.job_id).await;
        info!(job_id = %run.job_id, "cron: completed, removed");
        return;
    }

    // Recurring jobs with a bounded repeat count advance toward completion.
    if let Some(repeat_count) = job.as_ref().and_then(|j| j.repeat_count) {
        let repeat_completed = job.as_ref().map(|j| j.repeat_completed).unwrap_or(0);
        if let Err(e) = cron_repo.increment_repeat(&run.job_id).await {
            warn!(job_id = %run.job_id, error = %e, "cron: failed to increment repeat");
        }
        if repeat_completed + 1 >= repeat_count {
            let _ = cron_repo
                .set_state(&run.job_id, domain::cron::CronJobState::Completed)
                .await;
            let _ = cron_repo.delete(&run.job_id).await;
            info!(job_id = %run.job_id, "cron: repeat count reached, completed and removed");
        }
    }
}

/// Number of attempts to persist a subagent task's result before logging a hard
/// failure. The parent wait loop reads the task state directly from subagent
/// storage.
const COMPLETE_SUBAGENT_TASK_RESULT_ATTEMPTS: usize = 3;

async fn complete_subagent_task_result_or_force_fail(
    subagent_task_repo: &dyn SubagentTaskRepo,
    job_id: &str,
    error: Option<&str>,
    result_text: &str,
) {
    let state = if error.is_some() {
        SubagentTaskState::Failed
    } else {
        SubagentTaskState::Completed
    };
    for attempt in 1..=COMPLETE_SUBAGENT_TASK_RESULT_ATTEMPTS {
        match subagent_task_repo
            .complete(job_id, state, Utc::now(), result_text, error)
            .await
        {
            Ok(()) => return,
            Err(e) => {
                warn!(
                    error = %e,
                    job_id = %job_id,
                    attempt,
                    "failed to record subagent task result"
                );
                if attempt < COMPLETE_SUBAGENT_TASK_RESULT_ATTEMPTS {
                    tokio::time::sleep(std::time::Duration::from_millis(200 * attempt as u64))
                        .await;
                }
            }
        }
    }
}

async fn emit_user_message_received(
    emitter: &OutboundEmitter,
    inbound: &InboundEvent,
    source: &'static str,
    queued: bool,
) {
    emitter
        .emit(OutboundEvent::new(
            event_types::USER_MESSAGE_RECEIVED,
            serde_json::json!({
                "envelope_id": inbound.envelope_id,
                "session_id": inbound.session_id.as_str(),
                "source": source,
                "user": inbound.user,
                "user_display_name": inbound.user_display_name,
                "text": inbound.text,
                "attachments": inbound.attachments.iter().map(|attachment| {
                    serde_json::json!({
                        "name": attachment.name,
                        "mime_type": attachment.mime_type,
                        "size_bytes": attachment.size_bytes,
                    })
                }).collect::<Vec<_>>(),
                "link_previews": inbound.link_previews,
                "is_direct_message": inbound.is_direct_message,
                "is_directly_addressed": inbound.is_directly_addressed,
                "queued": queued,
            }),
        ))
        .await;
}

fn inbound_event_source(inbound: &InboundEvent) -> &'static str {
    match inbound
        .raw
        .get("source")
        .and_then(|value| value.as_str())
        .unwrap_or_default()
    {
        "session" => "session",
        "trigger" => "trigger",
        "cron" => "cron",
        "wake" => "wake",
        "web" => "web",
        "subagent_task" => "subagent_task",
        "subagent_task_result" => "subagent_task_result",
        _ => "session",
    }
}

fn is_subagent_task_inbound(inbound: &InboundEvent) -> bool {
    inbound
        .raw
        .get("job_kind")
        .and_then(Value::as_str)
        .is_some_and(|kind| kind == "subagent_task")
        && inbound
            .raw
            .get("parent_session_id")
            .and_then(Value::as_str)
            .is_some()
        && inbound.raw.get("job_id").and_then(Value::as_str).is_some()
}

fn copy_inbound_metadata(payload: &mut Value, inbound: &InboundEvent) {
    let Some(map) = payload.as_object_mut() else {
        return;
    };
    for key in [
        "session_stream_id",
        "trace_id",
        "turn_id",
        "provider",
        "route_id",
        "dedupe_key",
        "external_message_id",
        // Subagent task metadata:
        "job_kind",
        "job_id",
        "agent_name",
        "parent_session_id",
        "child_session_id",
        "stream_scope",
        "subagent_task_goal",
    ] {
        if let Some(value) = inbound.raw.get(key) {
            map.insert(key.to_string(), value.clone());
        }
        if let Some(value) = inbound
            .raw
            .get("raw")
            .and_then(Value::as_object)
            .and_then(|raw| raw.get(key))
        {
            map.insert(key.to_string(), value.clone());
        }
    }
}

pub(crate) struct StreamOutcome {
    pub text: String,
    pub final_message: Option<String>,
    pub error: Option<String>,
}

async fn consume_agent_stream(
    mut stream: futures::stream::BoxStream<'static, AgentEvent>,
    stream_id: Option<String>,
    session_id: &SessionId,
    emitter: &OutboundEmitter,
    source: &'static str,
    event_sink: &dyn TurnEventSink,
    metadata: Option<&StreamEventMetadata>,
) -> StreamOutcome {
    let mut accumulated = String::new();
    let mut final_message: Option<String> = None;
    let mut error_message: Option<String> = None;
    let mut sequence: u64 = 0;
    while let Some(event) = stream.next().await {
        sequence += 1;
        let client_event = sanitize_agent_event_for_clients(&event, session_id, source, sequence);
        if let Some(stream_id) = stream_id.as_deref() {
            event_sink
                .publish_agent_event(stream_id, session_id, &client_event, metadata)
                .await;
        }
        emit_agent_stream_event(emitter, session_id, source, sequence, &client_event).await;
        match event {
            AgentEvent::ThinkingChunk { .. } => {}
            AgentEvent::TokenChunk { text } => push_visible_delta(&mut accumulated, &text),
            AgentEvent::FinalMessage { text } => {
                let normalized = normalize_visible_text(&text);
                if !normalized.trim().is_empty() {
                    final_message = Some(normalized);
                }
                merge_final_text(&mut accumulated, &text);
            }
            AgentEvent::Error { message } => error_message = Some(message),
            AgentEvent::RunEvent { .. } => {}
            _ => {}
        }
    }
    emitter
        .flush_streams_for_session_background(session_id.as_str())
        .await;
    StreamOutcome {
        text: accumulated,
        final_message,
        error: error_message,
    }
}

fn push_visible_delta(accumulated: &mut String, delta: &str) {
    if delta.is_empty() {
        return;
    }
    accumulated.push_str(delta);
}

fn normalize_visible_text(text: &str) -> String {
    text.to_string()
}

fn merge_final_text(accumulated: &mut String, final_text: &str) {
    let normalized = normalize_visible_text(final_text);
    if accumulated.trim().is_empty() {
        *accumulated = normalized;
        return;
    }
    if normalized.starts_with(accumulated.as_str()) && normalized.len() > accumulated.len() {
        accumulated.push_str(&normalized[accumulated.len()..]);
    }
}

fn sanitize_agent_event_for_clients(
    event: &AgentEvent,
    session_id: &SessionId,
    source: &'static str,
    sequence: u64,
) -> AgentEvent {
    match event {
        AgentEvent::Error { message } => {
            capture_agent_internal_error(session_id, source, sequence, "agent.error", message);
            AgentEvent::Error {
                message: USER_RETRY_MESSAGE.to_string(),
            }
        }
        AgentEvent::RunEvent { event, payload } => {
            let mut next = payload.clone();
            if let Some(error) = payload.get("error").and_then(Value::as_str) {
                capture_agent_internal_error(session_id, source, sequence, event, error);
                if let Some(map) = next.as_object_mut() {
                    map.insert(
                        "error".to_string(),
                        Value::String("internal error captured".to_string()),
                    );
                    map.insert("team_notified".to_string(), Value::Bool(true));
                }
            }
            AgentEvent::RunEvent {
                event: event.clone(),
                payload: next,
            }
        }
        AgentEvent::ToolResult { id, result } => {
            let mut next = result.clone();
            if let Some(error) = result.get("error").and_then(Value::as_str) {
                capture_agent_internal_error(
                    session_id,
                    source,
                    sequence,
                    "agent.tool.result",
                    error,
                );
                if let Some(map) = next.as_object_mut() {
                    map.insert(
                        "error".to_string(),
                        Value::String(USER_RETRY_MESSAGE.to_string()),
                    );
                    map.insert("team_notified".to_string(), Value::Bool(true));
                }
            }
            AgentEvent::ToolResult {
                id: id.clone(),
                result: next,
            }
        }
        _ => event.clone(),
    }
}

fn capture_agent_internal_error(
    session_id: &SessionId,
    source: &'static str,
    sequence: u64,
    event: &str,
    error: &str,
) {
    warn!(session_id = %session_id.as_str(), source, sequence, event, error = %error, "agent internal error captured");
    sentry::with_scope(
        |scope| {
            scope.set_tag("runtime.error_source", source);
            scope.set_tag("runtime.agent_event", event);
            scope.set_tag("runtime.session_id", session_id.as_str());
            scope.set_extra("sequence", sequence.into());
            scope.set_extra("internal_error", error.into());
        },
        || {
            sentry::capture_message("agent turn internal error", sentry::Level::Error);
        },
    );
}

async fn emit_agent_stream_event(
    emitter: &OutboundEmitter,
    session_id: &SessionId,
    source: &'static str,
    sequence: u64,
    event: &AgentEvent,
) {
    let agent_event = serde_json::to_value(event).unwrap_or_else(|_| {
        serde_json::json!({
            "kind": "serialization_error",
        })
    });
    let event_type = match event {
        AgentEvent::ThinkingChunk { .. } => "agent.stream.thinking",
        AgentEvent::TokenChunk { .. } => "agent.stream.token",
        AgentEvent::ToolCall { .. } => "agent.tool.call",
        AgentEvent::ToolResult { .. } => "agent.tool.result",
        AgentEvent::RunEvent { event: name, .. } => {
            let sanitized = name
                .chars()
                .map(|ch| match ch {
                    'a'..='z' | '0'..='9' | '_' | '-' => ch,
                    'A'..='Z' => ch.to_ascii_lowercase(),
                    _ => '.',
                })
                .collect::<String>()
                .replace('_', ".");
            let event_type = format!("agent.run.{sanitized}");
            let payload = serde_json::json!({
                "session_id": session_id.as_str(),
                "source": source,
                "sequence": sequence,
                "agent_event": agent_event,
            });
            emitter.emit(OutboundEvent::new(event_type, payload)).await;
            return;
        }
        AgentEvent::FinalMessage { .. } => "agent.final_message",
        AgentEvent::Error { .. } => "agent.error",
    };
    emitter
        .emit(OutboundEvent::new(
            event_type,
            serde_json::json!({
                "session_id": session_id.as_str(),
                "source": source,
                "sequence": sequence,
                "agent_event": agent_event,
            }),
        ))
        .await;
}

fn session_stream_id(inbound: &InboundEvent) -> Option<String> {
    inbound
        .raw
        .get("session_stream_id")
        .and_then(serde_json::Value::as_str)
        .map(ToString::to_string)
}

fn stream_event_metadata(inbound: &InboundEvent) -> Option<StreamEventMetadata> {
    if !is_subagent_task_inbound(inbound) {
        return None;
    }
    let parent_session_id = inbound.raw.get("parent_session_id")?.as_str()?.to_string();
    let job_id = inbound.raw.get("job_id")?.as_str()?.to_string();
    let agent_name = inbound
        .raw
        .get("agent_name")
        .and_then(Value::as_str)
        .unwrap_or("sub-agent")
        .to_string();
    Some(StreamEventMetadata {
        subagent: SubagentStreamMetadata {
            job_id,
            agent_name,
            parent_session_id,
            child_session_id: inbound.session_id.as_str().to_string(),
        },
    })
}

fn format_final_message(outcome: &StreamOutcome) -> String {
    if let Some(internal_error) = &outcome.error {
        warn!(error = %internal_error, "agent turn errored; replying with generic error");
        sentry::with_scope(
            |scope| scope.set_extra("internal_error", internal_error.clone().into()),
            || sentry::capture_message("agent turn final response error", sentry::Level::Error),
        );
        return USER_RETRY_MESSAGE.to_string();
    }
    let text = outcome
        .final_message
        .as_deref()
        .filter(|text| !text.trim().is_empty())
        .unwrap_or(&outcome.text);
    if text.trim().is_empty() {
        warn!("agent produced no response after recovery attempts");
        sentry::capture_message("agent produced empty final response", sentry::Level::Error);
        return USER_RETRY_MESSAGE.to_string();
    }
    text.to_string()
}

#[cfg(test)]
mod tests {
    use super::{
        format_final_message, merge_final_text, normalize_visible_text, push_visible_delta,
        sanitize_agent_event_for_clients, StreamOutcome, USER_RETRY_MESSAGE,
    };
    use agent::AgentEvent;
    use domain::SessionId;
    use serde_json::json;

    #[test]
    fn empty_outcome_does_not_use_generic_apology() {
        let text = format_final_message(&StreamOutcome {
            text: String::new(),
            final_message: None,
            error: None,
        });

        assert!(!text.contains("Sorry, something went wrong"));
        assert_eq!(text, USER_RETRY_MESSAGE);
    }

    #[test]
    fn visible_delta_preserves_provider_token_boundaries() {
        let mut text = String::new();
        for delta in ["not", "ion", " rail", "way", " De", "ploy", " Hiv", "y's"] {
            push_visible_delta(&mut text, delta);
        }
        assert_eq!(text, "notion railway Deploy Hivy's");
    }

    #[test]
    fn visible_delta_spacing_preserves_existing_spaces() {
        let mut text = String::new();
        for delta in ["Hello ", "there", "."] {
            push_visible_delta(&mut text, delta);
        }
        assert_eq!(text, "Hello there.");
    }

    #[test]
    fn visible_final_text_is_not_rewritten() {
        assert_eq!(normalize_visible_text("Hey!What'sup?"), "Hey!What'sup?");
    }

    #[test]
    fn final_event_keeps_stream_tokens_when_final_repeats_text() {
        let mut text = String::new();
        for delta in ["Hey! ", "What's ", "up?"] {
            push_visible_delta(&mut text, delta);
        }
        merge_final_text(&mut text, "Hey! What's up?");
        assert_eq!(text, "Hey! What's up?");
    }

    #[test]
    fn final_message_replaces_pre_tool_stream_text() {
        let text = format_final_message(&StreamOutcome {
            text: "Let me check the database.Going good!".to_string(),
            final_message: Some("Going good!".to_string()),
            error: None,
        });

        assert_eq!(text, "Going good!");
    }

    #[test]
    fn final_message_falls_back_to_tokens_when_absent() {
        let text = format_final_message(&StreamOutcome {
            text: "Streaming answer".to_string(),
            final_message: None,
            error: None,
        });

        assert_eq!(text, "Streaming answer");
    }

    #[test]
    fn error_outcome_does_not_expose_internal_details() {
        let text = format_final_message(&StreamOutcome {
            text: String::new(),
            final_message: Some("Do not show this".to_string()),
            error: Some(
                "error returned from database: attempt to write a readonly database".into(),
            ),
        });

        assert!(!text.contains("readonly database"));
        assert!(!text.contains("error returned from database"));
        assert_eq!(text, USER_RETRY_MESSAGE);
    }

    #[test]
    fn client_agent_error_event_does_not_expose_internal_details() {
        let event = AgentEvent::Error {
            message: "model error: env var HIVY_PROXY_API_KEY not set".to_string(),
        };
        let sanitized =
            sanitize_agent_event_for_clients(&event, &SessionId::from("session-1"), "http", 1);

        match sanitized {
            AgentEvent::Error { message } => {
                assert_eq!(message, USER_RETRY_MESSAGE);
                assert!(!message.contains("HIVY_PROXY_API_KEY"));
            }
            other => panic!("unexpected event: {other:?}"),
        }
    }

    #[test]
    fn client_run_event_redacts_error_payload() {
        let event = AgentEvent::RunEvent {
            event: "model_request_failed".to_string(),
            payload: json!({"error": "database is readonly", "model": "test"}),
        };
        let sanitized =
            sanitize_agent_event_for_clients(&event, &SessionId::from("session-1"), "http", 1);

        match sanitized {
            AgentEvent::RunEvent { payload, .. } => {
                assert_eq!(payload["error"], "internal error captured");
                assert_eq!(payload["team_notified"], true);
                assert!(!payload.to_string().contains("readonly"));
            }
            other => panic!("unexpected event: {other:?}"),
        }
    }

    #[test]
    fn client_tool_result_redacts_error_payload() {
        let event = AgentEvent::ToolResult {
            id: "tool-1".to_string(),
            result: json!({"error": "secret storage failure"}),
        };
        let sanitized =
            sanitize_agent_event_for_clients(&event, &SessionId::from("session-1"), "http", 1);

        match sanitized {
            AgentEvent::ToolResult { result, .. } => {
                assert_eq!(result["error"], USER_RETRY_MESSAGE);
                assert_eq!(result["team_notified"], true);
                assert!(!result.to_string().contains("secret storage"));
            }
            other => panic!("unexpected event: {other:?}"),
        }
    }
}

#[cfg(test)]
mod stream_tests {
    use super::{consume_agent_stream, TurnEventSink};
    use agent::AgentEvent;
    use async_trait::async_trait;
    use chrono::{DateTime, Utc};
    use domain::SessionId;
    use futures::StreamExt;
    use outbound::{OutboundEmitter, OutboundRegistry};
    use serde_json::json;
    use std::sync::{Arc, Mutex};
    use storage::{OutboxRepo, OutboxRow};
    use tokio::sync::RwLock;

    #[derive(Default)]
    struct RecordingSink {
        events: Mutex<Vec<(String, String)>>,
    }

    #[async_trait]
    impl TurnEventSink for RecordingSink {
        async fn publish_final(
            &self,
            stream_id: &str,
            _session_id: &SessionId,
            _text: &str,
            _metadata: Option<&super::StreamEventMetadata>,
        ) {
            self.events
                .lock()
                .expect("events lock")
                .push((stream_id.to_string(), "final".to_string()));
        }

        async fn publish_done(&self, stream_id: &str, _session_id: &SessionId) {
            self.events
                .lock()
                .expect("events lock")
                .push((stream_id.to_string(), "done".to_string()));
        }

        async fn publish_agent_event(
            &self,
            stream_id: &str,
            _session_id: &SessionId,
            event: &AgentEvent,
            _metadata: Option<&super::StreamEventMetadata>,
        ) {
            let name = match event {
                AgentEvent::ThinkingChunk { .. } => "thinking",
                AgentEvent::TokenChunk { .. } => "token",
                AgentEvent::ToolCall { .. } => "tool_call",
                AgentEvent::ToolResult { .. } => "tool_result",
                AgentEvent::RunEvent { event, .. } => event.as_str(),
                AgentEvent::Error { .. } => "error",
                AgentEvent::FinalMessage { .. } => "final_message",
            };
            self.events
                .lock()
                .expect("events lock")
                .push((stream_id.to_string(), name.to_string()));
        }
    }

    #[derive(Default)]
    struct NoopOutbox;

    #[async_trait]
    impl OutboxRepo for NoopOutbox {
        async fn enqueue(
            &self,
            _channel_name: &str,
            _event_type: &str,
            _payload: serde_json::Value,
        ) -> storage::Result<i64> {
            Ok(1)
        }

        async fn claim_due(&self, _limit: u32) -> storage::Result<Vec<OutboxRow>> {
            Ok(Vec::new())
        }

        async fn mark_delivered(&self, _id: i64) -> storage::Result<()> {
            Ok(())
        }

        async fn schedule_retry(
            &self,
            _id: i64,
            _attempts: i32,
            _next_retry_at: DateTime<Utc>,
        ) -> storage::Result<()> {
            Ok(())
        }

        async fn mark_failed(&self, _id: i64) -> storage::Result<()> {
            Ok(())
        }
    }

    #[tokio::test]
    async fn session_stream_receives_full_runtime_events() {
        let emitter = OutboundEmitter::new(
            Arc::new(NoopOutbox),
            Arc::new(RwLock::new(OutboundRegistry::new())),
        );
        let sink = RecordingSink::default();
        let session_id = SessionId::from("http-thread-1");
        let stream = futures::stream::iter(vec![
            AgentEvent::ThinkingChunk {
                text: "thinking".to_string(),
            },
            AgentEvent::ToolCall {
                id: "call-1".to_string(),
                tool: "search".to_string(),
                args: json!({}),
            },
            AgentEvent::TokenChunk {
                text: "hello".to_string(),
            },
        ])
        .boxed();

        let outcome = consume_agent_stream(
            stream,
            Some("full-stream".to_string()),
            &session_id,
            &emitter,
            "session",
            &sink,
            None,
        )
        .await;

        assert_eq!(outcome.text, "hello");
        let events = sink.events.lock().expect("events lock").clone();
        assert!(events.contains(&("full-stream".to_string(), "thinking".to_string())));
        assert!(events.contains(&("full-stream".to_string(), "tool_call".to_string())));
        assert!(events.contains(&("full-stream".to_string(), "token".to_string())));
    }

    #[tokio::test]
    async fn bridge_terminal_publishes_final_and_done_to_each_followup_stream() {
        // Regression: the merged turn must deliver the real answer plus a
        // terminal `done` to every queued follow-up's stream (which were only
        // told `session_waiting`), not leave them hanging.
        let sink = RecordingSink::default();
        let session_id = SessionId::from("C123-T1");
        let bridged = vec!["stream-a".to_string(), "stream-b".to_string()];

        super::bridge_terminal_to_streams(&sink, &bridged, &session_id, "the answer").await;

        let events = sink.events.lock().expect("events lock").clone();
        assert_eq!(
            events,
            vec![
                ("stream-a".to_string(), "final".to_string()),
                ("stream-a".to_string(), "done".to_string()),
                ("stream-b".to_string(), "final".to_string()),
                ("stream-b".to_string(), "done".to_string()),
            ]
        );
    }
}

#[cfg(test)]
mod scheduled_run_tests {
    use super::{complete_scheduled_run, InboundEvent, ScheduledRunContext};
    use async_trait::async_trait;
    use chrono::{DateTime, Utc};
    use domain::cron::{CronJob, CronJobState};
    use domain::SessionId;
    use outbound::{OutboundChannel, OutboundEmitter, OutboundError, OutboundRegistry};
    use serde_json::{json, Value};
    use std::collections::HashMap;
    use std::sync::{Arc, Mutex};
    use storage::{CronJobRepo, OutboxRepo, OutboxRow, Result as StorageResult};
    use tokio::sync::RwLock;

    #[derive(Default)]
    struct RecordingOutbox {
        rows: Mutex<Vec<(String, Value)>>,
    }

    #[async_trait]
    impl OutboxRepo for RecordingOutbox {
        async fn enqueue(
            &self,
            _channel: &str,
            event_type: &str,
            payload: Value,
        ) -> StorageResult<i64> {
            let mut rows = self.rows.lock().unwrap();
            rows.push((event_type.to_string(), payload));
            Ok(rows.len() as i64)
        }
        async fn claim_due(&self, _limit: u32) -> StorageResult<Vec<OutboxRow>> {
            Ok(Vec::new())
        }
        async fn mark_delivered(&self, _id: i64) -> StorageResult<()> {
            Ok(())
        }
        async fn schedule_retry(
            &self,
            _id: i64,
            _attempts: i32,
            _next: DateTime<Utc>,
        ) -> StorageResult<()> {
            Ok(())
        }
        async fn mark_failed(&self, _id: i64) -> StorageResult<()> {
            Ok(())
        }
    }

    struct ScheduleChannel;

    #[async_trait]
    impl OutboundChannel for ScheduleChannel {
        fn name(&self) -> &str {
            "schedule"
        }
        fn kind(&self) -> &'static str {
            "test"
        }
        fn accepts(&self, event_type: &str) -> bool {
            event_type.starts_with("schedule.")
        }
        async fn deliver(&self, _event: &domain::OutboundEvent) -> outbound::Result<()> {
            Err(OutboundError::Delivery("unused".into()))
        }
    }

    #[derive(Default)]
    struct FakeCronRepo {
        jobs: Mutex<HashMap<String, CronJob>>,
        run_records: Mutex<Vec<(String, String)>>,
        increment_calls: Mutex<u32>,
    }

    impl FakeCronRepo {
        fn with_job(job: CronJob) -> Arc<Self> {
            let repo = Self::default();
            repo.jobs.lock().unwrap().insert(job.id.clone(), job);
            Arc::new(repo)
        }
        fn job_exists(&self, id: &str) -> bool {
            self.jobs.lock().unwrap().contains_key(id)
        }
    }

    #[async_trait]
    impl CronJobRepo for FakeCronRepo {
        async fn create(&self, _job: &CronJob) -> StorageResult<()> {
            Ok(())
        }
        async fn get(&self, id: &str) -> StorageResult<Option<CronJob>> {
            Ok(self.jobs.lock().unwrap().get(id).cloned())
        }
        async fn list_all(&self) -> StorageResult<Vec<CronJob>> {
            Ok(Vec::new())
        }
        async fn list_due(&self) -> StorageResult<Vec<CronJob>> {
            Ok(Vec::new())
        }
        async fn update_prompt(&self, _id: &str, _p: String) -> StorageResult<()> {
            Ok(())
        }
        async fn update_interval(&self, _id: &str, _i: u64) -> StorageResult<()> {
            Ok(())
        }
        async fn update_next_run(&self, _id: &str, _n: DateTime<Utc>) -> StorageResult<()> {
            Ok(())
        }
        async fn set_state(&self, id: &str, state: CronJobState) -> StorageResult<()> {
            if let Some(job) = self.jobs.lock().unwrap().get_mut(id) {
                job.state = state;
            }
            Ok(())
        }
        async fn record_run(
            &self,
            id: &str,
            _at: DateTime<Utc>,
            status: &str,
            _error: Option<&str>,
        ) -> StorageResult<()> {
            self.run_records
                .lock()
                .unwrap()
                .push((id.to_string(), status.to_string()));
            Ok(())
        }
        async fn increment_repeat(&self, id: &str) -> StorageResult<()> {
            *self.increment_calls.lock().unwrap() += 1;
            if let Some(job) = self.jobs.lock().unwrap().get_mut(id) {
                job.repeat_completed += 1;
            }
            Ok(())
        }
        async fn delete(&self, id: &str) -> StorageResult<()> {
            self.jobs.lock().unwrap().remove(id);
            Ok(())
        }
    }

    fn emitter(outbox: Arc<RecordingOutbox>) -> Arc<OutboundEmitter> {
        let registry = OutboundRegistry::new().with_channel(Arc::new(ScheduleChannel));
        Arc::new(OutboundEmitter::new(
            outbox,
            Arc::new(RwLock::new(registry)),
        ))
    }

    fn test_job(id: &str, interval: Option<u64>) -> CronJob {
        CronJob {
            id: id.to_string(),
            description: "test".to_string(),
            channel: "C123".to_string(),
            task_prompt: "do work".to_string(),
            cron_expression: None,
            interval_seconds: interval,
            repeat_count: None,
            repeat_completed: 0,
            state: CronJobState::Active,
            next_run_at: Utc::now(),
            last_run_at: None,
            last_status: None,
            last_error: None,
            session_continuation_id: None,
            created_at: Utc::now(),
            created_by_session: "scheduler".to_string(),
        }
    }

    fn scheduled_inbound(job_id: &str, is_one_shot: bool, is_wake: bool) -> InboundEvent {
        let now = Utc::now();
        InboundEvent {
            envelope_id: "cron-1".to_string(),
            session_id: SessionId::from("C123-cron-1"),
            user: "cron".to_string(),
            user_display_name: Some("Scheduler".to_string()),
            text: "do work".to_string(),
            attachments: Vec::new(),
            dynamic_context: Vec::new(),
            raw: json!({
                "source": "cron",
                "job_kind": "cron",
                "job_id": job_id,
                "schedule_run_key": format!("{job_id}:run"),
                "schedule_scheduled_at": now,
                "schedule_started_at": now,
                "schedule_is_one_shot": is_one_shot,
                "schedule_is_wake": is_wake,
            }),
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        }
    }

    #[test]
    fn scheduled_run_context_parses_only_scheduler_dispatched_inbounds() {
        // A normal user message carries no schedule keys.
        let ordinary = scheduled_inbound("job-1", false, false);
        assert!(ScheduledRunContext::from_inbound(&ordinary).is_some());

        let mut not_scheduled = ordinary.clone();
        not_scheduled.raw = json!({"source": "http"});
        assert!(ScheduledRunContext::from_inbound(&not_scheduled).is_none());
    }

    #[tokio::test]
    async fn completing_a_one_shot_run_emits_completed_and_deletes_job() {
        let outbox = Arc::new(RecordingOutbox::default());
        let repo = FakeCronRepo::with_job(test_job("oneshot-1", Some(0)));
        let inbound = scheduled_inbound("oneshot-1", true, false);
        let run = ScheduledRunContext::from_inbound(&inbound).expect("scheduled context");

        complete_scheduled_run(
            repo.as_ref(),
            emitter(outbox.clone()),
            &run,
            &SessionId::from("C123-cron-1"),
            None,
        )
        .await;

        // The run was recorded as completed and a SCHEDULE_RUN_COMPLETED emitted.
        assert_eq!(
            *repo.run_records.lock().unwrap(),
            vec![("oneshot-1".to_string(), "completed".to_string())]
        );
        let rows = outbox.rows.lock().unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].0, "schedule.run_completed");
        // One-shot jobs are deleted after the turn runs.
        assert!(!repo.job_exists("oneshot-1"));
    }

    #[tokio::test]
    async fn completing_a_failed_run_emits_run_failed() {
        let outbox = Arc::new(RecordingOutbox::default());
        let repo = FakeCronRepo::with_job(test_job("recurring-1", Some(600)));
        let inbound = scheduled_inbound("recurring-1", false, false);
        let run = ScheduledRunContext::from_inbound(&inbound).expect("scheduled context");

        complete_scheduled_run(
            repo.as_ref(),
            emitter(outbox.clone()),
            &run,
            &SessionId::from("C123-cron-1"),
            Some("model exploded"),
        )
        .await;

        assert_eq!(
            *repo.run_records.lock().unwrap(),
            vec![("recurring-1".to_string(), "error".to_string())]
        );
        let rows = outbox.rows.lock().unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].0, "schedule.run_failed");
        assert_eq!(rows[0].1["error"], "model exploded");
        // A recurring job survives a failed run (it will fire again).
        assert!(repo.job_exists("recurring-1"));
    }

    #[tokio::test]
    async fn recurring_run_with_repeat_count_advances_and_completes_on_last() {
        let outbox = Arc::new(RecordingOutbox::default());
        let mut job = test_job("repeat-1", Some(600));
        job.repeat_count = Some(2);
        job.repeat_completed = 1; // this is the final allowed run
        let repo = FakeCronRepo::with_job(job);
        let inbound = scheduled_inbound("repeat-1", false, false);
        let run = ScheduledRunContext::from_inbound(&inbound).expect("scheduled context");

        complete_scheduled_run(
            repo.as_ref(),
            emitter(outbox.clone()),
            &run,
            &SessionId::from("C123-cron-1"),
            None,
        )
        .await;

        assert_eq!(*repo.increment_calls.lock().unwrap(), 1);
        // repeat_completed (1) + 1 >= repeat_count (2): job removed.
        assert!(!repo.job_exists("repeat-1"));
    }
}
