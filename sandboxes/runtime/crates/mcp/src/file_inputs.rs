use std::path::{Path, PathBuf};

use anyhow::Context;
use domain::{ToolInputBinding, ToolInputBindingKind};
use rmcp::model::JsonObject;
use serde_json::{json, Value};
use tokio::io::AsyncReadExt;

const MAX_CONFIGURED_FILE_BYTES: u64 = 16 * 1024 * 1024;

pub(crate) fn project_tool_schema(
    parameters: &mut Value,
    bindings: &[ToolInputBinding],
) -> anyhow::Result<()> {
    for binding in bindings {
        validate_binding(binding)?;
        let schema = parameters
            .as_object_mut()
            .context("MCP tool input schema must be an object")?;
        let properties = schema
            .get_mut("properties")
            .and_then(Value::as_object_mut)
            .context("MCP tool input schema must define object properties")?;
        if properties.contains_key(&binding.path_argument) {
            anyhow::bail!(
                "MCP file input path argument '{}' collides with an upstream property",
                binding.path_argument
            );
        }
        let content_schema = properties
            .remove(&binding.content_argument)
            .ok_or_else(|| {
                anyhow::anyhow!(
                    "MCP file input content argument '{}' is not present in the upstream schema",
                    binding.content_argument
                )
            })?;
        if content_schema.get("type").and_then(Value::as_str) != Some("string") {
            anyhow::bail!(
                "MCP file input content argument '{}' must have type string",
                binding.content_argument
            );
        }
        properties.insert(
            binding.path_argument.clone(),
            json!({
                "type": "string",
                "description": format!(
                    "Path to a UTF-8 {} file inside the sandbox workspace. The runtime reads the file and supplies its contents without exposing them to the model tool call.",
                    binding.allowed_extensions.join(" or ")
                )
            }),
        );
        if let Some(required) = schema.get_mut("required").and_then(Value::as_array_mut) {
            for name in required {
                if name.as_str() == Some(&binding.content_argument) {
                    *name = Value::String(binding.path_argument.clone());
                }
            }
        }
    }
    Ok(())
}

pub(crate) async fn apply_tool_input_bindings(
    workspace_root: &Path,
    bindings: &[ToolInputBinding],
    arguments: &mut JsonObject,
) -> anyhow::Result<()> {
    for binding in bindings {
        validate_binding(binding)?;
        let Some(path_value) = arguments.remove(&binding.path_argument) else {
            continue;
        };
        if arguments.contains_key(&binding.content_argument) {
            anyhow::bail!(
                "provide '{}' or '{}', not both",
                binding.path_argument,
                binding.content_argument
            );
        }
        let supplied_path = path_value.as_str().ok_or_else(|| {
            anyhow::anyhow!("'{}' must be a file path string", binding.path_argument)
        })?;
        let content = read_workspace_text_file(workspace_root, supplied_path, binding).await?;
        arguments.insert(binding.content_argument.clone(), Value::String(content));
    }
    Ok(())
}

fn validate_binding(binding: &ToolInputBinding) -> anyhow::Result<()> {
    if binding.kind != ToolInputBindingKind::WorkspaceTextFile {
        anyhow::bail!("unsupported MCP tool input binding kind");
    }
    if binding.tool.trim().is_empty()
        || binding.path_argument.trim().is_empty()
        || binding.content_argument.trim().is_empty()
        || binding.path_argument == binding.content_argument
    {
        anyhow::bail!("MCP tool input binding has invalid argument names");
    }
    if binding.allowed_extensions.is_empty()
        || binding
            .allowed_extensions
            .iter()
            .any(|extension| normalize_extension(extension).is_empty())
    {
        anyhow::bail!("MCP workspace text file binding requires allowed extensions");
    }
    if binding.max_bytes == 0 || binding.max_bytes > MAX_CONFIGURED_FILE_BYTES {
        anyhow::bail!(
            "MCP workspace text file binding max_bytes must be between 1 and {MAX_CONFIGURED_FILE_BYTES}"
        );
    }
    if !matches!(
        binding.encoding.to_ascii_lowercase().as_str(),
        "utf-8" | "utf8"
    ) {
        anyhow::bail!("MCP workspace text file binding only supports UTF-8 encoding");
    }
    Ok(())
}

async fn read_workspace_text_file(
    workspace_root: &Path,
    supplied_path: &str,
    binding: &ToolInputBinding,
) -> anyhow::Result<String> {
    if supplied_path.trim().is_empty() {
        anyhow::bail!("'{}' must not be empty", binding.path_argument);
    }
    let canonical_root = tokio::fs::canonicalize(workspace_root)
        .await
        .context("resolve sandbox workspace root")?;
    let supplied = PathBuf::from(supplied_path);
    let candidate = if supplied.is_absolute() {
        supplied
    } else {
        canonical_root.join(supplied)
    };
    let canonical_file = tokio::fs::canonicalize(&candidate)
        .await
        .with_context(|| format!("resolve workspace file '{}': not found", supplied_path))?;
    if !canonical_file.starts_with(&canonical_root) {
        anyhow::bail!("workspace file path escapes the sandbox workspace");
    }
    let extension = canonical_file
        .extension()
        .and_then(|value| value.to_str())
        .map(str::to_ascii_lowercase)
        .unwrap_or_default();
    if !binding
        .allowed_extensions
        .iter()
        .map(|allowed| normalize_extension(allowed))
        .any(|allowed| allowed == extension)
    {
        anyhow::bail!(
            "workspace file extension is not allowed; expected {}",
            binding.allowed_extensions.join(", ")
        );
    }
    let metadata = tokio::fs::metadata(&canonical_file)
        .await
        .context("inspect workspace file")?;
    if !metadata.is_file() {
        anyhow::bail!("workspace file path must identify a regular file");
    }
    if metadata.len() > binding.max_bytes {
        anyhow::bail!(
            "workspace file exceeds the configured {} byte limit",
            binding.max_bytes
        );
    }

    let file = tokio::fs::File::open(&canonical_file)
        .await
        .context("open workspace file")?;
    let mut bytes = Vec::with_capacity(metadata.len() as usize);
    file.take(binding.max_bytes + 1)
        .read_to_end(&mut bytes)
        .await
        .context("read workspace file")?;
    if bytes.len() as u64 > binding.max_bytes {
        anyhow::bail!(
            "workspace file exceeds the configured {} byte limit",
            binding.max_bytes
        );
    }
    String::from_utf8(bytes).context("workspace file is not valid UTF-8")
}

fn normalize_extension(extension: &str) -> String {
    extension
        .trim()
        .trim_start_matches('.')
        .to_ascii_lowercase()
}

#[cfg(test)]
mod tests {
    use super::{apply_tool_input_bindings, project_tool_schema};
    use domain::{ToolInputBinding, ToolInputBindingKind};
    use rmcp::model::JsonObject;
    use serde_json::{json, Value};

    fn binding(max_bytes: u64) -> ToolInputBinding {
        ToolInputBinding {
            tool: "send_email".to_string(),
            kind: ToolInputBindingKind::WorkspaceTextFile,
            path_argument: "markdown_file_path".to_string(),
            content_argument: "markdown".to_string(),
            allowed_extensions: vec![".md".to_string(), ".markdown".to_string()],
            max_bytes,
            encoding: "utf-8".to_string(),
        }
    }

    #[test]
    fn projects_content_argument_as_workspace_path() {
        let mut schema = json!({
            "type": "object",
            "properties": {"markdown": {"type": "string"}},
            "required": ["markdown"]
        });
        project_tool_schema(&mut schema, &[binding(1_048_576)]).expect("project schema");

        assert!(schema["properties"].get("markdown").is_none());
        assert_eq!(schema["properties"]["markdown_file_path"]["type"], "string");
        assert_eq!(schema["required"], json!(["markdown_file_path"]));
    }

    #[tokio::test]
    async fn reads_utf8_markdown_within_workspace() {
        let root = std::env::temp_dir().join(format!("hivy-mcp-input-{}", uuid_like()));
        tokio::fs::create_dir_all(root.join("investigations"))
            .await
            .expect("create workspace");
        tokio::fs::write(root.join("investigations/report.md"), "# Healthy\n")
            .await
            .expect("write report");
        let mut arguments = JsonObject::from_iter([(
            "markdown_file_path".to_string(),
            Value::String("investigations/report.md".to_string()),
        )]);

        apply_tool_input_bindings(&root, &[binding(1_048_576)], &mut arguments)
            .await
            .expect("apply file input");

        assert_eq!(arguments.get("markdown"), Some(&json!("# Healthy\n")));
        assert!(!arguments.contains_key("markdown_file_path"));
        tokio::fs::remove_dir_all(root)
            .await
            .expect("remove workspace");
    }

    #[tokio::test]
    async fn rejects_files_outside_workspace_and_over_limit() {
        let root = std::env::temp_dir().join(format!("hivy-mcp-root-{}", uuid_like()));
        let outside = std::env::temp_dir().join(format!("hivy-mcp-outside-{}.md", uuid_like()));
        tokio::fs::create_dir_all(&root)
            .await
            .expect("create workspace");
        tokio::fs::write(&outside, "outside")
            .await
            .expect("write outside file");
        let mut outside_arguments = JsonObject::from_iter([(
            "markdown_file_path".to_string(),
            Value::String(outside.to_string_lossy().into_owned()),
        )]);
        let outside_error =
            apply_tool_input_bindings(&root, &[binding(1_048_576)], &mut outside_arguments)
                .await
                .expect_err("outside file must fail");
        assert!(outside_error.to_string().contains("escapes"));

        #[cfg(unix)]
        {
            std::os::unix::fs::symlink(&outside, root.join("linked.md"))
                .expect("create escaping symlink");
            let mut symlink_arguments = JsonObject::from_iter([(
                "markdown_file_path".to_string(),
                Value::String("linked.md".to_string()),
            )]);
            let symlink_error =
                apply_tool_input_bindings(&root, &[binding(1_048_576)], &mut symlink_arguments)
                    .await
                    .expect_err("escaping symlink must fail");
            assert!(symlink_error.to_string().contains("escapes"));
        }

        tokio::fs::write(root.join("large.md"), "too large")
            .await
            .expect("write large file");
        let mut large_arguments = JsonObject::from_iter([(
            "markdown_file_path".to_string(),
            Value::String("large.md".to_string()),
        )]);
        let large_error = apply_tool_input_bindings(&root, &[binding(4)], &mut large_arguments)
            .await
            .expect_err("large file must fail");
        assert!(large_error.to_string().contains("byte limit"));

        tokio::fs::remove_dir_all(root)
            .await
            .expect("remove workspace");
        tokio::fs::remove_file(outside)
            .await
            .expect("remove outside file");
    }

    fn uuid_like() -> String {
        format!(
            "{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time")
                .as_nanos()
        )
    }
}
