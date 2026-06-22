use std::sync::Arc;
use std::time::Duration;

use chrono::{Duration as ChronoDuration, Utc};
use domain::OutboundEvent;
use storage::{OutboxRepo, OutboxRow};
use tokio::sync::{oneshot, RwLock, Semaphore};
use tracing::{info, warn};

use crate::OutboundRegistry;

const MAX_RETRY_ATTEMPTS: i32 = 8;
const POLL_INTERVAL: Duration = Duration::from_millis(250);
const CLAIM_BATCH_SIZE: u32 = 32;
const MAX_CONCURRENT_DELIVERIES: usize = 32;

pub struct OutboundDispatcher {
    outbox: Arc<dyn OutboxRepo>,
    registry: Arc<RwLock<OutboundRegistry>>,
    permits: Arc<Semaphore>,
}

impl OutboundDispatcher {
    pub fn new(outbox: Arc<dyn OutboxRepo>, registry: Arc<RwLock<OutboundRegistry>>) -> Self {
        Self {
            outbox,
            registry,
            permits: Arc::new(Semaphore::new(MAX_CONCURRENT_DELIVERIES)),
        }
    }

    pub fn spawn(self) -> (tokio::task::JoinHandle<()>, oneshot::Sender<()>) {
        let (cancel_signal, mut cancel_receiver) = oneshot::channel();
        let handle = tokio::spawn(async move {
            info!("outbound dispatcher started");
            loop {
                tokio::select! {
                    _ = &mut cancel_receiver => {
                        info!("outbound dispatcher shutting down");
                        break;
                    }
                    _ = tokio::time::sleep(POLL_INTERVAL) => {
                        if let Err(error) = self.drain_one_batch().await {
                            warn!(%error, "outbound dispatcher batch failed");
                        }
                    }
                }
            }
        });
        (handle, cancel_signal)
    }

    async fn drain_one_batch(&self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let capacity = self
            .permits
            .available_permits()
            .min(CLAIM_BATCH_SIZE as usize);
        if capacity == 0 {
            return Ok(());
        }

        let due = self.outbox.claim_due(capacity as u32).await?;
        if due.is_empty() {
            return Ok(());
        }
        for row in due {
            let Ok(permit) = self.permits.clone().try_acquire_owned() else {
                let _ = self
                    .outbox
                    .schedule_retry(row.id, row.attempts, Utc::now())
                    .await;
                continue;
            };
            let outbox = self.outbox.clone();
            let registry = self.registry.clone();
            tokio::spawn(async move {
                Self::deliver_one(outbox, registry, row).await;
                drop(permit);
            });
        }
        Ok(())
    }

    async fn deliver_one(
        outbox: Arc<dyn OutboxRepo>,
        registry: Arc<RwLock<OutboundRegistry>>,
        row: OutboxRow,
    ) {
        let registry_snapshot = registry.read().await;
        let channel = registry_snapshot.find(&row.channel_name);
        drop(registry_snapshot);

        let Some(channel) = channel else {
            warn!(channel = %row.channel_name, id = row.id, "channel not registered; marking failed");
            let _ = outbox.mark_failed(row.id).await;
            return;
        };

        let event = OutboundEvent {
            event_type: row.event_type.clone(),
            payload: row.payload.clone(),
            at: Utc::now(),
        };

        match channel.deliver(&event).await {
            Ok(()) => {
                if let Err(error) = outbox.mark_delivered(row.id).await {
                    warn!(%error, id = row.id, "mark_delivered failed");
                }
            }
            Err(error) => {
                warn!(%error, id = row.id, channel = %row.channel_name, "delivery failed");
                let next_attempts = row.attempts + 1;
                if next_attempts >= MAX_RETRY_ATTEMPTS {
                    let _ = outbox.mark_failed(row.id).await;
                    return;
                }
                let backoff_seconds = 2_i64.pow(next_attempts.min(10) as u32) * 5;
                let next_retry_at = Utc::now() + ChronoDuration::seconds(backoff_seconds);
                let _ = outbox
                    .schedule_retry(row.id, next_attempts, next_retry_at)
                    .await;
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::sync::Arc;
    use std::time::Duration;

    use async_trait::async_trait;
    use chrono::{DateTime, Utc};
    use serde_json::json;
    use storage::{OutboxRepo, OutboxRow};
    use tokio::sync::{Mutex, Notify, RwLock};

    use super::*;
    use crate::{OutboundChannel, OutboundRegistry};

    struct FakeOutbox {
        due: Mutex<Vec<OutboxRow>>,
        delivered: Mutex<Vec<i64>>,
        retries: Mutex<Vec<i64>>,
        delivered_notify: Notify,
    }

    impl Default for FakeOutbox {
        fn default() -> Self {
            Self {
                due: Mutex::new(Vec::new()),
                delivered: Mutex::new(Vec::new()),
                retries: Mutex::new(Vec::new()),
                delivered_notify: Notify::new(),
            }
        }
    }

    #[async_trait]
    impl OutboxRepo for FakeOutbox {
        async fn enqueue(
            &self,
            _channel_name: &str,
            _event_type: &str,
            _payload: serde_json::Value,
        ) -> storage::Result<i64> {
            Ok(0)
        }

        async fn claim_due(&self, limit: u32) -> storage::Result<Vec<OutboxRow>> {
            let mut due = self.due.lock().await;
            let count = (limit as usize).min(due.len());
            Ok(due.drain(..count).collect())
        }

        async fn pending_count(&self) -> storage::Result<i64> {
            Ok(self.due.lock().await.len() as i64)
        }

        async fn mark_delivered(&self, id: i64) -> storage::Result<()> {
            self.delivered.lock().await.push(id);
            self.delivered_notify.notify_waiters();
            Ok(())
        }

        async fn schedule_retry(
            &self,
            id: i64,
            _attempts: i32,
            _next_retry_at: DateTime<Utc>,
        ) -> storage::Result<()> {
            self.retries.lock().await.push(id);
            Ok(())
        }

        async fn mark_failed(&self, id: i64) -> storage::Result<()> {
            self.retries.lock().await.push(id);
            Ok(())
        }
    }

    struct SessionDelayChannel {
        delays: HashMap<String, Duration>,
    }

    #[async_trait]
    impl OutboundChannel for SessionDelayChannel {
        fn name(&self) -> &str {
            "runtime-ws"
        }

        fn kind(&self) -> &'static str {
            "websocket"
        }

        fn accepts(&self, _event_type: &str) -> bool {
            true
        }

        async fn deliver(&self, event: &OutboundEvent) -> crate::Result<()> {
            let session_id = event
                .payload
                .get("session_id")
                .and_then(serde_json::Value::as_str)
                .unwrap_or_default();
            if let Some(delay) = self.delays.get(session_id) {
                tokio::time::sleep(*delay).await;
            }
            Ok(())
        }
    }

    fn outbox_row(id: i64, session_id: &str, runtime_seq: i64) -> OutboxRow {
        OutboxRow {
            id,
            channel_name: "runtime-ws".to_string(),
            event_type: "token".to_string(),
            payload: json!({
                "session_id": session_id,
                "runtime_seq": runtime_seq,
                "text": "hello"
            }),
            attempts: 0,
            session_id: Some(session_id.to_string()),
            runtime_seq: Some(runtime_seq),
        }
    }

    #[tokio::test]
    async fn slow_session_delivery_does_not_block_other_session_delivery() {
        let outbox = Arc::new(FakeOutbox::default());
        {
            let mut due = outbox.due.lock().await;
            due.push(outbox_row(1, "stuck-session", 1));
            due.push(outbox_row(2, "ready-session", 1));
        }
        let registry = OutboundRegistry::new().with_channel(Arc::new(SessionDelayChannel {
            delays: HashMap::from([("stuck-session".to_string(), Duration::from_millis(200))]),
        }));
        let dispatcher = OutboundDispatcher::new(outbox.clone(), Arc::new(RwLock::new(registry)));

        dispatcher.drain_one_batch().await.expect("drain one batch");

        tokio::time::timeout(Duration::from_millis(75), async {
            loop {
                let notified = outbox.delivered_notify.notified();
                if outbox.delivered.lock().await.contains(&2) {
                    break;
                }
                notified.await;
            }
        })
        .await
        .expect("ready session delivered before stuck session completed");

        let delivered = outbox.delivered.lock().await;
        assert!(delivered.contains(&2));
        assert!(!delivered.contains(&1));
    }
}
