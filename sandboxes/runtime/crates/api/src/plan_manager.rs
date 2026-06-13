use std::sync::Arc;

use agent::PlanUpdater;
use async_trait::async_trait;
use domain::{validate_update_plan_payload, SessionId, UpdatePlanPayload, UpdatePlanResult};

use crate::session_stream::SessionStreamBroker;

pub struct PlanManager {
    broker: Arc<SessionStreamBroker>,
}

impl PlanManager {
    pub fn new(broker: Arc<SessionStreamBroker>) -> Self {
        Self { broker }
    }

    async fn publish_updated(&self, session_id: &SessionId, payload: &UpdatePlanPayload) {
        if let Some(stream_id) = self.broker.stream_id_for_session(session_id.as_str()).await {
            self.broker
                .publish(
                    &stream_id,
                    "plan_updated",
                    serde_json::json!({
                        "session_id": session_id.as_str(),
                        "explanation": &payload.explanation,
                        "plan": &payload.plan,
                    }),
                )
                .await;
        }
    }
}

#[async_trait]
impl PlanUpdater for PlanManager {
    async fn update_plan(
        &self,
        session_id: &SessionId,
        payload: UpdatePlanPayload,
    ) -> anyhow::Result<UpdatePlanResult> {
        validate_update_plan_payload(&payload)
            .map_err(|error| anyhow::anyhow!("invalid update_plan arguments: {error}"))?;
        self.publish_updated(session_id, &payload).await;
        Ok(UpdatePlanResult {
            ok: true,
            explanation: payload.explanation,
            plan: payload.plan,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use domain::{PlanItem, PlanItemStatus};

    fn plan_payload() -> UpdatePlanPayload {
        UpdatePlanPayload {
            explanation: Some("Starting work".to_string()),
            plan: vec![
                PlanItem {
                    step: "Inspect runtime".to_string(),
                    status: PlanItemStatus::Completed,
                },
                PlanItem {
                    step: "Add tests".to_string(),
                    status: PlanItemStatus::InProgress,
                },
            ],
        }
    }

    #[tokio::test]
    async fn update_plan_publishes_exact_session_stream_event() {
        let broker = Arc::new(SessionStreamBroker::new());
        let manager = PlanManager::new(broker.clone());
        let session_id = SessionId::from("plan-session");
        let stream_id = broker.create_stream().await;
        broker
            .register_session(session_id.as_str(), &stream_id)
            .await;
        let (_history, mut stream) = broker.subscribe(&stream_id).await.expect("stream");

        let result = manager
            .update_plan(&session_id, plan_payload())
            .await
            .expect("update plan");

        assert!(result.ok);
        assert_eq!(result.plan[1].status, PlanItemStatus::InProgress);
        let event = tokio::time::timeout(std::time::Duration::from_secs(1), stream.recv())
            .await
            .expect("plan event")
            .expect("stream event");
        assert_eq!(event.event, "plan_updated");
        assert_eq!(event.payload["session_id"], "plan-session");
        assert_eq!(event.payload["explanation"], "Starting work");
        assert_eq!(event.payload["plan"][0]["step"], "Inspect runtime");
        assert_eq!(event.payload["plan"][1]["status"], "in_progress");
    }
}
