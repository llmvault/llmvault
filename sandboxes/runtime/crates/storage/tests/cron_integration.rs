use chrono::Utc;
use domain::cron::{CronJob, CronJobState};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use storage::{init_sqlite_store, CronJobRepo, SqliteCronJobRepo};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

fn test_job(id: &str, interval: u64) -> CronJob {
    CronJob {
        id: id.to_string(),
        description: "test job".into(),
        channel: "C123".into(),
        task_prompt: "test prompt".into(),
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
        created_by_session: "test-session".into(),
    }
}

async fn setup_repo() -> Arc<dyn CronJobRepo> {
    let unique = DB_COUNTER.fetch_add(1, Ordering::Relaxed);
    let db_path = std::env::temp_dir().join(format!(
        "cron-integration-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None).await.unwrap();
    Arc::new(SqliteCronJobRepo::new(&store))
}

#[tokio::test]
async fn create_get_and_list_cron_jobs() {
    let repo = setup_repo().await;
    repo.create(&test_job("a", 60)).await.unwrap();
    repo.create(&test_job("b", 120)).await.unwrap();

    let fetched = repo.get("a").await.unwrap().unwrap();
    assert_eq!(fetched.id, "a");
    assert_eq!(fetched.task_prompt, "test prompt");
    assert_eq!(fetched.state, CronJobState::Active);
    assert_eq!(fetched.interval_seconds, Some(60));

    let all = repo.list_all().await.unwrap();
    assert_eq!(all.len(), 2);
}

#[tokio::test]
async fn list_due_only_returns_active_due_jobs() {
    let repo = setup_repo().await;
    let mut due = test_job("active-due", 60);
    due.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&due).await.unwrap();

    let mut paused = test_job("paused-due", 60);
    paused.state = CronJobState::Paused;
    paused.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&paused).await.unwrap();

    let mut future = test_job("active-future", 60);
    future.next_run_at = Utc::now() + chrono::Duration::seconds(3600);
    repo.create(&future).await.unwrap();

    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1);
    assert_eq!(due[0].id, "active-due");
}

#[tokio::test]
async fn update_and_state_transitions_persist() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();

    repo.update_prompt("job-1", "new prompt".into())
        .await
        .unwrap();
    repo.update_interval("job-1", 300).await.unwrap();
    repo.set_state("job-1", CronJobState::Paused).await.unwrap();
    let next = Utc::now() + chrono::Duration::seconds(3600);
    repo.update_next_run("job-1", next).await.unwrap();

    let job = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(job.task_prompt, "new prompt");
    assert_eq!(job.interval_seconds, Some(300));
    assert_eq!(job.state, CronJobState::Paused);
    assert!((job.next_run_at - next).num_seconds().abs() < 2);
}

#[tokio::test]
async fn record_run_and_repeat_count_update_lifecycle_fields() {
    let repo = setup_repo().await;
    let mut job = test_job("tracked", 60);
    job.repeat_count = Some(3);
    repo.create(&job).await.unwrap();

    let now = Utc::now();
    repo.record_run("tracked", now, "ok", None).await.unwrap();
    repo.increment_repeat("tracked").await.unwrap();
    repo.record_run("tracked", now, "error", Some("timeout"))
        .await
        .unwrap();

    let job = repo.get("tracked").await.unwrap().unwrap();
    assert!(job.last_run_at.is_some());
    assert_eq!(job.last_status.as_deref(), Some("error"));
    assert_eq!(job.last_error.as_deref(), Some("timeout"));
    assert_eq!(job.repeat_completed, 1);
}

#[tokio::test]
async fn wake_jobs_store_session_continuation_id_and_can_be_due() {
    let repo = setup_repo().await;
    let mut job = test_job("wake-due", 60);
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    job.session_continuation_id = Some("session-continue".into());
    repo.create(&job).await.unwrap();

    let fetched = repo.get("wake-due").await.unwrap().unwrap();
    assert_eq!(
        fetched.session_continuation_id.as_deref(),
        Some("session-continue")
    );

    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1);
    assert_eq!(
        due[0].session_continuation_id.as_deref(),
        Some("session-continue")
    );
}

#[tokio::test]
async fn delete_removes_job() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();
    repo.delete("job-1").await.unwrap();
    assert!(repo.get("job-1").await.unwrap().is_none());
}
