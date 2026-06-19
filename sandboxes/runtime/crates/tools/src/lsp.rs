use std::collections::{HashMap, HashSet, VecDeque};
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex as StdMutex, RwLock};
use std::time::{Duration, Instant};

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::{LspConfig, LspServerConfig};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::process::{Child, Command};
use tokio::sync::{mpsc, oneshot, Mutex};
use url::Url;

use crate::path::{resolve_within_workspace, PathPolicyError};
use crate::{schema_for, JsonTool, ToolDefinition};

const TOOL_NAME: &str = "lsp";
const TOOL_DESCRIPTION: &str =
    "Interact with persistent Language Server Protocol servers for source code \
     intelligence. Supports hover, definitions, references, document symbols, \
     workspace symbols, implementations, call hierarchy, diagnostics, and \
     status. File positions are 1-based, matching editor display. The runtime \
     starts matching language servers lazily per project root and keeps them \
     warm across tool calls.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub enum LspOperation {
    Status,
    Diagnostics,
    Hover,
    GoToDefinition,
    FindReferences,
    DocumentSymbol,
    WorkspaceSymbol,
    GoToImplementation,
    PrepareCallHierarchy,
    IncomingCalls,
    OutgoingCalls,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct LspArgs {
    pub operation: LspOperation,
    /// Absolute or workspace-relative path used to select the language server.
    #[serde(default, rename = "filePath", alias = "file_path")]
    pub file_path: Option<String>,
    /// 1-based line number for cursor operations.
    #[serde(default)]
    pub line: Option<usize>,
    /// 1-based character/column for cursor operations.
    #[serde(default)]
    pub character: Option<usize>,
    /// Search query for workspaceSymbol. Empty string requests all symbols.
    #[serde(default)]
    pub query: Option<String>,
}

pub struct LspTool {
    config: LspConfig,
    workspace_root: PathBuf,
    service: LspService,
}

#[derive(Clone)]
pub struct LspService {
    workspace_root: PathBuf,
    config: Arc<RwLock<LspConfig>>,
    state: Arc<Mutex<LspState>>,
}

#[derive(Default)]
struct LspState {
    clients: HashMap<ClientKey, Arc<LspClient>>,
    broken: HashSet<ClientKey>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct ClientKey {
    server_id: String,
    root: PathBuf,
}

#[derive(Debug, Clone)]
struct ServerDefinition {
    id: String,
    command: Vec<String>,
    extensions: Vec<String>,
    root_markers: Vec<String>,
    strict_root: bool,
    initialization_options: Option<Value>,
}

struct LspClient {
    server_id: String,
    root: PathBuf,
    sender: mpsc::Sender<Value>,
    pending: Arc<Mutex<HashMap<u64, oneshot::Sender<Result<Value, String>>>>>,
    diagnostics: Arc<Mutex<HashMap<String, Value>>>,
    documents: Arc<Mutex<HashMap<PathBuf, DocumentState>>>,
    capabilities: Value,
    next_id: AtomicU64,
    child: StdMutex<Child>,
    stderr: Arc<Mutex<String>>,
}

#[derive(Debug)]
struct DocumentState {
    version: i32,
}

#[derive(Debug, Clone)]
struct Symbol {
    name: String,
    kind: String,
    line: usize,
    character: usize,
    container: Option<String>,
    text: String,
}

impl LspService {
    pub fn new(workspace_root: PathBuf) -> Self {
        let mut config = LspConfig::default();
        config.enabled = false;
        Self {
            workspace_root,
            config: Arc::new(RwLock::new(config)),
            state: Arc::new(Mutex::new(LspState::default())),
        }
    }

    pub fn configure(&self, config: LspConfig) {
        if let Ok(mut guard) = self.config.write() {
            *guard = config;
        }
    }

    pub async fn touch_file(&self, path: &Path) {
        if !self.current_config().enabled {
            return;
        }
        let Ok(clients) = self.clients_for_path(path).await else {
            return;
        };
        for client in clients {
            let _ = client.open_or_change(path).await;
        }
    }

    async fn status(&self) -> Value {
        let config = self.current_config();
        let definitions = server_definitions(&config);
        let clients = self.state.lock().await.clients.clone();
        let servers = definitions
            .iter()
            .map(|definition| {
                let roots: Vec<String> = clients
                    .iter()
                    .filter(|(key, _)| key.server_id == definition.id)
                    .map(|(key, _)| relative_path(&self.workspace_root, &key.root))
                    .collect();
                json!({
                    "id": definition.id,
                    "command": definition.command,
                    "available": definition.command.first().map(|command| command_available(command)).unwrap_or(false),
                    "extensions": definition.extensions,
                    "root_markers": definition.root_markers,
                    "strict_root": definition.strict_root,
                    "running_roots": roots,
                })
            })
            .collect::<Vec<_>>();
        json!({
            "enabled": config.enabled,
            "backend": "lsp",
            "workspace_root": self.workspace_root.display().to_string(),
            "servers": servers,
            "operations": [
                "status",
                "diagnostics",
                "hover",
                "goToDefinition",
                "findReferences",
                "documentSymbol",
                "workspaceSymbol",
                "goToImplementation",
                "prepareCallHierarchy",
                "incomingCalls",
                "outgoingCalls"
            ]
        })
    }

    async fn diagnostics(&self, path: &Path) -> Result<Option<Value>> {
        let clients = self.clients_for_path(path).await?;
        if clients.is_empty() {
            return Ok(None);
        }
        for client in &clients {
            client.open_or_change(path).await?;
        }
        let timeout = Duration::from_millis(
            (self.current_config().timeout_seconds as u64 * 1000).clamp(250, 3000),
        );
        let mut diagnostics = Vec::new();
        for client in &clients {
            diagnostics.push(json!({
                "server_id": client.server_id,
                "root": client.root.display().to_string(),
                "diagnostics": client.wait_for_diagnostics(path, timeout).await?,
            }));
        }
        Ok(Some(json!({
            "backend": "lsp",
            "path": path.display().to_string(),
            "servers": client_ids(&clients),
            "diagnostics": diagnostics,
        })))
    }

    async fn hover(&self, path: &Path, line: usize, character: usize) -> Result<Option<Value>> {
        self.request_for_path(
            path,
            "textDocument/hover",
            text_document_position_params(path, line, character)?,
        )
        .await
    }

    async fn definition(
        &self,
        path: &Path,
        line: usize,
        character: usize,
    ) -> Result<Option<Value>> {
        self.request_for_path(
            path,
            "textDocument/definition",
            text_document_position_params(path, line, character)?,
        )
        .await
    }

    async fn references(
        &self,
        path: &Path,
        line: usize,
        character: usize,
    ) -> Result<Option<Value>> {
        let mut params = text_document_position_params(path, line, character)?;
        params["context"] = json!({"includeDeclaration": true});
        self.request_for_path(path, "textDocument/references", params)
            .await
    }

    async fn implementation(
        &self,
        path: &Path,
        line: usize,
        character: usize,
    ) -> Result<Option<Value>> {
        self.request_for_path(
            path,
            "textDocument/implementation",
            text_document_position_params(path, line, character)?,
        )
        .await
    }

    async fn document_symbol(&self, path: &Path) -> Result<Option<Value>> {
        self.request_for_path(
            path,
            "textDocument/documentSymbol",
            json!({"textDocument": {"uri": file_uri(path)?}}),
        )
        .await
    }

    async fn workspace_symbol(&self, path: Option<&Path>, query: &str) -> Result<Option<Value>> {
        let clients = if let Some(path) = path {
            self.clients_for_path(path).await?
        } else {
            self.state.lock().await.clients.values().cloned().collect()
        };
        if clients.is_empty() {
            return Ok(None);
        }
        let mut results = Vec::new();
        let mut used_clients = Vec::new();
        for client in &clients {
            if !client.supports_method("workspace/symbol") {
                continue;
            }
            let result = client
                .request(
                    "workspace/symbol",
                    json!({"query": query}),
                    self.request_timeout(),
                )
                .await?;
            results.push(result);
            used_clients.push(client.clone());
        }
        if results.is_empty() {
            return Ok(None);
        }
        Ok(Some(json!({
            "backend": "lsp",
            "operation": "workspaceSymbol",
            "servers": client_ids(&used_clients),
            "result": merge_lsp_results(results),
        })))
    }

    async fn prepare_call_hierarchy(
        &self,
        path: &Path,
        line: usize,
        character: usize,
    ) -> Result<Option<Value>> {
        self.request_for_path(
            path,
            "textDocument/prepareCallHierarchy",
            text_document_position_params(path, line, character)?,
        )
        .await
    }

    async fn call_hierarchy(
        &self,
        path: &Path,
        line: usize,
        character: usize,
        method: &str,
    ) -> Result<Option<Value>> {
        let clients = self.clients_for_path(path).await?;
        if clients.is_empty() {
            return Ok(None);
        }
        let mut results = Vec::new();
        let mut used_clients = Vec::new();
        for client in &clients {
            if !client.supports_method("textDocument/prepareCallHierarchy") {
                continue;
            }
            client.open_or_change(path).await?;
            let items = client
                .request(
                    "textDocument/prepareCallHierarchy",
                    text_document_position_params(path, line, character)?,
                    self.request_timeout(),
                )
                .await?;
            let Some(item) = items.as_array().and_then(|items| items.first()).cloned() else {
                results.push(json!([]));
                continue;
            };
            results.push(
                client
                    .request(method, json!({"item": item}), self.request_timeout())
                    .await?,
            );
            used_clients.push(client.clone());
        }
        if results.is_empty() {
            return Ok(None);
        }
        Ok(Some(json!({
            "backend": "lsp",
            "operation": method,
            "servers": client_ids(&used_clients),
            "result": merge_lsp_results(results),
        })))
    }

    async fn request_for_path(
        &self,
        path: &Path,
        method: &str,
        params: Value,
    ) -> Result<Option<Value>> {
        let clients = self.clients_for_path(path).await?;
        if clients.is_empty() {
            return Ok(None);
        }
        let mut results = Vec::new();
        let mut used_clients = Vec::new();
        for client in &clients {
            if !client.supports_method(method) {
                continue;
            }
            client.open_or_change(path).await?;
            results.push(
                client
                    .request(method, params.clone(), self.request_timeout())
                    .await?,
            );
            used_clients.push(client.clone());
        }
        if results.is_empty() {
            return Ok(None);
        }
        Ok(Some(json!({
            "backend": "lsp",
            "operation": method,
            "path": path.display().to_string(),
            "servers": client_ids(&used_clients),
            "result": merge_lsp_results(results),
        })))
    }

    async fn clients_for_path(&self, path: &Path) -> Result<Vec<Arc<LspClient>>> {
        let config = self.current_config();
        if !config.enabled {
            return Ok(Vec::new());
        }
        let definitions = server_definitions(&config);
        let mut clients = Vec::new();
        for definition in definitions {
            if !definition.matches(path) {
                continue;
            }
            let Some(command) = definition.command.first() else {
                continue;
            };
            if !command_available(command) {
                continue;
            }
            let Some(root) = resolve_server_root(path, &self.workspace_root, &definition) else {
                continue;
            };
            let key = ClientKey {
                server_id: definition.id.clone(),
                root: root.clone(),
            };
            if let Some(existing) = self.state.lock().await.clients.get(&key).cloned() {
                clients.push(existing);
                continue;
            }
            if self.state.lock().await.broken.contains(&key) {
                continue;
            }
            let client = match LspClient::spawn(
                definition.clone(),
                root,
                self.workspace_root.clone(),
                self.request_timeout(),
            )
            .await
            {
                Ok(client) => Arc::new(client),
                Err(error) => {
                    self.state.lock().await.broken.insert(key);
                    tracing::warn!("lsp server startup failed: {error}");
                    continue;
                }
            };
            self.state.lock().await.clients.insert(key, client.clone());
            clients.push(client);
        }
        Ok(clients)
    }

    fn current_config(&self) -> LspConfig {
        self.config
            .read()
            .map(|guard| guard.clone())
            .unwrap_or_default()
    }

    fn request_timeout(&self) -> Duration {
        Duration::from_secs(self.current_config().timeout_seconds.max(1) as u64)
    }
}

impl LspClient {
    async fn spawn(
        definition: ServerDefinition,
        root: PathBuf,
        workspace_root: PathBuf,
        timeout: Duration,
    ) -> Result<Self> {
        let mut command = Command::new(&definition.command[0]);
        command
            .args(&definition.command[1..])
            .current_dir(&root)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .kill_on_drop(true);
        #[cfg(unix)]
        {
            command.process_group(0);
        }
        let mut child = command
            .spawn()
            .map_err(|error| anyhow!("spawn {}: {error}", definition.command.join(" ")))?;
        let stdin = child
            .stdin
            .take()
            .ok_or_else(|| anyhow!("lsp stdin unavailable"))?;
        let stdout = child
            .stdout
            .take()
            .ok_or_else(|| anyhow!("lsp stdout unavailable"))?;
        let stderr = child.stderr.take();
        let (sender, mut receiver) = mpsc::channel::<Value>(128);
        let pending = Arc::new(Mutex::new(HashMap::new()));
        let diagnostics = Arc::new(Mutex::new(HashMap::new()));
        let stderr_buffer = Arc::new(Mutex::new(String::new()));

        tokio::spawn(async move {
            let mut stdin = stdin;
            while let Some(message) = receiver.recv().await {
                if write_lsp_message(&mut stdin, &message).await.is_err() {
                    break;
                }
            }
        });

        tokio::spawn(read_loop(
            definition.id.clone(),
            stdout,
            pending.clone(),
            diagnostics.clone(),
            sender.clone(),
        ));

        if let Some(stderr) = stderr {
            tokio::spawn(stderr_loop(stderr, stderr_buffer.clone()));
        }

        let mut client = Self {
            server_id: definition.id.clone(),
            root: root.clone(),
            sender,
            pending,
            diagnostics,
            documents: Arc::new(Mutex::new(HashMap::new())),
            capabilities: Value::Null,
            next_id: AtomicU64::new(1),
            child: StdMutex::new(child),
            stderr: stderr_buffer,
        };

        let initialize_result = client
            .request(
                "initialize",
                json!({
                    "processId": std::process::id(),
                    "rootPath": root.display().to_string(),
                    "rootUri": file_uri(&root)?,
                    "workspaceFolders": [{
                        "uri": file_uri(&root)?,
                        "name": root.file_name().and_then(|value| value.to_str()).unwrap_or("workspace")
                    }],
                    "initializationOptions": definition.initialization_options.unwrap_or(Value::Null),
                    "capabilities": client_capabilities(),
                    "clientInfo": {
                        "name": "hivy-sandboxes-runtime",
                        "version": env!("CARGO_PKG_VERSION")
                    },
                    "trace": "off"
                }),
                timeout,
            )
            .await?;
        client.capabilities = initialize_result
            .get("capabilities")
            .cloned()
            .unwrap_or(Value::Null);
        client.notify("initialized", json!({})).await?;
        tracing::info!(
            server_id = definition.id,
            root = %relative_path(&workspace_root, &root),
            "lsp server initialized"
        );
        Ok(client)
    }

    async fn request(&self, method: &str, params: Value, timeout: Duration) -> Result<Value> {
        let id = self.next_id.fetch_add(1, Ordering::SeqCst);
        let (sender, receiver) = oneshot::channel();
        self.pending.lock().await.insert(id, sender);
        self.sender
            .send(json!({
                "jsonrpc": "2.0",
                "id": id,
                "method": method,
                "params": params,
            }))
            .await
            .map_err(|_| anyhow!("lsp server {} is not accepting requests", self.server_id))?;
        match tokio::time::timeout(timeout, receiver).await {
            Ok(Ok(Ok(value))) => Ok(value),
            Ok(Ok(Err(error))) => Err(anyhow!(
                "lsp {method} failed on {}: {error}",
                self.server_id
            )),
            Ok(Err(_)) => Err(anyhow!(
                "lsp {method} response channel closed for {}",
                self.server_id
            )),
            Err(_) => {
                self.pending.lock().await.remove(&id);
                Err(anyhow!("lsp {method} timed out for {}", self.server_id))
            }
        }
    }

    async fn notify(&self, method: &str, params: Value) -> Result<()> {
        self.sender
            .send(json!({
                "jsonrpc": "2.0",
                "method": method,
                "params": params,
            }))
            .await
            .map_err(|_| {
                anyhow!(
                    "lsp server {} is not accepting notifications",
                    self.server_id
                )
            })
    }

    async fn open_or_change(&self, path: &Path) -> Result<()> {
        let text = tokio::fs::read_to_string(path)
            .await
            .map_err(|error| anyhow!("read {} for lsp: {error}", path.display()))?;
        let uri = file_uri(path)?;
        let mut documents = self.documents.lock().await;
        if let Some(document) = documents.get_mut(path) {
            document.version += 1;
            let version = document.version;
            drop(documents);
            self.notify(
                "textDocument/didChange",
                json!({
                    "textDocument": {
                        "uri": uri,
                        "version": version,
                    },
                    "contentChanges": [{"text": text}],
                }),
            )
            .await?;
            return Ok(());
        }
        documents.insert(path.to_path_buf(), DocumentState { version: 1 });
        drop(documents);
        self.notify(
            "textDocument/didOpen",
            json!({
                "textDocument": {
                    "uri": uri,
                    "languageId": language_id(path),
                    "version": 1,
                    "text": text,
                }
            }),
        )
        .await
    }

    async fn wait_for_diagnostics(&self, path: &Path, timeout: Duration) -> Result<Value> {
        let uri = file_uri(path)?;
        let started = Instant::now();
        loop {
            if let Some(diagnostics) = self.diagnostics.lock().await.get(&uri).cloned() {
                return Ok(diagnostics);
            }
            if started.elapsed() >= timeout {
                return Ok(json!([]));
            }
            tokio::time::sleep(Duration::from_millis(50)).await;
        }
    }

    fn supports_method(&self, method: &str) -> bool {
        let provider = match method {
            "textDocument/hover" => "hoverProvider",
            "textDocument/definition" => "definitionProvider",
            "textDocument/references" => "referencesProvider",
            "textDocument/documentSymbol" => "documentSymbolProvider",
            "workspace/symbol" => "workspaceSymbolProvider",
            "textDocument/implementation" => "implementationProvider",
            "textDocument/prepareCallHierarchy"
            | "callHierarchy/incomingCalls"
            | "callHierarchy/outgoingCalls" => "callHierarchyProvider",
            _ => return true,
        };
        match self.capabilities.get(provider) {
            Some(Value::Bool(enabled)) => *enabled,
            Some(Value::Null) | None => false,
            Some(_) => true,
        }
    }
}

impl Drop for LspClient {
    fn drop(&mut self) {
        if let Ok(mut child) = self.child.lock() {
            let _ = child.start_kill();
        }
    }
}

impl ServerDefinition {
    fn matches(&self, path: &Path) -> bool {
        let selector = file_selector(path);
        self.extensions
            .iter()
            .any(|extension| extension == &selector)
    }
}

impl LspTool {
    pub fn new(config: LspConfig, workspace_root: PathBuf, service: LspService) -> Self {
        service.configure(config.clone());
        Self {
            config,
            workspace_root,
            service,
        }
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: LspArgs =
            serde_json::from_value(args).map_err(|error| anyhow!("invalid arguments: {error}"))?;
        if !self.config.enabled {
            return Ok(json!({"enabled": false, "message": "lsp is disabled in tool config"}));
        }
        match parsed.operation {
            LspOperation::Status => Ok(self.service.status().await),
            LspOperation::Diagnostics => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                if let Some(result) = self.service.diagnostics(&path).await? {
                    return Ok(result);
                }
                self.fallback_or_error(|| async { self.fallback_diagnostics(&path).await })
                    .await
            }
            LspOperation::DocumentSymbol => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                if let Some(result) = self.service.document_symbol(&path).await? {
                    return Ok(result);
                }
                self.fallback_or_error(|| async { self.fallback_document_symbols(&path).await })
                    .await
            }
            LspOperation::WorkspaceSymbol => {
                let path = self.resolve_optional_path(parsed.file_path.as_deref())?;
                if let Some(result) = self
                    .service
                    .workspace_symbol(path.as_deref(), parsed.query.as_deref().unwrap_or_default())
                    .await?
                {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_workspace_symbols(parsed.query.as_deref())
                        .await
                })
                .await
            }
            LspOperation::Hover => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                let (line, character) = require_position(parsed.line, parsed.character)?;
                if let Some(result) = self.service.hover(&path, line, character).await? {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_hover(&path, line, character).await
                })
                .await
            }
            LspOperation::GoToDefinition => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                let (line, character) = require_position(parsed.line, parsed.character)?;
                if let Some(result) = self.service.definition(&path, line, character).await? {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_definition_like(&path, line, character, "goToDefinition")
                        .await
                })
                .await
            }
            LspOperation::FindReferences => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                let (line, character) = require_position(parsed.line, parsed.character)?;
                if let Some(result) = self.service.references(&path, line, character).await? {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_references_like(&path, line, character, "findReferences")
                        .await
                })
                .await
            }
            LspOperation::GoToImplementation => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                let (line, character) = require_position(parsed.line, parsed.character)?;
                if let Some(result) = self.service.implementation(&path, line, character).await? {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_definition_like(&path, line, character, "goToImplementation")
                        .await
                })
                .await
            }
            LspOperation::PrepareCallHierarchy => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                let (line, character) = require_position(parsed.line, parsed.character)?;
                if let Some(result) = self
                    .service
                    .prepare_call_hierarchy(&path, line, character)
                    .await?
                {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_definition_like(&path, line, character, "prepareCallHierarchy")
                        .await
                })
                .await
            }
            LspOperation::IncomingCalls => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                let (line, character) = require_position(parsed.line, parsed.character)?;
                if let Some(result) = self
                    .service
                    .call_hierarchy(&path, line, character, "callHierarchy/incomingCalls")
                    .await?
                {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_references_like(&path, line, character, "incomingCalls")
                        .await
                })
                .await
            }
            LspOperation::OutgoingCalls => {
                let path = self.resolve_required_path(parsed.file_path.as_deref())?;
                let (line, character) = require_position(parsed.line, parsed.character)?;
                if let Some(result) = self
                    .service
                    .call_hierarchy(&path, line, character, "callHierarchy/outgoingCalls")
                    .await?
                {
                    return Ok(result);
                }
                self.fallback_or_error(|| async {
                    self.fallback_references_like(&path, line, character, "outgoingCalls")
                        .await
                })
                .await
            }
        }
    }

    async fn fallback_or_error<F, Fut>(&self, fallback: F) -> Result<Value>
    where
        F: FnOnce() -> Fut,
        Fut: std::future::Future<Output = Result<Value>>,
    {
        if self.config.fallback_enabled {
            return fallback().await;
        }
        Err(anyhow!("No LSP server available for this file type."))
    }

    fn resolve_required_path(&self, raw: Option<&str>) -> Result<PathBuf> {
        let raw = raw
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| anyhow!("filePath is required for this LSP operation"))?;
        resolve_within_workspace(&self.workspace_root, raw, &self.config.allowed_roots)
            .map_err(map_path_error)
    }

    fn resolve_optional_path(&self, raw: Option<&str>) -> Result<Option<PathBuf>> {
        let Some(raw) = raw.map(str::trim).filter(|value| !value.is_empty()) else {
            return Ok(None);
        };
        resolve_within_workspace(&self.workspace_root, raw, &self.config.allowed_roots)
            .map(Some)
            .map_err(map_path_error)
    }

    async fn fallback_diagnostics(&self, path: &Path) -> Result<Value> {
        let extension = path
            .extension()
            .and_then(|value| value.to_str())
            .unwrap_or_default();
        let output = match extension {
            "py" => {
                run_command(
                    &self.workspace_root,
                    "python3",
                    &["-m", "py_compile", &path.display().to_string()],
                    self.config.timeout_seconds,
                )
                .await?
            }
            "go" => {
                run_command(
                    &self.workspace_root,
                    "go",
                    &["test", "./..."],
                    self.config.timeout_seconds,
                )
                .await?
            }
            "rs" => {
                run_command(
                    &self.workspace_root,
                    "cargo",
                    &["check"],
                    self.config.timeout_seconds,
                )
                .await?
            }
            "ts" | "tsx" | "js" | "jsx" => ToolCommandOutput::skipped("no LSP server available"),
            _ => ToolCommandOutput::skipped("no diagnostics fallback for this file type"),
        };
        Ok(json!({
            "backend": "static_fallback",
            "path": path.display().to_string(),
            "exit_code": output.exit_code,
            "timed_out": output.timed_out,
            "skipped": output.skipped,
            "output": output.output,
        }))
    }

    async fn fallback_document_symbols(&self, path: &Path) -> Result<Value> {
        let content = tokio::fs::read_to_string(path).await?;
        let symbols = extract_symbols(&content);
        Ok(json!({
            "backend": "static_fallback",
            "path": path.display().to_string(),
            "symbols": symbols.iter().map(symbol_json).collect::<Vec<_>>(),
        }))
    }

    async fn fallback_workspace_symbols(&self, query: Option<&str>) -> Result<Value> {
        let query = query.unwrap_or_default().trim().to_lowercase();
        let mut symbols = Vec::new();
        for path in workspace_text_files(&self.workspace_root, 400)? {
            let Ok(content) = std::fs::read_to_string(&path) else {
                continue;
            };
            for symbol in extract_symbols(&content) {
                if query.is_empty() || symbol.name.to_lowercase().contains(&query) {
                    symbols.push(json!({
                        "path": path.display().to_string(),
                        "relative_path": relative_path(&self.workspace_root, &path),
                        "symbol": symbol_json(&symbol),
                    }));
                }
                if symbols.len() >= 100 {
                    break;
                }
            }
            if symbols.len() >= 100 {
                break;
            }
        }
        Ok(json!({
            "backend": "static_fallback",
            "query": query,
            "symbols": symbols,
        }))
    }

    async fn fallback_hover(&self, path: &Path, line: usize, character: usize) -> Result<Value> {
        let content = tokio::fs::read_to_string(path).await?;
        let lines: Vec<&str> = content.lines().collect();
        let current = lines
            .get(line.saturating_sub(1))
            .copied()
            .unwrap_or_default();
        let word = word_at(current, character.saturating_sub(1));
        let symbols = extract_symbols(&content);
        let containing = symbols
            .iter()
            .rev()
            .find(|symbol| symbol.line <= line)
            .map(symbol_json);
        Ok(json!({
            "backend": "static_fallback",
            "path": path.display().to_string(),
            "line": line,
            "character": character,
            "word": word,
            "line_text": current,
            "containing_symbol": containing,
        }))
    }

    async fn fallback_definition_like(
        &self,
        path: &Path,
        line: usize,
        character: usize,
        operation: &str,
    ) -> Result<Value> {
        let symbol = self.fallback_symbol_at_path(path, line, character).await?;
        let locations = self.find_symbol_definitions(&symbol)?;
        Ok(json!({
            "backend": "static_fallback",
            "operation": operation,
            "symbol": symbol,
            "locations": locations,
        }))
    }

    async fn fallback_references_like(
        &self,
        path: &Path,
        line: usize,
        character: usize,
        operation: &str,
    ) -> Result<Value> {
        let symbol = self.fallback_symbol_at_path(path, line, character).await?;
        let locations = self.find_symbol_references(&symbol)?;
        Ok(json!({
            "backend": "static_fallback",
            "operation": operation,
            "symbol": symbol,
            "locations": locations,
        }))
    }

    async fn fallback_symbol_at_path(
        &self,
        path: &Path,
        line: usize,
        character: usize,
    ) -> Result<String> {
        let content = tokio::fs::read_to_string(path).await?;
        let lines: Vec<&str> = content.lines().collect();
        let current = lines
            .get(line.saturating_sub(1))
            .copied()
            .unwrap_or_default();
        let symbol = word_at(current, character.saturating_sub(1));
        if symbol.is_empty() {
            return Err(anyhow!(
                "no symbol found at line {line}, character {character}"
            ));
        }
        Ok(symbol)
    }

    fn find_symbol_definitions(&self, symbol: &str) -> Result<Vec<Value>> {
        let mut locations = Vec::new();
        for path in workspace_text_files(&self.workspace_root, 500)? {
            let Ok(content) = std::fs::read_to_string(&path) else {
                continue;
            };
            for item in extract_symbols(&content) {
                if item.name == symbol {
                    locations.push(json!({
                        "path": path.display().to_string(),
                        "relative_path": relative_path(&self.workspace_root, &path),
                        "line": item.line,
                        "character": item.character,
                        "kind": item.kind,
                        "preview": item.text,
                    }));
                }
            }
            if locations.len() >= 100 {
                break;
            }
        }
        Ok(locations)
    }

    fn find_symbol_references(&self, symbol: &str) -> Result<Vec<Value>> {
        let mut locations = Vec::new();
        for path in workspace_text_files(&self.workspace_root, 500)? {
            let Ok(content) = std::fs::read_to_string(&path) else {
                continue;
            };
            for (line_index, line) in content.lines().enumerate() {
                let mut start = 0;
                while let Some(offset) = line[start..].find(symbol) {
                    let column = start + offset;
                    if is_word_boundary(line, column, symbol.len()) {
                        locations.push(json!({
                            "path": path.display().to_string(),
                            "relative_path": relative_path(&self.workspace_root, &path),
                            "line": line_index + 1,
                            "character": column + 1,
                            "preview": line,
                        }));
                        if locations.len() >= 200 {
                            return Ok(locations);
                        }
                    }
                    start = column + symbol.len();
                }
            }
        }
        Ok(locations)
    }
}

#[async_trait]
impl JsonTool for LspTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: TOOL_NAME.to_string(),
            description: TOOL_DESCRIPTION.to_string(),
            parameters: schema_for::<LspArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

async fn read_loop(
    server_id: String,
    stdout: tokio::process::ChildStdout,
    pending: Arc<Mutex<HashMap<u64, oneshot::Sender<Result<Value, String>>>>>,
    diagnostics: Arc<Mutex<HashMap<String, Value>>>,
    sender: mpsc::Sender<Value>,
) {
    let mut reader = BufReader::new(stdout);
    loop {
        let message = match read_lsp_message(&mut reader).await {
            Ok(Some(message)) => message,
            Ok(None) => break,
            Err(error) => {
                tracing::warn!("lsp read failed for {server_id}: {error}");
                break;
            }
        };
        if let Some(method) = message.get("method").and_then(Value::as_str) {
            if message.get("id").is_some() {
                respond_to_server_request(method, &message, &sender).await;
                continue;
            }
            if method == "textDocument/publishDiagnostics" {
                if let Some(uri) = message
                    .get("params")
                    .and_then(|params| params.get("uri"))
                    .and_then(Value::as_str)
                {
                    let value = message
                        .get("params")
                        .and_then(|params| params.get("diagnostics"))
                        .cloned()
                        .unwrap_or_else(|| json!([]));
                    diagnostics.lock().await.insert(uri.to_string(), value);
                }
            }
            continue;
        }
        let Some(id) = message.get("id").and_then(Value::as_u64) else {
            continue;
        };
        let Some(sender) = pending.lock().await.remove(&id) else {
            continue;
        };
        if let Some(error) = message.get("error") {
            let _ = sender.send(Err(error.to_string()));
            continue;
        }
        let _ = sender.send(Ok(message.get("result").cloned().unwrap_or(Value::Null)));
    }
}

async fn respond_to_server_request(method: &str, message: &Value, sender: &mpsc::Sender<Value>) {
    let Some(id) = message.get("id").cloned() else {
        return;
    };
    let result = match method {
        "workspace/configuration" => json!([]),
        "workspace/workspaceFolders" => json!(null),
        "client/registerCapability"
        | "client/unregisterCapability"
        | "window/workDoneProgress/create" => json!(null),
        _ => json!(null),
    };
    let _ = sender
        .send(json!({
            "jsonrpc": "2.0",
            "id": id,
            "result": result,
        }))
        .await;
}

async fn read_lsp_message(
    reader: &mut BufReader<tokio::process::ChildStdout>,
) -> std::io::Result<Option<Value>> {
    let mut content_length = None;
    loop {
        let mut line = String::new();
        let bytes = reader.read_line(&mut line).await?;
        if bytes == 0 {
            return Ok(None);
        }
        let trimmed = line.trim_end_matches(['\r', '\n']);
        if trimmed.is_empty() {
            break;
        }
        if let Some(value) = trimmed.strip_prefix("Content-Length:") {
            content_length = value.trim().parse::<usize>().ok();
        }
    }
    let Some(content_length) = content_length else {
        return Ok(None);
    };
    let mut content = vec![0; content_length];
    reader.read_exact(&mut content).await?;
    serde_json::from_slice(&content)
        .map(Some)
        .map_err(|error| std::io::Error::new(std::io::ErrorKind::InvalidData, error))
}

async fn write_lsp_message(
    stdin: &mut tokio::process::ChildStdin,
    message: &Value,
) -> std::io::Result<()> {
    let body = serde_json::to_vec(message)
        .map_err(|error| std::io::Error::new(std::io::ErrorKind::InvalidData, error))?;
    stdin
        .write_all(format!("Content-Length: {}\r\n\r\n", body.len()).as_bytes())
        .await?;
    stdin.write_all(&body).await?;
    stdin.flush().await
}

async fn stderr_loop(stderr: tokio::process::ChildStderr, buffer: Arc<Mutex<String>>) {
    let mut reader = BufReader::new(stderr);
    let mut scratch = String::new();
    loop {
        scratch.clear();
        let Ok(bytes) = reader.read_line(&mut scratch).await else {
            break;
        };
        if bytes == 0 {
            break;
        }
        let mut guard = buffer.lock().await;
        guard.push_str(&scratch);
        if guard.len() > 64 * 1024 {
            let mut start = guard.len() - 64 * 1024;
            while start < guard.len() && !guard.is_char_boundary(start) {
                start += 1;
            }
            *guard = guard[start..].to_string();
        }
    }
}

fn server_definitions(config: &LspConfig) -> Vec<ServerDefinition> {
    let mut definitions = builtin_server_definitions();
    for server in &config.servers {
        if server.disabled {
            definitions.retain(|definition| definition.id != server.id);
            continue;
        }
        if server.command.is_empty() {
            continue;
        }
        let custom = custom_server_definition(server);
        if let Some(existing) = definitions
            .iter_mut()
            .find(|definition| definition.id == custom.id)
        {
            *existing = custom;
            continue;
        }
        definitions.push(custom);
    }
    definitions
}

fn custom_server_definition(server: &LspServerConfig) -> ServerDefinition {
    ServerDefinition {
        id: server.id.clone(),
        command: server.command.clone(),
        extensions: server
            .extensions
            .iter()
            .map(|value| normalize_selector(value))
            .collect(),
        root_markers: server.root_markers.clone(),
        strict_root: server.strict_root,
        initialization_options: server.initialization_options.clone(),
    }
}

fn builtin_server_definitions() -> Vec<ServerDefinition> {
    vec![
        ServerDefinition {
            id: "deno".to_string(),
            command: vec!["deno".to_string(), "lsp".to_string()],
            extensions: selectors(&[".ts", ".tsx", ".js", ".jsx", ".mjs"]),
            root_markers: strings(&["deno.json", "deno.jsonc"]),
            strict_root: true,
            initialization_options: None,
        },
        ServerDefinition {
            id: "typescript".to_string(),
            command: vec![
                "typescript-language-server".to_string(),
                "--stdio".to_string(),
            ],
            extensions: selectors(&[".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts"]),
            root_markers: strings(&[
                "package-lock.json",
                "bun.lockb",
                "bun.lock",
                "pnpm-lock.yaml",
                "yarn.lock",
                "package.json",
            ]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "pyright".to_string(),
            command: vec!["pyright-langserver".to_string(), "--stdio".to_string()],
            extensions: selectors(&[".py"]),
            root_markers: strings(&[
                "pyproject.toml",
                "setup.py",
                "setup.cfg",
                "requirements.txt",
                "uv.lock",
                "poetry.lock",
                ".git",
            ]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "gopls".to_string(),
            command: vec!["gopls".to_string()],
            extensions: selectors(&[".go"]),
            root_markers: strings(&["go.work", "go.mod", ".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "rust-analyzer".to_string(),
            command: vec!["rust-analyzer".to_string()],
            extensions: selectors(&[".rs"]),
            root_markers: strings(&["Cargo.toml", "rust-project.json", ".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "clangd".to_string(),
            command: vec!["clangd".to_string(), "--background-index".to_string()],
            extensions: selectors(&[".c", ".h", ".cc", ".hh", ".cpp", ".hpp", ".cxx", ".hxx"]),
            root_markers: strings(&["compile_commands.json", "compile_flags.txt", ".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "json".to_string(),
            command: vec![
                "vscode-json-language-server".to_string(),
                "--stdio".to_string(),
            ],
            extensions: selectors(&[".json", ".jsonc"]),
            root_markers: strings(&["package.json", ".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "yaml".to_string(),
            command: vec!["yaml-language-server".to_string(), "--stdio".to_string()],
            extensions: selectors(&[".yaml", ".yml"]),
            root_markers: strings(&[".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "html".to_string(),
            command: vec![
                "vscode-html-language-server".to_string(),
                "--stdio".to_string(),
            ],
            extensions: selectors(&[".html", ".htm"]),
            root_markers: strings(&["package.json", ".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "css".to_string(),
            command: vec![
                "vscode-css-language-server".to_string(),
                "--stdio".to_string(),
            ],
            extensions: selectors(&[".css", ".scss", ".less"]),
            root_markers: strings(&["package.json", ".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "tailwindcss".to_string(),
            command: vec![
                "tailwindcss-language-server".to_string(),
                "--stdio".to_string(),
            ],
            extensions: selectors(&[
                ".html", ".htm", ".css", ".scss", ".sass", ".less", ".ts", ".tsx", ".js", ".jsx",
                ".mjs", ".cjs", ".mts", ".cts", ".vue", ".svelte", ".astro", ".php", ".rb", ".erb",
                ".heex", ".ex", ".exs", ".eex", ".leex",
            ]),
            root_markers: strings(&[
                "tailwind.config.js",
                "tailwind.config.cjs",
                "tailwind.config.mjs",
                "tailwind.config.ts",
                "tailwind.config.cts",
                "tailwind.config.mts",
                "package.json:tailwindcss",
            ]),
            strict_root: true,
            initialization_options: None,
        },
        ServerDefinition {
            id: "bash".to_string(),
            command: vec!["bash-language-server".to_string(), "start".to_string()],
            extensions: selectors(&[".sh", ".bash", ".zsh"]),
            root_markers: strings(&[".git"]),
            strict_root: false,
            initialization_options: None,
        },
        ServerDefinition {
            id: "dockerfile".to_string(),
            command: vec!["docker-langserver".to_string(), "--stdio".to_string()],
            extensions: selectors(&["Dockerfile", ".dockerfile"]),
            root_markers: strings(&["Dockerfile", ".git"]),
            strict_root: false,
            initialization_options: None,
        },
    ]
}

fn resolve_server_root(
    path: &Path,
    workspace_root: &Path,
    definition: &ServerDefinition,
) -> Option<PathBuf> {
    let start = path.parent().unwrap_or(workspace_root);
    for ancestor in start.ancestors() {
        if !ancestor.starts_with(workspace_root) {
            break;
        }
        if definition
            .root_markers
            .iter()
            .any(|marker| root_marker_matches(ancestor, marker))
        {
            return Some(ancestor.to_path_buf());
        }
        if ancestor == workspace_root {
            break;
        }
    }
    if definition.strict_root {
        return None;
    }
    Some(workspace_root.to_path_buf())
}

fn root_marker_matches(root: &Path, marker: &str) -> bool {
    if marker == "package.json:tailwindcss" {
        return package_json_mentions_tailwind(root);
    }
    root.join(marker).exists()
}

fn package_json_mentions_tailwind(root: &Path) -> bool {
    let Ok(content) = std::fs::read_to_string(root.join("package.json")) else {
        return false;
    };
    serde_json::from_str::<Value>(&content)
        .ok()
        .map(|package| {
            [
                "dependencies",
                "devDependencies",
                "peerDependencies",
                "optionalDependencies",
            ]
            .iter()
            .any(|section| {
                package
                    .get(section)
                    .and_then(Value::as_object)
                    .map(|dependencies| {
                        dependencies.contains_key("tailwindcss")
                            || dependencies
                                .keys()
                                .any(|key| key.starts_with("@tailwindcss/"))
                    })
                    .unwrap_or(false)
            })
        })
        .unwrap_or(false)
}

fn client_capabilities() -> Value {
    json!({
        "workspace": {
            "configuration": true,
            "workspaceFolders": true,
            "symbol": {
                "dynamicRegistration": true,
                "symbolKind": {
                    "valueSet": (1..=26).collect::<Vec<_>>()
                }
            }
        },
        "textDocument": {
            "synchronization": {
                "dynamicRegistration": true,
                "willSave": false,
                "willSaveWaitUntil": false,
                "didSave": true
            },
            "hover": {
                "dynamicRegistration": true,
                "contentFormat": ["markdown", "plaintext"]
            },
            "definition": {
                "dynamicRegistration": true,
                "linkSupport": true
            },
            "references": {
                "dynamicRegistration": true
            },
            "implementation": {
                "dynamicRegistration": true,
                "linkSupport": true
            },
            "documentSymbol": {
                "dynamicRegistration": true,
                "hierarchicalDocumentSymbolSupport": true,
                "symbolKind": {
                    "valueSet": (1..=26).collect::<Vec<_>>()
                }
            },
            "callHierarchy": {
                "dynamicRegistration": true
            },
            "publishDiagnostics": {
                "relatedInformation": true,
                "versionSupport": true,
                "codeDescriptionSupport": true,
                "dataSupport": true
            }
        },
        "window": {
            "workDoneProgress": false
        }
    })
}

fn text_document_position_params(path: &Path, line: usize, character: usize) -> Result<Value> {
    Ok(json!({
        "textDocument": {"uri": file_uri(path)?},
        "position": {
            "line": line.saturating_sub(1),
            "character": character.saturating_sub(1),
        }
    }))
}

fn merge_lsp_results(results: Vec<Value>) -> Value {
    if results.len() == 1 {
        return results.into_iter().next().unwrap_or(Value::Null);
    }
    let all_arrays = results.iter().all(Value::is_array);
    if all_arrays {
        return Value::Array(
            results
                .into_iter()
                .flat_map(|value| value.as_array().cloned().unwrap_or_default())
                .collect(),
        );
    }
    Value::Array(results)
}

fn client_ids(clients: &[Arc<LspClient>]) -> Vec<Value> {
    clients
        .iter()
        .map(|client| {
            let stderr = client
                .stderr
                .try_lock()
                .ok()
                .map(|value| truncate_text(&value, 4096));
            json!({
                "server_id": client.server_id,
                "root": client.root.display().to_string(),
                "stderr": stderr,
            })
        })
        .collect()
}

#[derive(Debug)]
struct ToolCommandOutput {
    exit_code: Option<i32>,
    timed_out: bool,
    skipped: bool,
    output: String,
}

impl ToolCommandOutput {
    fn skipped(reason: &str) -> Self {
        Self {
            exit_code: None,
            timed_out: false,
            skipped: true,
            output: reason.to_string(),
        }
    }
}

async fn run_command(
    workdir: &Path,
    command: &str,
    args: &[&str],
    timeout_seconds: u32,
) -> Result<ToolCommandOutput> {
    if !command_available(command) {
        return Ok(ToolCommandOutput::skipped(&format!(
            "{command} is not installed"
        )));
    }
    let child = Command::new(command)
        .args(args)
        .current_dir(workdir)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .stdin(Stdio::null())
        .kill_on_drop(true)
        .spawn()
        .map_err(|error| anyhow!("spawn {command}: {error}"))?;
    let output = match tokio::time::timeout(
        Duration::from_secs(timeout_seconds.max(1) as u64),
        child.wait_with_output(),
    )
    .await
    {
        Ok(output) => {
            let output = output?;
            ToolCommandOutput {
                exit_code: output.status.code(),
                timed_out: false,
                skipped: false,
                output: truncate_text(
                    &format!(
                        "{}{}",
                        String::from_utf8_lossy(&output.stdout),
                        String::from_utf8_lossy(&output.stderr)
                    ),
                    64 * 1024,
                ),
            }
        }
        Err(_) => ToolCommandOutput {
            exit_code: None,
            timed_out: true,
            skipped: false,
            output: format!("{command} timed out"),
        },
    };
    Ok(output)
}

fn extract_symbols(content: &str) -> Vec<Symbol> {
    let mut symbols = Vec::new();
    let mut container: Option<String> = None;
    for (index, line) in content.lines().enumerate() {
        let trimmed = line.trim_start();
        let indent = line.len().saturating_sub(trimmed.len());
        let candidates = [
            ("function", "fn "),
            ("function", "def "),
            ("function", "func "),
            ("function", "function "),
            ("class", "class "),
            ("struct", "struct "),
            ("enum", "enum "),
            ("trait", "trait "),
            ("interface", "interface "),
            ("type", "type "),
            ("impl", "impl "),
        ];
        for (kind, prefix) in candidates {
            if let Some(rest) = trimmed.strip_prefix(prefix) {
                let name = rest
                    .trim_start()
                    .split(|ch: char| !(ch.is_ascii_alphanumeric() || ch == '_'))
                    .next()
                    .unwrap_or_default();
                if !name.is_empty() {
                    let symbol = Symbol {
                        name: name.to_string(),
                        kind: kind.to_string(),
                        line: index + 1,
                        character: indent + prefix.len() + 1,
                        container: container.clone(),
                        text: line.to_string(),
                    };
                    if matches!(
                        kind,
                        "class" | "struct" | "enum" | "trait" | "interface" | "impl"
                    ) {
                        container = Some(name.to_string());
                    }
                    symbols.push(symbol);
                }
                break;
            }
        }
    }
    symbols
}

fn symbol_json(symbol: &Symbol) -> Value {
    json!({
        "name": symbol.name,
        "kind": symbol.kind,
        "line": symbol.line,
        "character": symbol.character,
        "container": symbol.container,
        "preview": symbol.text,
    })
}

fn workspace_text_files(root: &Path, max_files: usize) -> Result<Vec<PathBuf>> {
    let mut files = Vec::new();
    let mut queue = VecDeque::from([root.to_path_buf()]);
    while let Some(path) = queue.pop_front() {
        if files.len() >= max_files {
            break;
        }
        let Ok(entries) = std::fs::read_dir(&path) else {
            continue;
        };
        for entry in entries.flatten() {
            let path = entry.path();
            let name = path
                .file_name()
                .and_then(|value| value.to_str())
                .unwrap_or_default();
            if should_skip(name) {
                continue;
            }
            if path.is_dir() {
                queue.push_back(path);
                continue;
            }
            if is_text_source(&path) {
                files.push(path);
                if files.len() >= max_files {
                    break;
                }
            }
        }
    }
    Ok(files)
}

fn should_skip(name: &str) -> bool {
    matches!(
        name,
        ".git" | "node_modules" | "target" | "dist" | "build" | ".next" | "vendor"
    )
}

fn is_text_source(path: &Path) -> bool {
    matches!(
        path.extension()
            .and_then(|value| value.to_str())
            .unwrap_or_default(),
        "rs" | "go"
            | "py"
            | "ts"
            | "tsx"
            | "js"
            | "jsx"
            | "java"
            | "kt"
            | "c"
            | "h"
            | "cpp"
            | "hpp"
            | "rb"
            | "php"
            | "swift"
            | "cs"
    )
}

fn word_at(line: &str, character: usize) -> String {
    let bytes = line.as_bytes();
    let mut start = character.min(bytes.len());
    while start > 0 && is_ident(bytes[start - 1]) {
        start -= 1;
    }
    let mut end = character.min(bytes.len());
    while end < bytes.len() && is_ident(bytes[end]) {
        end += 1;
    }
    line[start..end].to_string()
}

fn is_word_boundary(line: &str, start: usize, len: usize) -> bool {
    let bytes = line.as_bytes();
    let before = start == 0 || !is_ident(bytes[start - 1]);
    let after_index = start + len;
    let after = after_index >= bytes.len() || !is_ident(bytes[after_index]);
    before && after
}

fn is_ident(byte: u8) -> bool {
    byte.is_ascii_alphanumeric() || byte == b'_'
}

fn require_position(line: Option<usize>, character: Option<usize>) -> Result<(usize, usize)> {
    let line = line.ok_or_else(|| anyhow!("line is required for this LSP operation"))?;
    let character =
        character.ok_or_else(|| anyhow!("character is required for this LSP operation"))?;
    if line == 0 || character == 0 {
        return Err(anyhow!(
            "line and character are 1-based and must be greater than zero"
        ));
    }
    Ok((line, character))
}

fn file_uri(path: &Path) -> Result<String> {
    Url::from_file_path(path)
        .map(|url| url.to_string())
        .map_err(|_| anyhow!("could not convert {} to file URI", path.display()))
}

fn language_id(path: &Path) -> &'static str {
    match path
        .extension()
        .and_then(|value| value.to_str())
        .unwrap_or_default()
    {
        "rs" => "rust",
        "go" => "go",
        "py" => "python",
        "ts" | "mts" | "cts" => "typescript",
        "tsx" => "typescriptreact",
        "js" | "mjs" | "cjs" => "javascript",
        "jsx" => "javascriptreact",
        "json" | "jsonc" => "json",
        "yaml" | "yml" => "yaml",
        "html" | "htm" => "html",
        "css" => "css",
        "scss" => "scss",
        "sass" => "sass",
        "less" => "less",
        "vue" => "vue",
        "svelte" => "svelte",
        "astro" => "astro",
        "php" => "php",
        "rb" => "ruby",
        "erb" => "erb",
        "heex" => "phoenix-heex",
        "ex" | "exs" => "elixir",
        "eex" | "leex" => "eex",
        "sh" | "bash" | "zsh" => "shellscript",
        "c" | "h" => "c",
        "cc" | "hh" | "cpp" | "hpp" | "cxx" | "hxx" => "cpp",
        _ if path.file_name().and_then(|value| value.to_str()) == Some("Dockerfile") => {
            "dockerfile"
        }
        _ => "plaintext",
    }
}

fn file_selector(path: &Path) -> String {
    if let Some(name) = path.file_name().and_then(|value| value.to_str()) {
        if name == "Dockerfile" {
            return name.to_string();
        }
    }
    path.extension()
        .and_then(|value| value.to_str())
        .map(|extension| format!(".{extension}"))
        .unwrap_or_default()
}

fn selectors(values: &[&str]) -> Vec<String> {
    values
        .iter()
        .map(|value| normalize_selector(value))
        .collect()
}

fn strings(values: &[&str]) -> Vec<String> {
    values.iter().map(|value| value.to_string()).collect()
}

fn normalize_selector(value: &str) -> String {
    if value.starts_with('.') || value == "Dockerfile" {
        return value.to_string();
    }
    format!(".{value}")
}

fn command_available(command: &str) -> bool {
    let path = Path::new(command);
    if path.components().count() > 1 {
        return path.exists();
    }
    std::env::var_os("PATH")
        .map(|paths| std::env::split_paths(&paths).any(|path| path.join(command).exists()))
        .unwrap_or(false)
}

fn relative_path(root: &Path, path: &Path) -> String {
    path.strip_prefix(root)
        .unwrap_or(path)
        .to_string_lossy()
        .replace('\\', "/")
}

fn truncate_text(text: &str, max_bytes: usize) -> String {
    if text.len() <= max_bytes {
        return text.to_string();
    }
    let mut end = max_bytes;
    while !text.is_char_boundary(end) {
        end -= 1;
    }
    format!("{}...[truncated]", &text[..end])
}

fn map_path_error(error: PathPolicyError) -> anyhow::Error {
    anyhow!(error.to_string())
}
