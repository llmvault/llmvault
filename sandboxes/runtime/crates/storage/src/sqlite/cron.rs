use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::cron::{CronJob, CronJobState};
use sqlx::SqlitePool;
use std::sync::Arc;

use crate::repos::{CronJobRepo, Result, StorageError};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteCronJobRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteCronJobRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

const SELECT_COLS: &str = "id, description, channel, task_prompt, cron_expression, \
    interval_seconds, repeat_count, repeat_completed, state, next_run_at, last_run_at, \
    last_status, last_error, session_continuation_id, created_at, created_by_session";

#[async_trait]
impl CronJobRepo for SqliteCronJobRepo {
    async fn create(&self, job: &CronJob) -> Result<()> {
        self.writer.create_cron(job.clone()).await
    }

    async fn get(&self, id: &str) -> Result<Option<CronJob>> {
        let row: Option<CronJobRow> =
            sqlx::query_as(&format!("SELECT {SELECT_COLS} FROM cron_jobs WHERE id = ?"))
                .bind(id)
                .fetch_optional(self.pool.as_ref())
                .await
                .map_err(StorageError::from)?;
        Ok(row.map(|r| r.into()))
    }

    async fn list_all(&self) -> Result<Vec<CronJob>> {
        let rows: Vec<CronJobRow> = sqlx::query_as(&format!(
            "SELECT {SELECT_COLS} FROM cron_jobs ORDER BY created_at DESC"
        ))
        .fetch_all(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(rows.into_iter().map(|r| r.into()).collect())
    }

    async fn list_due(&self) -> Result<Vec<CronJob>> {
        let now = Utc::now().to_rfc3339();
        let rows: Vec<CronJobRow> = sqlx::query_as(&format!(
            "SELECT {SELECT_COLS} FROM cron_jobs
             WHERE state = 'active'
               AND next_run_at <= ?
             ORDER BY next_run_at ASC
             LIMIT 25"
        ))
        .bind(&now)
        .fetch_all(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(rows.into_iter().map(|r| r.into()).collect())
    }

    async fn update_prompt(&self, id: &str, task_prompt: String) -> Result<()> {
        self.writer
            .update_cron_prompt(id.to_string(), task_prompt)
            .await
    }

    async fn update_interval(&self, id: &str, interval_seconds: u64) -> Result<()> {
        self.writer
            .update_cron_interval(id.to_string(), interval_seconds)
            .await
    }

    async fn update_next_run(&self, id: &str, next_run_at: DateTime<Utc>) -> Result<()> {
        self.writer
            .update_cron_next_run(id.to_string(), next_run_at)
            .await
    }

    async fn set_state(&self, id: &str, state: CronJobState) -> Result<()> {
        self.writer.set_cron_state(id.to_string(), state).await
    }

    async fn claim_due_run(
        &self,
        id: &str,
        now: DateTime<Utc>,
        started_at: DateTime<Utc>,
    ) -> Result<bool> {
        self.writer
            .claim_due_cron_run(id.to_string(), now, started_at)
            .await
    }

    async fn reset_stale_running(&self, before: DateTime<Utc>) -> Result<u64> {
        self.writer.reset_stale_cron_running(before).await
    }

    async fn record_run(
        &self,
        id: &str,
        run_at: DateTime<Utc>,
        status: &str,
        error: Option<&str>,
    ) -> Result<()> {
        self.writer
            .record_cron_run(
                id.to_string(),
                run_at,
                status.to_string(),
                error.map(ToString::to_string),
            )
            .await
    }

    async fn increment_repeat(&self, id: &str) -> Result<()> {
        self.writer.increment_cron_repeat(id.to_string()).await
    }

    async fn delete(&self, id: &str) -> Result<()> {
        self.writer.delete_cron(id.to_string()).await
    }
}

fn parse_state(s: &str) -> CronJobState {
    match s {
        "active" => CronJobState::Active,
        "running" => CronJobState::Running,
        "paused" => CronJobState::Paused,
        "completed" => CronJobState::Completed,
        _ => CronJobState::Active,
    }
}

fn parse_opt_dt(s: &Option<String>) -> Option<DateTime<Utc>> {
    s.as_deref().and_then(|v| {
        DateTime::parse_from_rfc3339(v)
            .ok()
            .map(|d| d.with_timezone(&Utc))
    })
}

fn parse_dt(s: &str) -> DateTime<Utc> {
    DateTime::parse_from_rfc3339(s)
        .unwrap_or_default()
        .with_timezone(&Utc)
}

#[derive(Debug, sqlx::FromRow)]
struct CronJobRow {
    id: String,
    description: String,
    channel: String,
    task_prompt: String,
    cron_expression: Option<String>,
    interval_seconds: Option<i64>,
    repeat_count: Option<i32>,
    repeat_completed: i32,
    state: String,
    next_run_at: String,
    last_run_at: Option<String>,
    last_status: Option<String>,
    last_error: Option<String>,
    session_continuation_id: Option<String>,
    created_at: String,
    created_by_session: String,
}

impl From<CronJobRow> for CronJob {
    fn from(r: CronJobRow) -> Self {
        Self {
            id: r.id,
            description: r.description,
            channel: r.channel,
            task_prompt: r.task_prompt,
            cron_expression: r.cron_expression,
            interval_seconds: r.interval_seconds.map(|v| v as u64),
            repeat_count: r.repeat_count.map(|v| v as u32),
            repeat_completed: r.repeat_completed as u32,
            state: parse_state(&r.state),
            next_run_at: parse_dt(&r.next_run_at),
            last_run_at: parse_opt_dt(&r.last_run_at),
            last_status: r.last_status,
            last_error: r.last_error,
            session_continuation_id: r.session_continuation_id,
            created_at: parse_dt(&r.created_at),
            created_by_session: r.created_by_session,
        }
    }
}
