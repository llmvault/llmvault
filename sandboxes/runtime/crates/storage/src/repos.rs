use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{
    AgentDefinition, EventKind, QuestionAnswerPayload, QuestionRequest, Session, SessionEvent,
    SessionId, SessionStatus, SubagentTask, SubagentTaskState, WorkspaceConfig,
};
use std::collections::HashMap;

#[derive(Debug, thiserror::Error)]
pub enum StorageError {
    #[error("not found")]
    NotFound,
    #[error("conflict")]
    Conflict,
    #[error(transparent)]
    Sqlx(#[from] sqlx::Error),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

pub type Result<T> = std::result::Result<T, StorageError>;

#[derive(Debug, Clone)]
pub struct ConfigSnapshot {
    pub definition: AgentDefinition,
    pub runtime_env: HashMap<String, String>,
    pub workspace: WorkspaceConfig,
}

#[async_trait]
pub trait ConfigRepo: Send + Sync + 'static {
    async fn load(&self) -> Result<Option<ConfigSnapshot>>;
    async fn upsert(&self, snapshot: &ConfigSnapshot) -> Result<()>;
}

#[derive(Debug, Clone)]
pub struct SessionListCursor {
    pub last_activity_at: DateTime<Utc>,
    pub id: Option<String>,
}

#[derive(Debug, Clone, Default)]
pub struct SessionListFilter {
    pub cursor: Option<SessionListCursor>,
    pub status: Option<SessionStatus>,
    pub session_id: Option<String>,
    pub search: Option<String>,
}

#[derive(Debug, Clone)]
pub struct SessionSearchResult {
    pub session_id: String,
    pub event_id: String,
    pub kind: String,
    pub content: String,
    pub snippet: String,
    pub created_at: DateTime<Utc>,
    pub score: f64,
}

#[async_trait]
pub trait SessionRepo: Send + Sync + 'static {
    async fn get(&self, id: &SessionId) -> Result<Option<Session>>;
    async fn create(&self, session: &Session) -> Result<()>;
    async fn touch(&self, id: &SessionId, at: DateTime<Utc>) -> Result<()>;
    async fn set_status(&self, id: &SessionId, status: SessionStatus) -> Result<()>;
    async fn list(&self, filter: SessionListFilter, limit: u32) -> Result<Vec<Session>>;
}

#[async_trait]
pub trait EventRepo: Send + Sync + 'static {
    async fn append(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
    ) -> Result<i64>;
    async fn append_idempotent(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
        idempotency_key: &str,
    ) -> Result<Option<i64>>;
    async fn list_recent(&self, session_id: &SessionId, limit: u32) -> Result<Vec<SessionEvent>>;
    async fn list_chronological(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> Result<Vec<SessionEvent>>;
    async fn search_sessions(
        &self,
        query: &str,
        session_id: Option<&SessionId>,
        limit: u32,
    ) -> Result<Vec<SessionSearchResult>>;
}

#[derive(Debug, Clone)]
pub struct OutboxRow {
    pub id: i64,
    pub channel_name: String,
    pub event_type: String,
    pub payload: serde_json::Value,
    pub attempts: i32,
    pub session_id: Option<String>,
    pub runtime_seq: Option<i64>,
    pub occurred_at: DateTime<Utc>,
}

#[async_trait]
pub trait OutboxRepo: Send + Sync + 'static {
    async fn enqueue(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: serde_json::Value,
    ) -> Result<i64>;
    async fn enqueue_runtime_event(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: serde_json::Value,
    ) -> Result<i64> {
        self.enqueue(channel_name, event_type, payload).await
    }
    async fn claim_due(&self, limit: u32) -> Result<Vec<OutboxRow>>;
    async fn pending_count(&self) -> Result<i64>;
    async fn mark_delivered(&self, id: i64) -> Result<()>;
    async fn schedule_retry(
        &self,
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
    ) -> Result<()>;
    async fn mark_failed(&self, id: i64) -> Result<()>;
}

#[async_trait]
pub trait InboundDedupeRepo: Send + Sync + 'static {
    async fn check_and_record(&self, envelope_id: &str) -> Result<bool>;
    async fn cleanup_older_than(&self, before: DateTime<Utc>) -> Result<u64>;
}

#[async_trait]
pub trait SubagentTaskRepo: Send + Sync + 'static {
    async fn create(&self, task: &SubagentTask) -> Result<()>;
    async fn get(&self, id: &str) -> Result<Option<SubagentTask>>;
    async fn list_queued(&self, limit: u32) -> Result<Vec<SubagentTask>>;
    async fn list_active_by_parent(
        &self,
        parent_session_id: &SessionId,
    ) -> Result<Vec<SubagentTask>>;
    async fn mark_running(&self, id: &str, started_at: DateTime<Utc>) -> Result<bool>;
    async fn complete(
        &self,
        id: &str,
        state: SubagentTaskState,
        completed_at: DateTime<Utc>,
        result: &str,
        error: Option<&str>,
    ) -> Result<()>;
    async fn fail_active_for_parent(
        &self,
        parent_session_id: &SessionId,
        completed_at: DateTime<Utc>,
        error: &str,
    ) -> Result<()>;
}

#[async_trait]
pub trait QuestionRequestRepo: Send + Sync + 'static {
    async fn create(&self, request: &QuestionRequest) -> Result<()>;
    async fn get(&self, id: &str) -> Result<Option<QuestionRequest>>;
    async fn answer(
        &self,
        id: &str,
        answer: &QuestionAnswerPayload,
        answered_at: DateTime<Utc>,
    ) -> Result<bool>;
}
