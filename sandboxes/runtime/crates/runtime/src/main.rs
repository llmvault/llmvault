mod activity;
mod canvas;
mod drain;
mod handler;
mod sentry_support;
mod session_coordinator;
mod subagent_worker;

use session_coordinator::SessionCoordinator;
use subagent_worker::SubagentWorker;

use std::collections::HashMap;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Instant;

use activity::RuntimeActivityReporter;
use agent::{AgentRunner, RigAgentRunner};
use anyhow::{Context, Result};
use api::{ApiState, OutboundConfigReloader};
use async_trait::async_trait;
use domain::{
    AgentDefinition, AgentMeta, ConfigStore, InboundEvent, ModelConfig, OutboundChannelSpec,
    ReasoningEffort, StaticPromptSegment, SystemPromptConfig, SystemPromptSegment,
};
use mcp::McpRegistry;
use outbound::{
    build_registry_with_env, DatabaseEventQueue, OutboundDispatcher, OutboundEmitter,
    OutboundRegistry, StreamBatcher, DATABASE_BATCH_FLUSH_INTERVAL, DATABASE_BATCH_MAX_BYTES,
    DATABASE_BATCH_MAX_EVENTS,
};
use storage::{
    init_sqlite_store, SqliteConfigRepo, SqliteEventRepo, SqliteInboundDedupeRepo,
    SqliteOutboxRepo, SqliteQuestionRequestRepo, SqliteSessionRepo, SqliteSubagentTaskRepo,
};
use tokio::sync::{mpsc, RwLock};
use tools::LocalBashOperations;
use tracing::{error, info, warn};

#[tokio::main]
async fn main() -> Result<()> {
    let startup_started = Instant::now();
    let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
    let _ = dotenvy::dotenv();

    let sentry_guard = sentry_support::init_sentry();
    let sentry_enabled = sentry_guard.is_some();
    let sentry_dsn_set = std::env::var("SENTRY_DSN")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .is_some();
    sentry_support::init_tracing(sentry_enabled);
    if sentry_enabled {
        sentry::add_breadcrumb(sentry::Breadcrumb {
            ty: "default".into(),
            category: Some("runtime.startup".into()),
            message: Some("sentry reporting enabled".into()),
            level: sentry::Level::Info,
            ..Default::default()
        });
        info!("sentry reporting enabled");
    } else {
        info!("sentry reporting disabled; set SENTRY_DSN or SENTRY_SPOTLIGHT=true to enable");
    }
    let mut phase_started =
        log_runtime_startup_phase("initialize process", startup_started, startup_started);

    let runtime_env: HashMap<String, String> = std::env::vars().collect();
    let activity_reporter = RuntimeActivityReporter::from_env(&runtime_env);
    let runtime_secret =
        required_runtime_env(&runtime_env, "HIVY_RUNTIME_SECRET", "runtime bearer token")?;
    let bind_addr_text = runtime_env
        .get("HIVY_RUNTIME_BIND_ADDR")
        .cloned()
        .unwrap_or_else(|| "0.0.0.0:7080".into());
    let bind_addr: SocketAddr = bind_addr_text
        .parse()
        .context("HIVY_RUNTIME_BIND_ADDR must be a socket address")?;
    phase_started = log_runtime_startup_phase("read runtime env", phase_started, startup_started);
    let database_path = runtime_env
        .get("HIVY_DB_PATH")
        .cloned()
        .unwrap_or_else(|| "./data/hivy-sandboxes-runtime.db".into());
    let workspace_root_string = runtime_env
        .get("HIVY_WORKSPACE_ROOT")
        .cloned()
        .unwrap_or_else(|| "./workspace".into());
    let workspace_root = PathBuf::from(&workspace_root_string);
    if let Err(error) = std::fs::create_dir_all(&workspace_root) {
        warn!(workspace = %workspace_root.display(), %error, "failed to create workspace root");
    }
    info!(workspace = %workspace_root.display(), "workspace ready");
    phase_started = log_runtime_startup_phase("prepare workspace", phase_started, startup_started);
    info!(database = %database_path, "initializing storage");
    let database_path = PathBuf::from(&database_path);
    let sqlite_store = init_sqlite_store(database_path.clone()).await?;
    let config_repo: Arc<dyn storage::ConfigRepo> = Arc::new(SqliteConfigRepo::new(&sqlite_store));
    let session_repo: Arc<dyn storage::SessionRepo> =
        Arc::new(SqliteSessionRepo::new(&sqlite_store));
    let event_repo: Arc<dyn storage::EventRepo> = Arc::new(SqliteEventRepo::new(&sqlite_store));
    let outbox_repo: Arc<dyn storage::OutboxRepo> = Arc::new(SqliteOutboxRepo::new(&sqlite_store));
    let _dedupe_repo: Arc<dyn storage::InboundDedupeRepo> =
        Arc::new(SqliteInboundDedupeRepo::new(&sqlite_store));
    let subagent_task_repo: Arc<dyn storage::SubagentTaskRepo> =
        Arc::new(SqliteSubagentTaskRepo::new(&sqlite_store));
    let question_request_repo: Arc<dyn storage::QuestionRequestRepo> =
        Arc::new(SqliteQuestionRequestRepo::new(&sqlite_store));
    phase_started = log_runtime_startup_phase("initialize storage", phase_started, startup_started);

    let (initial_definition, initial_runtime_env, initial_workspace, config_loaded_from_database) =
        match config_repo.load().await? {
            Some(persisted) => {
                info!("loaded agent config snapshot from database");
                let merged_runtime_env =
                    merge_persisted_runtime_env(runtime_env, persisted.runtime_env);
                (
                    persisted.definition,
                    merged_runtime_env,
                    persisted.workspace,
                    true,
                )
            }
            None => {
                info!("no persisted definition; waiting for first control-plane config push");
                (
                    bootstrap_agent_definition(),
                    runtime_env,
                    domain::WorkspaceConfig::default(),
                    false,
                )
            }
        };
    phase_started =
        log_runtime_startup_phase("load persisted config", phase_started, startup_started);

    let config = ConfigStore::with_runtime_env(initial_definition.clone(), initial_runtime_env);
    let initial_runtime_env = config.runtime_env();
    let mcp_registry = Arc::new(
        McpRegistry::from_specs(
            &initial_definition.mcp_servers,
            &initial_runtime_env,
            workspace_root.clone(),
        )
        .await,
    );
    phase_started =
        log_runtime_startup_phase("initialize mcp registry", phase_started, startup_started);
    let registry =
        build_registry_with_env(&initial_definition.outbound_channels, &initial_runtime_env)
            .map_err(|e| anyhow::anyhow!("build outbound registry: {e}"))?;
    let registry = Arc::new(RwLock::new(registry));
    let stream_batcher = Arc::new(RwLock::new(None));
    let database_event_queue = DatabaseEventQueue::new(sqlite_store.writer());
    let emitter = Arc::new(
        OutboundEmitter::new(outbox_repo.clone(), registry.clone())
            .with_stream_batcher(stream_batcher.clone())
            .with_database_queue(database_event_queue.clone()),
    );
    let outbound_reloader: Arc<dyn OutboundConfigReloader> = Arc::new(RegistryReloader {
        registry: registry.clone(),
        stream_batcher: stream_batcher.clone(),
        database_event_queue: database_event_queue.clone(),
    });
    phase_started = log_runtime_startup_phase(
        "initialize outbound registry",
        phase_started,
        startup_started,
    );

    if config_loaded_from_database {
        info!(
            agent = %initial_definition.agent.name,
            persisted_mcp_servers = initial_definition.mcp_servers.len(),
            persisted_outbound_channels = initial_definition.outbound_channels.len(),
            "persisted control-plane config loaded; starting agent runtime services"
        );
    } else {
        info!(
            agent = %initial_definition.agent.name,
            "waiting for first control-plane config push before starting agent runtime services"
        );
    }

    let session_stream_broker = Arc::new(api::SessionStreamBroker::new());
    let repo_service = Arc::new(api::RepoService::new(
        workspace_root.clone(),
        session_stream_broker.clone(),
    ));
    repo_service.clone().start();
    if config_loaded_from_database {
        repo_service.apply_workspace_config(initial_workspace).await;
    }
    phase_started = log_runtime_startup_phase("start repo service", phase_started, startup_started);
    let plan_manager = Arc::new(
        api::PlanManager::new(session_stream_broker.clone()).with_outbound_emitter(emitter.clone()),
    );
    let question_manager = Arc::new(
        api::QuestionManager::new(question_request_repo.clone(), session_stream_broker.clone())
            .with_outbound_emitter(emitter.clone()),
    );
    let (inbound_sink, mut inbound_events) = mpsc::channel::<InboundEvent>(256);
    let attachment_downloader: Arc<dyn handler::AttachmentDownloader> =
        Arc::new(handler::HttpAttachmentDownloader::new());
    let coordinator = Arc::new(SessionCoordinator::new());
    let drain_controller = Arc::new(drain::RuntimeDrainController::new(
        coordinator.clone(),
        inbound_sink.clone(),
        emitter.clone(),
        outbox_repo.clone(),
        sqlite_store.clone(),
    ));

    let api_state = ApiState::new(
        config.clone(),
        config_repo.clone(),
        session_repo.clone(),
        event_repo.clone(),
        runtime_secret,
        workspace_root.clone(),
        Arc::new(LocalBashOperations),
        Some(api::SessionMessageState {
            inbound_sink: inbound_sink.clone(),
            broker: session_stream_broker.clone(),
            interrupter: Some(coordinator.clone()),
        }),
        Some(question_manager.clone()),
        Some(repo_service.clone()),
        Some(mcp_registry.clone()),
        Some(outbound_reloader),
        Some(drain_controller),
        sentry_enabled,
        sentry_dsn_set,
    );
    phase_started = log_runtime_startup_phase("build api state", phase_started, startup_started);
    let (api_handle, api_cancel) = api::serve(bind_addr, api_state.clone()).await;
    api_state.mark_session_api_ready();
    if config_loaded_from_database {
        api_state.mark_config_loaded();
    }
    phase_started =
        log_runtime_startup_phase("bind control-plane http", phase_started, startup_started);
    let canvas_runtime_env = config.runtime_env();
    let canvas_service = Arc::new(canvas::CanvasRuntimeService::new(
        workspace_root.clone(),
        canvas_runtime_env.as_ref(),
        session_stream_broker.clone(),
        api_state.canvas_sessions.clone(),
        api_state.canvas_source_session.clone(),
    ));
    canvas_service.start();
    phase_started =
        log_runtime_startup_phase("start canvas service", phase_started, startup_started);

    api_state.wait_for_config_loaded().await;
    phase_started = log_runtime_startup_phase("wait first config", phase_started, startup_started);
    let active_definition = config.snapshot();
    info!(
        agent = %active_definition.agent.name,
        mcp_servers = active_definition.mcp_servers.len(),
        outbound_channels = active_definition.outbound_channels.len(),
        "first control-plane config loaded; starting agent runtime services"
    );

    let dispatcher = OutboundDispatcher::new(outbox_repo.clone(), registry.clone());
    let (dispatcher_handle, dispatcher_cancel) = dispatcher.spawn();
    phase_started =
        log_runtime_startup_phase("start outbound dispatcher", phase_started, startup_started);
    info!(
        database_channel = "queued",
        db_flush_max_events = DATABASE_BATCH_MAX_EVENTS,
        db_flush_max_bytes = DATABASE_BATCH_MAX_BYTES,
        db_flush_interval_ms = DATABASE_BATCH_FLUSH_INTERVAL.as_millis(),
        "database event queue enabled"
    );
    let _database_event_queue_handle = database_event_queue.clone().spawn();
    phase_started =
        log_runtime_startup_phase("start database queue", phase_started, startup_started);
    let rig_runner = RigAgentRunner::new(config.clone(), workspace_root.clone())
        .with_outbound_emitter(emitter.clone())
        .with_subagent_task_repo(subagent_task_repo.clone())
        .with_plan_updater(plan_manager.clone())
        .with_question_requester(question_manager.clone())
        .with_event_repo(event_repo.clone())
        .with_mcp_registry(mcp_registry.clone());
    let agent_runner: Arc<dyn AgentRunner> = Arc::new(rig_runner);

    let subagent_worker = SubagentWorker::new(
        subagent_task_repo.clone(),
        config.clone(),
        inbound_sink.clone(),
        session_stream_broker.clone(),
        emitter.clone(),
    );
    let _subagent_worker_handle = tokio::spawn(subagent_worker.run());
    let _ = log_runtime_startup_phase("start subagent worker", phase_started, startup_started);

    let event_loop = async {
        info!("listening for inbound events");
        while let Some(inbound) = inbound_events.recv().await {
            let runner = agent_runner.clone();
            let attachment_downloader = attachment_downloader.clone();
            let cfg = config.clone();
            let emitter = emitter.clone();
            let session_repo = session_repo.clone();
            let coordinator = coordinator.clone();
            let turn_event_sink: Arc<dyn handler::TurnEventSink> = session_stream_broker.clone();
            let subagent_task_repo = subagent_task_repo.clone();
            let inbound_sink = inbound_sink.clone();
            let activity_reporter = activity_reporter.clone();
            let canvas_source_session = api_state.canvas_source_session.clone();
            // Capture the session id before moving `inbound` into the task so a
            // panicking turn can be cleaned up from the coordinator.
            let session_id = inbound.session_id.clone();
            let panic_guard_coordinator = coordinator.clone();
            tokio::spawn(async move {
                use futures::FutureExt;
                let result = std::panic::AssertUnwindSafe(handler::handle_inbound(
                    runner,
                    attachment_downloader,
                    cfg,
                    emitter,
                    session_repo,
                    coordinator,
                    turn_event_sink,
                    subagent_task_repo,
                    inbound_sink,
                    inbound,
                    activity_reporter,
                    canvas_source_session,
                ))
                .catch_unwind()
                .await;
                match result {
                    Ok(Ok(())) => {}
                    Ok(Err(e)) => {
                        error!(error = %e, "handler::handle_inbound failed");
                        sentry::with_scope(
                            |scope| scope.set_extra("internal_error", e.to_string().into()),
                            || {
                                sentry::capture_message(
                                    "handler::handle_inbound failed",
                                    sentry::Level::Error,
                                )
                            },
                        );
                    }
                    Err(panic) => {
                        // The turn task panicked. Release the coordinator entry so
                        // future inbound messages for this session are not queued
                        // forever behind a turn that will never call finish_turn.
                        panic_guard_coordinator.finish_active_turn(&session_id);
                        panic_guard_coordinator.finish_turn(&session_id);
                        let panic_msg = panic
                            .downcast_ref::<&str>()
                            .map(|s| s.to_string())
                            .or_else(|| panic.downcast_ref::<String>().cloned())
                            .unwrap_or_else(|| "unknown panic".to_string());
                        error!(
                            session_id = %session_id.as_str(),
                            panic = %panic_msg,
                            "handler::handle_inbound panicked; released session coordinator entry"
                        );
                        sentry::with_scope(
                            |scope| scope.set_extra("panic", panic_msg.into()),
                            || {
                                sentry::capture_message(
                                    "handler::handle_inbound panicked",
                                    sentry::Level::Error,
                                )
                            },
                        );
                    }
                }
            });
        }
    };

    tokio::select! {
        _ = event_loop => warn!("event loop exited"),
        _ = tokio::signal::ctrl_c() => info!("ctrl-c received; shutting down"),
    }

    let _ = dispatcher_cancel.send(());
    let _ = dispatcher_handle.await;
    if let Err(error) = database_event_queue.flush().await {
        warn!(%error, "database event queue final flush failed");
        sentry::with_scope(
            |scope| scope.set_extra("internal_error", error.to_string().into()),
            || {
                sentry::capture_message(
                    "database event queue final flush failed",
                    sentry::Level::Error,
                )
            },
        );
    }
    if let Err(error) = sqlite_store.flush_writes().await {
        warn!(%error, "sqlite write gateway final flush failed");
        sentry::with_scope(
            |scope| scope.set_extra("internal_error", error.to_string().into()),
            || {
                sentry::capture_message(
                    "sqlite write gateway final flush failed",
                    sentry::Level::Error,
                )
            },
        );
    }
    let _ = api_cancel.send(());
    let _ = api_handle.await;
    drop(sentry_guard);
    Ok(())
}

struct RegistryReloader {
    registry: Arc<RwLock<OutboundRegistry>>,
    stream_batcher: Arc<RwLock<Option<Arc<StreamBatcher>>>>,
    database_event_queue: Arc<DatabaseEventQueue>,
}

#[async_trait]
impl OutboundConfigReloader for RegistryReloader {
    async fn reload_outbound_channels(
        &self,
        specs: &[OutboundChannelSpec],
        runtime_env: &HashMap<String, String>,
    ) -> anyhow::Result<()> {
        let next = build_registry_with_env(specs, runtime_env)
            .map_err(|error| anyhow::anyhow!("build outbound registry: {error}"))?;
        let names = next.names();
        let next_batcher = StreamBatcher::from_specs(specs, runtime_env)
            .map_err(|error| anyhow::anyhow!("build stream batcher: {error}"))?
            .map(|b| b.with_requeue(self.database_event_queue.clone()));
        *self.registry.write().await = next;
        *self.stream_batcher.write().await = next_batcher;
        info!(channels = ?names, "outbound registry reloaded from config");
        Ok(())
    }
}

fn required_runtime_env(env: &HashMap<String, String>, key: &str, hint: &str) -> Result<String> {
    match env.get(key) {
        Some(value) if !value.is_empty() => Ok(value.clone()),
        _ => anyhow::bail!("env var `{key}` must be set ({hint})"),
    }
}

fn log_runtime_startup_phase(
    phase: &'static str,
    phase_started: Instant,
    total_started: Instant,
) -> Instant {
    info!(
        phase,
        duration_ms = phase_started.elapsed().as_millis(),
        total_ms = total_started.elapsed().as_millis(),
        "runtime startup phase"
    );
    Instant::now()
}

fn merge_persisted_runtime_env(
    mut process_env: HashMap<String, String>,
    persisted_env: HashMap<String, String>,
) -> HashMap<String, String> {
    process_env.extend(persisted_env);
    process_env
}

fn bootstrap_agent_definition() -> AgentDefinition {
    AgentDefinition {
        agent: AgentMeta {
            name: "Aria".into(),
            description: "Hivy AI agent".into(),
        },
        system_prompt: bootstrap_system_prompt(),
        model: placeholder_model(),
        limits: Default::default(),
        context: Default::default(),
        tools: None,
        mcp_servers: Vec::new(),
        mcp_tool_filter: None,
        outbound_channels: Vec::new(),
        sub_agents: Default::default(),
        safety: Default::default(),
        auto_load_skills: Default::default(),
    }
}

fn placeholder_model() -> ModelConfig {
    ModelConfig::OpenaiCompatible {
        base_url: "http://127.0.0.1/unused".into(),
        model_id: "unclaimed-runtime-placeholder".into(),
        canonical_model_id: Some("unclaimed-runtime-placeholder".into()),
        provider_id: Some("atlascloud".into()),
        upstream_model_id: Some("unclaimed-runtime-placeholder".into()),
        model_profile: None,
        provider_options: Default::default(),
        capabilities: None,
        api_key_env: "HIVY_PROXY_API_KEY".into(),
        temperature: Some(0.3),
        max_output_tokens: Some(1024),
        reasoning_effort: Some(ReasoningEffort::Low),
        extra_headers: Default::default(),
        fallback: None,
    }
}

fn bootstrap_system_prompt() -> SystemPromptConfig {
    SystemPromptConfig {
        cacheable_segments: vec![SystemPromptSegment::StaticText(StaticPromptSegment {
            title: String::new(),
            content: "You are Aria, a friendly AI agent. Reply concisely. Use search_sessions for recent local conversation context and search_knowledge_base for indexed company knowledge when past context would materially improve the answer. Never invent features. If you do not know something, say so.".into(),
        })],
        dynamic_segments: vec![],
    }
}

#[cfg(test)]
mod tests {
    use super::merge_persisted_runtime_env;
    use std::collections::HashMap;

    #[test]
    fn persisted_runtime_env_overrides_process_env() {
        let process_env = HashMap::from([
            ("HIVY_PROXY_API_KEY".to_string(), "ptok_startup".to_string()),
            (
                "HIVY_RUNTIME_SECRET".to_string(),
                "runtime-secret".to_string(),
            ),
        ]);
        let persisted_env =
            HashMap::from([("HIVY_PROXY_API_KEY".to_string(), "ptok_config".to_string())]);

        let merged = merge_persisted_runtime_env(process_env, persisted_env);

        assert_eq!(
            merged.get("HIVY_PROXY_API_KEY"),
            Some(&"ptok_config".to_string())
        );
        assert_eq!(
            merged.get("HIVY_RUNTIME_SECRET"),
            Some(&"runtime-secret".to_string())
        );
    }
}
