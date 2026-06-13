use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use chrono::Utc;
use domain::cron::{CronJob, CronJobState};
use storage::{init_sqlite_store, CronJobRepo, SqliteCronJobRepo};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

fn test_job(id: &str) -> CronJob {
    CronJob {
        id: id.to_string(),
        description: "test".into(),
        channel: "C123".into(),
        task_prompt: "test".into(),
        cron_expression: None,
        interval_seconds: Some(60),
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
        "cron-tool-integration-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None).await.unwrap();
    Arc::new(SqliteCronJobRepo::new(&store))
}

#[tokio::test]
async fn user_creates_daily_report_with_cron_tool_shape() {
    let repo = setup_repo().await;
    let mut job = test_job("daily-report");
    job.description = "Daily team summary".into();
    job.task_prompt = "Summarize today's Linear issues and post to channel".into();
    job.interval_seconds = Some(86_400);
    repo.create(&job).await.unwrap();

    let fetched = repo.get("daily-report").await.unwrap().unwrap();
    assert_eq!(fetched.description, "Daily team summary");
    assert_eq!(fetched.interval_seconds, Some(86_400));
    assert!(fetched.session_continuation_id.is_none());
}

#[tokio::test]
async fn user_cancels_daily_report() {
    let repo = setup_repo().await;
    repo.create(&test_job("daily")).await.unwrap();
    assert!(repo.get("daily").await.unwrap().is_some());

    repo.delete("daily").await.unwrap();
    assert!(repo.get("daily").await.unwrap().is_none());
}

#[tokio::test]
async fn user_pauses_resumes_and_updates_a_cron() {
    let repo = setup_repo().await;
    repo.create(&test_job("weekly")).await.unwrap();

    repo.set_state("weekly", CronJobState::Paused)
        .await
        .unwrap();
    assert_eq!(
        repo.get("weekly").await.unwrap().unwrap().state,
        CronJobState::Paused
    );
    assert!(repo.list_due().await.unwrap().is_empty());

    repo.update_prompt("weekly", "New prompt".into())
        .await
        .unwrap();
    repo.update_interval("weekly", 3600).await.unwrap();
    repo.set_state("weekly", CronJobState::Active)
        .await
        .unwrap();
    let past = Utc::now() - chrono::Duration::seconds(10);
    repo.update_next_run("weekly", past).await.unwrap();

    let updated = repo.get("weekly").await.unwrap().unwrap();
    assert_eq!(updated.task_prompt, "New prompt");
    assert_eq!(updated.interval_seconds, Some(3600));
    assert_eq!(updated.state, CronJobState::Active);
    assert!(!repo.list_due().await.unwrap().is_empty());
}

#[tokio::test]
async fn recording_run_updates_execution_history() {
    let repo = setup_repo().await;
    repo.create(&test_job("tracked")).await.unwrap();

    let now = Utc::now();
    repo.record_run("tracked", now, "ok", None).await.unwrap();
    let job = repo.get("tracked").await.unwrap().unwrap();
    assert!(job.last_run_at.is_some());
    assert_eq!(job.last_status.as_deref(), Some("ok"));

    repo.record_run("tracked", now, "error", Some("timeout"))
        .await
        .unwrap();
    let job = repo.get("tracked").await.unwrap().unwrap();
    assert_eq!(job.last_status.as_deref(), Some("error"));
    assert_eq!(job.last_error.as_deref(), Some("timeout"));
}

#[tokio::test]
async fn wake_cron_uses_session_continuation() {
    let repo = setup_repo().await;
    let mut wake = test_job("wake-reminder");
    wake.session_continuation_id = Some("session-thread".into());
    wake.interval_seconds = Some(300);
    repo.create(&wake).await.unwrap();

    let fetched = repo.get("wake-reminder").await.unwrap().unwrap();
    assert_eq!(
        fetched.session_continuation_id.as_deref(),
        Some("session-thread")
    );
    assert_eq!(fetched.interval_seconds, Some(300));
}
