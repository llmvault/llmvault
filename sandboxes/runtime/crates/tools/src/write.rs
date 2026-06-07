use std::path::PathBuf;
use std::sync::Arc;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::WriteFileConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::mutation_queue::with_file_lock;
use crate::operations::WriteOperations;
use crate::path::{build_glob_set, enforce_deny_globs, resolve_writable_path, PathPolicyError};
use crate::{schema_for, JsonTool, ToolDefinition};

const TOOL_NAME: &str = "write_file";
const TOOL_DESCRIPTION: &str =
    "Write content to a file inside the workspace, /tmp, /var/tmp, $HOME, \
     or configured allowed roots. Creates the file if it does not exist, \
     overwrites if it does. Parent directories are created automatically. \
     Refuses paths outside writable roots or paths matching a deny glob.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct WriteArgs {
    /// Path to the file to write (relative to the workspace root or absolute).
    pub path: String,
    /// Full UTF-8 content to place in the file.
    pub content: String,
}

pub struct WriteTool {
    config: WriteFileConfig,
    workspace_root: PathBuf,
    operations: Arc<dyn WriteOperations>,
}

impl WriteTool {
    pub fn new(
        config: WriteFileConfig,
        workspace_root: PathBuf,
        operations: Arc<dyn WriteOperations>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            operations,
        }
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: WriteArgs =
            serde_json::from_value(args).map_err(|e| anyhow!("invalid arguments: {e}"))?;
        let resolved = resolve_writable_path(
            &self.workspace_root,
            &parsed.path,
            &self.config.allowed_roots,
        )
        .map_err(map_path_error)?;
        let deny_globs = build_glob_set(&self.config.deny_globs);
        enforce_deny_globs(&resolved, &deny_globs).map_err(map_path_error)?;

        let bytes = parsed.content.as_bytes();
        let max_bytes = self.config.max_file_size_bytes as usize;
        if bytes.len() > max_bytes {
            return Err(anyhow!(
                "content size {} exceeds max_file_size_bytes ({})",
                bytes.len(),
                max_bytes
            ));
        }

        if let Some(parent) = resolved.parent() {
            self.operations
                .mkdir_all(parent)
                .await
                .map_err(|e| anyhow!("mkdir {}: {e}", parent.display()))?;
        }

        let resolved_for_lock = resolved.clone();
        let path_for_write = resolved.clone();
        let operations = self.operations.clone();
        let payload = parsed.content.clone();
        let bytes_count = bytes.len();
        let outcome = with_file_lock(&resolved_for_lock, move || {
            let operations = operations.clone();
            let path_for_write = path_for_write.clone();
            let payload = payload.into_bytes();
            async move { operations.write_file(&path_for_write, &payload).await }
        })
        .await;
        outcome.map_err(|e| anyhow!("write failed for {}: {e}", parsed.path))?;

        Ok(json!({
            "path": resolved.display().to_string(),
            "bytes_written": bytes_count,
        }))
    }
}

#[async_trait]
impl JsonTool for WriteTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: TOOL_NAME.to_string(),
            description: TOOL_DESCRIPTION.to_string(),
            parameters: schema_for::<WriteArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

fn map_path_error(error: PathPolicyError) -> anyhow::Error {
    anyhow!(error.to_string())
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use domain::WriteFileConfig;

    use crate::operations::LocalFsOperations;

    #[tokio::test]
    async fn write_file_allows_absolute_tmp_path() {
        let unique = format!(
            "hivy-write-file-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        );
        let path = std::path::PathBuf::from("/tmp")
            .join(unique)
            .join("out.txt");
        let tool = super::WriteTool::new(
            WriteFileConfig {
                allowed_roots: Vec::new(),
                max_file_size_bytes: 1024,
                deny_globs: Vec::new(),
                atomic: true,
            },
            std::env::current_dir().unwrap(),
            Arc::new(LocalFsOperations),
        );

        let result = tool
            .execute(serde_json::json!({
                "path": path.display().to_string(),
                "content": "written under tmp",
            }))
            .await
            .expect("tmp file should be writable");

        assert_eq!(result["bytes_written"], 17);
        assert_eq!(
            tokio::fs::read_to_string(&path).await.unwrap(),
            "written under tmp"
        );
        if let Some(dir) = path.parent() {
            let _ = tokio::fs::remove_dir_all(dir).await;
        }
    }
}
