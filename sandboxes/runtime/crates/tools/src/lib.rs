mod apply_patch;
mod bash;
mod diff;
mod edit;
mod lsp;
mod mutation_queue;
mod operations;
mod path;
mod process_registry;
mod read;
mod search;
mod truncate;
mod write;

use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use domain::ToolSpec;

pub use operations::*;
pub use truncate::*;

pub use apply_patch::ApplyPatchTool;
pub use bash::BashTool;
pub use edit::EditTool;
pub use lsp::{LspService, LspTool};
pub use process_registry::ProcessRegistry;
pub use read::ReadTool;
pub use search::{FileSearchTool, GlobTool, GrepTool, MultiGrepTool, SearchService};
pub use write::WriteTool;

#[async_trait::async_trait]
pub trait JsonTool: Send + Sync {
    fn definition(&self) -> ToolDefinition;
    async fn call(&self, args: serde_json::Value) -> anyhow::Result<serde_json::Value>;
}

pub fn schema_for<T: schemars::JsonSchema>() -> serde_json::Value {
    serde_json::to_value(schemars::schema_for!(T))
        .unwrap_or_else(|_| serde_json::json!({"type":"object"}))
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct ToolDefinition {
    pub name: String,
    pub description: String,
    pub parameters: serde_json::Value,
}

#[derive(Debug, Clone)]
pub struct FileReadSnapshot {
    pub hash: u64,
    pub len: usize,
}

pub type FileReadRegistry = Arc<Mutex<HashMap<PathBuf, FileReadSnapshot>>>;

pub fn file_read_snapshot(bytes: &[u8]) -> FileReadSnapshot {
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    bytes.hash(&mut hasher);
    FileReadSnapshot {
        hash: hasher.finish(),
        len: bytes.len(),
    }
}

#[derive(Clone)]
pub struct ToolBuildContext {
    pub workspace_root: PathBuf,
    pub fs: Arc<LocalFsOperations>,
    pub bash: Arc<LocalBashOperations>,
    pub runtime_env: Arc<HashMap<String, String>>,
    pub process_registry: Arc<ProcessRegistry>,
    pub search: SearchService,
    pub lsp: LspService,
    pub files_read: FileReadRegistry,
}

impl ToolBuildContext {
    pub fn new(workspace_root: PathBuf) -> Self {
        Self {
            workspace_root: workspace_root.clone(),
            fs: Arc::new(LocalFsOperations),
            bash: Arc::new(LocalBashOperations),
            runtime_env: Arc::new(HashMap::new()),
            process_registry: Arc::new(ProcessRegistry::new()),
            search: SearchService::new(workspace_root.clone()),
            lsp: LspService::new(workspace_root.clone()),
            files_read: Arc::new(Mutex::new(HashMap::new())),
        }
    }
}

pub fn build_builtin_tools(
    specs: &[ToolSpec],
    context: &ToolBuildContext,
    session_id: &domain::SessionId,
) -> Vec<Arc<dyn JsonTool>> {
    let mut tools: Vec<Arc<dyn JsonTool>> = Vec::new();
    for spec in specs {
        match spec {
            ToolSpec::Bash(config) => {
                tools.push(
                    BashTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.bash.clone(),
                        context.runtime_env.clone(),
                    )
                    .with_process_registry(context.process_registry.clone())
                    .with_session_id(session_id.as_str())
                    .into_tool(),
                );
            }
            ToolSpec::ReadFile(config) => {
                tools.push(
                    ReadTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.fs.clone(),
                    )
                    .with_files_read(context.files_read.clone())
                    .with_search_service(context.search.clone())
                    .with_lsp_service(context.lsp.clone())
                    .into_tool(),
                );
            }
            ToolSpec::WriteFile(config) => {
                tools.push(
                    WriteTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.fs.clone(),
                    )
                    .with_search_service(context.search.clone())
                    .with_lsp_service(context.lsp.clone())
                    .into_tool(),
                );
                tools.push(
                    EditTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.fs.clone(),
                    )
                    .with_files_read(context.files_read.clone())
                    .with_search_service(context.search.clone())
                    .with_lsp_service(context.lsp.clone())
                    .into_tool(),
                );
            }
            ToolSpec::FileSearch(config) => {
                tools.push(FileSearchTool::new(config.clone(), context.search.clone()).into_tool());
            }
            ToolSpec::Glob(config) => {
                tools.push(GlobTool::new(config.clone(), context.search.clone()).into_tool());
            }
            ToolSpec::Grep(config) => {
                tools.push(GrepTool::new(config.clone(), context.search.clone()).into_tool());
            }
            ToolSpec::MultiGrep(config) => {
                tools.push(MultiGrepTool::new(config.clone(), context.search.clone()).into_tool());
            }
            ToolSpec::ApplyPatch(config) => {
                tools.push(
                    ApplyPatchTool::new(config.clone(), context.workspace_root.clone())
                        .with_search_service(context.search.clone())
                        .with_lsp_service(context.lsp.clone())
                        .into_tool(),
                );
            }
            ToolSpec::Lsp(config) => {
                tools.push(
                    LspTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.lsp.clone(),
                    )
                    .into_tool(),
                );
            }
            ToolSpec::Cron
            | ToolSpec::SubagentTask(_)
            | ToolSpec::CheckSubagentTaskStatus
            | ToolSpec::CheckBashStatus
            | ToolSpec::Wake
            | ToolSpec::SearchSessions
            | ToolSpec::RequestUserInput
            | ToolSpec::UpdatePlan
            | ToolSpec::SkillsList
            | ToolSpec::SkillView
            | ToolSpec::SkillManage => {}
        }
    }
    tools
}
