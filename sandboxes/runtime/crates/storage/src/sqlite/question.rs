use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{
    QuestionAnswerPayload, QuestionRequest, QuestionRequestState, RequestUserInputPayload,
    SessionId,
};
use sqlx::{Row, SqlitePool};

use crate::repos::{QuestionRequestRepo, Result, StorageError};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteQuestionRequestRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteQuestionRequestRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

#[async_trait]
impl QuestionRequestRepo for SqliteQuestionRequestRepo {
    async fn create(&self, request: &QuestionRequest) -> Result<()> {
        self.writer.create_question_request(request.clone()).await
    }

    async fn get(&self, id: &str) -> Result<Option<QuestionRequest>> {
        let row = sqlx::query("SELECT * FROM question_requests WHERE id = ?")
            .bind(id)
            .fetch_optional(self.pool.as_ref())
            .await?;
        row.map(|row| row_to_question_request(&row)).transpose()
    }

    async fn answer(
        &self,
        id: &str,
        answer: &QuestionAnswerPayload,
        answered_at: DateTime<Utc>,
    ) -> Result<bool> {
        self.writer
            .answer_question_request(id.to_string(), answer.clone(), answered_at)
            .await
    }
}

fn row_to_question_request(row: &sqlx::sqlite::SqliteRow) -> Result<QuestionRequest> {
    let state: String = row.try_get("state")?;
    let request_json: String = row.try_get("request_json")?;
    let answer_json: Option<String> = row.try_get("answer_json")?;
    Ok(QuestionRequest {
        id: row.try_get("id")?,
        session_id: SessionId::from(row.try_get::<String, _>("session_id")?),
        request: serde_json::from_str::<RequestUserInputPayload>(&request_json)?,
        answer: answer_json
            .as_deref()
            .map(serde_json::from_str::<QuestionAnswerPayload>)
            .transpose()?,
        state: state_from_str(&state)?,
        created_at: parse_ts(&row.try_get::<String, _>("created_at")?)?,
        answered_at: parse_opt_ts(row.try_get("answered_at")?)?,
        updated_at: parse_ts(&row.try_get::<String, _>("updated_at")?)?,
    })
}

fn state_from_str(value: &str) -> Result<QuestionRequestState> {
    match value {
        "pending" => Ok(QuestionRequestState::Pending),
        "answered" => Ok(QuestionRequestState::Answered),
        other => Err(StorageError::Other(anyhow::anyhow!(
            "unknown question request state: {other}"
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
