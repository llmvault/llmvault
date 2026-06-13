use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use chrono::Utc;
use domain::cron::{CronJob, CronJobState};
use storage::{init_sqlite_store, CronJobRepo, SqliteCronJobRepo};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

fn test_job(id: &str, interval: u64) -> CronJob {
    CronJob {
        id: id.to_string(),
        description: "test".into(),
        channel: "C123".into(),
        task_prompt: "test".into(),
        cron_expression: None,
        interval_seconds: Some(interval),
        repeat_count: None,
        repeat_completed: 0,
        state: CronJobState::Active,
        next_run_at: Utc::now(),
        last_run_at: None,
        last_status: None,
        last_error: None,
        session_continuation_id: None,
        created_at: Utc::now(),
        created_by_session: "test".into(),
    }
}

async fn setup_repo() -> Arc<dyn CronJobRepo> {
    let unique = DB_COUNTER.fetch_add(1, Ordering::Relaxed);
    let db_path = std::env::temp_dir().join(format!(
        "scheduler-integration-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None).await.unwrap();
    Arc::new(SqliteCronJobRepo::new(&store))
}

#[tokio::test]
async fn daily_report_advances_next_run_before_dispatch() {
    let repo = setup_repo().await;
    let mut job = test_job("daily", 86_400);
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&job).await.unwrap();

    assert_eq!(repo.list_due().await.unwrap().len(), 1);

    let next = Utc::now() + chrono::Duration::seconds(86_400);
    repo.update_next_run("daily", next).await.unwrap();

    assert!(repo.list_due().await.unwrap().is_empty());
}

#[tokio::test]
async fn wake_reminder_preserves_session_continuation() {
    let repo = setup_repo().await;
    let mut job = test_job("wake-1", 300);
    job.session_continuation_id = Some("session-1778247607".into());
    job.description = "wake-up reminder".into();
    repo.create(&job).await.unwrap();

    let fetched = repo.get("wake-1").await.unwrap().unwrap();
    assert_eq!(
        fetched.session_continuation_id.as_deref(),
        Some("session-1778247607")
    );
    assert_eq!(fetched.interval_seconds, Some(300));
}

#[tokio::test]
async fn regular_worker_cron_has_no_session_continuation() {
    let repo = setup_repo().await;
    let mut job = test_job("worker", 3600);
    job.channel = "system".into();
    job.task_prompt = "post daily summary".into();
    repo.create(&job).await.unwrap();

    let fetched = repo.get("worker").await.unwrap().unwrap();
    assert!(fetched.session_continuation_id.is_none());
    assert_eq!(fetched.channel, "system");
}

#[tokio::test]
async fn stale_daily_report_is_detectably_stale_for_fast_forwarding() {
    let mut job = test_job("stale-daily", 86_400);
    job.next_run_at = Utc::now() - chrono::Duration::hours(48);

    let interval = job.interval_seconds.unwrap();
    let stale_threshold = (interval as f64 * 0.5).max(120.0) as i64;
    let lag = Utc::now()
        .signed_duration_since(job.next_run_at)
        .num_seconds();

    assert!(lag > stale_threshold * 2);
}
