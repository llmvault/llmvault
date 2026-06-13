use chrono::Utc;
use domain::{Session, SessionId, SessionStatus, SubagentTask, SubagentTaskState};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use storage::{
    init_sqlite_store, SessionRepo, SqliteSessionRepo, SqliteSubagentTaskRepo, SubagentTaskRepo,
};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

async fn setup_repos() -> (Arc<dyn SubagentTaskRepo>, SqliteSessionRepo) {
    let unique = DB_COUNTER.fetch_add(1, Ordering::Relaxed);
    let db_path = std::env::temp_dir().join(format!(
        "subagent-task-integration-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None).await.unwrap();
    (
        Arc::new(SqliteSubagentTaskRepo::new(&store)),
        SqliteSessionRepo::new(&store),
    )
}

async fn create_session(repo: &SqliteSessionRepo, id: &str) {
    let now = Utc::now();
    repo.create(&Session {
        id: SessionId::from(id),
        status: SessionStatus::Active,
        created_at: now,
        last_activity_at: now,
    })
    .await
    .unwrap();
}

fn task(id: &str, parent: &str, child: &str) -> SubagentTask {
    let now = Utc::now();
    SubagentTask {
        id: id.to_string(),
        parent_session_id: SessionId::from(parent),
        child_session_id: SessionId::from(child),
        agent_name: "helper".to_string(),
        goal: "verify something".to_string(),
        stream_id: Some(format!("stream-{id}")),
        state: SubagentTaskState::Queued,
        result: None,
        error: None,
        created_at: now,
        started_at: None,
        completed_at: None,
        updated_at: now,
    }
}

#[tokio::test]
async fn subagent_task_lifecycle_is_first_class_storage() {
    let (repo, sessions) = setup_repos().await;
    create_session(&sessions, "parent-session").await;
    repo.create(&task("job-1", "parent-session", "subagent-job-1"))
        .await
        .unwrap();

    let queued = repo.list_queued(10).await.unwrap();
    assert_eq!(queued.len(), 1);
    assert_eq!(queued[0].child_session_id.as_str(), "subagent-job-1");

    let started_at = Utc::now();
    assert!(repo.mark_running("job-1", started_at).await.unwrap());
    assert!(!repo.mark_running("job-1", started_at).await.unwrap());

    let active = repo
        .list_active_by_parent(&SessionId::from("parent-session"))
        .await
        .unwrap();
    assert_eq!(active.len(), 1);
    assert_eq!(active[0].state, SubagentTaskState::Running);

    repo.complete(
        "job-1",
        SubagentTaskState::Completed,
        Utc::now(),
        "SUBAGENT_DONE",
        None,
    )
    .await
    .unwrap();

    let fetched = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(fetched.state, SubagentTaskState::Completed);
    assert_eq!(fetched.result.as_deref(), Some("SUBAGENT_DONE"));
    assert!(fetched.completed_at.is_some());
    assert!(repo
        .list_active_by_parent(&SessionId::from("parent-session"))
        .await
        .unwrap()
        .is_empty());
}

#[tokio::test]
async fn fail_active_for_parent_marks_all_active_tasks_failed() {
    let (repo, sessions) = setup_repos().await;
    create_session(&sessions, "parent-session").await;
    repo.create(&task("job-1", "parent-session", "subagent-job-1"))
        .await
        .unwrap();
    repo.create(&task("job-2", "parent-session", "subagent-job-2"))
        .await
        .unwrap();
    repo.mark_running("job-2", Utc::now()).await.unwrap();

    repo.fail_active_for_parent(
        &SessionId::from("parent-session"),
        Utc::now(),
        "parent wait timed out",
    )
    .await
    .unwrap();

    for id in ["job-1", "job-2"] {
        let fetched = repo.get(id).await.unwrap().unwrap();
        assert_eq!(fetched.state, SubagentTaskState::Failed);
        assert_eq!(fetched.error.as_deref(), Some("parent wait timed out"));
    }
}
