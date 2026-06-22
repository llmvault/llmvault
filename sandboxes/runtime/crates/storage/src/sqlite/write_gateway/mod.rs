mod event_ops;
mod outbox_ops;
mod question_ops;
mod request;
mod session_ops;
mod subagent_ops;

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use chrono::{DateTime, Utc};
use domain::{
    EventKind, QuestionAnswerPayload, QuestionRequest, Session, SessionId, SessionStatus,
    SubagentTask, SubagentTaskState,
};
use serde_json::Value;
use sqlx::sqlite::SqliteConnectOptions;
use sqlx::{Connection, SqliteConnection};
use tokio::sync::{mpsc, oneshot};

use crate::repos::{ConfigSnapshot, OutboxRow, Result, StorageError};

use request::{run_writer, WriteRequest, WRITE_QUEUE_CAPACITY};

#[derive(Clone)]
pub struct SqliteWriteGateway {
    tx: mpsc::Sender<WriteRequest>,
    queued: Arc<AtomicUsize>,
}

#[derive(Clone)]
pub struct EventsLogWrite {
    pub event_type: String,
    pub payload_json: String,
    pub occurred_at: String,
}

impl SqliteWriteGateway {
    pub async fn spawn(options: SqliteConnectOptions) -> Result<Arc<Self>> {
        let mut conn = SqliteConnection::connect_with(&options)
            .await
            .map_err(StorageError::from)?;
        session_ops::configure_write_connection(&mut conn).await?;
        let (tx, rx) = mpsc::channel(WRITE_QUEUE_CAPACITY);
        let gateway = Arc::new(Self {
            tx,
            queued: Arc::new(AtomicUsize::new(0)),
        });
        tokio::spawn(run_writer(rx, conn, gateway.queued.clone()));
        Ok(gateway)
    }

    pub fn queued_writes(&self) -> usize {
        self.queued.load(Ordering::Relaxed)
    }

    pub async fn upsert_config(&self, snapshot: &ConfigSnapshot) -> Result<()> {
        let definition_json = serde_json::to_string(&snapshot.definition)?;
        let runtime_env_json = serde_json::to_string(&snapshot.runtime_env)?;
        let workspace_json = serde_json::to_string(&snapshot.workspace)?;
        let updated_at = Utc::now().to_rfc3339();
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::ConfigUpsert {
            definition_json,
            runtime_env_json,
            workspace_json,
            updated_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn create_session(&self, session: Session) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SessionCreate {
            session: Box::new(session),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn touch_session(&self, id: SessionId, at: DateTime<Utc>) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SessionTouch { id, at, resp })
            .await?;
        recv(rx).await
    }

    pub async fn set_session_status(&self, id: SessionId, status: SessionStatus) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SessionSetStatus { id, status, resp })
            .await?;
        recv(rx).await
    }

    pub async fn append_event(
        &self,
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
    ) -> Result<i64> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::EventAppend {
            session_id,
            kind,
            payload,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn append_event_idempotent(
        &self,
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
        idempotency_key: String,
    ) -> Result<Option<i64>> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::EventAppendIdempotent {
            session_id,
            kind,
            payload,
            idempotency_key,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn check_and_record_inbound(
        &self,
        envelope_id: String,
        received_at: String,
    ) -> Result<bool> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::InboundDedupeCheckAndRecord {
            envelope_id,
            received_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn cleanup_inbound_before(&self, before: String) -> Result<u64> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::InboundDedupeCleanup { before, resp })
            .await?;
        recv(rx).await
    }

    pub async fn create_subagent_task(&self, task: SubagentTask) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SubagentTaskCreate {
            task: Box::new(task),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn mark_subagent_task_running(
        &self,
        id: String,
        started_at: DateTime<Utc>,
    ) -> Result<bool> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SubagentTaskMarkRunning {
            id,
            started_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn complete_subagent_task(
        &self,
        id: String,
        state: SubagentTaskState,
        completed_at: DateTime<Utc>,
        result: String,
        error: Option<String>,
    ) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SubagentTaskComplete {
            id,
            state,
            completed_at,
            result,
            error,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn fail_active_subagent_tasks_for_parent(
        &self,
        parent_session_id: SessionId,
        completed_at: DateTime<Utc>,
        error: String,
    ) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SubagentTaskFailActiveForParent {
            parent_session_id,
            completed_at,
            error,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn create_question_request(&self, request: QuestionRequest) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::QuestionRequestCreate {
            request: Box::new(request),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn answer_question_request(
        &self,
        id: String,
        answer: QuestionAnswerPayload,
        answered_at: DateTime<Utc>,
    ) -> Result<bool> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::QuestionRequestAnswer {
            id,
            answer: Box::new(answer),
            answered_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn enqueue_outbox(
        &self,
        channel_name: String,
        event_type: String,
        payload_json: String,
    ) -> Result<i64> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxEnqueue {
            channel_name,
            event_type,
            payload_json,
            now: Utc::now().to_rfc3339(),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn enqueue_runtime_outbox(
        &self,
        channel_name: String,
        event_type: String,
        payload_json: String,
    ) -> Result<i64> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxEnqueueRuntime {
            channel_name,
            event_type,
            payload_json,
            now: Utc::now().to_rfc3339(),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn claim_due_outbox(
        &self,
        limit: u32,
        lease_until: DateTime<Utc>,
    ) -> Result<Vec<OutboxRow>> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxClaimDue {
            limit,
            now: Utc::now().to_rfc3339(),
            lease_until: lease_until.to_rfc3339(),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn mark_outbox_delivered(&self, id: i64) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxMarkDelivered { id, resp })
            .await?;
        recv(rx).await
    }

    pub async fn schedule_outbox_retry(
        &self,
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
    ) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxScheduleRetry {
            id,
            attempts,
            next_retry_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn mark_outbox_failed(&self, id: i64) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxMarkFailed { id, resp })
            .await?;
        recv(rx).await
    }

    pub async fn append_events_log_batch(&self, events: Vec<EventsLogWrite>) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::EventsLogBatch {
            events,
            recorded_at: Utc::now().to_rfc3339(),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn flush(&self) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::Flush { resp }).await?;
        recv(rx).await
    }

    async fn send(&self, request: WriteRequest) -> Result<()> {
        self.queued.fetch_add(1, Ordering::Relaxed);
        if self.tx.send(request).await.is_err() {
            self.queued.fetch_sub(1, Ordering::Relaxed);
            return Err(StorageError::Other(anyhow::anyhow!(
                "sqlite write gateway is closed"
            )));
        }
        Ok(())
    }
}

async fn recv<T>(rx: oneshot::Receiver<Result<T>>) -> Result<T> {
    rx.await.map_err(|_| {
        StorageError::Other(anyhow::anyhow!(
            "sqlite write gateway dropped response before completing write"
        ))
    })?
}
