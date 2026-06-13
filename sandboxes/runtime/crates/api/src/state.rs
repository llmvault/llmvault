use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use async_trait::async_trait;
use domain::{ConfigStore, OutboundChannelSpec};
use mcp::McpRegistry;
use observability::ObservabilityRecorder;
use skills::SkillWriter;
use storage::{ConfigRepo, CronJobRepo, EventRepo, SessionRepo};
use tokio::sync::{Notify, RwLock};
use tools::LocalBashOperations;

use crate::question_manager::QuestionManager;
use crate::session_stream::SessionMessageState;

#[derive(Clone)]
pub struct ApiState {
    pub config_store: ConfigStore,
    pub config_repo: Arc<dyn ConfigRepo>,
    pub session_repo: Arc<dyn SessionRepo>,
    pub event_repo: Arc<dyn EventRepo>,
    pub cron_repo: Arc<dyn CronJobRepo>,
    pub bearer_token: Arc<RwLock<String>>,
    pub stream_token: Arc<RwLock<Option<String>>>,
    pub workspace_root: PathBuf,
    pub bash: Arc<LocalBashOperations>,
    pub session_api_ready: Arc<AtomicBool>,
    pub config_loaded: Arc<AtomicBool>,
    pub config_notify: Arc<Notify>,
    pub skill_writer: Arc<SkillWriter>,
    pub session_stream: Option<SessionMessageState>,
    pub question_manager: Option<Arc<QuestionManager>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
    pub outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
    pub observability: ObservabilityRecorder,
    pub sentry_enabled: bool,
    pub sentry_dsn_set: bool,
}

#[async_trait]
pub trait OutboundConfigReloader: Send + Sync {
    async fn reload_outbound_channels(&self, specs: &[OutboundChannelSpec]) -> anyhow::Result<()>;
}

impl ApiState {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        config_store: ConfigStore,
        config_repo: Arc<dyn ConfigRepo>,
        session_repo: Arc<dyn SessionRepo>,
        event_repo: Arc<dyn EventRepo>,
        cron_repo: Arc<dyn CronJobRepo>,
        bearer_token: String,
        stream_token: Option<String>,
        workspace_root: PathBuf,
        bash: Arc<LocalBashOperations>,
        skill_writer: Arc<SkillWriter>,
        session_stream: Option<SessionMessageState>,
        question_manager: Option<Arc<QuestionManager>>,
        mcp_registry: Option<Arc<McpRegistry>>,
        outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
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
            cron_repo,
            bearer_token: Arc::new(RwLock::new(bearer_token)),
            stream_token: Arc::new(RwLock::new(
                stream_token.filter(|token| !token.trim().is_empty()),
            )),
            workspace_root,
            bash,
            session_api_ready: Arc::new(AtomicBool::new(false)),
            config_loaded: Arc::new(AtomicBool::new(false)),
            config_notify: Arc::new(Notify::new()),
            skill_writer,
            session_stream,
            question_manager,
            mcp_registry,
            outbound_reloader,
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
}
