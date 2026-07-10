#![allow(clippy::items_after_test_module)]

mod composition;
mod media;
mod queue;
mod session;

use std::collections::HashMap;
use std::sync::Arc;

use agent::{AgentEvent, AgentRunner, TurnInput};
use anyhow::Result;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{
    event_types, ConfigStore, InboundEvent, OutboundEvent, SessionId, SessionStatus,
    SubagentTaskState,
};
use futures::StreamExt;
use outbound::OutboundEmitter;
use serde_json::{json, Value};
use storage::{SessionRepo, SubagentTaskRepo};
use tokio::sync::{mpsc, RwLock};
use tracing::{info, warn};

use composition::compose_annotated_text;
use media::collect_media_for_turn;
pub use media::{AttachmentDownloader, HttpAttachmentDownloader};
use queue::{next_queued_inbound, QueuedInboundBacklog};
use session::ensure_session_persisted;

use crate::activity::RuntimeActivityReporter;
use crate::session_coordinator::{SessionCoordinator, Submission, TurnCancellation};

const USER_RETRY_MESSAGE: &str =
    "Something went wrong while generating that response. Please retry. The Hivy team has been notified of this error.";
const INTERRUPTED_MESSAGE: &str = "Interrupted.";
const INTERRUPTED_ERROR: &str = "interrupted by user";
const MAX_DURABLE_DELTA_TEXT_BYTES: usize = 128 * 1024;
const MAX_DURABLE_DELTA_COUNT: u64 = 1500;
const TOOL_SUMMARY_CHARS: usize = 2_000;

#[derive(Clone, Debug)]
pub struct StreamEventMetadata {
    pub trace_id: Option<String>,
    pub turn_id: Option<String>,
    pub subagent: Option<SubagentStreamMetadata>,
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

    async fn publish_turn_terminal(
        &self,
        _stream_id: &str,
        _session_id: &SessionId,
        _metadata: Option<&StreamEventMetadata>,
        _error: Option<&str>,
        _interrupted: bool,
    ) {
    }

    async fn publish_done(
        &self,
        _stream_id: &str,
        _session_id: &SessionId,
        _metadata: Option<&StreamEventMetadata>,
    ) {
    }

    async fn publish_waiting(
        &self,
        _stream_id: &str,
        _session_id: &SessionId,
        _reason: &str,
        _metadata: Option<&StreamEventMetadata>,
    ) {
    }

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

    async fn publish_turn_terminal(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        metadata: Option<&StreamEventMetadata>,
        error: Option<&str>,
        interrupted: bool,
    ) {
        let mut payload = serde_json::json!({
            "session_id": session_id.as_str(),
        });
        if let Some(map) = payload.as_object_mut() {
            if let Some(error) = error {
                map.insert("error".to_string(), serde_json::json!(error));
            }
            if interrupted {
                map.insert("interrupted".to_string(), serde_json::json!(true));
            }
        }
        self.publish(
            stream_id,
            if error.is_some() {
                "turn_failed"
            } else {
                "turn_completed"
            },
            stream_payload(payload, metadata),
        )
        .await;
    }

    async fn publish_done(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        metadata: Option<&StreamEventMetadata>,
    ) {
        self.publish(
            stream_id,
            "done",
            stream_payload(
                serde_json::json!({
                    "session_id": session_id.as_str(),
                }),
                metadata,
            ),
        )
        .await;
    }

    async fn publish_waiting(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        reason: &str,
        metadata: Option<&StreamEventMetadata>,
    ) {
        self.publish(
            stream_id,
            "session_waiting",
            stream_payload(
                serde_json::json!({
                    "session_id": session_id.as_str(),
                    "reason": reason,
                }),
                metadata,
            ),
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
                if event == "turn_completed" {
                    return;
                }
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
    let Some(map) = payload.as_object_mut() else {
        return payload;
    };
    let Some(metadata) = metadata else {
        map.entry("scope".to_string())
            .or_insert_with(|| serde_json::json!("main"));
        return payload;
    };
    if let Some(trace_id) = metadata.trace_id.as_deref() {
        map.entry("trace_id")
            .or_insert_with(|| serde_json::json!(trace_id));
    }
    if let Some(turn_id) = metadata.turn_id.as_deref() {
        map.entry("turn_id")
            .or_insert_with(|| serde_json::json!(turn_id));
    }
    if let Some(subagent) = metadata.subagent.as_ref() {
        map.insert("scope".to_string(), serde_json::json!("subagent"));
        map.insert(
            "subagent".to_string(),
            serde_json::json!({
                "job_id": subagent.job_id.as_str(),
                "agent_name": subagent.agent_name.as_str(),
                "parent_session_id": subagent.parent_session_id.as_str(),
                "child_session_id": subagent.child_session_id.as_str(),
            }),
        );
    } else {
        map.entry("scope".to_string())
            .or_insert_with(|| serde_json::json!("main"));
    }
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
            trace_id: None,
            turn_id: None,
            subagent: Some(SubagentStreamMetadata {
                job_id: job_id.to_string(),
                agent_name: agent_name.to_string(),
                parent_session_id: parent_session_id.to_string(),
                child_session_id: child_session_id.to_string(),
            }),
        }),
    )
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum TurnProcessResult {
    Completed,
    Interrupted,
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
    subagent_task_repo: Arc<dyn SubagentTaskRepo>,
    inbound_sink: mpsc::Sender<InboundEvent>,
    inbound: InboundEvent,
    activity_reporter: Option<Arc<RuntimeActivityReporter>>,
    canvas_source_session: Arc<RwLock<Option<String>>>,
) -> Result<()> {
    let submission = coordinator.submit_or_queue(inbound.clone());
    if matches!(submission, Submission::Queued) {
        let event_source = inbound_event_source(&inbound);
        emit_user_message_received(&emitter, &inbound, event_source, true).await;
        // Follow-ups share the stable session stream. Emit a live waiting event
        // so subscribers can show that the message is queued behind the active
        // turn without opening a second stream.
        if let Some(stream_id) = session_stream_id(&inbound) {
            let metadata = stream_event_metadata(&inbound);
            turn_event_sink
                .publish_waiting(
                    &stream_id,
                    &inbound.session_id,
                    "merged_into_active_turn",
                    metadata.as_ref(),
                )
                .await;
        }
        return Ok(());
    }

    let mut current_inbound = inbound;
    let _activity_guard = activity_reporter
        .as_ref()
        .map(|reporter| reporter.busy_period());
    let session_stream_id = session_stream_id(&current_inbound);
    let mut queued_backlog = QueuedInboundBacklog::default();
    'turns: loop {
        let cancellation = coordinator.start_turn(&current_inbound.session_id);
        let turn_result = process_single_turn(
            runner.clone(),
            attachment_downloader.clone(),
            config.clone(),
            emitter.clone(),
            session_repo.clone(),
            &current_inbound,
            turn_event_sink.clone(),
            subagent_task_repo.clone(),
            coordinator.clone(),
            inbound_sink.clone(),
            canvas_source_session.clone(),
            cancellation,
        )
        .await;
        coordinator.finish_active_turn(&current_inbound.session_id);
        if matches!(turn_result?, TurnProcessResult::Interrupted) {
            let interrupted_follow_ups = coordinator.finish_turn(&current_inbound.session_id);
            if interrupted_follow_ups.is_empty() && !is_subagent_task_inbound(&current_inbound) {
                if let Some(stream_id) = session_stream_id.as_deref() {
                    let metadata = stream_event_metadata(&current_inbound);
                    turn_event_sink
                        .publish_done(stream_id, &current_inbound.session_id, metadata.as_ref())
                        .await;
                }
            }
            for follow_up in interrupted_follow_ups {
                if let Err(e) = inbound_sink.send(follow_up).await {
                    warn!(
                        error = %e,
                        session = %current_inbound.session_id,
                        "failed to re-dispatch queued session message after interrupt"
                    );
                }
            }
            break 'turns;
        }

        let mut published_waiting = false;
        let mut subagent_task_wait_deadline: Option<std::time::Instant> = None;
        loop {
            let follow_ups = coordinator.drain_queued(&current_inbound.session_id);
            if let Some(next) =
                next_queued_inbound(&current_inbound, follow_ups, &mut queued_backlog)
            {
                current_inbound = next;
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
                        let metadata = stream_event_metadata(&current_inbound);
                        turn_event_sink
                            .publish_waiting(
                                stream_id,
                                &current_inbound.session_id,
                                "subagent_tasks",
                                metadata.as_ref(),
                            )
                            .await;
                    }
                    published_waiting = true;
                }
                tokio::time::sleep(std::time::Duration::from_millis(500)).await;
                continue;
            }
            subagent_task_wait_deadline = None;

            if published_waiting {
                tokio::time::sleep(std::time::Duration::from_millis(100)).await;
                let follow_ups = coordinator.drain_queued(&current_inbound.session_id);
                if let Some(next) =
                    next_queued_inbound(&current_inbound, follow_ups, &mut queued_backlog)
                {
                    current_inbound = next;
                    continue 'turns;
                }
            }

            let follow_ups = coordinator.finish_turn(&current_inbound.session_id);
            if let Some(next) =
                next_queued_inbound(&current_inbound, follow_ups, &mut queued_backlog)
            {
                current_inbound = next;
                coordinator.reserve(&current_inbound.session_id);
                continue 'turns;
            }

            if queued_backlog.is_empty() {
                if !is_subagent_task_inbound(&current_inbound) {
                    if let Some(stream_id) = session_stream_id.as_deref() {
                        let metadata = stream_event_metadata(&current_inbound);
                        turn_event_sink
                            .publish_done(stream_id, &current_inbound.session_id, metadata.as_ref())
                            .await;
                    }
                }
                break 'turns;
            }
        }
    }

    Ok(())
}

/// Maximum time the parent turn will wait for its subagent tasks to report back
/// before force-failing them. Without this bound a task whose result write
/// never lands keeps the job Active and spins the parent loop forever.
const MAX_SUBAGENT_TASK_WAIT: std::time::Duration = std::time::Duration::from_secs(15 * 60);

async fn remember_canvas_source_session(
    canvas_source_session: &Arc<RwLock<Option<String>>>,
    session_id: &SessionId,
) {
    *canvas_source_session.write().await = Some(session_id.as_str().to_string());
}

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

#[allow(clippy::too_many_arguments)]
async fn process_single_turn(
    runner: Arc<dyn AgentRunner>,
    attachment_downloader: Arc<dyn AttachmentDownloader>,
    _config: ConfigStore,
    emitter: Arc<OutboundEmitter>,
    session_repo: Arc<dyn SessionRepo>,
    inbound: &InboundEvent,
    turn_event_sink: Arc<dyn TurnEventSink>,
    subagent_task_repo: Arc<dyn SubagentTaskRepo>,
    _coordinator: Arc<SessionCoordinator>,
    _inbound_sink: mpsc::Sender<InboundEvent>,
    canvas_source_session: Arc<RwLock<Option<String>>>,
    mut cancellation: TurnCancellation,
) -> Result<TurnProcessResult> {
    if let ScheduledRunStatus::Malformed(malformed) = ScheduledRunStatus::from_inbound(inbound) {
        fail_malformed_scheduled_run(&malformed, &inbound.session_id).await;
        return Ok(TurnProcessResult::Completed);
    }

    if inbound.text.trim().is_empty() && inbound.attachments.is_empty() {
        return Ok(TurnProcessResult::Completed);
    }
    info!(
        session = %inbound.session_id,
        user = %inbound.user,
        text_len = inbound.text.len(),
        attachments = inbound.attachments.len(),
        "inbound message"
    );

    let session_id = inbound.session_id.clone();
    remember_canvas_source_session(&canvas_source_session, &session_id).await;

    let was_new_session = ensure_session_persisted(session_repo.as_ref(), inbound, &emitter).await;
    if !was_new_session {
        let _ = session_repo.touch(&session_id, Utc::now()).await;
    }
    let event_source = inbound_event_source(inbound);
    emit_user_message_received(&emitter, inbound, event_source, false).await;

    let media = collect_media_for_turn(attachment_downloader.as_ref(), &inbound.attachments).await;
    let annotated_text = compose_annotated_text(inbound, &media);

    let mut turn_input = TurnInput::text(annotated_text);
    turn_input = turn_input.with_actor_user(inbound.actor_user_id.clone());
    if let Some(model) = inbound.model_definition.clone() {
        turn_input = turn_input.with_model_override(model);
    }
    if let Some(stream_id) = session_stream_id(inbound) {
        turn_input = turn_input.with_session_stream_id(stream_id);
    }
    if let (Some(trace_id), Some(turn_id)) = (trace_id(inbound), turn_id(inbound)) {
        turn_input = turn_input.with_turn_context(trace_id, turn_id);
    }
    for context in &inbound.session_context {
        turn_input = turn_input.with_session_context(context.clone());
    }

    let session_stream_id = session_stream_id(inbound);
    let stream_metadata = stream_event_metadata(inbound);
    if let Some(stream_id) = session_stream_id.as_deref() {
        turn_event_sink
            .activate_session_stream(&session_id, stream_id)
            .await;
    }
    let durable_session = durable_session_for(&session_id, stream_metadata.as_ref());
    let event_context = DurableEventContext {
        emitter: emitter.as_ref(),
        session_id: &session_id,
        durable_session_id: &durable_session,
        source: event_source,
        stream_id: session_stream_id.as_deref(),
        metadata: stream_metadata.as_ref(),
    };
    emit_canonical_runtime_event(
        &event_context,
        event_types::TURN_STARTED,
        0,
        json!({
            "input_text_len": inbound.text.len(),
        }),
    )
    .await;

    let outcome = tokio::select! {
        _ = cancellation.cancelled() => StreamOutcome::interrupted(),
        stream_result = runner.run_turn(&session_id, turn_input, inbound.agent_definition.clone()) => {
            match stream_result {
                Ok(stream) => {
                    tokio::select! {
                        _ = cancellation.cancelled() => StreamOutcome::interrupted(),
                        outcome = consume_agent_stream(
                            stream,
                            session_stream_id.clone(),
                            &session_id,
                            &emitter,
                            event_source,
                            turn_event_sink.as_ref(),
                            stream_metadata.as_ref(),
                        ) => outcome,
                    }
                }
                Err(e) => StreamOutcome::error(e.to_string()),
            }
        }
    };

    let final_text = format_final_message(&outcome);
    if let Some(stream_id) = session_stream_id.as_deref() {
        let terminal_error = outcome.terminal_error();
        turn_event_sink
            .publish_final(
                stream_id,
                &session_id,
                &final_text,
                stream_metadata.as_ref(),
            )
            .await;
        turn_event_sink
            .publish_turn_terminal(
                stream_id,
                &session_id,
                stream_metadata.as_ref(),
                terminal_error.as_deref(),
                outcome.interrupted,
            )
            .await;
    }
    emit_canonical_runtime_event(
        &event_context,
        event_types::FINAL,
        outcome.sequence + 1,
        json!({ "text": final_text }),
    )
    .await;
    if outcome.interrupted {
        emit_canonical_runtime_event(
            &event_context,
            event_types::TURN_FAILED,
            outcome.sequence + 2,
            json!({
                "error": INTERRUPTED_ERROR,
                "interrupted": true,
            }),
        )
        .await;
    } else if let Some(error_message) = outcome.error.as_ref() {
        emit_canonical_runtime_event(
            &event_context,
            event_types::TURN_FAILED,
            outcome.sequence + 2,
            json!({ "error": error_message }),
        )
        .await;
        let _ = session_repo
            .set_status(&session_id, SessionStatus::Errored)
            .await;
    } else {
        emit_canonical_runtime_event(
            &event_context,
            event_types::TURN_COMPLETED,
            outcome.sequence + 2,
            json!({}),
        )
        .await;
    }
    emitter.flush_database().await;

    if let Some(stream_id) = session_stream_id.as_deref() {
        turn_event_sink
            .clear_active_session_stream(&session_id, stream_id)
            .await;
    }

    info!(session = %session_id, len = outcome.text.len(), "turn complete");

    if outcome.interrupted {
        return Ok(TurnProcessResult::Interrupted);
    }

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
            let parent_session = SessionId::from(parent_session_id);
            let parent_event_context = DurableEventContext {
                emitter: emitter.as_ref(),
                session_id: &parent_session,
                // Lifecycle events are already parent-scoped; durable copy
                // matches the live session.
                durable_session_id: &parent_session,
                source: "subagent_task",
                stream_id: session_stream_id.as_deref(),
                metadata: stream_metadata.as_ref(),
            };
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
                emit_canonical_runtime_event(
                    &parent_event_context,
                    event_types::SUBAGENT_ERRORED,
                    outcome.sequence + 3,
                    json!({
                        "job_id": job_id,
                        "agent_name": agent_name,
                        "child_session_id": session_id.as_str(),
                        "error": subagent_error.unwrap_or("unknown"),
                    }),
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
                emit_canonical_runtime_event(
                    &parent_event_context,
                    event_types::SUBAGENT_COMPLETED,
                    outcome.sequence + 3,
                    json!({
                        "job_id": job_id,
                        "agent_name": agent_name,
                        "child_session_id": session_id.as_str(),
                        "result": &result_text,
                    }),
                )
                .await;
            }

            // Mark the job complete so the foreground subagent_task tool can
            // return this result directly to the parent model. No synthetic
            // parent inbound is needed; OpenCode-style task results are tool
            // outputs, while lifecycle events remain available to clients.
            complete_subagent_task_result_or_force_fail(
                subagent_task_repo.as_ref(),
                job_id,
                subagent_error,
                &result_text,
            )
            .await;
        }
    } else if let Some(run) = ScheduledRunContext::from_inbound(inbound) {
        // A scheduled cron run only reaches a terminal status now that
        // the turn has actually executed — not when the scheduler enqueued it.
        // Emit SCHEDULE_RUN_COMPLETED/FAILED, record the run, delete
        // one-shot jobs, and advance repeat counts here.
        complete_scheduled_run(emitter.clone(), &run, &session_id, outcome.error.as_deref()).await;
    }

    Ok(TurnProcessResult::Completed)
}

/// Run-lifecycle context the scheduler embeds in a scheduled job's inbound `raw`
/// so the turn handler can complete the run after the turn executes.
struct ScheduledRunContext {
    job_id: String,
    run_key: String,
    scheduled_at: chrono::DateTime<Utc>,
    started_at: chrono::DateTime<Utc>,
}

struct MalformedScheduledRunContext {
    job_id: Option<String>,
    reason: String,
}

enum ScheduledRunStatus {
    None,
    Valid(ScheduledRunContext),
    Malformed(MalformedScheduledRunContext),
}

impl ScheduledRunContext {
    fn from_inbound(inbound: &InboundEvent) -> Option<Self> {
        match ScheduledRunStatus::from_inbound(inbound) {
            ScheduledRunStatus::Valid(run) => Some(run),
            ScheduledRunStatus::None | ScheduledRunStatus::Malformed(_) => None,
        }
    }
}

impl ScheduledRunStatus {
    fn from_inbound(inbound: &InboundEvent) -> Self {
        let Some(raw) = inbound.raw.as_object() else {
            return Self::None;
        };
        let has_schedule_metadata = raw.contains_key("schedule_run_key")
            || raw.contains_key("schedule_scheduled_at")
            || raw.contains_key("schedule_started_at");
        if !has_schedule_metadata {
            return Self::None;
        }

        let job_id = raw
            .get("job_id")
            .and_then(Value::as_str)
            .map(str::to_string);
        let malformed = |reason: &str| {
            Self::Malformed(MalformedScheduledRunContext {
                job_id: job_id.clone(),
                reason: reason.to_string(),
            })
        };

        let Some(run_key) = raw.get("schedule_run_key").and_then(Value::as_str) else {
            return malformed("missing schedule_run_key");
        };
        let Some(job_id) = job_id.clone() else {
            return malformed("missing job_id");
        };
        let Some(scheduled_at_raw) = raw.get("schedule_scheduled_at").and_then(Value::as_str)
        else {
            return malformed("missing schedule_scheduled_at");
        };
        let Ok(scheduled_at) = scheduled_at_raw.parse() else {
            return malformed("invalid schedule_scheduled_at");
        };
        let Some(started_at_raw) = raw.get("schedule_started_at").and_then(Value::as_str) else {
            return malformed("missing schedule_started_at");
        };
        let Ok(started_at) = started_at_raw.parse() else {
            return malformed("invalid schedule_started_at");
        };

        Self::Valid(ScheduledRunContext {
            job_id,
            run_key: run_key.to_string(),
            scheduled_at,
            started_at,
        })
    }
}

async fn fail_malformed_scheduled_run(
    malformed: &MalformedScheduledRunContext,
    session_id: &SessionId,
) {
    warn!(
        session = %session_id,
        job_id = malformed.job_id.as_deref().unwrap_or("<missing>"),
        reason = %malformed.reason,
        "schedule: malformed scheduled run metadata"
    );
}

/// Complete a backend-dispatched scheduled run after its turn executed. The
/// backend owns schedule durability; runtime only reports the terminal outcome
/// from the inbound metadata.
async fn complete_scheduled_run(
    emitter: Arc<OutboundEmitter>,
    run: &ScheduledRunContext,
    session_id: &SessionId,
    error: Option<&str>,
) {
    let completed_at = Utc::now();
    let event_type = if error.is_some() {
        event_types::SCHEDULE_RUN_FAILED
    } else {
        event_types::SCHEDULE_RUN_COMPLETED
    };
    emitter
        .emit(OutboundEvent::new(
            event_type,
            json!({
                "source": "cron",
                "job_id": run.job_id,
                "run_key": run.run_key,
                "scheduled_at": run.scheduled_at.to_rfc3339(),
                "started_at": run.started_at.to_rfc3339(),
                "completed_at": completed_at.to_rfc3339(),
                "duration_ms": (completed_at - run.started_at).num_milliseconds(),
                "error": error,
                "session_id": session_id.as_str(),
            }),
        ))
        .await;
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

#[cfg(test)]
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
    pub interrupted: bool,
    pub sequence: u64,
}

impl StreamOutcome {
    fn error(message: String) -> Self {
        Self {
            text: String::new(),
            final_message: None,
            error: Some(message),
            interrupted: false,
            sequence: 0,
        }
    }

    fn interrupted() -> Self {
        Self {
            text: String::new(),
            final_message: Some(INTERRUPTED_MESSAGE.to_string()),
            error: None,
            interrupted: true,
            sequence: 0,
        }
    }

    fn terminal_error(&self) -> Option<String> {
        if self.interrupted {
            return Some(INTERRUPTED_ERROR.to_string());
        }
        self.error.clone()
    }
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
    let mut durable = DurableAgentStream::default();
    let durable_session = durable_session_for(session_id, metadata);
    let event_context = DurableEventContext {
        emitter,
        session_id,
        durable_session_id: &durable_session,
        source,
        stream_id: stream_id.as_deref(),
        metadata,
    };
    while let Some(event) = stream.next().await {
        sequence += 1;
        let client_event = sanitize_agent_event_for_clients(&event, session_id, source, sequence);
        if let Some(stream_id) = stream_id.as_deref() {
            let publish_to_session_stream = !matches!(
                &client_event,
                AgentEvent::RunEvent { event, .. } if event == "turn_completed"
            );
            if publish_to_session_stream {
                event_sink
                    .publish_agent_event(stream_id, session_id, &client_event, metadata)
                    .await;
            }
        }
        emit_preview_runtime_event(&event_context, sequence, &client_event).await;
        durable
            .record_event(&event_context, sequence, &client_event)
            .await;
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
    durable.flush_all(&event_context).await;
    emitter
        .flush_streams_for_session_background(session_id.as_str())
        .await;
    StreamOutcome {
        text: accumulated,
        final_message,
        error: error_message,
        interrupted: false,
        sequence,
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
                if result
                    .get("safe_error")
                    .and_then(Value::as_bool)
                    .unwrap_or(false)
                {
                    if let Some(map) = next.as_object_mut() {
                        map.remove("safe_error");
                    }
                    return AgentEvent::ToolResult {
                        id: id.clone(),
                        result: next,
                    };
                }
                capture_agent_internal_error(
                    session_id,
                    source,
                    sequence,
                    event_types::TOOL_RESULT,
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

#[derive(Default)]
struct DurableAgentStream {
    pending_delta: Option<DurableDelta>,
    tool_calls: HashMap<String, DurableToolCall>,
}

struct DurableEventContext<'a> {
    emitter: &'a OutboundEmitter,
    /// Session the LIVE turn runs under. For a subagent turn this is the child
    /// session id, and it is what the live preview copy is scoped to.
    session_id: &'a SessionId,
    /// Session the DURABLE copy is persisted under. Equal to `session_id` for a
    /// normal turn, but the PARENT session id for a subagent turn so the
    /// subagent detail survives a parent page refresh. runtime_seq is assigned
    /// downstream per this session id (see storage outbox_enqueue_runtime).
    durable_session_id: &'a SessionId,
    source: &'static str,
    stream_id: Option<&'a str>,
    metadata: Option<&'a StreamEventMetadata>,
}

/// The session the DURABLE canonical copy must be scoped to. For a subagent
/// turn (metadata carries a subagent block) this is the PARENT session id so the
/// subagent detail is persisted under, and replays with, the parent session.
fn durable_session_for(
    session_id: &SessionId,
    metadata: Option<&StreamEventMetadata>,
) -> SessionId {
    metadata
        .and_then(|metadata| metadata.subagent.as_ref())
        .map(|subagent| SessionId::from(subagent.parent_session_id.as_str()))
        .unwrap_or_else(|| session_id.clone())
}

struct DurableDelta {
    event_type: &'static str,
    text: String,
    sequence_start: u64,
    sequence_end: u64,
    delta_count: u64,
    started_at: DateTime<Utc>,
    ended_at: DateTime<Utc>,
}

struct DurableToolCall {
    tool: String,
    args_summary: String,
    started_at: DateTime<Utc>,
}

impl DurableAgentStream {
    async fn record_event(
        &mut self,
        context: &DurableEventContext<'_>,
        sequence: u64,
        event: &AgentEvent,
    ) {
        match event {
            AgentEvent::ThinkingChunk { text } => {
                self.record_delta(context, sequence, event_types::THINKING, text)
                    .await;
            }
            AgentEvent::TokenChunk { text } => {
                self.record_delta(context, sequence, event_types::TOKEN, text)
                    .await;
            }
            AgentEvent::ToolCall { id, tool, args } => {
                self.tool_calls.insert(
                    id.clone(),
                    DurableToolCall {
                        tool: tool.clone(),
                        args_summary: summarize_json(args, TOOL_SUMMARY_CHARS),
                        started_at: Utc::now(),
                    },
                );
            }
            AgentEvent::ToolResult { id, result } => {
                self.flush_all(context).await;
                let call = self.tool_calls.remove(id);
                let error = result.get("error").and_then(Value::as_str).map(str::trim);
                let status = if error.is_some_and(|value| !value.is_empty()) {
                    "errored"
                } else {
                    "completed"
                };
                let mut payload = json!({
                    "id": id,
                    "tool": call
                        .as_ref()
                        .map(|call| call.tool.as_str())
                        .unwrap_or("unknown"),
                    "status": status,
                });
                if let Some(call) = call.as_ref() {
                    if let Some(map) = payload.as_object_mut() {
                        map.insert("args_summary".to_string(), json!(call.args_summary));
                        map.insert(
                            "duration_ms".to_string(),
                            json!((Utc::now() - call.started_at).num_milliseconds()),
                        );
                    }
                }
                if status == "errored" {
                    if let Some(map) = payload.as_object_mut() {
                        map.insert(
                            "error".to_string(),
                            json!(error.unwrap_or("tool call failed")),
                        );
                    }
                } else if let Some(map) = payload.as_object_mut() {
                    map.insert(
                        "result_summary".to_string(),
                        json!(summarize_json(result, TOOL_SUMMARY_CHARS)),
                    );
                }
                emit_canonical_runtime_event(context, event_types::TOOL_RESULT, sequence, payload)
                    .await;
            }
            AgentEvent::RunEvent { event, payload } => {
                if is_durable_run_event(event) {
                    self.flush_all(context).await;
                    emit_canonical_runtime_event(context, event, sequence, payload.clone()).await;
                }
            }
            AgentEvent::Error { message } => {
                self.flush_all(context).await;
                emit_canonical_runtime_event(
                    context,
                    event_types::ERROR,
                    sequence,
                    json!({ "message": message }),
                )
                .await;
            }
            AgentEvent::FinalMessage { .. } => {}
        }
    }

    async fn record_delta(
        &mut self,
        context: &DurableEventContext<'_>,
        sequence: u64,
        event_type: &'static str,
        text: &str,
    ) {
        if text.is_empty() {
            return;
        }
        let must_flush = self.pending_delta.as_ref().is_some_and(|pending| {
            pending.event_type != event_type
                || sequence == 0
                || sequence != pending.sequence_end + 1
                || pending.delta_count >= MAX_DURABLE_DELTA_COUNT
                || pending.text.len() + text.len() > MAX_DURABLE_DELTA_TEXT_BYTES
        });
        if must_flush {
            self.flush_pending_delta(context).await;
        }
        let now = Utc::now();
        let pending = self.pending_delta.get_or_insert_with(|| DurableDelta {
            event_type,
            text: String::new(),
            sequence_start: sequence,
            sequence_end: sequence,
            delta_count: 0,
            started_at: now,
            ended_at: now,
        });
        pending.sequence_end = sequence;
        pending.delta_count += 1;
        pending.text.push_str(text);
        pending.ended_at = now;
        if pending.delta_count >= MAX_DURABLE_DELTA_COUNT
            || pending.text.len() >= MAX_DURABLE_DELTA_TEXT_BYTES
        {
            self.flush_pending_delta(context).await;
        }
    }

    async fn flush_all(&mut self, context: &DurableEventContext<'_>) {
        self.flush_pending_delta(context).await;
    }

    async fn flush_pending_delta(&mut self, context: &DurableEventContext<'_>) {
        let Some(delta) = self.pending_delta.take() else {
            return;
        };
        emit_canonical_runtime_event(
            context,
            delta.event_type,
            delta.sequence_end,
            json!({
                "text": delta.text,
                "coalesced": true,
                "delta_count": delta.delta_count,
                "sequence_start": delta.sequence_start,
                "sequence_end": delta.sequence_end,
                "started_at": delta.started_at,
                "ended_at": delta.ended_at,
            }),
        )
        .await;
    }
}

fn is_durable_run_event(event: &str) -> bool {
    matches!(
        event,
        event_types::MODEL_USAGE
            | event_types::QUESTION_REQUESTED
            | event_types::QUESTION_ANSWERED
            | event_types::PLAN_UPDATED
            | event_types::SUBAGENT_STARTED
            | event_types::SUBAGENT_COMPLETED
            | event_types::SUBAGENT_ERRORED
            | event_types::SESSION_WAITING
            | event_types::ERROR
    )
}

async fn emit_canonical_runtime_event(
    context: &DurableEventContext<'_>,
    event_type: &str,
    sequence: u64,
    payload: Value,
) {
    context
        .emitter
        .emit(OutboundEvent::new(
            event_type,
            // The durable copy is scoped to the parent session for subagent
            // turns; the metadata still stamps scope:"subagent" + the subagent
            // block, and runtime_seq is drawn from this session id downstream.
            canonical_runtime_payload(
                event_type,
                context.durable_session_id,
                context.source,
                sequence,
                context.stream_id,
                context.metadata,
                payload,
            ),
        ))
        .await;
}

async fn emit_preview_runtime_event(
    context: &DurableEventContext<'_>,
    sequence: u64,
    event: &AgentEvent,
) {
    let (event_type, payload) = match event {
        AgentEvent::ThinkingChunk { text } => (event_types::THINKING, json!({ "text": text })),
        AgentEvent::TokenChunk { text } => (event_types::TOKEN, json!({ "text": text })),
        AgentEvent::ToolCall { id, tool, args } => (
            "tool_call",
            json!({
                "id": id,
                "tool": tool,
                "args": args,
            }),
        ),
        AgentEvent::ToolResult { id, result } => (
            event_types::TOOL_RESULT,
            json!({
                "id": id,
                "result": result,
            }),
        ),
        AgentEvent::RunEvent { event, payload } => (event.as_str(), payload.clone()),
        AgentEvent::FinalMessage { text } => (event_types::FINAL, json!({ "text": text })),
        AgentEvent::Error { message } => (event_types::ERROR, json!({ "message": message })),
    };
    let mut payload = canonical_runtime_payload(
        event_type,
        context.session_id,
        context.source,
        sequence,
        context.stream_id,
        context.metadata,
        payload,
    );
    if let Some(map) = payload.as_object_mut() {
        map.insert("durability".to_string(), json!("preview"));
    }
    context
        .emitter
        .emit(OutboundEvent::new(event_type, payload))
        .await;
}

fn canonical_runtime_payload(
    event_type: &str,
    session_id: &SessionId,
    source: &'static str,
    sequence: u64,
    stream_id: Option<&str>,
    metadata: Option<&StreamEventMetadata>,
    payload: Value,
) -> Value {
    let mut payload = match payload {
        Value::Object(map) => Value::Object(map),
        value => json!({ "value": value }),
    };
    let Some(map) = payload.as_object_mut() else {
        return payload;
    };
    map.insert("event_name".to_string(), json!(event_type));
    map.insert("session_id".to_string(), json!(session_id.as_str()));
    map.insert("source".to_string(), json!(source));
    map.insert("sequence".to_string(), json!(sequence));
    map.entry("durability".to_string())
        .or_insert_with(|| json!("durable"));
    map.insert("occurred_at".to_string(), json!(Utc::now()));
    if let Some(stream_id) = stream_id {
        map.insert("stream_id".to_string(), json!(stream_id));
    }
    if let Some(metadata) = metadata {
        if let Some(trace_id) = metadata.trace_id.as_ref() {
            map.insert("trace_id".to_string(), json!(trace_id));
        }
        if let Some(turn_id) = metadata.turn_id.as_ref() {
            map.insert("turn_id".to_string(), json!(turn_id));
        }
        if let Some(subagent) = metadata.subagent.as_ref() {
            map.insert("scope".to_string(), json!("subagent"));
            map.insert(
                "subagent".to_string(),
                json!({
                    "job_id": subagent.job_id,
                    "agent_name": subagent.agent_name,
                    "parent_session_id": subagent.parent_session_id,
                    "child_session_id": subagent.child_session_id,
                }),
            );
        }
    }
    map.entry("scope".to_string())
        .or_insert_with(|| json!("main"));
    map.entry("event_id".to_string()).or_insert_with(|| {
        json!(canonical_runtime_event_id(
            session_id, metadata, event_type, sequence
        ))
    });
    payload
}

fn canonical_runtime_event_id(
    session_id: &SessionId,
    metadata: Option<&StreamEventMetadata>,
    event_type: &str,
    sequence: u64,
) -> String {
    let turn = metadata
        .and_then(|metadata| metadata.turn_id.as_deref())
        .unwrap_or("session");
    let scope = metadata
        .and_then(|metadata| metadata.subagent.as_ref())
        .map(|subagent| subagent.job_id.as_str())
        .unwrap_or("main");
    format!(
        "evt_rt_{}_{}_{}_{}_{}",
        event_id_part(session_id.as_str()),
        event_id_part(turn),
        event_id_part(scope),
        sequence,
        event_id_part(event_type)
    )
}

fn event_id_part(value: &str) -> String {
    value
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || ch == '-' || ch == '_' {
                ch
            } else {
                '-'
            }
        })
        .collect()
}

fn summarize_json(value: &Value, max_chars: usize) -> String {
    let raw = serde_json::to_string(value).unwrap_or_else(|_| "<unserializable>".to_string());
    if raw.chars().count() <= max_chars {
        return raw;
    }
    let mut summary = raw.chars().take(max_chars).collect::<String>();
    summary.push_str("...");
    summary
}

fn session_stream_id(inbound: &InboundEvent) -> Option<String> {
    inbound
        .raw
        .get("session_stream_id")
        .and_then(serde_json::Value::as_str)
        .map(ToString::to_string)
}

fn trace_id(inbound: &InboundEvent) -> Option<String> {
    inbound
        .raw
        .get("trace_id")
        .and_then(serde_json::Value::as_str)
        .map(ToString::to_string)
}

fn turn_id(inbound: &InboundEvent) -> Option<String> {
    inbound
        .raw
        .get("turn_id")
        .and_then(serde_json::Value::as_str)
        .map(ToString::to_string)
}

fn stream_event_metadata(inbound: &InboundEvent) -> Option<StreamEventMetadata> {
    let trace_id = trace_id(inbound);
    let turn_id = turn_id(inbound);
    if !is_subagent_task_inbound(inbound) {
        if trace_id.is_none() && turn_id.is_none() {
            return None;
        }
        return Some(StreamEventMetadata {
            trace_id,
            turn_id,
            subagent: None,
        });
    }
    let parent_session_id = inbound
        .raw
        .get("parent_session_id")
        .and_then(Value::as_str)?;
    let job_id = inbound.raw.get("job_id")?.as_str()?.to_string();
    let agent_name = inbound
        .raw
        .get("agent_name")
        .and_then(Value::as_str)
        .unwrap_or("sub-agent")
        .to_string();
    Some(StreamEventMetadata {
        trace_id,
        turn_id,
        subagent: Some(SubagentStreamMetadata {
            job_id,
            agent_name,
            parent_session_id: parent_session_id.to_string(),
            child_session_id: inbound.session_id.as_str().to_string(),
        }),
    })
}

fn format_final_message(outcome: &StreamOutcome) -> String {
    if outcome.interrupted {
        return INTERRUPTED_MESSAGE.to_string();
    }
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
            interrupted: false,
            sequence: 0,
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
            interrupted: false,
            sequence: 0,
        });

        assert_eq!(text, "Going good!");
    }

    #[test]
    fn final_message_falls_back_to_tokens_when_absent() {
        let text = format_final_message(&StreamOutcome {
            text: "Streaming answer".to_string(),
            final_message: None,
            error: None,
            interrupted: false,
            sequence: 0,
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
            interrupted: false,
            sequence: 0,
        });

        assert!(!text.contains("readonly database"));
        assert!(!text.contains("error returned from database"));
        assert_eq!(text, USER_RETRY_MESSAGE);
    }

    #[test]
    fn interrupted_outcome_uses_interrupt_message() {
        let text = format_final_message(&StreamOutcome::interrupted());

        assert_eq!(text, "Interrupted.");
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

    #[test]
    fn client_tool_result_preserves_safe_model_error_payload() {
        let event = AgentEvent::ToolResult {
            id: "tool-1".to_string(),
            result: json!({
                "error": "The model produced an invalid `read_file` tool call: missing required argument(s): path.",
                "safe_error": true,
            }),
        };
        let sanitized =
            sanitize_agent_event_for_clients(&event, &SessionId::from("session-1"), "http", 1);

        match sanitized {
            AgentEvent::ToolResult { result, .. } => {
                assert_eq!(
	                    result["error"],
	                    "The model produced an invalid `read_file` tool call: missing required argument(s): path."
	                );
                assert!(result.get("safe_error").is_none());
                assert!(result.get("team_notified").is_none());
            }
            other => panic!("unexpected event: {other:?}"),
        }
    }
}

#[cfg(test)]
mod stream_tests {
    use super::{
        consume_agent_stream, stream_payload, StreamEventMetadata, SubagentStreamMetadata,
        TurnEventSink,
    };
    use agent::AgentEvent;
    use async_trait::async_trait;
    use chrono::{DateTime, Utc};
    use domain::{OutboundEvent, SessionId};
    use futures::StreamExt;
    use outbound::{OutboundChannel, OutboundEmitter, OutboundError, OutboundRegistry};
    use serde_json::{json, Value};
    use std::collections::HashMap;
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

        async fn publish_done(
            &self,
            stream_id: &str,
            _session_id: &SessionId,
            _metadata: Option<&super::StreamEventMetadata>,
        ) {
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

        async fn pending_count(&self) -> storage::Result<i64> {
            Ok(0)
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

    #[test]
    fn stream_payload_stamps_main_scope() {
        let payload = stream_payload(json!({"text": "answer"}), None);

        assert_eq!(payload["scope"], "main");
    }

    #[test]
    fn stream_payload_stamps_subagent_scope() {
        let metadata = StreamEventMetadata {
            trace_id: Some("trace-1".to_string()),
            turn_id: Some("turn-1".to_string()),
            subagent: Some(SubagentStreamMetadata {
                job_id: "job-1".to_string(),
                agent_name: "helper".to_string(),
                parent_session_id: "parent-session".to_string(),
                child_session_id: "child-session".to_string(),
            }),
        };
        let payload = stream_payload(json!({"text": "answer"}), Some(&metadata));

        assert_eq!(payload["scope"], "subagent");
        assert_eq!(payload["trace_id"], "trace-1");
        assert_eq!(payload["turn_id"], "turn-1");
        assert_eq!(payload["subagent"]["job_id"], "job-1");
        assert_eq!(payload["subagent"]["agent_name"], "helper");
        assert_eq!(payload["subagent"]["parent_session_id"], "parent-session");
        assert_eq!(payload["subagent"]["child_session_id"], "child-session");
    }

    /// Outbox that records every runtime event and assigns a monotonic
    /// per-session `runtime_seq`, mirroring the production
    /// `outbox_enqueue_runtime` gateway. This lets the durable-scoping test
    /// assert that subagent detail is persisted under the PARENT session and
    /// carries a non-zero `runtime_seq`.
    #[derive(Default)]
    struct RuntimeSeqOutbox {
        rows: Mutex<Vec<(String, Value)>>,
        seqs: Mutex<HashMap<String, i64>>,
    }

    #[async_trait]
    impl OutboxRepo for RuntimeSeqOutbox {
        async fn enqueue(
            &self,
            _channel_name: &str,
            event_type: &str,
            payload: Value,
        ) -> storage::Result<i64> {
            let mut rows = self.rows.lock().expect("rows lock");
            rows.push((event_type.to_string(), payload));
            Ok(rows.len() as i64)
        }

        async fn enqueue_runtime_event(
            &self,
            _channel_name: &str,
            event_type: &str,
            mut payload: Value,
        ) -> storage::Result<i64> {
            if let Some(session_id) = payload
                .get("session_id")
                .and_then(Value::as_str)
                .map(str::to_string)
            {
                let mut seqs = self.seqs.lock().expect("seqs lock");
                let seq = seqs.entry(session_id).or_insert(0);
                *seq += 1;
                if let Some(map) = payload.as_object_mut() {
                    map.insert("runtime_seq".to_string(), json!(*seq));
                }
            }
            let mut rows = self.rows.lock().expect("rows lock");
            rows.push((event_type.to_string(), payload));
            Ok(rows.len() as i64)
        }

        async fn claim_due(&self, _limit: u32) -> storage::Result<Vec<OutboxRow>> {
            Ok(Vec::new())
        }

        async fn pending_count(&self) -> storage::Result<i64> {
            Ok(0)
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

    /// Websocket-kind channel accepting every runtime event so the emitter
    /// routes durable events through `enqueue_runtime_event` (the runtime_seq
    /// path).
    struct RuntimeChannel;

    #[async_trait]
    impl OutboundChannel for RuntimeChannel {
        fn name(&self) -> &str {
            "runtime"
        }
        fn kind(&self) -> &'static str {
            "websocket"
        }
        fn accepts(&self, _event_type: &str) -> bool {
            true
        }
        async fn deliver(&self, _event: &OutboundEvent) -> outbound::Result<()> {
            Err(OutboundError::Delivery("unused".into()))
        }
    }

    #[tokio::test]
    async fn subagent_durable_events_are_scoped_to_parent_session() {
        let outbox = Arc::new(RuntimeSeqOutbox::default());
        let registry = OutboundRegistry::new().with_channel(Arc::new(RuntimeChannel));
        let emitter = OutboundEmitter::new(outbox.clone(), Arc::new(RwLock::new(registry)));
        let sink = RecordingSink::default();
        let child_session_id = SessionId::from("child-session");
        let metadata = StreamEventMetadata {
            trace_id: None,
            turn_id: Some("turn-1".to_string()),
            subagent: Some(SubagentStreamMetadata {
                job_id: "job-1".to_string(),
                agent_name: "helper".to_string(),
                parent_session_id: "parent-session".to_string(),
                child_session_id: "child-session".to_string(),
            }),
        };
        let stream = futures::stream::iter(vec![
            AgentEvent::ThinkingChunk {
                text: "let me think".to_string(),
            },
            AgentEvent::TokenChunk {
                text: "hel".to_string(),
            },
            AgentEvent::TokenChunk {
                text: "lo".to_string(),
            },
            AgentEvent::ToolCall {
                id: "call-1".to_string(),
                tool: "search".to_string(),
                args: json!({"q": "rust"}),
            },
            AgentEvent::ToolResult {
                id: "call-1".to_string(),
                result: json!({"ok": true}),
            },
        ])
        .boxed();

        consume_agent_stream(
            stream,
            Some("parent-stream".to_string()),
            &child_session_id,
            &emitter,
            "session",
            &sink,
            Some(&metadata),
        )
        .await;

        let rows = outbox.rows.lock().expect("rows lock").clone();
        let durable: Vec<&(String, Value)> = rows
            .iter()
            .filter(|(_, payload)| payload["durability"] == "durable")
            .collect();
        assert!(
            !durable.is_empty(),
            "expected durable subagent events to be emitted"
        );

        // Every durable subagent-detail event is persisted under the PARENT
        // session, stamped scope:"subagent" with the subagent block, and
        // carries a monotonic non-zero runtime_seq drawn from the parent
        // session sequence.
        for (event_type, payload) in &durable {
            assert_eq!(
                payload["session_id"], "parent-session",
                "durable {event_type} must be scoped to the parent session"
            );
            assert_eq!(payload["scope"], "subagent", "durable {event_type} scope");
            assert_eq!(payload["subagent"]["job_id"], "job-1");
            assert_eq!(payload["subagent"]["parent_session_id"], "parent-session");
            assert_eq!(payload["subagent"]["child_session_id"], "child-session");
            let runtime_seq = payload["runtime_seq"]
                .as_i64()
                .unwrap_or_else(|| panic!("durable {event_type} missing runtime_seq"));
            assert!(
                runtime_seq >= 1,
                "durable {event_type} runtime_seq must be non-zero, got {runtime_seq}"
            );
        }

        // Token deltas are coalesced into a single durable row, not one per
        // token (same as the main agent via DurableAgentStream).
        let token_rows: Vec<&&(String, Value)> = durable
            .iter()
            .filter(|(event_type, _)| event_type == "token")
            .collect();
        assert_eq!(
            token_rows.len(),
            1,
            "token deltas must coalesce into a single durable row"
        );
        assert_eq!(token_rows[0].1["text"], "hello");
        assert_eq!(token_rows[0].1["coalesced"], true);

        // The live preview copy stays scoped to the CHILD session (unchanged
        // path) so the in-runtime broker still shows subagent detail live.
        let preview_child = rows.iter().any(|(_, payload)| {
            payload["durability"] == "preview" && payload["session_id"] == "child-session"
        });
        assert!(
            preview_child,
            "live preview copy must remain scoped to the child session"
        );
    }
}

#[cfg(test)]
mod scheduled_run_tests {
    use super::{
        complete_scheduled_run, fail_malformed_scheduled_run, InboundEvent, ScheduledRunContext,
        ScheduledRunStatus,
    };
    use async_trait::async_trait;
    use chrono::{DateTime, Utc};
    use domain::SessionId;
    use outbound::{OutboundChannel, OutboundEmitter, OutboundError, OutboundRegistry};
    use serde_json::{json, Value};
    use std::sync::{Arc, Mutex};
    use storage::{OutboxRepo, OutboxRow, Result as StorageResult};
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
        async fn pending_count(&self) -> StorageResult<i64> {
            Ok(self.rows.lock().unwrap().len() as i64)
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

    fn emitter(outbox: Arc<RecordingOutbox>) -> Arc<OutboundEmitter> {
        let registry = OutboundRegistry::new().with_channel(Arc::new(ScheduleChannel));
        Arc::new(OutboundEmitter::new(
            outbox,
            Arc::new(RwLock::new(registry)),
        ))
    }

    fn scheduled_inbound(job_id: &str, is_one_shot: bool) -> InboundEvent {
        let now = Utc::now();
        InboundEvent {
            envelope_id: "cron-1".to_string(),
            session_id: SessionId::from("C123-cron-1"),
            user: "cron".to_string(),
            user_display_name: Some("Scheduler".to_string()),
            actor_user_id: None,
            text: "do work".to_string(),
            attachments: Vec::new(),
            session_context: Vec::new(),
            model_definition: None,
            raw: json!({
                "source": "cron",
                "job_kind": "cron",
                "job_id": job_id,
                "schedule_run_key": format!("{job_id}:run"),
                "schedule_scheduled_at": now,
                "schedule_started_at": now,
                "schedule_is_one_shot": is_one_shot,
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
        let ordinary = scheduled_inbound("job-1", false);
        assert!(ScheduledRunContext::from_inbound(&ordinary).is_some());

        let mut not_scheduled = ordinary.clone();
        not_scheduled.raw = json!({"source": "http"});
        assert!(ScheduledRunContext::from_inbound(&not_scheduled).is_none());
    }

    #[tokio::test]
    async fn completing_a_run_emits_completed() {
        let outbox = Arc::new(RecordingOutbox::default());
        let inbound = scheduled_inbound("schedule-1", false);
        let run = ScheduledRunContext::from_inbound(&inbound).expect("scheduled context");

        complete_scheduled_run(
            emitter(outbox.clone()),
            &run,
            &SessionId::from("C123-cron-1"),
            None,
        )
        .await;

        let rows = outbox.rows.lock().unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].0, "schedule.run_completed");
        assert_eq!(rows[0].1["job_id"], "schedule-1");
        assert_eq!(rows[0].1["run_key"], "schedule-1:run");
        assert_eq!(rows[0].1["source"], "cron");
        assert_eq!(rows[0].1["session_id"], "C123-cron-1");
    }

    #[tokio::test]
    async fn completing_a_failed_run_emits_run_failed() {
        let outbox = Arc::new(RecordingOutbox::default());
        let inbound = scheduled_inbound("recurring-1", false);
        let run = ScheduledRunContext::from_inbound(&inbound).expect("scheduled context");

        complete_scheduled_run(
            emitter(outbox.clone()),
            &run,
            &SessionId::from("C123-cron-1"),
            Some("model exploded"),
        )
        .await;

        let rows = outbox.rows.lock().unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].0, "schedule.run_failed");
        assert_eq!(rows[0].1["error"], "model exploded");
    }

    #[tokio::test]
    async fn malformed_scheduled_metadata_is_reported_without_storage_mutation() {
        let inbound = {
            let mut inbound = scheduled_inbound("cron-malformed", false);
            inbound.raw["schedule_scheduled_at"] = json!("not-a-date");
            inbound
        };
        let malformed = match ScheduledRunStatus::from_inbound(&inbound) {
            ScheduledRunStatus::Malformed(malformed) => malformed,
            ScheduledRunStatus::None | ScheduledRunStatus::Valid(_) => {
                panic!("expected malformed scheduled context")
            }
        };

        fail_malformed_scheduled_run(&malformed, &SessionId::from("C123-cron-1")).await;
    }
}
