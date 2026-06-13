use chrono::{DateTime, Utc};
use domain::{QuestionAnswerPayload, QuestionRequest, QuestionRequestState};
use sqlx::SqliteConnection;

use crate::repos::Result;

pub(super) async fn question_request_create(
    conn: &mut SqliteConnection,
    request: QuestionRequest,
) -> Result<()> {
    let request_json = serde_json::to_string(&request.request)?;
    let answer_json = request
        .answer
        .as_ref()
        .map(serde_json::to_string)
        .transpose()?;
    sqlx::query(
        "INSERT INTO question_requests \
         (id, session_id, request_json, answer_json, state, created_at, answered_at, updated_at) \
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
    )
    .bind(request.id)
    .bind(request.session_id.as_str())
    .bind(request_json)
    .bind(answer_json)
    .bind(state_to_str(request.state))
    .bind(request.created_at.to_rfc3339())
    .bind(request.answered_at.map(|at| at.to_rfc3339()))
    .bind(request.updated_at.to_rfc3339())
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn question_request_answer(
    conn: &mut SqliteConnection,
    id: &str,
    answer: QuestionAnswerPayload,
    answered_at: DateTime<Utc>,
) -> Result<bool> {
    let answer_json = serde_json::to_string(&answer)?;
    let result = sqlx::query(
        "UPDATE question_requests \
         SET state = 'answered', answer_json = ?, answered_at = ?, updated_at = ? \
         WHERE id = ? AND state = 'pending'",
    )
    .bind(answer_json)
    .bind(answered_at.to_rfc3339())
    .bind(answered_at.to_rfc3339())
    .bind(id)
    .execute(conn)
    .await?;
    Ok(result.rows_affected() == 1)
}

fn state_to_str(state: QuestionRequestState) -> &'static str {
    match state {
        QuestionRequestState::Pending => "pending",
        QuestionRequestState::Answered => "answered",
    }
}
