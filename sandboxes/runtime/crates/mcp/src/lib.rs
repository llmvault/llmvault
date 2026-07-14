mod legacy_sse;
mod materialize;
mod ssrf;

use std::collections::{BTreeMap, HashMap, HashSet};
use std::path::PathBuf;
use std::sync::{Arc, RwLock};
use std::time::Duration;

use arc_swap::ArcSwap;
use base64::prelude::{Engine as _, BASE64_URL_SAFE_NO_PAD};
use dashmap::DashMap;
use domain::{McpSpec, ToolFilter};
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
const MAX_CONCURRENT_CONNECTIONS: usize = 8;
const DEFAULT_SEARCH_LIMIT: usize = 12;
const MAX_SEARCH_LIMIT: usize = 50;
const MAX_MODEL_TOOL_NAME_BYTES: usize = 64;
const MODEL_TOOL_HASH_CHARS: usize = 43;

struct McpServerState {
    server_name: String,
    tool_filter: Option<ToolFilter>,
    tools: ArcSwap<Vec<McpToolDefinition>>,
    last_discovery_error: RwLock<Option<String>>,
}

impl McpServerState {
    fn new(server_name: String, tool_filter: Option<ToolFilter>) -> Self {
        Self {
            server_name,
            tool_filter,
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
            match discover_tools(&context.peer, &state.server_name, &state.tool_filter).await {
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
    _service: RunningService<RoleClient, RuntimeMcpClient>,
    peer: Peer<RoleClient>,
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
/// Full schemas are kept host-side. Each agent session starts with only the
/// lightweight discovery meta-tools and activates exact tool definitions on
/// demand. Activation order is retained so newly discovered definitions append
/// to the model-visible array instead of invalidating earlier prompt prefixes.
pub struct McpRegistry {
    entries: ArcSwap<Vec<McpEntry>>,
    statuses: ArcSwap<Vec<McpConnectionStatus>>,
    activated_by_session: DashMap<String, Vec<McpToolDefinition>>,
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
            OutboundNetworkPolicy::PublicOnly,
        )
        .await
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
        let (entries, statuses) = connect_specs(specs, runtime_env, network_policy).await;
        Self {
            entries: ArcSwap::from_pointee(entries),
            statuses: ArcSwap::from_pointee(statuses),
            activated_by_session: DashMap::new(),
            workspace_root,
            network_policy,
        }
    }

    pub async fn reload_from_specs(
        &self,
        specs: &[McpSpec],
        runtime_env: &HashMap<String, String>,
    ) {
        let (entries, statuses) = connect_specs(specs, runtime_env, self.network_policy).await;
        self.entries.store(Arc::new(entries));
        self.statuses.store(Arc::new(statuses));
        // A reload can represent revoked org/team/agent access. Never retain
        // activated definitions from the previous authorization snapshot.
        self.activated_by_session.clear();
    }

    pub fn connection_statuses(&self) -> Vec<McpConnectionStatus> {
        self.statuses.load().iter().cloned().collect()
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

    /// Legacy full-catalog accessor. Runtime model requests should use
    /// `activated_tools_filtered` instead so schemas are progressively loaded.
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
        tool_filter: Option<&ToolFilter>,
    ) -> Vec<McpToolDefinition> {
        self.activated_by_session
            .get(session_id)
            .map(|tools| {
                tools
                    .iter()
                    .filter(|tool| {
                        mcp_tool_allowed(
                            &tool.prefixed_name,
                            &tool.raw_name,
                            &tool.server_name,
                            tool_filter,
                        )
                    })
                    .cloned()
                    .collect()
            })
            .unwrap_or_default()
    }

    pub fn search_tools_filtered(
        &self,
        query: &str,
        detail_level: &str,
        limit: Option<usize>,
        tool_filter: Option<&ToolFilter>,
    ) -> Value {
        let query = query.trim();
        let limit = limit
            .unwrap_or(DEFAULT_SEARCH_LIMIT)
            .clamp(1, MAX_SEARCH_LIMIT);
        let mut matches: Vec<(i64, McpToolDefinition)> = self
            .all_tools_filtered(tool_filter)
            .into_iter()
            .filter_map(|tool| search_score(&tool, query).map(|score| (score, tool)))
            .collect();
        matches.sort_by(|(left_score, left), (right_score, right)| {
            right_score
                .cmp(left_score)
                .then_with(|| left.prefixed_name.cmp(&right.prefixed_name))
        });
        matches.truncate(limit);

        let mut grouped: BTreeMap<String, Vec<Value>> = BTreeMap::new();
        for (_, tool) in matches {
            grouped
                .entry(tool.server_name.clone())
                .or_default()
                .push(tool_json(&tool, detail_level));
        }
        let servers: Vec<Value> = grouped
            .into_iter()
            .map(|(server, tools)| json!({ "server": server, "tools": tools }))
            .collect();
        let total = servers
            .iter()
            .filter_map(|server| server.get("tools").and_then(Value::as_array))
            .map(Vec::len)
            .sum::<usize>();
        json!({
            "query": query,
            "detail_level": normalize_detail_level(detail_level),
            "total": total,
            "servers": servers,
            "next": "Call get_tool_details with an exact tool name to inspect its full schema and activate it for the next model request."
        })
    }

    pub fn activate_tool_filtered(
        &self,
        session_id: &str,
        name: &str,
        tool_filter: Option<&ToolFilter>,
    ) -> anyhow::Result<Value> {
        let name = name.trim();
        let tool = self
            .all_tools_filtered(tool_filter)
            .into_iter()
            .find(|tool| tool.prefixed_name == name)
            .ok_or_else(|| anyhow::anyhow!("MCP tool '{name}' not found or not permitted"))?;

        let mut active = self
            .activated_by_session
            .entry(session_id.to_string())
            .or_default();
        let already_active = active
            .iter()
            .any(|existing| existing.prefixed_name == tool.prefixed_name);
        if !already_active {
            active.push(tool.clone());
        }
        let mut details = tool_json(&tool, "full");
        if let Some(object) = details.as_object_mut() {
            object.insert("activated".to_string(), Value::Bool(true));
            object.insert("already_active".to_string(), Value::Bool(already_active));
            object.insert(
                "next".to_string(),
                Value::String(format!(
                    "The full '{name}' definition will be available on the next model request."
                )),
            );
        }
        Ok(details)
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
        let entries = self.entries.load();
        for entry in entries.iter() {
            let tools = entry.state.tools.load();
            if let Some(tool) = tools
                .iter()
                .find(|tool| tool.prefixed_name == prefixed_name)
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
                if entry.state.server_name == "hivy" {
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
                let result = entry
                    .peer
                    .call_tool(
                        CallToolRequestParams::new(tool.raw_name.clone()).with_arguments(arguments),
                    )
                    .await?;
                let mut value = serde_json::to_value(result)?;
                if entry.state.server_name == "hivy" {
                    materialize::apply_materialize(&self.workspace_root, &mut value);
                }
                return Ok(value);
            }
        }
        anyhow::bail!("MCP tool '{prefixed_name}' not found")
    }

    fn all_tools_filtered(&self, tool_filter: Option<&ToolFilter>) -> Vec<McpToolDefinition> {
        let entries = self.entries.load();
        let mut tools = Vec::new();
        let mut seen = HashSet::new();
        for entry in entries.iter() {
            for tool in entry.state.tools.load().iter() {
                if !mcp_tool_allowed(
                    &tool.prefixed_name,
                    &tool.raw_name,
                    &tool.server_name,
                    tool_filter,
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
}

struct ConnectOutcome {
    index: usize,
    entry: Option<McpEntry>,
    status: McpConnectionStatus,
}

async fn connect_specs(
    specs: &[McpSpec],
    runtime_env: &HashMap<String, String>,
    network_policy: OutboundNetworkPolicy,
) -> (Vec<McpEntry>, Vec<McpConnectionStatus>) {
    let runtime_env = Arc::new(runtime_env.clone());
    let mut outcomes: Vec<ConnectOutcome> = stream::iter(specs.iter().cloned().enumerate())
        .map(|(index, spec)| {
            let runtime_env = runtime_env.clone();
            async move {
                let server_name = spec.name().to_string();
                let timeout_duration = startup_timeout(&spec);
                match tokio::time::timeout(
                    timeout_duration,
                    connect_and_discover(&spec, runtime_env.as_ref(), network_policy),
                )
                .await
                {
                    Ok(Ok(entry)) => {
                        let tool_count = entry.state.tools.load().len();
                        info!(
                            server = %server_name,
                            tool_count,
                            "MCP server connected"
                        );
                        ConnectOutcome {
                            index,
                            entry: Some(entry),
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
                            entry: None,
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
                            entry: None,
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
        .buffer_unordered(MAX_CONCURRENT_CONNECTIONS)
        .collect()
        .await;
    outcomes.sort_by_key(|outcome| outcome.index);
    let mut entries = Vec::new();
    let mut statuses = Vec::with_capacity(outcomes.len());
    for outcome in outcomes {
        statuses.push(outcome.status);
        if let Some(entry) = outcome.entry {
            entries.push(entry);
        }
    }
    (entries, statuses)
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
    let server_name = spec.name().to_string();
    let state = Arc::new(McpServerState::new(
        server_name.clone(),
        spec_tool_filter(spec).clone(),
    ));
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

    let tools = discover_tools(&peer, &server_name, spec_tool_filter(spec)).await?;
    state.replace_tools(tools);
    Ok(McpEntry {
        _service: service,
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
    tool_filter: &Option<ToolFilter>,
) -> anyhow::Result<Vec<McpToolDefinition>> {
    let discovered = peer.list_all_tools().await?;
    let mut definitions = Vec::new();
    for tool in discovered {
        let raw = tool.name.to_string();
        let prefixed = model_safe_tool_name(server_name, &raw);
        if !mcp_tool_allowed(&prefixed, &raw, server_name, tool_filter.as_ref()) {
            continue;
        }
        definitions.push(McpToolDefinition {
            server_name: server_name.to_string(),
            prefixed_name: prefixed,
            raw_name: raw,
            title: tool.title,
            description: tool
                .description
                .map(|value| value.into_owned())
                .unwrap_or_default(),
            parameters: Value::Object((*tool.input_schema).clone()),
            output_schema: tool
                .output_schema
                .map(|schema| Value::Object((*schema).clone())),
            annotations: tool
                .annotations
                .and_then(|annotations| serde_json::to_value(annotations).ok()),
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

/// Maps the MCP `(server, raw tool name)` identity into the conservative
/// function-name grammar shared by OpenAI-compatible providers. Ordinary
/// names retain their readable form. Names that require rewriting, exceed the
/// 64-byte provider limit, or use an ambiguous server delimiter receive a
/// stable SHA-256 suffix derived from the length-delimited original identity.
fn model_safe_tool_name(server_name: &str, raw_name: &str) -> String {
    let source = format!("{server_name}_{raw_name}");
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
        && !server_name.contains('_')
        // Reserve the hashed-name suffix namespace so a deliberately named
        // MCP tool cannot collide with a rewritten name.
        && !has_model_tool_hash_suffix(&source);
    if already_safe {
        return source;
    }

    let mut hasher = Sha256::new();
    hasher.update((server_name.len() as u64).to_be_bytes());
    hasher.update(server_name.as_bytes());
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

fn normalize_detail_level(detail_level: &str) -> &'static str {
    match detail_level {
        "name" | "names" | "name_only" => "name",
        "full" | "schema" => "full",
        _ => "summary",
    }
}

fn tool_json(tool: &McpToolDefinition, detail_level: &str) -> Value {
    match normalize_detail_level(detail_level) {
        "name" => json!({ "name": tool.prefixed_name }),
        "full" => json!({
            "server": tool.server_name,
            "name": tool.prefixed_name,
            "raw_name": tool.raw_name,
            "title": tool.title,
            "description": tool.description,
            "input_schema": tool.parameters,
            "output_schema": tool.output_schema,
            "annotations": tool.annotations,
        }),
        _ => json!({
            "name": tool.prefixed_name,
            "description": one_line(&tool.description),
        }),
    }
}

fn one_line(value: &str) -> String {
    value
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
        .chars()
        .take(240)
        .collect()
}

fn search_score(tool: &McpToolDefinition, query: &str) -> Option<i64> {
    if query.is_empty() || query == "*" {
        return Some(1);
    }
    let query = query.to_ascii_lowercase();
    let prefixed = tool.prefixed_name.to_ascii_lowercase();
    let raw = tool.raw_name.to_ascii_lowercase();
    let source_prefixed = format!("{}_{}", tool.server_name, tool.raw_name).to_ascii_lowercase();
    let description = tool.description.to_ascii_lowercase();
    if query == prefixed || query == raw || query == source_prefixed {
        return Some(10_000);
    }

    let query_terms: Vec<&str> = query
        .split(|character: char| !character.is_ascii_alphanumeric())
        .filter(|term| !term.is_empty())
        .collect();
    let mut score = 0i64;
    if prefixed.starts_with(&query)
        || raw.starts_with(&query)
        || source_prefixed.starts_with(&query)
    {
        score += 1_000;
    } else if prefixed.contains(&query) || raw.contains(&query) || source_prefixed.contains(&query)
    {
        score += 600;
    }
    if description.contains(&query) {
        score += 300;
    }
    for term in query_terms {
        if prefixed.contains(term) || raw.contains(term) || source_prefixed.contains(term) {
            score += 120;
        }
        if description.contains(term) {
            score += 30;
        }
    }
    (score > 0).then_some(score)
}

#[cfg(test)]
mod tests {
    use super::{
        discover_tools, expand_env_placeholders, mcp_tool_allowed, model_safe_tool_name,
        normalize_detail_level, search_score, McpServerState, McpToolDefinition, RuntimeMcpClient,
        MAX_MODEL_TOOL_NAME_BYTES, MODEL_TOOL_HASH_CHARS,
    };
    use domain::ToolFilter;
    use rmcp::{
        handler::server::{router::tool::ToolRoute, tool::ToolCallContext},
        model::{CallToolResult, ServerCapabilities, ServerInfo, Tool},
        service::NotificationContext,
        RoleServer, ServerHandler, ServiceExt,
    };
    use serde_json::json;
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
    fn model_tool_names_are_safe_bounded_stable_and_collision_resistant() {
        let ordinary = model_safe_tool_name("salesforce", "update_record");
        assert_eq!(ordinary, "salesforce_update_record");

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

        for name in [
            dotted,
            slash_collision,
            underscore,
            long_a,
            long_b,
            ambiguous_left,
            reserved,
        ] {
            assert!(name.len() <= MAX_MODEL_TOOL_NAME_BYTES);
            assert!(name
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'_' | b'-')));
        }
    }

    #[test]
    fn exact_names_rank_above_description_keyword_matches() {
        let exact = tool("salesforce_update_record", "Update an account");
        let descriptive = tool("salesforce_help", "Use update record workflows");
        assert!(
            search_score(&exact, "salesforce_update_record")
                > search_score(&descriptive, "salesforce_update_record")
        );
        assert_eq!(normalize_detail_level("schema"), "full");
        assert_eq!(normalize_detail_level("unexpected"), "summary");
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

        let state = Arc::new(McpServerState::new("dynamic".to_string(), None));
        let client = RuntimeMcpClient {
            state: state.clone(),
            protocol_version: rmcp::model::ProtocolVersion::default(),
        };
        let client_service = client
            .serve(client_transport)
            .await
            .expect("connect in-process MCP client");
        let initial = discover_tools(client_service.peer(), "dynamic", &None)
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

    fn tool(name: &str, description: &str) -> McpToolDefinition {
        McpToolDefinition {
            server_name: "salesforce".to_string(),
            prefixed_name: name.to_string(),
            raw_name: name.trim_start_matches("salesforce_").to_string(),
            title: None,
            description: description.to_string(),
            parameters: json!({ "type": "object" }),
            output_schema: None,
            annotations: None,
        }
    }
}
