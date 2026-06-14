use super::*;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{
    QuestionAnswerPayload, QuestionAnswerValue, RequestUserInputPayload, ToolSpec,
    UpdatePlanPayload, UpdatePlanResult,
};
use outbound::{OutboundChannel, OutboundError, OutboundRegistry};
use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Mutex;
use storage::{OutboxRepo, OutboxRow};
use tokio::sync::RwLock;

fn test_agent_definition() -> domain::AgentDefinition {
    domain::AgentDefinition {
        agent: domain::AgentMeta {
            name: "Test".to_string(),
            description: "Test agent".to_string(),
        },
        system_prompt: Default::default(),
        model: domain::ModelConfig::OpenaiCompatible {
            base_url: "http://localhost".to_string(),
            model_id: "test".to_string(),
            api_key_env: "TEST_KEY".to_string(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: Default::default(),
            fallback: None,
        },
        multimodal_model: None,
        limits: Default::default(),
        context: Default::default(),
        tools: Vec::new(),
        mcp_servers: Vec::new(),
        skills: Vec::new(),
        outbound_channels: Vec::new(),
        sub_agents: Default::default(),
        safety: Default::default(),
    }
}

#[derive(Default)]
struct FakeOutbox {
    rows: Mutex<Vec<(String, String, Value)>>,
}

#[derive(Default)]
struct FakeCronRepo {
    jobs: Mutex<HashMap<String, CronJob>>,
}

#[derive(Default)]
struct FakeSubagentTaskRepo {
    tasks: Mutex<HashMap<String, SubagentTask>>,
}

#[derive(Default)]
struct FakeQuestionRequester {
    requests: Mutex<Vec<RequestUserInputPayload>>,
}

#[derive(Default)]
struct FakePlanUpdater {
    updates: Mutex<Vec<UpdatePlanPayload>>,
}

#[async_trait]
impl OutboxRepo for FakeOutbox {
    async fn enqueue(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: Value,
    ) -> storage::Result<i64> {
        let mut rows = self.rows.lock().expect("outbox lock");
        rows.push((channel_name.to_string(), event_type.to_string(), payload));
        Ok(rows.len() as i64)
    }

    async fn claim_due(&self, _limit: u32) -> storage::Result<Vec<OutboxRow>> {
        Ok(Vec::new())
    }

    async fn mark_delivered(&self, _id: i64) -> storage::Result<()> {
        Ok(())
    }

    async fn schedule_retry(
        &self,
        _id: i64,
        _attempts: i32,
        _next_retry_at: DateTime<Utc>,
    ) -> storage::Result<()> {
        Ok(())
    }

    async fn mark_failed(&self, _id: i64) -> storage::Result<()> {
        Ok(())
    }
}

#[async_trait]
impl CronJobRepo for FakeCronRepo {
    async fn create(&self, job: &CronJob) -> storage::Result<()> {
        self.jobs
            .lock()
            .expect("cron lock")
            .insert(job.id.clone(), job.clone());
        Ok(())
    }

    async fn get(&self, id: &str) -> storage::Result<Option<CronJob>> {
        Ok(self.jobs.lock().expect("cron lock").get(id).cloned())
    }

    async fn list_all(&self) -> storage::Result<Vec<CronJob>> {
        Ok(self
            .jobs
            .lock()
            .expect("cron lock")
            .values()
            .cloned()
            .collect())
    }

    async fn list_due(&self) -> storage::Result<Vec<CronJob>> {
        Ok(Vec::new())
    }

    async fn update_prompt(&self, id: &str, task_prompt: String) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.task_prompt = task_prompt;
        }
        Ok(())
    }

    async fn update_interval(&self, id: &str, interval_seconds: u64) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.interval_seconds = Some(interval_seconds);
        }
        Ok(())
    }

    async fn update_next_run(&self, id: &str, next_run_at: DateTime<Utc>) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.next_run_at = next_run_at;
        }
        Ok(())
    }

    async fn set_state(&self, id: &str, state: CronJobState) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.state = state;
        }
        Ok(())
    }

    async fn record_run(
        &self,
        id: &str,
        run_at: DateTime<Utc>,
        status: &str,
        error: Option<&str>,
    ) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.last_run_at = Some(run_at);
            job.last_status = Some(status.to_string());
            job.last_error = error.map(ToString::to_string);
        }
        Ok(())
    }

    async fn increment_repeat(&self, id: &str) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.repeat_completed += 1;
        }
        Ok(())
    }

    async fn delete(&self, id: &str) -> storage::Result<()> {
        self.jobs.lock().expect("cron lock").remove(id);
        Ok(())
    }
}

#[async_trait]
impl SubagentTaskRepo for FakeSubagentTaskRepo {
    async fn create(&self, task: &SubagentTask) -> storage::Result<()> {
        self.tasks
            .lock()
            .expect("subagent lock")
            .insert(task.id.clone(), task.clone());
        Ok(())
    }

    async fn get(&self, id: &str) -> storage::Result<Option<SubagentTask>> {
        Ok(self.tasks.lock().expect("subagent lock").get(id).cloned())
    }

    async fn list_queued(&self, _limit: u32) -> storage::Result<Vec<SubagentTask>> {
        Ok(self
            .tasks
            .lock()
            .expect("subagent lock")
            .values()
            .filter(|task| task.state == SubagentTaskState::Queued)
            .cloned()
            .collect())
    }

    async fn list_active_by_parent(
        &self,
        parent_session_id: &SessionId,
    ) -> storage::Result<Vec<SubagentTask>> {
        Ok(self
            .tasks
            .lock()
            .expect("subagent lock")
            .values()
            .filter(|task| {
                task.parent_session_id == *parent_session_id
                    && matches!(
                        task.state,
                        SubagentTaskState::Queued | SubagentTaskState::Running
                    )
            })
            .cloned()
            .collect())
    }

    async fn mark_running(&self, id: &str, started_at: DateTime<Utc>) -> storage::Result<bool> {
        let mut tasks = self.tasks.lock().expect("subagent lock");
        let Some(task) = tasks.get_mut(id) else {
            return Ok(false);
        };
        if task.state != SubagentTaskState::Queued {
            return Ok(false);
        }
        task.state = SubagentTaskState::Running;
        task.started_at = Some(started_at);
        Ok(true)
    }

    async fn complete(
        &self,
        id: &str,
        state: SubagentTaskState,
        completed_at: DateTime<Utc>,
        result: &str,
        error: Option<&str>,
    ) -> storage::Result<()> {
        if let Some(task) = self.tasks.lock().expect("subagent lock").get_mut(id) {
            task.state = state;
            task.completed_at = Some(completed_at);
            task.result = Some(result.to_string());
            task.error = error.map(ToString::to_string);
        }
        Ok(())
    }

    async fn fail_active_for_parent(
        &self,
        parent_session_id: &SessionId,
        completed_at: DateTime<Utc>,
        error: &str,
    ) -> storage::Result<()> {
        for task in self.tasks.lock().expect("subagent lock").values_mut() {
            if task.parent_session_id == *parent_session_id
                && matches!(
                    task.state,
                    SubagentTaskState::Queued | SubagentTaskState::Running
                )
            {
                task.state = SubagentTaskState::Failed;
                task.completed_at = Some(completed_at);
                task.error = Some(error.to_string());
            }
        }
        Ok(())
    }
}

#[async_trait]
impl QuestionRequester for FakeQuestionRequester {
    async fn request_user_input(
        &self,
        _session_id: &SessionId,
        request: RequestUserInputPayload,
    ) -> anyhow::Result<(String, QuestionAnswerPayload)> {
        self.requests.lock().expect("question lock").push(request);
        Ok((
            "question-request-test".to_string(),
            QuestionAnswerPayload {
                answers: BTreeMap::from([(
                    "choice".to_string(),
                    QuestionAnswerValue {
                        answers: vec!["Yes".to_string()],
                        other: None,
                    },
                )]),
                user: Some("user-1".to_string()),
                user_display_name: Some("Ada".to_string()),
            },
        ))
    }
}

#[async_trait]
impl PlanUpdater for FakePlanUpdater {
    async fn update_plan(
        &self,
        _session_id: &SessionId,
        payload: UpdatePlanPayload,
    ) -> anyhow::Result<UpdatePlanResult> {
        self.updates
            .lock()
            .expect("plan lock")
            .push(payload.clone());
        Ok(UpdatePlanResult {
            ok: true,
            explanation: payload.explanation,
            plan: payload.plan,
        })
    }
}

struct SkillSyncChannel;

#[async_trait]
impl OutboundChannel for SkillSyncChannel {
    fn name(&self) -> &str {
        "skill-sync"
    }

    fn kind(&self) -> &'static str {
        "test"
    }

    fn accepts(&self, event_type: &str) -> bool {
        event_type == event_types::SKILL_SYNCED || event_type.starts_with("schedule.")
    }

    async fn deliver(&self, _event: &OutboundEvent) -> outbound::Result<()> {
        Err(OutboundError::Delivery("not used in emitter tests".into()))
    }
}

static TEMP_WORKSPACE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn temp_workspace() -> PathBuf {
    let nonce = TEMP_WORKSPACE_COUNTER.fetch_add(1, Ordering::Relaxed);
    let path = std::env::temp_dir().join(format!(
        "hivy-sandboxes-runtime-skill-sync-{}-{}-{nonce}",
        std::process::id(),
        Utc::now().timestamp_nanos_opt().unwrap_or_default()
    ));
    fs::create_dir_all(&path).expect("create temp workspace");
    path
}

fn test_emitter(outbox: Arc<FakeOutbox>) -> Arc<OutboundEmitter> {
    let registry = OutboundRegistry::new().with_channel(Arc::new(SkillSyncChannel));
    Arc::new(OutboundEmitter::new(
        outbox,
        Arc::new(RwLock::new(registry)),
    ))
}

fn skill_manage_test_tool(workspace: PathBuf, outbox: Arc<FakeOutbox>) -> Arc<dyn JsonTool> {
    let emitter = test_emitter(outbox);
    let ctx = ToolContext {
        cron_repo: None,
        subagent_task_repo: None,
        event_repo: None,
        process_registry: None,
        question_requester: None,
        plan_updater: None,
        mcp_registry: None,
        workspace_root: workspace,
        outbound_emitter: Some(emitter),
        agent_registry: Arc::new(AgentDefinitionRegistry::from_definition(Arc::new(
            test_agent_definition(),
        ))),
        session_stream_id: None,
    };
    build_agent_tools(
        &[ToolSpec::SkillManage],
        &SessionId::from("C123-456.789"),
        &ctx,
    )
    .into_iter()
    .find(|tool| tool.definition().name == "skill_manage")
    .expect("skill_manage tool")
}

#[tokio::test]
async fn subagent_task_tool_creates_first_class_task_and_status_reads_repo() {
    let repo = Arc::new(FakeSubagentTaskRepo::default());
    let mut parent = test_agent_definition();
    let helper = test_agent_definition();
    parent.sub_agents.insert("helper".to_string(), helper);
    let registry = Arc::new(AgentDefinitionRegistry::from_definition(Arc::new(parent)));
    let task_tool = subagent_task_tool(
        repo.clone(),
        SessionId::from("parent-session"),
        registry,
        SubagentTaskConfig {
            agents: vec!["helper".to_string()],
        },
        Some("parent-stream-1".to_string()),
    );
    let created = task_tool
        .call(json!({"agent": "helper", "goal": "Return HELPER_OK"}))
        .await
        .expect("create subagent task");
    let job_id = created["job_id"].as_str().expect("job id");
    assert_eq!(created["state"], "queued");
    assert_eq!(created["session_id"], format!("subagent-{job_id}").as_str());
    assert!(created.get("stream_url").is_none());
    assert!(created.get("stream_id").is_none());

    let task = repo
        .get(job_id)
        .await
        .expect("repo get")
        .expect("stored task");
    assert_eq!(task.parent_session_id.as_str(), "parent-session");
    assert_eq!(task.child_session_id.as_str(), format!("subagent-{job_id}"));
    assert_eq!(task.agent_name, "helper");
    assert_eq!(task.goal, "Return HELPER_OK");
    assert_eq!(task.stream_id.as_deref(), Some("parent-stream-1"));
    assert_eq!(task.state, SubagentTaskState::Queued);

    let status_tool = check_subagent_task_status_tool(repo);
    let status = status_tool
        .call(json!({"job_id": job_id}))
        .await
        .expect("check subagent task status");
    assert_eq!(status["job_id"], job_id);
    assert_eq!(status["state"], "queued");
    assert_eq!(status["session_id"], format!("subagent-{job_id}").as_str());
}

#[tokio::test]
async fn request_user_input_tool_is_registered_and_returns_answer() {
    let requester = Arc::new(FakeQuestionRequester::default());
    let ctx = ToolContext {
        cron_repo: None,
        subagent_task_repo: None,
        event_repo: None,
        process_registry: None,
        question_requester: Some(requester.clone()),
        plan_updater: None,
        mcp_registry: None,
        workspace_root: temp_workspace(),
        outbound_emitter: None,
        agent_registry: Arc::new(AgentDefinitionRegistry::from_definition(Arc::new(
            test_agent_definition(),
        ))),
        session_stream_id: None,
    };
    let tool = build_agent_tools(
        &[ToolSpec::RequestUserInput],
        &SessionId::from("question-session"),
        &ctx,
    )
    .into_iter()
    .find(|tool| tool.definition().name == "request_user_input")
    .expect("request_user_input tool");

    let result = tool
        .call(json!({
            "questions": [{
                "id": "choice",
                "header": "Choice",
                "question": "Pick one.",
                "options": [
                    {"label": "Yes", "description": "Proceed."},
                    {"label": "No", "description": "Stop."}
                ]
            }]
        }))
        .await
        .expect("request user input");

    assert_eq!(result["question_request_id"], "question-request-test");
    assert_eq!(result["answers"]["choice"]["answers"][0], "Yes");
    let recorded = requester.requests.lock().expect("question lock");
    assert_eq!(recorded.len(), 1);
    assert_eq!(recorded[0].questions[0].id, "choice");
}

#[tokio::test]
async fn request_user_input_tool_rejects_invalid_schema() {
    let requester = Arc::new(FakeQuestionRequester::default());
    let tool = request_user_input_tool(requester.clone(), SessionId::from("question-session"));

    let error = tool
        .call(json!({
            "questions": [{
                "id": "choice",
                "header": "TooLongHeader",
                "question": "Pick one.",
                "options": [
                    {"label": "Yes", "description": "Proceed."}
                ]
            }]
        }))
        .await
        .expect_err("invalid schema must fail");

    assert!(
        error
            .to_string()
            .contains("invalid request_user_input arguments"),
        "unexpected error: {error}"
    );
    assert!(requester.requests.lock().expect("question lock").is_empty());
}

#[tokio::test]
async fn update_plan_tool_is_registered_and_returns_ack() {
    let updater = Arc::new(FakePlanUpdater::default());
    let ctx = ToolContext {
        cron_repo: None,
        subagent_task_repo: None,
        event_repo: None,
        process_registry: None,
        question_requester: None,
        plan_updater: Some(updater.clone()),
        mcp_registry: None,
        workspace_root: temp_workspace(),
        outbound_emitter: None,
        agent_registry: Arc::new(AgentDefinitionRegistry::from_definition(Arc::new(
            test_agent_definition(),
        ))),
        session_stream_id: None,
    };
    let tool = build_agent_tools(
        &[ToolSpec::UpdatePlan],
        &SessionId::from("plan-session"),
        &ctx,
    )
    .into_iter()
    .find(|tool| tool.definition().name == "update_plan")
    .expect("update_plan tool");

    let result = tool
        .call(json!({
            "explanation": "Working through runtime changes",
            "plan": [
                {"step": "Inspect code", "status": "completed"},
                {"step": "Add tests", "status": "in_progress"},
                {"step": "Run E2E", "status": "pending"}
            ]
        }))
        .await
        .expect("update plan");

    assert_eq!(result["ok"], true);
    assert_eq!(result["explanation"], "Working through runtime changes");
    assert_eq!(result["plan"][1]["status"], "in_progress");
    let recorded = updater.updates.lock().expect("plan lock");
    assert_eq!(recorded.len(), 1);
    assert_eq!(recorded[0].plan[1].step, "Add tests");
}

#[tokio::test]
async fn update_plan_tool_rejects_invalid_status() {
    let updater = Arc::new(FakePlanUpdater::default());
    let tool = update_plan_tool(updater.clone(), SessionId::from("plan-session"));

    let error = tool
        .call(json!({
            "plan": [{"step": "Inspect code", "status": "started"}]
        }))
        .await
        .expect_err("invalid status must fail");

    assert!(
        error.to_string().contains("invalid update_plan arguments"),
        "unexpected error: {error}"
    );
    assert!(updater.updates.lock().expect("plan lock").is_empty());
}

#[tokio::test]
async fn update_plan_tool_rejects_duplicate_in_progress() {
    let updater = Arc::new(FakePlanUpdater::default());
    let tool = update_plan_tool(updater.clone(), SessionId::from("plan-session"));

    let error = tool
        .call(json!({
            "plan": [
                {"step": "Inspect code", "status": "in_progress"},
                {"step": "Add tests", "status": "in_progress"}
            ]
        }))
        .await
        .expect_err("duplicate in_progress must fail");

    assert!(
        error.to_string().contains("at most one in_progress item"),
        "unexpected error: {error}"
    );
    assert!(updater.updates.lock().expect("plan lock").is_empty());
}

#[tokio::test]
async fn update_plan_tool_rejects_empty_steps() {
    let updater = Arc::new(FakePlanUpdater::default());
    let tool = update_plan_tool(updater.clone(), SessionId::from("plan-session"));

    let error = tool
        .call(json!({
            "plan": [{"step": "   ", "status": "pending"}]
        }))
        .await
        .expect_err("empty step must fail");

    assert!(
        error.to_string().contains("step must not be empty"),
        "unexpected error: {error}"
    );
    assert!(updater.updates.lock().expect("plan lock").is_empty());
}

#[tokio::test]
async fn update_plan_tool_is_not_registered_for_cron_sessions() {
    let updater = Arc::new(FakePlanUpdater::default());
    let ctx = ToolContext {
        cron_repo: None,
        subagent_task_repo: None,
        event_repo: None,
        process_registry: None,
        question_requester: None,
        plan_updater: Some(updater),
        mcp_registry: None,
        workspace_root: temp_workspace(),
        outbound_emitter: None,
        agent_registry: Arc::new(AgentDefinitionRegistry::from_definition(Arc::new(
            test_agent_definition(),
        ))),
        session_stream_id: None,
    };

    let tools = build_agent_tools(
        &[ToolSpec::UpdatePlan],
        &SessionId::from("wake-cron-session"),
        &ctx,
    );

    assert!(tools
        .iter()
        .all(|tool| tool.definition().name != "update_plan"));
}

#[tokio::test]
async fn skill_manage_create_emits_complete_sync_snapshot() {
    let workspace = temp_workspace();
    let outbox = Arc::new(FakeOutbox::default());
    let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

    tool.call(json!({
            "action": "create",
            "name": "debug-deploys",
            "category": "engineering",
            "content": "---\nname: debug-deploys\ndescription: Debug deploy failures.\ntags: deploy, debug\n---\n# Debug\nCheck logs first."
        }))
        .await
        .expect("skill create");
    tool.call(json!({
        "action": "write_file",
        "name": "debug-deploys",
        "file_path": "references/errors.md",
        "file_content": "# Errors"
    }))
    .await
    .expect("supporting file write");

    let rows = outbox.rows.lock().expect("outbox lock");
    assert_eq!(rows.len(), 2);
    let (_, event_type, payload) = &rows[1];
    assert_eq!(event_type, event_types::SKILL_SYNCED);
    assert_eq!(payload["action"], "write_file");
    assert_eq!(payload["name"], "debug-deploys");
    assert_eq!(payload["source"], "session");
    assert_eq!(payload["description"], "Debug deploy failures.");
    assert_eq!(payload["files"]["references/errors.md"], "# Errors");
    assert!(payload["content"]
        .as_str()
        .expect("content string")
        .contains("# Debug"));

    let _ = fs::remove_dir_all(workspace);
}

#[tokio::test]
async fn skill_manage_failed_call_emits_no_sync_event() {
    let workspace = temp_workspace();
    let outbox = Arc::new(FakeOutbox::default());
    let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

    let result = tool
        .call(json!({
            "action": "write_file",
            "name": "missing-skill",
            "file_path": "references/errors.md",
            "content": "# Errors"
        }))
        .await;

    assert!(result.is_err());
    assert!(outbox.rows.lock().expect("outbox lock").is_empty());
    let _ = fs::remove_dir_all(workspace);
}

#[tokio::test]
async fn skill_manage_delete_emits_tombstone() {
    let workspace = temp_workspace();
    let outbox = Arc::new(FakeOutbox::default());
    let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

    tool.call(json!({
        "action": "create",
        "name": "debug-deploys",
        "content": "---\nname: debug-deploys\n---\n# Debug"
    }))
    .await
    .expect("skill create");
    tool.call(json!({
        "action": "create",
        "name": "deploy-ops",
        "content": "---\nname: deploy-ops\n---\n# Deploy ops"
    }))
    .await
    .expect("absorbed target create");
    tool.call(json!({
        "action": "delete",
        "name": "debug-deploys",
        "absorbed_into": "deploy-ops"
    }))
    .await
    .expect("skill delete");

    let rows = outbox.rows.lock().expect("outbox lock");
    assert_eq!(rows.len(), 3);
    let (_, event_type, payload) = &rows[2];
    assert_eq!(event_type, event_types::SKILL_SYNCED);
    assert_eq!(payload["action"], "delete");
    assert_eq!(payload["deleted"], true);
    assert_eq!(payload["absorbed_into"], "deploy-ops");
    assert!(payload.get("content").is_none());

    let _ = fs::remove_dir_all(workspace);
}

#[tokio::test]
async fn cron_create_update_pause_resume_cancel_emit_schedule_events() {
    let repo = Arc::new(FakeCronRepo::default());
    let outbox = Arc::new(FakeOutbox::default());
    let tool = cron_tool(
        repo,
        SessionId::from("C123-456.789"),
        Some(test_emitter(outbox.clone())),
    );

    let created = tool
        .call(json!({
            "action": "create",
            "task_prompt": "Check deploy health",
            "interval_seconds": 3600,
            "description": "Deploy health"
        }))
        .await
        .expect("create cron");
    let job_id = created["job_id"].as_str().expect("job id").to_string();
    tool.call(json!({"action": "update", "job_id": job_id, "task_prompt": "Check API health", "interval_seconds": 7200}))
            .await
            .expect("update cron");
    tool.call(json!({"action": "pause", "job_id": job_id}))
        .await
        .expect("pause cron");
    tool.call(json!({"action": "resume", "job_id": job_id}))
        .await
        .expect("resume cron");
    tool.call(json!({"action": "cancel", "job_id": job_id}))
        .await
        .expect("cancel cron");

    let rows = outbox.rows.lock().expect("outbox lock");
    let event_types: Vec<_> = rows
        .iter()
        .map(|(_, event_type, _)| event_type.as_str())
        .collect();
    assert_eq!(
        event_types,
        vec![
            event_types::SCHEDULE_CREATED,
            event_types::SCHEDULE_UPDATED,
            event_types::SCHEDULE_PAUSED,
            event_types::SCHEDULE_RESUMED,
            event_types::SCHEDULE_CANCELLED,
        ]
    );
    assert_eq!(rows[0].2["session_id"], "C123-456.789");
    assert_eq!(rows[0].2["origin"], "tool");
    assert_eq!(rows[0].2["task_prompt"], "Check deploy health");
}

#[tokio::test]
async fn wake_jobs_do_not_emit_schedule_events() {
    let now = Utc::now();
    let outbox = Arc::new(FakeOutbox::default());
    let job = CronJob {
        id: "wake-1".to_string(),
        description: "Wake".to_string(),
        channel: "C123".to_string(),
        task_prompt: "Wake up".to_string(),
        cron_expression: None,
        interval_seconds: None,
        repeat_count: Some(1),
        repeat_completed: 0,
        state: CronJobState::Active,
        next_run_at: now,
        last_run_at: None,
        last_status: None,
        last_error: None,
        session_continuation_id: Some("C123-456.789".to_string()),
        created_at: now,
        created_by_session: "C123-456.789".to_string(),
    };
    emit_schedule_event(
        Some(test_emitter(outbox.clone())),
        event_types::SCHEDULE_CREATED,
        &job,
        &SessionId::from("C123-456.789"),
        "tool",
        None,
    )
    .await;
    assert!(outbox.rows.lock().expect("outbox lock").is_empty());
}

#[tokio::test]
async fn tool_invoked_event_is_not_emitted_directly() {
    let outbox = Arc::new(FakeOutbox::default());
    let session = SessionId::from("http-conversation-1");

    emit_tool_invoked(
        Some(test_emitter(outbox.clone())),
        &session,
        "http_request",
        &json!({
            "url": "https://internal.example.com/admin",
            "authorization": "Bearer super-secret-token",
            "query": "customer PII question",
        }),
        &json!({"status": 200}),
    )
    .await;

    let rows = outbox.rows.lock().expect("outbox lock");
    assert!(
        rows.is_empty(),
        "tool results are emitted from canonical agent stream events"
    );
}

#[tokio::test]
async fn tool_error_event_is_not_emitted_directly() {
    let outbox = Arc::new(FakeOutbox::default());
    let session = SessionId::from("http-conversation-1");

    emit_tool_error(
        Some(test_emitter(outbox.clone())),
        &session,
        "send_email",
        &json!({
            "to": "ceo@example.com",
            "body": "confidential acquisition terms",
        }),
        "smtp authentication failed",
    )
    .await;

    let rows = outbox.rows.lock().expect("outbox lock");
    assert!(
        rows.is_empty(),
        "tool errors are emitted as canonical tool_result events"
    );
}
