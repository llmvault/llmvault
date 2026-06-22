use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Duration as ChronoDuration, Utc};
use sqlx::SqlitePool;

use crate::repos::{OutboxRepo, OutboxRow, Result};

use super::{SqliteStore, SqliteWriteGateway};

const STATUS_PENDING: &str = "pending";
const OUTBOX_LEASE_SECONDS: i64 = 120;

pub struct SqliteOutboxRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteOutboxRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

#[async_trait]
impl OutboxRepo for SqliteOutboxRepo {
    async fn enqueue(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: serde_json::Value,
    ) -> Result<i64> {
        let payload_json = serde_json::to_string(&payload)?;
        self.writer
            .enqueue_outbox(
                channel_name.to_string(),
                event_type.to_string(),
                payload_json,
            )
            .await
    }

    async fn enqueue_runtime_event(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: serde_json::Value,
    ) -> Result<i64> {
        let payload_json = serde_json::to_string(&payload)?;
        self.writer
            .enqueue_runtime_outbox(
                channel_name.to_string(),
                event_type.to_string(),
                payload_json,
            )
            .await
    }

    async fn claim_due(&self, limit: u32) -> Result<Vec<OutboxRow>> {
        let lease_until = Utc::now() + ChronoDuration::seconds(OUTBOX_LEASE_SECONDS);
        self.writer.claim_due_outbox(limit, lease_until).await
    }

    async fn pending_count(&self) -> Result<i64> {
        let count = sqlx::query_scalar("SELECT COUNT(*) FROM outbound_outbox WHERE status = ?")
            .bind(STATUS_PENDING)
            .fetch_one(self.pool.as_ref())
            .await?;
        Ok(count)
    }

    async fn mark_delivered(&self, id: i64) -> Result<()> {
        self.writer.mark_outbox_delivered(id).await
    }

    async fn schedule_retry(
        &self,
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
    ) -> Result<()> {
        self.writer
            .schedule_outbox_retry(id, attempts, next_retry_at)
            .await
    }

    async fn mark_failed(&self, id: i64) -> Result<()> {
        self.writer.mark_outbox_failed(id).await
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;

    use serde_json::json;

    use super::*;
    use crate::init_sqlite_store;

    async fn setup_repo() -> (tempfile::TempDir, SqliteOutboxRepo) {
        let dir = tempfile::tempdir().expect("tempdir");
        let store = init_sqlite_store(dir.path().join("outbox.db"))
            .await
            .expect("init sqlite store");
        let repo = SqliteOutboxRepo::new(&store);
        (dir, repo)
    }

    fn rows_by_session(rows: &[OutboxRow]) -> HashMap<String, i64> {
        rows.iter()
            .filter_map(|row| Some((row.session_id.clone()?, row.runtime_seq?)))
            .collect()
    }

    #[tokio::test]
    async fn claim_due_returns_one_head_row_per_runtime_session() {
        let (_dir, repo) = setup_repo().await;

        repo.enqueue_runtime_event("runtime-ws", "token", json!({"session_id": "session-a"}))
            .await
            .expect("enqueue a1");
        repo.enqueue_runtime_event("runtime-ws", "token", json!({"session_id": "session-a"}))
            .await
            .expect("enqueue a2");
        repo.enqueue_runtime_event("runtime-ws", "token", json!({"session_id": "session-b"}))
            .await
            .expect("enqueue b1");

        let rows = repo.claim_due(10).await.expect("claim due");
        let by_session = rows_by_session(&rows);
        assert_eq!(by_session.len(), 2);
        assert_eq!(by_session.get("session-a"), Some(&1));
        assert_eq!(by_session.get("session-b"), Some(&1));
        assert!(!rows.iter().any(
            |row| row.session_id.as_deref() == Some("session-a") && row.runtime_seq == Some(2)
        ));
    }

    #[tokio::test]
    async fn leased_session_head_does_not_block_other_session_heads() {
        let (_dir, repo) = setup_repo().await;

        repo.enqueue_runtime_event("runtime-ws", "token", json!({"session_id": "stuck"}))
            .await
            .expect("enqueue stuck 1");
        repo.enqueue_runtime_event("runtime-ws", "token", json!({"session_id": "stuck"}))
            .await
            .expect("enqueue stuck 2");
        repo.enqueue_runtime_event("runtime-ws", "token", json!({"session_id": "ready"}))
            .await
            .expect("enqueue ready 1");

        let first = repo.claim_due(1).await.expect("claim first");
        assert_eq!(first.len(), 1);
        assert_eq!(first[0].session_id.as_deref(), Some("stuck"));

        let second = repo.claim_due(10).await.expect("claim second");
        assert_eq!(second.len(), 1);
        assert_eq!(second[0].session_id.as_deref(), Some("ready"));
        assert_eq!(second[0].runtime_seq, Some(1));
    }
}
