use std::sync::Arc;

use agent::PlanUpdater;
use async_trait::async_trait;
use chrono::Utc;
use domain::{
    event_types, validate_update_plan_payload, OutboundEvent, SessionId, UpdatePlanPayload,
    UpdatePlanResult,
};
use nanoid::nanoid;
use outbound::OutboundEmitter;

use crate::session_stream::SessionStreamBroker;

pub struct PlanManager {
    broker: Arc<SessionStreamBroker>,
    outbound: Option<Arc<OutboundEmitter>>,
}

impl PlanManager {
    pub fn new(broker: Arc<SessionStreamBroker>) -> Self {
        Self {
            broker,
            outbound: None,
        }
    }

    pub fn with_outbound_emitter(mut self, outbound: Arc<OutboundEmitter>) -> Self {
        self.outbound = Some(outbound);
        self
    }

    async fn publish_updated(&self, session_id: &SessionId, payload: &UpdatePlanPayload) {
        if let Some(stream_id) = self.broker.stream_id_for_session(session_id.as_str()).await {
            let stream_payload = serde_json::json!({
                "session_id": session_id.as_str(),
                "stream_id": &stream_id,
                "explanation": &payload.explanation,
                "plan": &payload.plan,
            });
            self.broker
                .publish(&stream_id, "plan_updated", stream_payload.clone())
                .await;
            self.emit_outbound(event_types::PLAN_UPDATED, stream_payload)
                .await;
        }
    }

    async fn emit_outbound(&self, event_type: &str, mut payload: serde_json::Value) {
        let Some(outbound) = self.outbound.as_ref() else {
            return;
        };
        if let Some(map) = payload.as_object_mut() {
            map.insert("event_name".to_string(), serde_json::json!(event_type));
            map.entry("event_id".to_string())
                .or_insert_with(|| serde_json::json!(format!("evt_rt_{}", nanoid!(16))));
            map.entry("sequence".to_string())
                .or_insert_with(|| serde_json::json!(0));
            map.entry("occurred_at".to_string())
                .or_insert_with(|| serde_json::json!(Utc::now()));
            map.entry("scope".to_string())
                .or_insert_with(|| serde_json::json!("main"));
        }
        outbound.emit(OutboundEvent::new(event_type, payload)).await;
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
