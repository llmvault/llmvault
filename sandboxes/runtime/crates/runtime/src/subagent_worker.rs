use std::sync::Arc;
use std::time::Duration;

use chrono::Utc;
use domain::{
    event_types, ConfigStore, InboundEvent, OutboundEvent, SessionId, SubagentTask,
    SubagentTaskState,
};
use outbound::OutboundEmitter;
use storage::SubagentTaskRepo;
use tokio::sync::mpsc;
use tracing::{error, warn};

use crate::handler::TurnEventSink;

const POLL_INTERVAL: Duration = Duration::from_millis(500);
const QUEUE_BATCH_SIZE: u32 = 10;

pub struct SubagentWorker {
    repo: Arc<dyn SubagentTaskRepo>,
    config: ConfigStore,
    inbound_sink: mpsc::Sender<InboundEvent>,
    event_sink: Arc<dyn TurnEventSink>,
    emitter: Arc<OutboundEmitter>,
}

impl SubagentWorker {
    pub fn new(
        repo: Arc<dyn SubagentTaskRepo>,
        config: ConfigStore,
        inbound_sink: mpsc::Sender<InboundEvent>,
        event_sink: Arc<dyn TurnEventSink>,
        emitter: Arc<OutboundEmitter>,
    ) -> Self {
        Self {
            repo,
            config,
            inbound_sink,
            event_sink,
            emitter,
        }
    }

    pub async fn run(self) {
        let mut interval = tokio::time::interval(POLL_INTERVAL);
        loop {
            interval.tick().await;
            let tasks = match self.repo.list_queued(QUEUE_BATCH_SIZE).await {
                Ok(tasks) => tasks,
                Err(error) => {
                    error!(%error, "subagent worker: list queued failed");
                    continue;
                }
            };
            for task in tasks {
                self.dispatch_task(task).await;
            }
        }
    }

    async fn dispatch_task(&self, task: SubagentTask) {
        let now = Utc::now();
        match self.repo.mark_running(&task.id, now).await {
            Ok(true) => {}
            Ok(false) => return,
            Err(error) => {
                warn!(job_id = %task.id, %error, "subagent worker: mark running failed");
                return;
            }
        }

        self.event_sink
            .publish_subagent_started(
                task.parent_session_id.as_str(),
                task.stream_id.as_deref(),
                &task.id,
                &task.agent_name,
                task.child_session_id.as_str(),
            )
            .await;
        self.emit_subagent_lifecycle(event_types::SUBAGENT_STARTED, &task, None)
            .await;

        let Some(agent_definition) = self.config.agent_registry().resolve(&task.agent_name) else {
            let error = format!("subagent '{}' not found", task.agent_name);
            let _ = self
                .repo
                .complete(
                    &task.id,
                    SubagentTaskState::Failed,
                    Utc::now(),
                    "",
                    Some(&error),
                )
                .await;
            self.event_sink
                .publish_subagent_errored(
                    task.parent_session_id.as_str(),
                    task.stream_id.as_deref(),
                    &task.id,
                    &task.agent_name,
                    task.child_session_id.as_str(),
                    &error,
                )
                .await;
            self.emit_subagent_lifecycle(event_types::SUBAGENT_ERRORED, &task, Some(&error))
                .await;
            return;
        };

        let inbound = InboundEvent {
            envelope_id: format!("subagent-{}", Utc::now().timestamp_millis()),
            session_id: SessionId::from(task.child_session_id.as_str()),
            user: "subagent".to_string(),
            user_display_name: Some(task.agent_name.clone()),
            text: task.goal.clone(),
            attachments: Vec::new(),
            session_context: Vec::new(),
            model_definition: None,
            raw: serde_json::json!({
                "source": "subagent_task",
                "job_kind": "subagent_task",
                "job_id": task.id.clone(),
                "agent_name": task.agent_name.clone(),
                "parent_session_id": task.parent_session_id.as_str(),
                "child_session_id": task.child_session_id.as_str(),
                "stream_scope": "subagent",
                "subagent_task_goal": task.goal.clone(),
                "session_stream_id": task.stream_id.clone(),
            }),
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: Some(agent_definition),
        };

        if let Err(error) = self.inbound_sink.send(inbound).await {
            let message = error.to_string();
            let _ = self
                .repo
                .complete(
                    &task.id,
                    SubagentTaskState::Failed,
                    Utc::now(),
                    "",
                    Some(&message),
                )
                .await;
            self.event_sink
                .publish_subagent_errored(
                    task.parent_session_id.as_str(),
                    task.stream_id.as_deref(),
                    &task.id,
                    &task.agent_name,
                    task.child_session_id.as_str(),
                    &message,
                )
                .await;
            self.emit_subagent_lifecycle(event_types::SUBAGENT_ERRORED, &task, Some(&message))
                .await;
        }
    }

    async fn emit_subagent_lifecycle(
        &self,
        event_type: &str,
        task: &SubagentTask,
        error: Option<&str>,
    ) {
        let mut payload = serde_json::json!({
            "event_name": event_type,
            "event_id": format!("evt_rt_{}_{}_{}", task.parent_session_id.as_str(), task.id, event_type),
            "session_id": task.parent_session_id.as_str(),
            "stream_id": &task.stream_id,
            "sequence": 0,
            "occurred_at": Utc::now(),
            "scope": "subagent",
            "subagent": {
                "job_id": &task.id,
                "agent_name": &task.agent_name,
                "parent_session_id": task.parent_session_id.as_str(),
                "child_session_id": task.child_session_id.as_str(),
            },
            "job_id": &task.id,
            "agent_name": &task.agent_name,
            "child_session_id": task.child_session_id.as_str(),
        });
        if let Some(error) = error {
            if let Some(map) = payload.as_object_mut() {
                map.insert("error".to_string(), serde_json::json!(error));
            }
        }
        self.emitter
            .emit(OutboundEvent::new(event_type, payload))
            .await;
    }
}
