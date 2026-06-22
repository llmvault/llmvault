use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{ConfigStore, OutboundChannelSpec};
use mcp::McpRegistry;
use observability::ObservabilityRecorder;
use serde::Serialize;
use skills::SkillWriter;
use storage::{ConfigRepo, EventRepo, SessionRepo};
use tokio::sync::{Notify, RwLock};
use tools::LocalBashOperations;

use crate::question_manager::QuestionManager;
use crate::repos::RepoService;
use crate::session_stream::SessionMessageState;

#[derive(Clone)]
pub struct ApiState {
    pub config_store: ConfigStore,
    pub config_repo: Arc<dyn ConfigRepo>,
    pub session_repo: Arc<dyn SessionRepo>,
    pub event_repo: Arc<dyn EventRepo>,
    pub bearer_token: Arc<RwLock<String>>,
    pub workspace_root: PathBuf,
    pub bash: Arc<LocalBashOperations>,
    pub session_api_ready: Arc<AtomicBool>,
    pub config_loaded: Arc<AtomicBool>,
    pub config_notify: Arc<Notify>,
    pub skill_writer: Arc<SkillWriter>,
    pub session_stream: Option<SessionMessageState>,
    pub question_manager: Option<Arc<QuestionManager>>,
    pub repo_service: Option<Arc<RepoService>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
    pub outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
    pub drain_controller: Option<Arc<dyn DrainController>>,
    pub observability: ObservabilityRecorder,
    pub sentry_enabled: bool,
    pub sentry_dsn_set: bool,
}

#[async_trait]
pub trait OutboundConfigReloader: Send + Sync {
    async fn reload_outbound_channels(
        &self,
        specs: &[OutboundChannelSpec],
        runtime_env: &HashMap<String, String>,
    ) -> anyhow::Result<()>;
}

#[derive(Debug, Clone, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct DrainStatusResponse {
    pub status: String,
    pub draining: bool,
    pub complete: bool,
    pub active_turns: usize,
    pub pending_accepted_messages: usize,
    pub pending_outbox_events: i64,
    pub started_at: Option<DateTime<Utc>>,
    pub completed_at: Option<DateTime<Utc>>,
}

#[async_trait]
pub trait DrainController: Send + Sync {
    fn is_draining(&self) -> bool;
    async fn start(&self) -> anyhow::Result<DrainStatusResponse>;
    async fn status(&self) -> anyhow::Result<DrainStatusResponse>;
    async fn cancel(&self) -> anyhow::Result<DrainStatusResponse>;
}

impl ApiState {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        config_store: ConfigStore,
        config_repo: Arc<dyn ConfigRepo>,
        session_repo: Arc<dyn SessionRepo>,
        event_repo: Arc<dyn EventRepo>,
        bearer_token: String,
        workspace_root: PathBuf,
        bash: Arc<LocalBashOperations>,
        skill_writer: Arc<SkillWriter>,
        session_stream: Option<SessionMessageState>,
        question_manager: Option<Arc<QuestionManager>>,
        repo_service: Option<Arc<RepoService>>,
        mcp_registry: Option<Arc<McpRegistry>>,
        outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
        drain_controller: Option<Arc<dyn DrainController>>,
        sentry_enabled: bool,
        sentry_dsn_set: bool,
    ) -> Self {
        let observability = session_stream
            .as_ref()
            .map(|session_stream| session_stream.broker.observability())
            .unwrap_or_default();
        Self {
            config_store,
            config_repo,
            session_repo,
            event_repo,
            bearer_token: Arc::new(RwLock::new(bearer_token)),
            workspace_root,
            bash,
            session_api_ready: Arc::new(AtomicBool::new(false)),
            config_loaded: Arc::new(AtomicBool::new(false)),
            config_notify: Arc::new(Notify::new()),
            skill_writer,
            session_stream,
            question_manager,
            repo_service,
            mcp_registry,
            outbound_reloader,
            drain_controller,
            observability,
            sentry_enabled,
            sentry_dsn_set,
        }
    }

    pub fn mark_session_api_ready(&self) {
        self.session_api_ready.store(true, Ordering::Release);
    }

    pub fn mark_config_loaded(&self) {
        if !self.config_loaded.swap(true, Ordering::AcqRel) {
            self.config_notify.notify_waiters();
        }
    }

    pub async fn wait_for_config_loaded(&self) {
        loop {
            if self.config_loaded.load(Ordering::Acquire) {
                return;
            }
            let notified = self.config_notify.notified();
            if self.config_loaded.load(Ordering::Acquire) {
                return;
            }
            notified.await;
        }
    }

    pub fn is_ready(&self) -> bool {
        self.session_api_ready.load(Ordering::Acquire) && self.config_loaded.load(Ordering::Acquire)
    }

    pub fn is_draining(&self) -> bool {
        self.drain_controller
            .as_ref()
            .is_some_and(|controller| controller.is_draining())
    }
}
