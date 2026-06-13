use chrono::Utc;
use domain::{event_types, InboundEvent, OutboundEvent, Session, SessionStatus};
use outbound::OutboundEmitter;
use storage::SessionRepo;
use tracing::warn;

pub async fn ensure_session_persisted(
    session_repo: &dyn SessionRepo,
    inbound: &InboundEvent,
    emitter: &OutboundEmitter,
) -> bool {
    match session_repo.get(&inbound.session_id).await {
        Ok(Some(_)) => false,
        Ok(None) => {
            let now = Utc::now();
            let session = Session {
                id: inbound.session_id.clone(),
                status: SessionStatus::Active,
                created_at: now,
                last_activity_at: now,
            };
            if let Err(error) = session_repo.create(&session).await {
                warn!(session = %inbound.session_id, %error, "session_repo create failed");
                return false;
            }
            emitter
                .emit(OutboundEvent::new(
                    event_types::SESSION_CREATED,
                    serde_json::json!({
                        "session_id": inbound.session_id.as_str(),
                        "is_direct_message": inbound.is_direct_message,
                    }),
                ))
                .await;
            true
        }
        Err(error) => {
            warn!(session = %inbound.session_id, %error, "session_repo get failed");
            false
        }
    }
}
