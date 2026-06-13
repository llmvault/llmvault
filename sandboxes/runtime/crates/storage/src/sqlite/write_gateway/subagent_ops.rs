use chrono::{DateTime, Utc};
use domain::{SessionId, SubagentTask, SubagentTaskState};
use sqlx::SqliteConnection;

use crate::repos::Result;

pub(super) async fn subagent_task_create(
    conn: &mut SqliteConnection,
    task: SubagentTask,
) -> Result<()> {
    sqlx::query(
        "INSERT INTO subagent_tasks \
         (id, parent_session_id, child_session_id, agent_name, goal, stream_id, state, result, \
          error, created_at, started_at, completed_at, updated_at) \
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
    )
    .bind(task.id)
    .bind(task.parent_session_id.as_str())
    .bind(task.child_session_id.as_str())
    .bind(task.agent_name)
    .bind(task.goal)
    .bind(task.stream_id)
    .bind(state_to_str(task.state))
    .bind(task.result)
    .bind(task.error)
    .bind(task.created_at.to_rfc3339())
    .bind(task.started_at.map(|at| at.to_rfc3339()))
    .bind(task.completed_at.map(|at| at.to_rfc3339()))
    .bind(task.updated_at.to_rfc3339())
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn subagent_task_mark_running(
    conn: &mut SqliteConnection,
    id: &str,
    started_at: DateTime<Utc>,
) -> Result<bool> {
    let result = sqlx::query(
        "UPDATE subagent_tasks \
         SET state = 'running', started_at = ?, updated_at = ? \
         WHERE id = ? AND state = 'queued'",
    )
    .bind(started_at.to_rfc3339())
    .bind(started_at.to_rfc3339())
    .bind(id)
    .execute(conn)
    .await?;
    Ok(result.rows_affected() == 1)
}

pub(super) async fn subagent_task_complete(
    conn: &mut SqliteConnection,
    id: &str,
    state: SubagentTaskState,
    completed_at: DateTime<Utc>,
    result: &str,
    error: Option<&str>,
) -> Result<()> {
    sqlx::query(
        "UPDATE subagent_tasks \
         SET state = ?, completed_at = ?, result = ?, error = ?, updated_at = ? \
         WHERE id = ?",
    )
    .bind(state_to_str(state))
    .bind(completed_at.to_rfc3339())
    .bind(result)
    .bind(error)
    .bind(completed_at.to_rfc3339())
    .bind(id)
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn subagent_task_fail_active_for_parent(
    conn: &mut SqliteConnection,
    parent_session_id: &SessionId,
    completed_at: DateTime<Utc>,
    error: &str,
) -> Result<()> {
    sqlx::query(
        "UPDATE subagent_tasks \
         SET state = 'failed', completed_at = ?, error = ?, updated_at = ? \
         WHERE parent_session_id = ? AND state IN ('queued', 'running')",
    )
    .bind(completed_at.to_rfc3339())
    .bind(error)
    .bind(completed_at.to_rfc3339())
    .bind(parent_session_id.as_str())
    .execute(conn)
    .await?;
    Ok(())
}

fn state_to_str(state: SubagentTaskState) -> &'static str {
    match state {
        SubagentTaskState::Queued => "queued",
        SubagentTaskState::Running => "running",
        SubagentTaskState::Completed => "completed",
        SubagentTaskState::Failed => "failed",
    }
}
