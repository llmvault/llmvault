use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use anyhow::Result;
use api::{DrainController, DrainStatusResponse};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::InboundEvent;
use outbound::OutboundEmitter;
use storage::{OutboxRepo, SqliteStore};
use tokio::sync::{mpsc, Mutex};

use crate::session_coordinator::SessionCoordinator;

#[derive(Default)]
struct DrainLifecycle {
    started_at: Option<DateTime<Utc>>,
    completed_at: Option<DateTime<Utc>>,
}

pub struct RuntimeDrainController {
    draining: AtomicBool,
    lifecycle: Mutex<DrainLifecycle>,
    coordinator: Arc<SessionCoordinator>,
    inbound_sink: mpsc::Sender<InboundEvent>,
    emitter: Arc<OutboundEmitter>,
    outbox: Arc<dyn OutboxRepo>,
    sqlite_store: SqliteStore,
}

impl RuntimeDrainController {
    pub fn new(
        coordinator: Arc<SessionCoordinator>,
        inbound_sink: mpsc::Sender<InboundEvent>,
        emitter: Arc<OutboundEmitter>,
        outbox: Arc<dyn OutboxRepo>,
        sqlite_store: SqliteStore,
    ) -> Self {
        Self {
            draining: AtomicBool::new(false),
            lifecycle: Mutex::new(DrainLifecycle::default()),
            coordinator,
            inbound_sink,
            emitter,
            outbox,
            sqlite_store,
        }
    }

    async fn snapshot(&self, flush: bool) -> Result<DrainStatusResponse> {
        if flush && self.is_draining() {
            self.emitter.flush_for_drain().await;
            self.sqlite_store.flush_writes().await?;
        }

        let active_turns = self.coordinator.active_turn_count();
        let pending_accepted_messages =
            self.coordinator.queued_message_count() + self.inbound_channel_backlog();
        let pending_outbox_events = self.outbox.pending_count().await?;
        let draining = self.is_draining();
        let complete = draining
            && active_turns == 0
            && pending_accepted_messages == 0
            && pending_outbox_events == 0;

        let mut lifecycle = self.lifecycle.lock().await;
        if complete && lifecycle.completed_at.is_none() {
            lifecycle.completed_at = Some(Utc::now());
        }
        if !draining {
            lifecycle.completed_at = None;
        }
        let status = if lifecycle.completed_at.is_some() {
            "drained"
        } else if draining {
            "draining"
        } else {
            "running"
        };

        Ok(DrainStatusResponse {
            status: status.to_string(),
            draining,
            complete: lifecycle.completed_at.is_some(),
            active_turns,
            pending_accepted_messages,
            pending_outbox_events,
            started_at: lifecycle.started_at,
            completed_at: lifecycle.completed_at,
        })
    }

    fn inbound_channel_backlog(&self) -> usize {
        self.inbound_sink
            .max_capacity()
            .saturating_sub(self.inbound_sink.capacity())
    }
}

#[async_trait]
impl DrainController for RuntimeDrainController {
    fn is_draining(&self) -> bool {
        self.draining.load(Ordering::Acquire)
    }

    async fn start(&self) -> Result<DrainStatusResponse> {
        if !self.draining.swap(true, Ordering::AcqRel) {
            let mut lifecycle = self.lifecycle.lock().await;
            lifecycle.started_at = Some(Utc::now());
            lifecycle.completed_at = None;
        }
        self.snapshot(true).await
    }

    async fn status(&self) -> Result<DrainStatusResponse> {
        self.snapshot(true).await
    }

    async fn cancel(&self) -> Result<DrainStatusResponse> {
        self.draining.store(false, Ordering::Release);
        {
            let mut lifecycle = self.lifecycle.lock().await;
            lifecycle.completed_at = None;
        }
        self.snapshot(false).await
    }
}
