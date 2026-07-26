mod file_inputs;
mod legacy_sse;
mod materialize;
mod ssrf;

use std::collections::{HashMap, HashSet};
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, RwLock};
use std::time::Duration;

use arc_swap::ArcSwap;
use base64::prelude::{Engine as _, BASE64_URL_SAFE_NO_PAD};
use dashmap::DashMap;
use domain::{McpSpec, ToolFilter, ToolInputBinding};
use futures::{stream, StreamExt};
use http::{HeaderName, HeaderValue};
use rmcp::{
    model::{
        CallToolRequestParams, ClientCapabilities, ClientInfo, Implementation, JsonObject,
        ProtocolVersion,
    },
    service::{MaybeSendFuture, NotificationContext, RunningService},
    transport::{
        streamable_http_client::{
            StreamableHttpClientTransport, StreamableHttpClientTransportConfig,
        },
        TokioChildProcess,
    },
    ClientHandler, Peer, RoleClient, ServiceExt,
};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use tokio::process::Command;
use tracing::{error, info, warn};

use legacy_sse::LegacySseClientTransport;
use ssrf::{prepare_http_target, OutboundNetworkPolicy};

const DEFAULT_STARTUP_TIMEOUT: Duration = Duration::from_secs(30);
const MAX_MODEL_TOOL_NAME_BYTES: usize = 64;
const MODEL_TOOL_HASH_CHARS: usize = 43;
const TRUSTED_PRIVATE_MCP_HOSTS_ENV: &str = "HIVY_TRUSTED_PRIVATE_MCP_HOSTS";
const RESERVED_RUNTIME_TOOL_NAMES: &[&str] = &[
    "bash",
    "read_file",
    "write_file",
    "edit_file",
    "apply_patch",
    "lsp",
    "subagent_task",
    "search_sessions",
    "request_user_input",
    "update_plan",
    "drive_upload",
    "drive_download",
    "load_tools",
];

struct McpServerState {
    server_name: String,
    tool_name_prefix: Option<String>,
    tool_filter: Option<ToolFilter>,
    tool_input_bindings: Vec<ToolInputBinding>,
    tools: ArcSwap<Vec<McpToolDefinition>>,
    last_discovery_error: RwLock<Option<String>>,
}

impl McpServerState {
    fn new(
        server_name: String,
        tool_name_prefix: Option<String>,
        tool_filter: Option<ToolFilter>,
        tool_input_bindings: Vec<ToolInputBinding>,
    ) -> Self {
        Self {
            server_name,
            tool_name_prefix,
            tool_filter,
            tool_input_bindings,
            tools: ArcSwap::from_pointee(Vec::new()),
            last_discovery_error: RwLock::new(None),
        }
    }

    fn replace_tools(&self, tools: Vec<McpToolDefinition>) {
        self.tools.store(Arc::new(tools));
        if let Ok(mut last_error) = self.last_discovery_error.write() {
            *last_error = None;
        }
    }

    fn record_discovery_error(&self, message: String) {
        if let Ok(mut last_error) = self.last_discovery_error.write() {
            *last_error = Some(message);
        }
    }
}

#[derive(Clone)]
struct RuntimeMcpClient {
    state: Arc<McpServerState>,
    protocol_version: ProtocolVersion,
}

impl ClientHandler for RuntimeMcpClient {
    fn get_info(&self) -> ClientInfo {
        ClientInfo::new(
            ClientCapabilities::default(),
            Implementation::new("hivy-runtime", env!("CARGO_PKG_VERSION")),
        )
        .with_protocol_version(self.protocol_version.clone())
    }

    fn on_tool_list_changed(
        &self,
        context: NotificationContext<RoleClient>,
    ) -> impl std::future::Future<Output = ()> + MaybeSendFuture + '_ {
        let state = self.state.clone();
        // Do not await a peer request from inside the notification handler:
        // transports that dispatch notifications serially would be unable to
        // process the tools/list response until this handler returned.
        tokio::spawn(async move {
            match discover_tools(
                &context.peer,
                &state.server_name,
                state.tool_name_prefix.as_deref(),
                &state.tool_filter,
                &state.tool_input_bindings,
            )
            .await
            {
                Ok(tools) => {
                    info!(
                        server = %state.server_name,
                        tool_count = tools.len(),
                        "refreshed MCP tool catalog after tools/list_changed"
                    );
                    state.replace_tools(tools);
                }
                Err(discovery_error) => {
                    let message = discovery_error.to_string();
                    state.record_discovery_error(message.clone());
                    warn!(
                        server = %state.server_name,
                        error = %message,
                        "failed to refresh MCP tool catalog after tools/list_changed"
                    );
                }
            }
        });
        std::future::ready(())
    }
}

struct McpEntry {
    service: RunningService<RoleClient, RuntimeMcpClient>,
    peer: Peer<RoleClient>,
    state: Arc<McpServerState>,
}

impl McpEntry {
    async fn close(mut self) {
        if let Err(error) = self
            .service
            .close_with_timeout(Duration::from_secs(2))
            .await
        {
            warn!(error = %error, server = %self.state.server_name, "failed to close MCP discovery transport");
        }
    }

    fn cancel(&self) {
        self.service.cancellation_token().cancel();
    }

    fn is_closed(&self) -> bool {
        self.service.is_closed()
    }
}

#[derive(Clone)]
struct McpServerConfig {
    spec: McpSpec,
    state: Arc<McpServerState>,
}

#[derive(Debug, Clone)]
pub struct McpToolDefinition {
    pub server_name: String,
    pub prefixed_name: String,
    pub raw_name: String,
    pub title: Option<String>,
    pub description: String,
    pub parameters: Value,
    pub output_schema: Option<Value>,
    pub annotations: Option<Value>,
    input_bindings: Vec<ToolInputBinding>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct McpConnectionStatus {
    pub server_name: String,
    pub connected: bool,
    pub tool_count: usize,
    pub error: Option<String>,
}

/// Registry of connected MCP servers and their cached tool catalogs.
///
/// Full schemas are kept host-side. Each model turn starts with the compact
/// catalog of exact tool names in its system prompt and activates exact tool
/// definitions on demand. Activation order is retained per session turn so
/// newly loaded definitions append to the model-visible array without leaking
/// into later turns.
pub struct McpRegistry {
    servers: ArcSwap<Vec<McpServerConfig>>,
    live_entries: DashMap<String, Arc<McpEntry>>,
    connect_locks: DashMap<String, Arc<tokio::sync::Mutex<()>>>,
    runtime_env: RwLock<HashMap<String, String>>,
    statuses: ArcSwap<Vec<McpConnectionStatus>>,
    activated_by_turn: DashMap<(String, String), Vec<McpToolDefinition>>,
    discovery_generation: AtomicU64,
    discovery_ready: tokio::sync::watch::Sender<u64>,
    workspace_root: PathBuf,
    network_policy: OutboundNetworkPolicy,
}

impl McpRegistry {
    pub async fn from_specs(
        specs: &[McpSpec],
        runtime_env: &HashMap<String, String>,
        workspace_root: PathBuf,
    ) -> Self {
        Self::from_specs_with_policy(
            specs,
            runtime_env,
            workspace_root,
            Self::production_network_policy(runtime_env),
        )
        .await
    }

    fn production_network_policy(runtime_env: &HashMap<String, String>) -> OutboundNetworkPolicy {
        let trusted_hosts = runtime_env
            .get(TRUSTED_PRIVATE_MCP_HOSTS_ENV)
            .into_iter()
            .flat_map(|hosts| hosts.split(','))
            .map(str::trim)
            .map(|host| host.trim_end_matches('.').to_ascii_lowercase())
            .filter(|host| !host.is_empty() && host.parse::<std::net::IpAddr>().is_err())
            .collect::<HashSet<_>>();
        OutboundNetworkPolicy::public_with_trusted_private_hosts(trusted_hosts)
    }

    /// Allows HTTP loopback targets for local integration fixtures only.
    ///
    /// The policy is injected by the test harness rather than read from agent,
    /// org, or process configuration, so untrusted MCP specs cannot weaken the
    /// production outbound boundary.
    #[doc(hidden)]
    pub async fn from_specs_allowing_loopback_for_tests(
        specs: &[McpSpec],
        runtime_env: &HashMap<String, String>,
        workspace_root: PathBuf,
    ) -> Self {
        Self::from_specs_with_policy(
            specs,
            runtime_env,
            workspace_root,
            OutboundNetworkPolicy::AllowLoopbackForTests,
        )
        .await
    }

    async fn from_specs_with_policy(
        specs: &[McpSpec],
        runtime_env: &HashMap<String, String>,
        workspace_root: PathBuf,
        network_policy: OutboundNetworkPolicy,
    ) -> Self {
        let (servers, statuses) = discover_specs(specs, runtime_env, network_policy.clone()).await;
        let (discovery_ready, _) = tokio::sync::watch::channel(0);
        Self {
            servers: ArcSwap::from_pointee(servers),
            live_entries: DashMap::new(),
            connect_locks: DashMap::new(),
            runtime_env: RwLock::new(runtime_env.clone()),
            statuses: ArcSwap::from_pointee(statuses),
            activated_by_turn: DashMap::new(),
            discovery_generation: AtomicU64::new(0),
            discovery_ready,
            workspace_root,
            network_policy,
        }
    }

    fn replace_activations(&self, activations: Vec<(String, String, String)>) {
        let available = self
            .all_tools_filtered(None)
            .into_iter()
            .map(|tool| (tool.prefixed_name.clone(), tool))
            .collect::<HashMap<_, _>>();
        self.activated_by_turn.clear();
        for (session_id, turn_id, tool_name) in activations {
            let Some(tool) = available.get(&tool_name) else {
                continue;
            };
            self.activated_by_turn
                .entry((session_id, turn_id))
                .or_default()
                .push(tool.clone());
        }
    }

    /// Start an in-memory turn and evict older turn activations for the same
    /// serialized session. Sandboxes sleep only while idle, so waking always
    /// starts a new turn with no MCP schemas loaded.
    pub fn begin_turn(&self, session_id: &str, turn_id: &str) -> anyhow::Result<()> {
        if session_id.trim().is_empty() || turn_id.trim().is_empty() {
            anyhow::bail!("session_id and turn_id are required for MCP tool loading");
        }
        self.activated_by_turn
            .retain(|(stored_session, stored_turn), _| {
                stored_session != session_id || stored_turn == turn_id
            });
        Ok(())
    }

    /// Immediately revokes the previous MCP snapshot, then discovers every
    /// server concurrently in a background task. The returned handle is only
    /// needed by tests and orderly shutdown paths; config push callers should
    /// not await it.
    pub fn reload_from_specs_in_background(
        self: &Arc<Self>,
        specs: &[McpSpec],
        runtime_env: &HashMap<String, String>,
    ) -> tokio::task::JoinHandle<()> {
        let generation = self.discovery_generation.fetch_add(1, Ordering::SeqCst) + 1;
        let active_activations = self
            .activated_by_turn
            .iter()
            .flat_map(|entry| {
                let ((session_id, turn_id), tools) = entry.pair();
                tools
                    .iter()
                    .map(|tool| {
                        (
                            session_id.clone(),
                            turn_id.clone(),
                            tool.prefixed_name.clone(),
                        )
                    })
                    .collect::<Vec<_>>()
            })
            .collect::<Vec<_>>();
        for entry in self.live_entries.iter() {
            entry.value().cancel();
        }
        self.live_entries.clear();
        self.connect_locks.clear();
        if let Ok(mut current_env) = self.runtime_env.write() {
            *current_env = runtime_env.clone();
        }
        // Immediately revoke model visibility while the authorized catalog is
        // being rebuilt. Current in-memory turn names are re-resolved only
        // against the new catalog after discovery completes.
        self.activated_by_turn.clear();

        let pending_servers = specs
            .iter()
            .cloned()
            .map(|spec| McpServerConfig {
                state: Arc::new(McpServerState::new(
                    spec.name().to_string(),
                    spec.tool_name_prefix().map(str::to_string),
                    spec_tool_filter(&spec).clone(),
                    spec.tool_input_bindings().to_vec(),
                )),
                spec,
            })
            .collect();
        let pending_statuses = specs
            .iter()
            .map(|spec| McpConnectionStatus {
                server_name: spec.name().to_string(),
                connected: false,
                tool_count: 0,
                error: None,
            })
            .collect();
        self.servers.store(Arc::new(pending_servers));
        self.statuses.store(Arc::new(pending_statuses));

        let registry = self.clone();
        let specs = specs.to_vec();
        let runtime_env = runtime_env.clone();
        tokio::spawn(async move {
            let (servers, statuses) =
                discover_specs(&specs, &runtime_env, registry.network_policy.clone()).await;
            if registry.discovery_generation.load(Ordering::SeqCst) != generation {
                return;
            }
            registry.servers.store(Arc::new(servers));
            registry.statuses.store(Arc::new(statuses));
            registry.replace_activations(active_activations);
            registry.discovery_ready.send_replace(generation);
        })
    }

    /// Wait until the latest authorized MCP snapshot has finished discovery.
    ///
    /// Turns call this before constructing their permanent tool catalog. This
    /// prevents a config reload's fail-closed, temporarily empty catalog from
    /// becoming the only catalog an agent sees for the entire turn.
    pub async fn wait_until_ready(&self) {
        let mut ready = self.discovery_ready.subscribe();
        loop {
            let target = self.discovery_generation.load(Ordering::SeqCst);
            while *ready.borrow() < target {
                if ready.changed().await.is_err() {
                    return;
                }
            }
            if self.discovery_generation.load(Ordering::SeqCst) == target {
                return;
            }
        }
    }

    pub fn connection_statuses(&self) -> Vec<McpConnectionStatus> {
        self.statuses.load().iter().cloned().collect()
    }

    /// Exposes live transports to integration tests without leaking peers.
    #[doc(hidden)]
    pub fn live_connection_names(&self) -> Vec<String> {
        let mut names: Vec<String> = self
            .live_entries
            .iter()
            .map(|entry| entry.key().clone())
            .collect();
        names.sort();
        names
    }

    pub fn available_tool_names(&self) -> Vec<String> {
        self.available_tool_names_filtered(None)
    }

    /// Complete compact catalog used in the system prompt. This intentionally
    /// contains exact callable names without the much larger JSON schemas.
    pub fn available_tool_names_filtered(&self, tool_filter: Option<&ToolFilter>) -> Vec<String> {
        let mut names: Vec<String> = self
            .all_tools_filtered(tool_filter)
            .into_iter()
            .map(|tool| tool.prefixed_name)
            .collect();
        names.sort();
        names.dedup();
        names
    }

    /// Full-catalog accessor for administrative and integration-test callers.
    /// Runtime model requests expose only per-turn activated schemas through
    /// `activated_tools_filtered`.
    pub fn loaded_tools(&self) -> Vec<McpToolDefinition> {
        self.loaded_tools_filtered(None)
    }

    pub fn loaded_tools_filtered(
        &self,
        tool_filter: Option<&ToolFilter>,
    ) -> Vec<McpToolDefinition> {
        self.all_tools_filtered(tool_filter)
    }

    pub fn activated_tools_filtered(
        &self,
        session_id: &str,
        turn_id: &str,
        tool_filter: Option<&ToolFilter>,
    ) -> Vec<McpToolDefinition> {
        let allowed: HashSet<String> = self
            .all_tools_filtered(tool_filter)
            .into_iter()
            .map(|tool| tool.prefixed_name)
            .collect();
        self.activated_by_turn
            .get(&(session_id.to_string(), turn_id.to_string()))
            .map(|tools| {
                tools
                    .iter()
                    .filter(|tool| allowed.contains(&tool.prefixed_name))
                    .cloned()
                    .collect()
            })
            .unwrap_or_default()
    }

    /// Validate and activate an exact batch of model-callable MCP tool names.
    ///
    /// All names are resolved before any activation is persisted, preventing an
    /// invalid or unauthorized name from leaving a partially loaded batch.
    /// Duplicate names are ignored while preserving first-seen activation order.
    pub async fn activate_tools_filtered(
        &self,
        session_id: &str,
        turn_id: &str,
        names: &[String],
        tool_filter: Option<&ToolFilter>,
    ) -> anyhow::Result<Value> {
        if session_id.trim().is_empty() || turn_id.trim().is_empty() {
            anyhow::bail!("session_id and turn_id are required for MCP tool loading");
        }
        let available = self
            .all_tools_filtered(tool_filter)
            .into_iter()
            .map(|tool| (tool.prefixed_name.clone(), tool))
            .collect::<HashMap<_, _>>();
        let mut seen = HashSet::new();
        let mut tools = Vec::new();
        let mut missing = Vec::new();
        for raw_name in names {
            let name = raw_name.trim();
            if name.is_empty() || !seen.insert(name.to_string()) {
                continue;
            }
            match available.get(name) {
                Some(tool) => tools.push(tool.clone()),
                None => missing.push(name.to_string()),
            }
        }
        if tools.is_empty() && missing.is_empty() {
            anyhow::bail!("tool_names must contain at least one exact MCP tool name");
        }
        if !missing.is_empty() {
            anyhow::bail!(
                "MCP tools not found or not permitted: {}",
                missing.join(", ")
            );
        }

        let mut connected_servers = HashSet::new();
        for tool in &tools {
            if connected_servers.insert(tool.server_name.clone()) {
                self.ensure_connected(&tool.server_name).await?;
            }
        }

        let mut loaded = Vec::new();
        let mut already_loaded = Vec::new();
        for tool in tools {
            let mut active = self
                .activated_by_turn
                .entry((session_id.to_string(), turn_id.to_string()))
                .or_default();
            let already_in_memory = active
                .iter()
                .any(|existing| existing.prefixed_name == tool.prefixed_name);
            if !already_in_memory {
                active.push(tool.clone());
            }
            if already_in_memory {
                already_loaded.push(tool.prefixed_name);
            } else {
                loaded.push(tool.prefixed_name);
            }
        }

        Ok(json!({
            "loaded": loaded,
            "already_loaded": already_loaded,
            "next": "The loaded tool definitions will be directly callable on the next model request."
        }))
    }

    pub async fn call_tool(&self, prefixed_name: &str, args: Value) -> anyhow::Result<Value> {
        self.call_tool_for_session("", None, prefixed_name, args)
            .await
    }

    /// Invoke an MCP tool. For the trusted "hivy" server, the runtime injects
    /// the current `_hivy_session_id` and, when a human is behind the turn, the
    /// `_hivy_actor_user_id`. Both are injected server-side and overwrite any
    /// model-supplied values.
    pub async fn call_tool_for_session(
        &self,
        session_id: &str,
        actor_user_id: Option<&str>,
        prefixed_name: &str,
        args: Value,
    ) -> anyhow::Result<Value> {
        let tool = self
            .all_tools_filtered(None)
            .into_iter()
            .find(|tool| tool.prefixed_name == prefixed_name)
            .ok_or_else(|| anyhow::anyhow!("MCP tool '{prefixed_name}' not found"))?;
        let entry = self.ensure_connected(&tool.server_name).await?;
        {
            let mut arguments = match args {
                Value::Object(map) => map,
                Value::Null => JsonObject::new(),
                other => {
                    let mut map = JsonObject::new();
                    map.insert("value".to_string(), other);
                    map
                }
            };
            if tool.server_name == "hivy" {
                arguments.insert(
                    "_hivy_session_id".to_string(),
                    Value::String(session_id.to_string()),
                );
                match actor_user_id {
                    Some(actor) if !actor.is_empty() => {
                        arguments.insert(
                            "_hivy_actor_user_id".to_string(),
                            Value::String(actor.to_string()),
                        );
                    }
                    _ => {
                        arguments.remove("_hivy_actor_user_id");
                    }
                }
            }
            file_inputs::apply_tool_input_bindings(
                &self.workspace_root,
                &tool.input_bindings,
                &mut arguments,
            )
            .await?;
            let result = entry
                .peer
                .call_tool(
                    CallToolRequestParams::new(tool.raw_name.clone()).with_arguments(arguments),
                )
                .await?;
            let mut value = serde_json::to_value(result)?;
            if tool.server_name == "hivy" {
                materialize::apply_materialize(&self.workspace_root, &mut value);
            }
            Ok(value)
        }
    }

    fn all_tools_filtered(&self, tool_filter: Option<&ToolFilter>) -> Vec<McpToolDefinition> {
        let servers = self.servers.load();
        let mut tools = Vec::new();
        let mut seen = HashSet::new();
        for server in servers.iter() {
            for tool in server.state.tools.load().iter() {
                if !agent_mcp_tool_allowed(
                    &tool.prefixed_name,
                    &tool.raw_name,
                    &tool.server_name,
                    tool_filter,
                    server.state.tool_filter.is_some(),
                ) {
                    continue;
                }
                if seen.insert(tool.prefixed_name.clone()) {
                    tools.push(tool.clone());
                }
            }
        }
        tools
    }

    async fn ensure_connected(&self, server_name: &str) -> anyhow::Result<Arc<McpEntry>> {
        if let Some(entry) = self.live_entries.get(server_name) {
            if !entry.is_closed() {
                return Ok(entry.clone());
            }
            drop(entry);
            self.live_entries.remove(server_name);
        }
        let connect_lock = self
            .connect_locks
            .entry(server_name.to_string())
            .or_insert_with(|| Arc::new(tokio::sync::Mutex::new(())))
            .clone();
        let _guard = connect_lock.lock().await;
        if let Some(entry) = self.live_entries.get(server_name) {
            if !entry.is_closed() {
                return Ok(entry.clone());
            }
            drop(entry);
            self.live_entries.remove(server_name);
        }
        let config = self
            .servers
            .load()
            .iter()
            .find(|config| config.state.server_name == server_name)
            .cloned()
            .ok_or_else(|| anyhow::anyhow!("MCP server '{server_name}' is not configured"))?;
        let runtime_env = self
            .runtime_env
            .read()
            .map_err(|_| anyhow::anyhow!("MCP runtime environment lock poisoned"))?
            .clone();
        let entry = Arc::new(
            connect_and_discover_with_state(
                &config.spec,
                &runtime_env,
                self.network_policy.clone(),
                Some(config.state),
            )
            .await?,
        );
        self.live_entries
            .insert(server_name.to_string(), entry.clone());
        Ok(entry)
    }
}

struct ConnectOutcome {
    index: usize,
    server: Option<McpServerConfig>,
    status: McpConnectionStatus,
}

async fn discover_specs(
    specs: &[McpSpec],
    runtime_env: &HashMap<String, String>,
    network_policy: OutboundNetworkPolicy,
) -> (Vec<McpServerConfig>, Vec<McpConnectionStatus>) {
    let runtime_env = Arc::new(runtime_env.clone());
    let concurrency = specs.len().max(1);
    let mut outcomes: Vec<ConnectOutcome> = stream::iter(specs.iter().cloned().enumerate())
        .map(|(index, spec)| {
            let runtime_env = runtime_env.clone();
            let network_policy = network_policy.clone();
            async move {
                let server_name = spec.name().to_string();
                let timeout_duration = startup_timeout(&spec);
                match tokio::time::timeout(
                    timeout_duration,
                    connect_and_discover(&spec, runtime_env.as_ref(), network_policy.clone()),
                )
                .await
                {
                    Ok(Ok(entry)) => {
                        let tool_count = entry.state.tools.load().len();
                        let state = entry.state.clone();
                        entry.close().await;
                        info!(
                            server = %server_name,
                            tool_count,
                            "MCP server connected"
                        );
                        ConnectOutcome {
                            index,
                            server: Some(McpServerConfig {
                                spec: spec.clone(),
                                state,
                            }),
                            status: McpConnectionStatus {
                                server_name,
                                connected: true,
                                tool_count,
                                error: None,
                            },
                        }
                    }
                    Ok(Err(connect_error)) => {
                        let message = connect_error.to_string();
                        error!(
                            name = %server_name,
                            error = %message,
                            "failed to connect MCP server"
                        );
                        ConnectOutcome {
                            index,
                            server: None,
                            status: McpConnectionStatus {
                                server_name,
                                connected: false,
                                tool_count: 0,
                                error: Some(message),
                            },
                        }
                    }
                    Err(_) => {
                        let message = format!(
                            "MCP server startup timed out after {} seconds",
                            timeout_duration.as_secs()
                        );
                        error!(name = %server_name, error = %message, "failed to connect MCP server");
                        ConnectOutcome {
                            index,
                            server: None,
                            status: McpConnectionStatus {
                                server_name,
                                connected: false,
                                tool_count: 0,
                                error: Some(message),
                            },
                        }
                    }
                }
            }
        })
        .buffer_unordered(concurrency)
        .collect()
        .await;
    outcomes.sort_by_key(|outcome| outcome.index);
    let mut servers = Vec::new();
    let mut statuses = Vec::with_capacity(outcomes.len());
    for outcome in outcomes {
        statuses.push(outcome.status);
        if let Some(server) = outcome.server {
            servers.push(server);
        }
    }
    // The discovery entries are dropped here, closing every transport. Their
    // catalog state remains cached in McpServerConfig until activation.
    (servers, statuses)
}

fn startup_timeout(spec: &McpSpec) -> Duration {
    match spec {
        McpSpec::Stdio {
            startup_timeout_seconds: Some(seconds),
            ..
        } => Duration::from_secs((*seconds).max(1) as u64),
        _ => DEFAULT_STARTUP_TIMEOUT,
    }
}

async fn connect_and_discover(
    spec: &McpSpec,
    runtime_env: &HashMap<String, String>,
    network_policy: OutboundNetworkPolicy,
) -> anyhow::Result<McpEntry> {
    connect_and_discover_with_state(spec, runtime_env, network_policy, None).await
}

async fn connect_and_discover_with_state(
    spec: &McpSpec,
    runtime_env: &HashMap<String, String>,
    network_policy: OutboundNetworkPolicy,
    existing_state: Option<Arc<McpServerState>>,
) -> anyhow::Result<McpEntry> {
    let server_name = spec.name().to_string();
    let state = existing_state.unwrap_or_else(|| {
        Arc::new(McpServerState::new(
            server_name.clone(),
            spec.tool_name_prefix().map(str::to_string),
            spec_tool_filter(spec).clone(),
            spec.tool_input_bindings().to_vec(),
        ))
    });
    let handler = RuntimeMcpClient {
        state: state.clone(),
        protocol_version: if matches!(spec, McpSpec::Sse { .. }) {
            ProtocolVersion::V_2024_11_05
        } else {
            ProtocolVersion::default()
        },
    };

    let (service, peer) = match spec {
        McpSpec::Stdio {
            name,
            command,
            args,
            env,
            ..
        } => {
            let mut cmd = Command::new(command);
            cmd.args(args.iter().map(String::as_str));
            for (key, value) in env {
                cmd.env(key, expand_env_placeholders(value, runtime_env)?);
            }
            info!(%name, "connecting MCP stdio server");
            let service = handler.serve(TokioChildProcess::new(cmd)?).await?;
            let peer = service.peer().clone();
            (service, peer)
        }
        McpSpec::Http {
            name, url, headers, ..
        }
        | McpSpec::StreamableHttp {
            name, url, headers, ..
        } => {
            let custom_headers = build_headers(headers, runtime_env)?;
            let target = prepare_http_target(url, network_policy).await?;
            let config = StreamableHttpClientTransportConfig::with_uri(target.url.to_string())
                .custom_headers(custom_headers)
                // rmcp performs one bounded re-initialization and request retry
                // when a stateful Streamable HTTP session expires with HTTP 404.
                .reinit_on_expired_session(true);
            info!(%name, "connecting MCP Streamable HTTP server");
            let service = handler
                .serve(StreamableHttpClientTransport::with_client(
                    target.client,
                    config,
                ))
                .await?;
            let peer = service.peer().clone();
            (service, peer)
        }
        McpSpec::Sse {
            name, url, headers, ..
        } => {
            let custom_headers = build_headers(headers, runtime_env)?;
            let target = prepare_http_target(url, network_policy).await?;
            info!(%name, "connecting legacy MCP HTTP+SSE server");
            let transport = LegacySseClientTransport::connect(target, custom_headers).await?;
            let service = handler.serve(transport).await?;
            let peer = service.peer().clone();
            (service, peer)
        }
    };

    let tools = discover_tools(
        &peer,
        &server_name,
        spec.tool_name_prefix(),
        spec_tool_filter(spec),
        spec.tool_input_bindings(),
    )
    .await?;
    state.replace_tools(tools);
    Ok(McpEntry {
        service,
        peer,
        state,
    })
}

fn spec_tool_filter(spec: &McpSpec) -> &Option<ToolFilter> {
    match spec {
        McpSpec::Stdio { tool_filter, .. }
        | McpSpec::Http { tool_filter, .. }
        | McpSpec::Sse { tool_filter, .. }
        | McpSpec::StreamableHttp { tool_filter, .. } => tool_filter,
    }
}

async fn discover_tools(
    peer: &Peer<RoleClient>,
    server_name: &str,
    tool_name_prefix: Option<&str>,
    tool_filter: &Option<ToolFilter>,
    tool_input_bindings: &[ToolInputBinding],
) -> anyhow::Result<Vec<McpToolDefinition>> {
    let discovered = peer.list_all_tools().await?;
    let mut definitions = Vec::new();
    for tool in discovered {
        let raw = tool.name.to_string();
        let prefixed = match tool_name_prefix {
            Some(prefix) => model_safe_explicit_tool_name(prefix, &raw),
            None => model_safe_tool_name(server_name, &raw),
        };
        if !mcp_tool_allowed(&prefixed, &raw, server_name, tool_filter.as_ref()) {
            continue;
        }
        let input_bindings: Vec<ToolInputBinding> = tool_input_bindings
            .iter()
            .filter(|binding| binding.tool() == raw)
            .cloned()
            .collect();
        let mut parameters = Value::Object((*tool.input_schema).clone());
        file_inputs::project_tool_schema(&mut parameters, &input_bindings)?;
        let mut description = tool
            .description
            .map(|value| value.into_owned())
            .unwrap_or_default();
        if !input_bindings.is_empty() {
            description.push_str(" File-backed inputs are read by the runtime from the sandbox workspace; pass the requested file path or paths, not file contents.");
        }
        definitions.push(McpToolDefinition {
            server_name: server_name.to_string(),
            prefixed_name: prefixed,
            raw_name: raw,
            title: tool.title,
            description,
            parameters,
            output_schema: tool
                .output_schema
                .map(|schema| Value::Object((*schema).clone())),
            annotations: tool
                .annotations
                .and_then(|annotations| serde_json::to_value(annotations).ok()),
            input_bindings,
        });
    }
    definitions.sort_by(|left, right| left.prefixed_name.cmp(&right.prefixed_name));
    Ok(definitions)
}

fn mcp_tool_allowed(
    prefixed: &str,
    raw: &str,
    server_name: &str,
    tool_filter: Option<&ToolFilter>,
) -> bool {
    let Some(filter) = tool_filter else {
        return true;
    };
    let source_prefixed = format!("{server_name}_{raw}");
    let matches = |name: &str| name == prefixed || name == raw || name == source_prefixed;
    if let Some(allow) = filter.allow.as_ref() {
        if !allow.iter().any(|name| matches(name)) {
            return false;
        }
    }
    if let Some(deny) = filter.deny.as_ref() {
        if deny.iter().any(|name| matches(name)) {
            return false;
        }
    }
    true
}

// A server-level filter is authoritative for that server. Generated connection MCP
// servers always carry an explicit deny filter (including an empty one), so the
// legacy top-level allow-list for Hivy capabilities cannot hide their tools.
// Servers without a local filter retain the previous global-filter behavior.
fn agent_mcp_tool_allowed(
    prefixed: &str,
    raw: &str,
    server_name: &str,
    tool_filter: Option<&ToolFilter>,
    has_server_filter: bool,
) -> bool {
    has_server_filter || mcp_tool_allowed(prefixed, raw, server_name, tool_filter)
}

/// Maps the MCP `(server, raw tool name)` identity into the conservative
/// function-name grammar shared by OpenAI-compatible providers. Ordinary
/// names retain their readable form. Names that require rewriting, exceed the
/// 64-byte provider limit, or use an ambiguous server delimiter receive a
/// stable SHA-256 suffix derived from the length-delimited original identity.
fn model_safe_tool_name(server_name: &str, raw_name: &str) -> String {
    model_safe_tool_name_with_namespace(server_name, raw_name, false)
}

fn model_safe_explicit_tool_name(prefix: &str, raw_name: &str) -> String {
    model_safe_tool_name_with_namespace(prefix, raw_name, true)
}

fn model_safe_tool_name_with_namespace(
    namespace: &str,
    raw_name: &str,
    namespace_is_explicit: bool,
) -> String {
    let source = format!("{namespace}_{raw_name}");
    let sanitized: String = source
        .chars()
        .map(|character| {
            if character.is_ascii_alphanumeric() || matches!(character, '_' | '-') {
                character
            } else {
                '_'
            }
        })
        .collect();
    let already_safe = !source.is_empty()
        && source.len() <= MAX_MODEL_TOOL_NAME_BYTES
        && source == sanitized
        // `_` separates the server and tool. A server containing `_` makes
        // otherwise-safe pairs ambiguous (`a_b`+`c` vs `a`+`b_c`).
        && (namespace_is_explicit || !namespace.contains('_'))
        // Reserve the hashed-name suffix namespace so a deliberately named
        // MCP tool cannot collide with a rewritten name.
        && !has_model_tool_hash_suffix(&source)
        // Runtime-owned tools are always model-visible and win de-duplication.
        // Rewrite colliding MCP names so loading one cannot report success
        // while leaving its schema unreachable.
        && !RESERVED_RUNTIME_TOOL_NAMES.contains(&source.as_str());
    if already_safe {
        return source;
    }

    let mut hasher = Sha256::new();
    hasher.update((namespace.len() as u64).to_be_bytes());
    hasher.update(namespace.as_bytes());
    hasher.update((raw_name.len() as u64).to_be_bytes());
    hasher.update(raw_name.as_bytes());
    let digest = hasher.finalize();
    let hash = BASE64_URL_SAFE_NO_PAD.encode(digest);
    let suffix = format!("__{hash}");
    let stem_limit = MAX_MODEL_TOOL_NAME_BYTES - suffix.len();
    let mut stem: String = sanitized.chars().take(stem_limit).collect();
    if stem.is_empty() {
        stem.push_str("mcp");
    }
    format!("{stem}{suffix}")
}

fn has_model_tool_hash_suffix(name: &str) -> bool {
    let bytes = name.as_bytes();
    let marker_offset = bytes.len().saturating_sub(MODEL_TOOL_HASH_CHARS + 2);
    if bytes.len() < MODEL_TOOL_HASH_CHARS + 2 || &bytes[marker_offset..marker_offset + 2] != b"__"
    {
        return false;
    }
    bytes[marker_offset + 2..]
        .iter()
        .copied()
        .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'_' | b'-'))
}

fn build_headers(
    headers: &HashMap<String, String>,
    runtime_env: &HashMap<String, String>,
) -> anyhow::Result<HashMap<HeaderName, HeaderValue>> {
    let mut map = HashMap::new();
    for (key, value) in headers {
        let name = HeaderName::from_bytes(key.as_bytes())?;
        let expanded = expand_env_placeholders(value, runtime_env)?;
        let header_value = HeaderValue::from_str(&expanded)?;
        map.insert(name, header_value);
    }
    Ok(map)
}

/// Expand `${NAME}` values without ever sending an unresolved credential
/// placeholder upstream. Unknown or malformed placeholders fail connection
/// setup instead of silently becoming an Authorization header value.
fn expand_env_placeholders(
    value: &str,
    runtime_env: &HashMap<String, String>,
) -> anyhow::Result<String> {
    let mut output = String::with_capacity(value.len());
    let mut remaining = value;
    while let Some(start) = remaining.find("${") {
        output.push_str(&remaining[..start]);
        let placeholder = &remaining[start + 2..];
        let end = placeholder
            .find('}')
            .ok_or_else(|| anyhow::anyhow!("unterminated environment placeholder in MCP value"))?;
        let key = &placeholder[..end];
        if key.is_empty()
            || !key
                .chars()
                .all(|character| character == '_' || character.is_ascii_alphanumeric())
        {
            anyhow::bail!("invalid environment placeholder '${{{key}}}' in MCP value");
        }
        let env_value = runtime_env.get(key).ok_or_else(|| {
            anyhow::anyhow!("missing runtime environment value for MCP placeholder '${{{key}}}'")
        })?;
        output.push_str(env_value);
        remaining = &placeholder[end + 1..];
    }
    output.push_str(remaining);
    Ok(output)
}

#[cfg(test)]
mod tests {
    use super::{
        agent_mcp_tool_allowed, discover_tools, expand_env_placeholders, mcp_tool_allowed,
        model_safe_explicit_tool_name, model_safe_tool_name, McpServerState, RuntimeMcpClient,
        MAX_MODEL_TOOL_NAME_BYTES, MODEL_TOOL_HASH_CHARS,
    };
    use domain::ToolFilter;
    use rmcp::{
        handler::server::{router::tool::ToolRoute, tool::ToolCallContext},
        model::{CallToolResult, ServerCapabilities, ServerInfo, Tool},
        service::NotificationContext,
        RoleServer, ServerHandler, ServiceExt,
    };
    use std::collections::HashMap;
    use std::sync::Arc;
    use tokio::sync::RwLock;

    #[test]
    fn expands_all_env_placeholders_in_header_values() {
        let runtime_env = HashMap::from([
            ("TOKEN".to_string(), "oauth-token".to_string()),
            ("TENANT".to_string(), "org-123".to_string()),
        ]);
        assert_eq!(
            expand_env_placeholders("Bearer ${TOKEN}:${TENANT}", &runtime_env).unwrap(),
            "Bearer oauth-token:org-123"
        );
    }

    #[test]
    fn unresolved_or_malformed_env_placeholders_fail_closed() {
        let runtime_env = HashMap::new();
        assert!(expand_env_placeholders("Bearer ${MISSING}", &runtime_env)
            .unwrap_err()
            .to_string()
            .contains("missing runtime environment value"));
        assert!(expand_env_placeholders("Bearer ${TOKEN", &runtime_env)
            .unwrap_err()
            .to_string()
            .contains("unterminated"));
    }

    #[test]
    fn tool_filter_accepts_raw_or_prefixed_tool_names() {
        let allow_raw = ToolFilter {
            allow: Some(vec!["web_search".to_string()]),
            deny: None,
        };
        assert!(mcp_tool_allowed(
            "hivy_web_search",
            "web_search",
            "hivy",
            Some(&allow_raw)
        ));
        assert!(!mcp_tool_allowed(
            "hivy_web_fetch",
            "web_fetch",
            "hivy",
            Some(&allow_raw)
        ));

        let deny_prefixed = ToolFilter {
            allow: None,
            deny: Some(vec!["hivy_cron".to_string()]),
        };
        assert!(!mcp_tool_allowed(
            "hivy_cron",
            "cron",
            "hivy",
            Some(&deny_prefixed)
        ));
        assert!(mcp_tool_allowed(
            "hivy_web_fetch",
            "web_fetch",
            "hivy",
            Some(&deny_prefixed)
        ));
    }

    #[test]
    fn deny_wins_when_allow_and_deny_both_match() {
        let filter = ToolFilter {
            allow: Some(vec!["hivy_cron".to_string()]),
            deny: Some(vec!["cron".to_string()]),
        };
        assert!(!mcp_tool_allowed(
            "hivy_cron",
            "cron",
            "hivy",
            Some(&filter)
        ));
    }

    #[test]
    fn server_filter_overrides_legacy_global_allow_list() {
        let global = ToolFilter {
            allow: Some(vec!["skill_view".to_string()]),
            deny: None,
        };
        assert!(agent_mcp_tool_allowed(
            "connection_slack_chat_post_message",
            "chat_post_message",
            "connection-slack",
            Some(&global),
            true,
        ));
        assert!(!agent_mcp_tool_allowed(
            "external_chat_post_message",
            "chat_post_message",
            "external",
            Some(&global),
            false,
        ));
    }

    #[test]
    fn model_tool_names_are_safe_bounded_stable_and_collision_resistant() {
        let ordinary = model_safe_tool_name("salesforce", "update_record");
        assert_eq!(ordinary, "salesforce_update_record");
        assert_eq!(
            model_safe_explicit_tool_name("postgres_primary", "run_query"),
            "postgres_primary_run_query"
        );

        let dotted = model_safe_tool_name("github", "issues.list");
        let slash_collision = model_safe_tool_name("github", "issues/list");
        let underscore = model_safe_tool_name("github", "issues_list");
        assert_ne!(dotted, slash_collision);
        assert_ne!(dotted, underscore);
        assert_ne!(slash_collision, underscore);
        assert_eq!(dotted, model_safe_tool_name("github", "issues.list"));

        let long_a = model_safe_tool_name("server", &format!("read_{}a", "x".repeat(120)));
        let long_b = model_safe_tool_name("server", &format!("read_{}b", "x".repeat(120)));
        assert_ne!(long_a, long_b, "truncated names retain stable identity");

        let ambiguous_left = model_safe_tool_name("a_b", "c");
        let ambiguous_right = model_safe_tool_name("a", "b_c");
        assert_ne!(ambiguous_left, ambiguous_right);

        let reserved_source = format!("github_tool__{}", "A".repeat(MODEL_TOOL_HASH_CHARS));
        let reserved = model_safe_tool_name(
            "github",
            &format!("tool__{}", "A".repeat(MODEL_TOOL_HASH_CHARS)),
        );
        assert_ne!(
            reserved, reserved_source,
            "hashed suffix namespace is reserved"
        );
        let runtime_collision = model_safe_tool_name("load", "tools");
        assert_ne!(runtime_collision, "load_tools");

        for name in [
            dotted,
            slash_collision,
            underscore,
            long_a,
            long_b,
            ambiguous_left,
            reserved,
            runtime_collision,
        ] {
            assert!(name.len() <= MAX_MODEL_TOOL_NAME_BYTES);
            assert!(name
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'_' | b'-')));
        }
    }

    #[derive(Clone)]
    struct ChangingToolServer {
        router: Arc<RwLock<rmcp::handler::server::router::tool::ToolRouter<Self>>>,
    }

    impl ChangingToolServer {
        fn new() -> Self {
            let mut router = rmcp::handler::server::router::tool::ToolRouter::<Self>::new();
            for name in ["tool_a", "tool_b"] {
                router.add_route(ToolRoute::new_dyn(
                    Tool::new(name, format!("Tool {name}"), Arc::new(Default::default())),
                    |_context| Box::pin(async { Ok(CallToolResult::default()) }),
                ));
            }
            Self {
                router: Arc::new(RwLock::new(router)),
            }
        }
    }

    impl ServerHandler for ChangingToolServer {
        fn get_info(&self) -> ServerInfo {
            ServerInfo::new(ServerCapabilities::builder().enable_tools().build())
        }

        async fn call_tool(
            &self,
            request: rmcp::model::CallToolRequestParams,
            context: rmcp::service::RequestContext<RoleServer>,
        ) -> Result<CallToolResult, rmcp::ErrorData> {
            let router = self.router.read().await;
            router
                .call(ToolCallContext::new(self, request, context))
                .await
        }

        async fn list_tools(
            &self,
            _request: Option<rmcp::model::PaginatedRequestParams>,
            _context: rmcp::service::RequestContext<RoleServer>,
        ) -> Result<rmcp::model::ListToolsResult, rmcp::ErrorData> {
            Ok(rmcp::model::ListToolsResult {
                tools: self.router.read().await.list_all(),
                ..Default::default()
            })
        }

        async fn on_initialized(&self, context: NotificationContext<RoleServer>) {
            self.router.write().await.bind_peer_notifier(&context.peer);
        }
    }

    #[tokio::test]
    async fn tools_list_changed_refreshes_cached_catalog_without_deadlock() {
        let server = ChangingToolServer::new();
        let router = server.router.clone();
        let (server_transport, client_transport) = tokio::io::duplex(8 * 1024);
        let server_task = tokio::spawn(async move { server.serve(server_transport).await });

        let state = Arc::new(McpServerState::new(
            "dynamic".to_string(),
            None,
            None,
            Vec::new(),
        ));
        let client = RuntimeMcpClient {
            state: state.clone(),
            protocol_version: rmcp::model::ProtocolVersion::default(),
        };
        let client_service = client
            .serve(client_transport)
            .await
            .expect("connect in-process MCP client");
        let initial = discover_tools(client_service.peer(), "dynamic", None, &None, &[])
            .await
            .expect("initial discovery");
        state.replace_tools(initial);
        assert_eq!(state.tools.load().len(), 2);

        router.write().await.disable_route("tool_a");
        tokio::time::timeout(std::time::Duration::from_secs(5), async {
            loop {
                if state.tools.load().len() == 1 {
                    break;
                }
                tokio::time::sleep(std::time::Duration::from_millis(10)).await;
            }
        })
        .await
        .expect("catalog refresh after tools/list_changed");
        assert_eq!(state.tools.load()[0].prefixed_name, "dynamic_tool_b");

        client_service.cancel().await.expect("cancel client");
        server_task.abort();
    }
}
