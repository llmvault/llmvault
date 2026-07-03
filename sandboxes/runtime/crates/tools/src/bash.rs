use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::BashConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Deserializer, Serialize};
use serde_json::{json, Value};

use crate::operations::{BashExecOptions, BashOperations};
use crate::process_registry::ProcessRegistry;
use crate::truncate::{truncate_tail, TruncationReason};
use crate::{schema_for, JsonTool, ToolDefinition};

const TOOL_NAME: &str = "bash";
/// Sentinel prefix applied by the control plane to user-supplied environment
/// variables inside `runtime_env`, to distinguish them from platform
/// `HIVY_*` control-plane variables. The prefix must never leak into a
/// spawned child process's environment.
const USER_ENV_PREFIX: &str = "__ENV__";
const TOOL_DESCRIPTION: &str =
    "Run a shell command in the workspace and return its combined stdout/stderr. \
     Output is truncated to the last 2000 lines or 50KB, whichever comes first. \
     Set run_in_background=true for commands that take a long time. Use \
     check_bash_status with its cursor to poll progress. Use this for terminal \
     operations such as tests, package managers, git, and servers. Do not use \
     bash for reading, writing, editing, finding, globbing, or grepping files; \
     use the specialized tools. Commands matching a denied pattern are rejected \
     before execution.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct BashArgs {
    pub command: String,
    #[serde(default, deserialize_with = "deserialize_timeout_seconds")]
    pub timeout_seconds: Option<u32>,
    #[serde(default)]
    pub run_in_background: bool,
}

pub struct BashTool {
    config: BashConfig,
    workspace_root: PathBuf,
    operations: Arc<dyn BashOperations>,
    runtime_env: Arc<HashMap<String, String>>,
    process_registry: Option<Arc<ProcessRegistry>>,
    session_id: Option<String>,
}

impl BashTool {
    pub fn new(
        config: BashConfig,
        workspace_root: PathBuf,
        operations: Arc<dyn BashOperations>,
        runtime_env: Arc<HashMap<String, String>>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            operations,
            runtime_env,
            process_registry: None,
            session_id: None,
        }
    }

    pub fn with_process_registry(mut self, registry: Arc<ProcessRegistry>) -> Self {
        self.process_registry = Some(registry);
        self
    }

    pub fn with_session_id(mut self, session_id: impl Into<String>) -> Self {
        self.session_id = Some(session_id.into());
        self
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: BashArgs =
            serde_json::from_value(args).map_err(|e| anyhow!("invalid arguments: {e}"))?;
        let command = parsed.command.trim();
        if command.is_empty() {
            return Err(anyhow!("`command` must not be empty"));
        }
        if let Some(matched) = command_matches_deny_pattern(command, &self.config.deny_patterns) {
            return Err(anyhow!(
                "command rejected: matches deny pattern `{matched}`"
            ));
        }

        let workdir = resolve_workdir(&self.workspace_root, &self.config.workdir);
        if !workdir.exists() {
            return Err(anyhow!("workdir does not exist: {}", workdir.display()));
        }

        let timeout = parsed
            .timeout_seconds
            .map(|seconds| seconds.max(1))
            .unwrap_or(self.config.timeout_seconds.max(1));

        let mut env: HashMap<String, String> = HashMap::new();
        if self.config.env_passthrough.is_empty() {
            env.extend(
                self.runtime_env
                    .iter()
                    .map(|(key, value)| (key.clone(), value.clone())),
            );
            strip_user_env_prefix(&mut env);
        } else {
            for key in &self.config.env_passthrough {
                if let Some(value) = self
                    .runtime_env
                    .get(key)
                    .or_else(|| self.runtime_env.get(&format!("{USER_ENV_PREFIX}{key}")))
                    .cloned()
                    .or_else(|| std::env::var(key).ok())
                {
                    env.insert(key.clone(), value);
                }
            }
        }
        env.entry("HOME".into())
            .or_insert_with(|| std::env::var("HOME").unwrap_or_default());
        env.entry("PATH".into())
            .or_insert_with(|| std::env::var("PATH").unwrap_or_default());

        if parsed.run_in_background {
            let registry = self
                .process_registry
                .as_ref()
                .ok_or_else(|| anyhow!("background processes not available"))?;
            let process_id = registry
                .spawn(
                    command,
                    workdir,
                    env,
                    timeout as u64,
                    self.config.max_output_bytes,
                    self.session_id.clone(),
                )
                .map_err(|error| anyhow!("background spawn failed: {error}"))?;
            return Ok(json!({
                "background": true,
                "process_id": process_id,
                "command": command,
            }));
        }

        let options = BashExecOptions {
            workdir,
            env,
            timeout: Some(Duration::from_secs(timeout as u64)),
            max_output_bytes: self.config.max_output_bytes,
        };

        let result = self
            .operations
            .exec(command, options)
            .await
            .map_err(|e| anyhow!("bash exec: {e}"))?;
        let output_text = String::from_utf8_lossy(&result.stdout_combined).to_string();
        let truncated = truncate_tail(&output_text, 2000, 50 * 1024);

        Ok(json!({
            "command": command,
            "exit_code": result.exit_code,
            "timed_out": result.timed_out,
            "truncated": truncated.truncated || result.truncated,
            "truncated_by": match truncated.truncated_by {
                TruncationReason::NotTruncated => "none",
                TruncationReason::Lines => "lines",
                TruncationReason::Bytes => "bytes",
            },
            "shown_lines": truncated.output_lines,
            "shown_bytes": truncated.output_bytes,
            "total_lines": truncated.total_lines,
            "total_bytes": truncated.total_bytes,
            "output": truncated.content,
        }))
    }
}

#[async_trait]
impl JsonTool for BashTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: TOOL_NAME.to_string(),
            description: TOOL_DESCRIPTION.to_string(),
            parameters: schema_for::<BashArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

/// Rewrites `env` in place: every key prefixed with [`USER_ENV_PREFIX`] is
/// replaced by its stripped name (e.g. `__ENV__DATABASE_URL` becomes
/// `DATABASE_URL`), and the prefixed key is removed so it never reaches the
/// child process. A pre-existing entry under the clean name is overwritten,
/// since the user-supplied value is authoritative for their own variable
/// names. Platform `HIVY_*` keys are left untouched.
fn strip_user_env_prefix(env: &mut HashMap<String, String>) {
    let prefixed_keys: Vec<String> = env
        .keys()
        .filter(|key| key.starts_with(USER_ENV_PREFIX))
        .cloned()
        .collect();
    for prefixed_key in prefixed_keys {
        if let Some(value) = env.remove(&prefixed_key) {
            let clean_key = prefixed_key
                .strip_prefix(USER_ENV_PREFIX)
                .unwrap_or(&prefixed_key)
                .to_string();
            env.insert(clean_key, value);
        }
    }
}

fn command_matches_deny_pattern<'a>(command: &str, deny_patterns: &'a [String]) -> Option<&'a str> {
    deny_patterns
        .iter()
        .find(|pattern| !pattern.is_empty() && command.contains(pattern.as_str()))
        .map(String::as_str)
}

fn resolve_workdir(workspace_root: &std::path::Path, configured: &str) -> PathBuf {
    if configured.trim().is_empty() {
        return workspace_root.to_path_buf();
    }
    let candidate = PathBuf::from(configured);
    if candidate.is_absolute() {
        candidate
    } else {
        workspace_root.join(candidate)
    }
}

fn deserialize_timeout_seconds<'de, D>(
    deserializer: D,
) -> std::result::Result<Option<u32>, D::Error>
where
    D: Deserializer<'de>,
{
    let value = Option::<Value>::deserialize(deserializer)?;
    let Some(value) = value else {
        return Ok(None);
    };
    match value {
        Value::Number(number) => number
            .as_u64()
            .and_then(|value| u32::try_from(value).ok())
            .map(Some)
            .ok_or_else(|| serde::de::Error::custom("timeout_seconds must be a u32")),
        Value::String(raw) => {
            let trimmed = raw.trim();
            if trimmed.is_empty() {
                return Err(serde::de::Error::custom(
                    "timeout_seconds must be a numeric string",
                ));
            }
            trimmed
                .parse::<u32>()
                .map(Some)
                .map_err(|_| serde::de::Error::custom("timeout_seconds must be a numeric string"))
        }
        _ => Err(serde::de::Error::custom(
            "timeout_seconds must be a number or numeric string",
        )),
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::env;
    use std::sync::Arc;

    use async_trait::async_trait;

    use crate::operations::{BashError, BashExecOptions, BashExecResult, BashOperations};

    use super::{BashArgs, BashConfig};

    struct EchoEnvOperations {
        key: &'static str,
    }

    #[async_trait]
    impl BashOperations for EchoEnvOperations {
        async fn exec(
            &self,
            _command: &str,
            options: BashExecOptions,
        ) -> Result<BashExecResult, BashError> {
            Ok(BashExecResult {
                stdout_combined: options
                    .env
                    .get(self.key)
                    .cloned()
                    .unwrap_or_default()
                    .into_bytes(),
                exit_code: Some(0),
                timed_out: false,
                truncated: false,
            })
        }
    }

    #[test]
    fn timeout_seconds_accepts_number() {
        let parsed: BashArgs = serde_json::from_value(serde_json::json!({
            "command": "echo ok",
            "timeout_seconds": 30,
        }))
        .expect("numeric timeout should parse");

        assert_eq!(parsed.timeout_seconds, Some(30));
    }

    #[test]
    fn timeout_seconds_accepts_numeric_string() {
        let parsed: BashArgs = serde_json::from_value(serde_json::json!({
            "command": "echo ok",
            "timeout_seconds": "30",
        }))
        .expect("numeric string timeout should parse");

        assert_eq!(parsed.timeout_seconds, Some(30));
    }

    #[test]
    fn timeout_seconds_rejects_non_numeric_string() {
        let err = serde_json::from_value::<BashArgs>(serde_json::json!({
            "command": "echo ok",
            "timeout_seconds": "abc",
        }))
        .expect_err("non-numeric timeout should fail");

        assert!(
            err.to_string().contains("numeric string"),
            "unexpected error: {err}"
        );
    }

    #[tokio::test]
    async fn destructive_deny_patterns_still_reject_commands() {
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: vec!["rm -rf /".to_string()],
                env_passthrough: Vec::new(),
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(EchoEnvOperations { key: "UNUSED" }),
            Arc::new(HashMap::new()),
        );

        let err = tool
            .execute(serde_json::json!({
                "command": "rm -rf /",
                "timeout_seconds": "30",
            }))
            .await
            .expect_err("destructive command should be rejected");

        assert!(
            err.to_string().contains("deny pattern"),
            "unexpected error: {err}"
        );
    }

    #[tokio::test]
    async fn runtime_env_overlays_process_for_bash_passthrough() {
        let runtime_env = Arc::new(HashMap::from([(
            "RUNTIME_ENV_OVERLAY".to_string(),
            "overlay-value".to_string(),
        )]));
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: Vec::new(),
                env_passthrough: vec!["RUNTIME_ENV_OVERLAY".to_string()],
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(EchoEnvOperations {
                key: "RUNTIME_ENV_OVERLAY",
            }),
            runtime_env,
        );

        let original = env::var("RUNTIME_ENV_OVERLAY").ok();
        env::set_var("RUNTIME_ENV_OVERLAY", "process-value");
        let result = tool
            .execute(serde_json::json!({
                "command": "printf \"$RUNTIME_ENV_OVERLAY\"",
                "timeout_seconds": 1,
                "run_in_background": false,
            }))
            .await
            .expect("command should succeed");
        match original {
            Some(value) => env::set_var("RUNTIME_ENV_OVERLAY", value),
            None => env::remove_var("RUNTIME_ENV_OVERLAY"),
        }

        assert_eq!(result["output"], "overlay-value");
    }

    #[tokio::test]
    async fn process_env_falls_back_when_runtime_overlay_missing() {
        let runtime_env = Arc::new(HashMap::new());
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: Vec::new(),
                env_passthrough: vec!["RUNTIME_ENV_FALLBACK".to_string()],
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(EchoEnvOperations {
                key: "RUNTIME_ENV_FALLBACK",
            }),
            runtime_env,
        );

        let original = env::var("RUNTIME_ENV_FALLBACK").ok();
        env::set_var("RUNTIME_ENV_FALLBACK", "process-fallback");
        let result = tool
            .execute(serde_json::json!({
                "command": "printf \"$RUNTIME_ENV_FALLBACK\"",
                "timeout_seconds": 1,
                "run_in_background": false,
            }))
            .await
            .expect("command should succeed");
        match original {
            Some(value) => env::set_var("RUNTIME_ENV_FALLBACK", value),
            None => env::remove_var("RUNTIME_ENV_FALLBACK"),
        }

        assert_eq!(result["output"], "process-fallback");
    }

    #[tokio::test]
    async fn empty_passthrough_passes_all_runtime_env() {
        let runtime_env = Arc::new(HashMap::from([
            (
                "HIVY_RAILWAY_API_URL".to_string(),
                "https://railway.test".to_string(),
            ),
            (
                "HIVY_VERCEL_API_URL".to_string(),
                "https://vercel.test".to_string(),
            ),
        ]));
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: Vec::new(),
                env_passthrough: Vec::new(),
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(EchoEnvOperations {
                key: "HIVY_VERCEL_API_URL",
            }),
            runtime_env,
        );

        let result = tool
            .execute(serde_json::json!({
                "command": "printf \"$HIVY_VERCEL_API_URL\"",
                "timeout_seconds": 1,
                "run_in_background": false,
            }))
            .await
            .expect("command should succeed");

        assert_eq!(result["output"], "https://vercel.test");
    }

    struct DumpEnvOperations;

    #[async_trait]
    impl BashOperations for DumpEnvOperations {
        async fn exec(
            &self,
            _command: &str,
            options: BashExecOptions,
        ) -> Result<BashExecResult, BashError> {
            Ok(BashExecResult {
                stdout_combined: format!("{:?}", options.env).into_bytes(),
                exit_code: Some(0),
                timed_out: false,
                truncated: false,
            })
        }
    }

    #[tokio::test]
    async fn empty_passthrough_strips_user_env_prefix() {
        let runtime_env = Arc::new(HashMap::from([
            (
                "__ENV__DATABASE_URL".to_string(),
                "postgres://x".to_string(),
            ),
            ("HIVY_ORG_ID".to_string(), "abc".to_string()),
        ]));
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: Vec::new(),
                env_passthrough: Vec::new(),
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(DumpEnvOperations),
            runtime_env,
        );

        let result = tool
            .execute(serde_json::json!({
                "command": "irrelevant",
                "timeout_seconds": 1,
                "run_in_background": false,
            }))
            .await
            .expect("command should succeed");

        let output = result["output"].as_str().expect("output should be a string");
        assert!(
            output.contains("\"DATABASE_URL\": \"postgres://x\""),
            "expected clean DATABASE_URL in env, got: {output}"
        );
        assert!(
            !output.contains("__ENV__DATABASE_URL"),
            "raw __ENV__ prefixed key leaked into env: {output}"
        );
        assert!(
            output.contains("\"HIVY_ORG_ID\": \"abc\""),
            "expected HIVY_ORG_ID to pass through unchanged, got: {output}"
        );
    }

    #[tokio::test]
    async fn explicit_passthrough_matches_user_env_prefixed_key() {
        let runtime_env = Arc::new(HashMap::from([(
            "__ENV__DATABASE_URL".to_string(),
            "postgres://explicit".to_string(),
        )]));
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: Vec::new(),
                env_passthrough: vec!["DATABASE_URL".to_string()],
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(EchoEnvOperations {
                key: "DATABASE_URL",
            }),
            runtime_env,
        );

        let result = tool
            .execute(serde_json::json!({
                "command": "printf \"$DATABASE_URL\"",
                "timeout_seconds": 1,
                "run_in_background": false,
            }))
            .await
            .expect("command should succeed");

        assert_eq!(result["output"], "postgres://explicit");
    }
}
