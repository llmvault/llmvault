use std::sync::Arc;
use std::time::Duration;

use agent::rig_tool_registry::WakeScheduler;
use anyhow::{anyhow, Result};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{InboundEvent, OutboundEvent, SessionId};
use outbound::OutboundEmitter;
use tokio::sync::mpsc;

pub struct RuntimeWakeScheduler {
    inbound_sink: mpsc::Sender<InboundEvent>,
    emitter: Option<Arc<OutboundEmitter>>,
    heartbeat_interval: Duration,
}

impl RuntimeWakeScheduler {
    pub fn new(
        inbound_sink: mpsc::Sender<InboundEvent>,
        emitter: Option<Arc<OutboundEmitter>>,
        heartbeat_interval: Duration,
    ) -> Self {
        Self {
            inbound_sink,
            emitter,
            heartbeat_interval,
        }
    }
}

#[async_trait]
impl WakeScheduler for RuntimeWakeScheduler {
    async fn schedule_wake(
        &self,
        session_id: SessionId,
        stream_id: Option<String>,
        seconds: u64,
        task_prompt: String,
    ) -> Result<(String, DateTime<Utc>)> {
        if seconds == 0 {
            return Err(anyhow!("seconds must be greater than zero"));
        }
        let task_prompt = task_prompt.trim().to_string();
        if task_prompt.is_empty() {
            return Err(anyhow!("task_prompt required"));
        }

        let now = Utc::now();
        let job_id = format!("wake-{}", now.timestamp_millis());
        let next_run_at = now + chrono::Duration::seconds(seconds as i64);
        emit_wake_event(
            self.emitter.clone(),
            "wake.scheduled",
            &job_id,
            &session_id,
            stream_id.as_deref(),
            next_run_at,
        )
        .await;

        let inbound_sink = self.inbound_sink.clone();
        let emitter = self.emitter.clone();
        let heartbeat_interval = self.heartbeat_interval;
        let task_job_id = job_id.clone();
        let task_session_id = session_id.clone();
        tokio::spawn(async move {
            loop {
                let now = Utc::now();
                if now >= next_run_at {
                    break;
                }
                let remaining = (next_run_at - now)
                    .to_std()
                    .unwrap_or_else(|_| Duration::from_secs(0));
                let sleep_for = remaining.min(heartbeat_interval);
                tokio::time::sleep(sleep_for).await;
                if Utc::now() < next_run_at {
                    emit_wake_event(
                        emitter.clone(),
                        "wake.heartbeat",
                        &task_job_id,
                        &task_session_id,
                        stream_id.as_deref(),
                        next_run_at,
                    )
                    .await;
                }
            }

            emit_wake_event(
                emitter.clone(),
                "wake.fired",
                &task_job_id,
                &task_session_id,
                stream_id.as_deref(),
                next_run_at,
            )
            .await;

            let raw = serde_json::json!({
                "source": "wake",
                "job_kind": "wake",
                "job_id": task_job_id,
                "session_stream_id": stream_id,
            });
            let inbound = InboundEvent {
                envelope_id: format!("wake-{}", Utc::now().timestamp_millis()),
                session_id: task_session_id,
                user: "system".to_string(),
                user_display_name: Some("Wake".to_string()),
                text: task_prompt,
                attachments: Vec::new(),
                dynamic_context: Vec::new(),
                model_definition: None,
                raw,
                is_direct_message: false,
                is_directly_addressed: true,
                link_previews: Vec::new(),
                agent_definition: None,
            };
            if let Err(error) = inbound_sink.send(inbound).await {
                tracing::warn!(%error, "wake timer failed to enqueue inbound event");
            }
        });

        Ok((job_id, next_run_at))
    }
}

async fn emit_wake_event(
    emitter: Option<Arc<OutboundEmitter>>,
    event_type: &'static str,
    job_id: &str,
    session_id: &SessionId,
    stream_id: Option<&str>,
    next_run_at: DateTime<Utc>,
) {
    let Some(emitter) = emitter else { return };
    emitter
        .emit(OutboundEvent::new(
            event_type,
            serde_json::json!({
                "source": "wake",
                "job_id": job_id,
                "session_id": session_id.as_str(),
                "session_stream_id": stream_id,
                "next_run_at": next_run_at.to_rfc3339(),
            }),
        ))
        .await;
}
