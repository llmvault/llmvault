use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use chrono::{DateTime, Utc};
use domain::{
    EventKind, QuestionAnswerPayload, QuestionRequest, Session, SessionId, SessionStatus,
    SubagentTask, SubagentTaskState,
};
use serde_json::Value;
use sqlx::SqliteConnection;
use tokio::sync::{mpsc, oneshot};

use crate::repos::{OutboxRow, Result};

use super::event_ops;
use super::outbox_ops;
use super::question_ops;
use super::session_ops;
use super::subagent_ops;
use super::EventsLogWrite;

pub(super) const WRITE_QUEUE_CAPACITY: usize = 50_000;

pub(super) type Resp<T> = oneshot::Sender<Result<T>>;

pub(super) enum WriteRequest {
    ConfigUpsert {
        definition_json: String,
        runtime_env_json: String,
        workspace_json: String,
        updated_at: String,
        resp: Resp<()>,
    },
    SessionCreate {
        session: Box<Session>,
        resp: Resp<()>,
    },
    SessionTouch {
        id: SessionId,
        at: DateTime<Utc>,
        resp: Resp<()>,
    },
    SessionSetStatus {
        id: SessionId,
        status: SessionStatus,
        resp: Resp<()>,
    },
    EventAppend {
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
        resp: Resp<i64>,
    },
    EventAppendIdempotent {
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
        idempotency_key: String,
        resp: Resp<Option<i64>>,
    },
    InboundDedupeCheckAndRecord {
        envelope_id: String,
        received_at: String,
        resp: Resp<bool>,
    },
    InboundDedupeCleanup {
        before: String,
        resp: Resp<u64>,
    },
    SubagentTaskCreate {
        task: Box<SubagentTask>,
        resp: Resp<()>,
    },
    SubagentTaskMarkRunning {
        id: String,
        started_at: DateTime<Utc>,
        resp: Resp<bool>,
    },
    SubagentTaskComplete {
        id: String,
        state: SubagentTaskState,
        completed_at: DateTime<Utc>,
        result: String,
        error: Option<String>,
        resp: Resp<()>,
    },
    SubagentTaskFailActiveForParent {
        parent_session_id: SessionId,
        completed_at: DateTime<Utc>,
        error: String,
        resp: Resp<()>,
    },
    QuestionRequestCreate {
        request: Box<QuestionRequest>,
        resp: Resp<()>,
    },
    QuestionRequestAnswer {
        id: String,
        answer: Box<QuestionAnswerPayload>,
        answered_at: DateTime<Utc>,
        resp: Resp<bool>,
    },
    OutboxEnqueue {
        channel_name: String,
        event_type: String,
        payload_json: String,
        now: String,
        resp: Resp<i64>,
    },
    OutboxEnqueueRuntime {
        channel_name: String,
        event_type: String,
        payload_json: String,
        now: String,
        resp: Resp<i64>,
    },
    OutboxClaimDue {
        limit: u32,
        now: String,
        lease_until: String,
        resp: Resp<Vec<OutboxRow>>,
    },
    OutboxMarkDelivered {
        id: i64,
        resp: Resp<()>,
    },
    OutboxScheduleRetry {
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
        resp: Resp<()>,
    },
    OutboxMarkFailed {
        id: i64,
        resp: Resp<()>,
    },
    EventsLogBatch {
        events: Vec<EventsLogWrite>,
        recorded_at: String,
        resp: Resp<()>,
    },
    Flush {
        resp: Resp<()>,
    },
}

pub(super) async fn run_writer(
    mut rx: mpsc::Receiver<WriteRequest>,
    mut conn: SqliteConnection,
    queued: Arc<AtomicUsize>,
) {
    while let Some(request) = rx.recv().await {
        queued.fetch_sub(1, Ordering::Relaxed);
        request.execute(&mut conn).await;
    }
}

impl WriteRequest {
    async fn execute(self, conn: &mut SqliteConnection) {
        match self {
            WriteRequest::ConfigUpsert {
                definition_json,
                runtime_env_json,
                workspace_json,
                updated_at,
                resp,
            } => respond(
                resp,
                session_ops::config_upsert(
                    conn,
                    definition_json,
                    runtime_env_json,
                    workspace_json,
                    updated_at,
                )
                .await,
            ),
            WriteRequest::SessionCreate { session, resp } => {
                respond(resp, session_ops::session_create(conn, *session).await)
            }
            WriteRequest::SessionTouch { id, at, resp } => {
                respond(resp, session_ops::session_touch(conn, &id, at).await)
            }
            WriteRequest::SessionSetStatus { id, status, resp } => respond(
                resp,
                session_ops::session_set_status(conn, &id, status).await,
            ),
            WriteRequest::EventAppend {
                session_id,
                kind,
                payload,
                resp,
            } => respond(
                resp,
                event_ops::event_append(conn, &session_id, kind, payload).await,
            ),
            WriteRequest::EventAppendIdempotent {
                session_id,
                kind,
                payload,
                idempotency_key,
                resp,
            } => respond(
                resp,
                event_ops::event_append_idempotent(
                    conn,
                    &session_id,
                    kind,
                    payload,
                    &idempotency_key,
                )
                .await,
            ),
            WriteRequest::InboundDedupeCheckAndRecord {
                envelope_id,
                received_at,
                resp,
            } => respond(
                resp,
                session_ops::inbound_dedupe_check_and_record(conn, &envelope_id, &received_at)
                    .await,
            ),
            WriteRequest::InboundDedupeCleanup { before, resp } => respond(
                resp,
                session_ops::inbound_dedupe_cleanup(conn, &before).await,
            ),
            WriteRequest::SubagentTaskCreate { task, resp } => {
                respond(resp, subagent_ops::subagent_task_create(conn, *task).await)
            }
            WriteRequest::SubagentTaskMarkRunning {
                id,
                started_at,
                resp,
            } => respond(
                resp,
                subagent_ops::subagent_task_mark_running(conn, &id, started_at).await,
            ),
            WriteRequest::SubagentTaskComplete {
                id,
                state,
                completed_at,
                result,
                error,
                resp,
            } => respond(
                resp,
                subagent_ops::subagent_task_complete(
                    conn,
                    &id,
                    state,
                    completed_at,
                    &result,
                    error.as_deref(),
                )
                .await,
            ),
            WriteRequest::SubagentTaskFailActiveForParent {
                parent_session_id,
                completed_at,
                error,
                resp,
            } => respond(
                resp,
                subagent_ops::subagent_task_fail_active_for_parent(
                    conn,
                    &parent_session_id,
                    completed_at,
                    &error,
                )
                .await,
            ),
            WriteRequest::QuestionRequestCreate { request, resp } => respond(
                resp,
                question_ops::question_request_create(conn, *request).await,
            ),
            WriteRequest::QuestionRequestAnswer {
                id,
                answer,
                answered_at,
                resp,
            } => respond(
                resp,
                question_ops::question_request_answer(conn, &id, *answer, answered_at).await,
            ),
            WriteRequest::OutboxEnqueue {
                channel_name,
                event_type,
                payload_json,
                now,
                resp,
            } => respond(
                resp,
                outbox_ops::outbox_enqueue(conn, &channel_name, &event_type, &payload_json, &now)
                    .await,
            ),
            WriteRequest::OutboxEnqueueRuntime {
                channel_name,
                event_type,
                payload_json,
                now,
                resp,
            } => respond(
                resp,
                outbox_ops::outbox_enqueue_runtime(
                    conn,
                    &channel_name,
                    &event_type,
                    &payload_json,
                    &now,
                )
                .await,
            ),
            WriteRequest::OutboxClaimDue {
                limit,
                now,
                lease_until,
                resp,
            } => respond(
                resp,
                outbox_ops::outbox_claim_due(conn, limit, &now, &lease_until).await,
            ),
            WriteRequest::OutboxMarkDelivered { id, resp } => respond(
                resp,
                outbox_ops::outbox_mark_status(conn, id, "delivered").await,
            ),
            WriteRequest::OutboxScheduleRetry {
                id,
                attempts,
                next_retry_at,
                resp,
            } => respond(
                resp,
                outbox_ops::outbox_schedule_retry(conn, id, attempts, next_retry_at).await,
            ),
            WriteRequest::OutboxMarkFailed { id, resp } => respond(
                resp,
                outbox_ops::outbox_mark_status(conn, id, "failed").await,
            ),
            WriteRequest::EventsLogBatch {
                events,
                recorded_at,
                resp,
            } => respond(
                resp,
                outbox_ops::events_log_batch(conn, &events, &recorded_at).await,
            ),
            WriteRequest::Flush { resp } => respond(resp, Ok(())),
        }
    }
}

fn respond<T>(resp: Resp<T>, result: Result<T>) {
    let _ = resp.send(result);
}
