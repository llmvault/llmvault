use chrono::{DateTime, Utc};
use serde_json::{json, Value};
use sqlx::{Connection, QueryBuilder, Row, Sqlite, SqliteConnection};

use crate::repos::{OutboxRow, Result, StorageError};

use super::EventsLogWrite;

const EVENTS_LOG_INSERT_CHUNK_EVENTS: usize = 200;
const SESSION_CLAIM_WINDOW: i64 = 100;

fn payload_occurred_at(payload_json: &str) -> Option<String> {
    let payload: Value = serde_json::from_str(payload_json).ok()?;
    payload
        .get("occurred_at")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
}

fn parse_rfc3339(raw: &str) -> Result<DateTime<Utc>> {
    DateTime::parse_from_rfc3339(raw)
        .map(|value| value.with_timezone(&Utc))
        .map_err(|error| {
            StorageError::Other(anyhow::anyhow!(
                "invalid outbound occurred_at {raw:?}: {error}"
            ))
        })
}

pub(super) async fn outbox_enqueue(
    conn: &mut SqliteConnection,
    channel_name: &str,
    event_type: &str,
    payload_json: &str,
    now: &str,
) -> Result<i64> {
    let occurred_at = payload_occurred_at(payload_json).unwrap_or_else(|| now.to_string());
    let id: i64 = sqlx::query_scalar(
        "INSERT INTO outbound_outbox \
         (channel_name, event_type, payload_json, attempts, next_retry_at, status, occurred_at, created_at) \
         VALUES (?, ?, ?, 0, ?, ?, ?, ?) RETURNING id",
    )
    .bind(channel_name)
    .bind(event_type)
    .bind(payload_json)
    .bind(now)
    .bind("pending")
    .bind(occurred_at)
    .bind(now)
    .fetch_one(conn)
    .await?;
    Ok(id)
}

pub(super) async fn outbox_enqueue_runtime(
    conn: &mut SqliteConnection,
    channel_name: &str,
    event_type: &str,
    payload_json: &str,
    now: &str,
) -> Result<i64> {
    let mut payload: Value = serde_json::from_str(payload_json)?;
    let Some(session_id) = payload
        .get("session_id")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
    else {
        return outbox_enqueue(conn, channel_name, event_type, payload_json, now).await;
    };

    let mut tx = conn.begin().await?;
    let runtime_seq: i64 = sqlx::query_scalar(
        "INSERT INTO runtime_event_sequences (session_id, last_seq, updated_at) \
         VALUES (?, 1, ?) \
         ON CONFLICT(session_id) DO UPDATE SET \
             last_seq = last_seq + 1, \
             updated_at = excluded.updated_at \
         RETURNING last_seq",
    )
    .bind(&session_id)
    .bind(now)
    .fetch_one(&mut *tx)
    .await?;

    if let Some(map) = payload.as_object_mut() {
        map.insert("runtime_seq".to_string(), json!(runtime_seq));
        map.entry("event_id".to_string())
            .or_insert_with(|| json!(format!("evt_rt_{}_{}", session_id, runtime_seq)));
    }
    let payload_json = serde_json::to_string(&payload)?;
    let occurred_at = payload_occurred_at(&payload_json).unwrap_or_else(|| now.to_string());
    let id: i64 = sqlx::query_scalar(
        "INSERT INTO outbound_outbox \
         (channel_name, event_type, payload_json, session_id, runtime_seq, attempts, next_retry_at, status, occurred_at, created_at) \
         VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?) RETURNING id",
    )
    .bind(channel_name)
    .bind(event_type)
    .bind(&payload_json)
    .bind(&session_id)
    .bind(runtime_seq)
    .bind(now)
    .bind("pending")
    .bind(occurred_at)
    .bind(now)
    .fetch_one(&mut *tx)
    .await?;
    tx.commit().await?;
    Ok(id)
}

pub(super) async fn outbox_claim_due(
    conn: &mut SqliteConnection,
    limit: u32,
    now: &str,
    lease_until: &str,
) -> Result<Vec<OutboxRow>> {
    let limit = limit.min(1024);
    if limit == 0 {
        return Ok(Vec::new());
    }

    let rows = sqlx::query(
        "WITH session_ordered AS ( \
             SELECT \
                 id, \
                 ROW_NUMBER() OVER ( \
                     PARTITION BY channel_name, session_id \
                     ORDER BY COALESCE(runtime_seq, id), id \
                 ) AS rn, \
                 SUM(CASE \
                     WHEN next_retry_at <= ? AND (lease_until IS NULL OR lease_until <= ?) THEN 0 \
                     ELSE 1 \
                 END) OVER ( \
                     PARTITION BY channel_name, session_id \
                     ORDER BY COALESCE(runtime_seq, id), id \
                     ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW \
                 ) AS blockers \
             FROM outbound_outbox \
             WHERE status = ? AND session_id IS NOT NULL \
         ), \
         session_due AS ( \
             SELECT id \
             FROM session_ordered \
             WHERE blockers = 0 AND rn <= ? \
         ), \
         non_session_due AS ( \
             SELECT id \
             FROM outbound_outbox \
             WHERE status = ? \
               AND session_id IS NULL \
               AND next_retry_at <= ? \
               AND (lease_until IS NULL OR lease_until <= ?) \
         ), \
         claim AS ( \
             SELECT id FROM session_due \
             UNION ALL \
             SELECT id FROM non_session_due \
             ORDER BY id ASC \
             LIMIT ? \
         ) \
         UPDATE outbound_outbox \
         SET lease_until = ? \
         WHERE id IN (SELECT id FROM claim) \
         RETURNING id, channel_name, event_type, payload_json, attempts, session_id, runtime_seq, occurred_at",
    )
    .bind(now)
    .bind(now)
    .bind("pending")
    .bind(SESSION_CLAIM_WINDOW)
    .bind("pending")
    .bind(now)
    .bind(now)
    .bind(limit as i64)
    .bind(lease_until)
    .fetch_all(conn)
    .await?;

    let mut outbox_rows = Vec::with_capacity(rows.len());
    for row in rows {
        let payload_text: String = row.try_get("payload_json")?;
        let occurred_at_text: String = row.try_get("occurred_at")?;
        outbox_rows.push(OutboxRow {
            id: row.try_get("id")?,
            channel_name: row.try_get("channel_name")?,
            event_type: row.try_get("event_type")?,
            payload: serde_json::from_str(&payload_text)?,
            attempts: row.try_get("attempts")?,
            session_id: row.try_get("session_id")?,
            runtime_seq: row.try_get("runtime_seq")?,
            occurred_at: parse_rfc3339(&occurred_at_text)?,
        });
    }
    Ok(outbox_rows)
}

pub(super) async fn outbox_mark_status(
    conn: &mut SqliteConnection,
    id: i64,
    status: &str,
) -> Result<()> {
    sqlx::query("UPDATE outbound_outbox SET status = ?, lease_until = NULL WHERE id = ?")
        .bind(status)
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn outbox_schedule_retry(
    conn: &mut SqliteConnection,
    id: i64,
    attempts: i32,
    next_retry_at: DateTime<Utc>,
) -> Result<()> {
    sqlx::query(
        "UPDATE outbound_outbox \
         SET attempts = ?, next_retry_at = ?, status = ?, lease_until = NULL \
         WHERE id = ?",
    )
    .bind(attempts)
    .bind(next_retry_at.to_rfc3339())
    .bind("pending")
    .bind(id)
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn events_log_batch(
    conn: &mut SqliteConnection,
    events: &[EventsLogWrite],
    recorded_at: &str,
) -> Result<()> {
    if events.is_empty() {
        return Ok(());
    }
    let mut tx = conn.begin().await?;
    for chunk in events.chunks(EVENTS_LOG_INSERT_CHUNK_EVENTS) {
        let mut builder = QueryBuilder::<Sqlite>::new(
            "INSERT INTO events_log (event_type, payload_json, occurred_at, recorded_at) ",
        );
        builder.push_values(chunk, |mut row, event| {
            row.push_bind(&event.event_type)
                .push_bind(&event.payload_json)
                .push_bind(&event.occurred_at)
                .push_bind(recorded_at);
        });
        builder.build().execute(&mut *tx).await?;
    }
    tx.commit().await?;
    Ok(())
}
