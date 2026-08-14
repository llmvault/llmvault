use std::collections::{BTreeSet, HashMap};
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{ConfigStore, OutboundChannelSpec};
use mcp::McpRegistry;
use observability::ObservabilityRecorder;
use serde::Serialize;
use storage::{ConfigRepo, ConfigSnapshot, EventRepo, SessionRepo};
use tokio::sync::{Mutex, Notify, RwLock};
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
    pub session_stream: Option<SessionMessageState>,
    pub question_manager: Option<Arc<QuestionManager>>,
    pub repo_service: Option<Arc<RepoService>>,
    pub canvas_sessions: Arc<RwLock<BTreeSet<String>>>,
    pub canvas_source_session: Arc<RwLock<Option<String>>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
    pub outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
    pub drain_controller: Option<Arc<dyn DrainController>>,
    pub observability: ObservabilityRecorder,
    pub sentry_enabled: bool,
    pub sentry_dsn_set: bool,
    pub desktop_mode: bool,
    pub desktop_agent_configs: Arc<RwLock<HashMap<String, (u64, ConfigSnapshot)>>>,
    pub desktop_active_agent: Arc<RwLock<Option<(String, u64)>>>,
    pub desktop_config_gate: Arc<Mutex<()>>,
    pub desktop_config_generation: Arc<AtomicU64>,
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
        session_stream: Option<SessionMessageState>,
        question_manager: Option<Arc<QuestionManager>>,
        repo_service: Option<Arc<RepoService>>,
        mcp_registry: Option<Arc<McpRegistry>>,
        outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
        drain_controller: Option<Arc<dyn DrainController>>,
        sentry_enabled: bool,
        sentry_dsn_set: bool,
        desktop_mode: bool,
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
            session_stream,
            question_manager,
            repo_service,
            canvas_sessions: Arc::new(RwLock::new(BTreeSet::new())),
            canvas_source_session: Arc::new(RwLock::new(None)),
            mcp_registry,
            outbound_reloader,
            drain_controller,
            observability,
            sentry_enabled,
            sentry_dsn_set,
            desktop_mode,
            desktop_agent_configs: Arc::new(RwLock::new(HashMap::new())),
            desktop_active_agent: Arc::new(RwLock::new(None)),
            desktop_config_gate: Arc::new(Mutex::new(())),
            desktop_config_generation: Arc::new(AtomicU64::new(0)),
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
