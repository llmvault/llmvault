use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{SessionId, SubagentTask, SubagentTaskState};
use sqlx::{Row, SqlitePool};

use crate::repos::{Result, StorageError, SubagentTaskRepo};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteSubagentTaskRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteSubagentTaskRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

#[async_trait]
impl SubagentTaskRepo for SqliteSubagentTaskRepo {
    async fn create(&self, task: &SubagentTask) -> Result<()> {
        self.writer.create_subagent_task(task.clone()).await
    }

    async fn get(&self, id: &str) -> Result<Option<SubagentTask>> {
        let row = sqlx::query("SELECT * FROM subagent_tasks WHERE id = ?")
            .bind(id)
            .fetch_optional(self.pool.as_ref())
            .await?;
        row.map(|row| row_to_task(&row)).transpose()
    }

    async fn list_queued(&self, limit: u32) -> Result<Vec<SubagentTask>> {
        let rows = sqlx::query(
            "SELECT * FROM subagent_tasks \
             WHERE state = 'queued' \
             ORDER BY created_at ASC \
             LIMIT ?",
        )
        .bind(limit.min(100) as i64)
        .fetch_all(self.pool.as_ref())
        .await?;
        rows.iter().map(row_to_task).collect()
    }

    async fn list_active_by_parent(
        &self,
        parent_session_id: &SessionId,
    ) -> Result<Vec<SubagentTask>> {
        let rows = sqlx::query(
            "SELECT * FROM subagent_tasks \
             WHERE parent_session_id = ? AND state IN ('queued', 'running') \
             ORDER BY created_at ASC",
        )
        .bind(parent_session_id.as_str())
        .fetch_all(self.pool.as_ref())
        .await?;
        rows.iter().map(row_to_task).collect()
    }

    async fn mark_running(&self, id: &str, started_at: DateTime<Utc>) -> Result<bool> {
        self.writer
            .mark_subagent_task_running(id.to_string(), started_at)
            .await
    }

    async fn complete(
        &self,
        id: &str,
        state: SubagentTaskState,
        completed_at: DateTime<Utc>,
        result: &str,
        error: Option<&str>,
    ) -> Result<()> {
        self.writer
            .complete_subagent_task(
                id.to_string(),
                state,
                completed_at,
                result.to_string(),
                error.map(ToString::to_string),
            )
            .await
    }

    async fn fail_active_for_parent(
        &self,
        parent_session_id: &SessionId,
        completed_at: DateTime<Utc>,
        error: &str,
    ) -> Result<()> {
        self.writer
            .fail_active_subagent_tasks_for_parent(
                parent_session_id.clone(),
                completed_at,
                error.to_string(),
            )
            .await
    }
}

fn row_to_task(row: &sqlx::sqlite::SqliteRow) -> Result<SubagentTask> {
    let state: String = row.try_get("state")?;
    Ok(SubagentTask {
        id: row.try_get("id")?,
        parent_session_id: SessionId::from(row.try_get::<String, _>("parent_session_id")?),
        child_session_id: SessionId::from(row.try_get::<String, _>("child_session_id")?),
        agent_name: row.try_get("agent_name")?,
        goal: row.try_get("goal")?,
        stream_id: row.try_get("stream_id")?,
        state: state_from_str(&state)?,
        result: row.try_get("result")?,
        error: row.try_get("error")?,
        created_at: parse_ts(&row.try_get::<String, _>("created_at")?)?,
        started_at: parse_opt_ts(row.try_get("started_at")?)?,
        completed_at: parse_opt_ts(row.try_get("completed_at")?)?,
        updated_at: parse_ts(&row.try_get::<String, _>("updated_at")?)?,
    })
}

fn state_from_str(value: &str) -> Result<SubagentTaskState> {
    match value {
        "queued" => Ok(SubagentTaskState::Queued),
        "running" => Ok(SubagentTaskState::Running),
        "completed" => Ok(SubagentTaskState::Completed),
        "failed" => Ok(SubagentTaskState::Failed),
        other => Err(StorageError::Other(anyhow::anyhow!(
            "unknown subagent task state: {other}"
        ))),
    }
}

fn parse_ts(raw: &str) -> Result<DateTime<Utc>> {
    DateTime::parse_from_rfc3339(raw)
        .map(|dt| dt.with_timezone(&Utc))
        .map_err(|e| StorageError::Other(anyhow::anyhow!("parse timestamp `{raw}`: {e}")))
}

fn parse_opt_ts(raw: Option<String>) -> Result<Option<DateTime<Utc>>> {
    raw.as_deref().map(parse_ts).transpose()
}
