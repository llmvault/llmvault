use std::collections::{BTreeMap, HashSet};
use std::path::{Component, Path, PathBuf};

use anyhow::Context;
use domain::ToolInputBinding;
use rmcp::model::JsonObject;
use serde_json::{json, Value};
use tokio::io::AsyncReadExt;

const MAX_CONFIGURED_FILE_BYTES: u64 = 16 * 1024 * 1024;
const MAX_CONFIGURED_BUNDLE_BYTES: u64 = 64 * 1024 * 1024;
const MAX_CONFIGURED_BUNDLE_FILES: u64 = 1_024;

pub(crate) fn project_tool_schema(
    parameters: &mut Value,
    bindings: &[ToolInputBinding],
) -> anyhow::Result<()> {
    for binding in bindings {
        validate_binding(binding)?;
        match binding {
            ToolInputBinding::WorkspaceTextFile { .. } => {
                project_workspace_text_file_schema(parameters, binding)?;
            }
            ToolInputBinding::WorkspaceBundle { .. } => {
                project_workspace_bundle_schema(parameters, binding)?;
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
        match binding {
            ToolInputBinding::WorkspaceTextFile { .. } => {
                apply_workspace_text_file_binding(workspace_root, binding, arguments).await?;
            }
            ToolInputBinding::WorkspaceBundle { .. } => {
                apply_workspace_bundle_binding(workspace_root, binding, arguments).await?;
            }
        }
    }
    Ok(())
}

fn project_workspace_text_file_schema(
    parameters: &mut Value,
    binding: &ToolInputBinding,
) -> anyhow::Result<()> {
    let ToolInputBinding::WorkspaceTextFile {
        path_argument,
        content_argument,
        allowed_extensions,
        ..
    } = binding
    else {
        anyhow::bail!("expected workspace text file binding");
    };
    let schema = parameters
        .as_object_mut()
        .context("MCP tool input schema must be an object")?;
    let properties = schema
        .get_mut("properties")
        .and_then(Value::as_object_mut)
        .context("MCP tool input schema must define object properties")?;
    if properties.contains_key(path_argument) {
        anyhow::bail!(
            "MCP file input path argument '{}' collides with an upstream property",
            path_argument
        );
    }
    let content_schema = properties.remove(content_argument).ok_or_else(|| {
        anyhow::anyhow!(
            "MCP file input content argument '{}' is not present in the upstream schema",
            content_argument
        )
    })?;
    if content_schema.get("type").and_then(Value::as_str) != Some("string") {
        anyhow::bail!(
            "MCP file input content argument '{}' must have type string",
            content_argument
        );
    }
    properties.insert(
        path_argument.clone(),
        json!({
            "type": "string",
            "description": format!(
                "Path to a UTF-8 {} file inside the sandbox workspace. The runtime reads the file and supplies its contents without exposing them to the model tool call.",
                allowed_extensions.join(" or ")
            )
        }),
    );
    if let Some(required) = schema.get_mut("required").and_then(Value::as_array_mut) {
        for name in required {
            if name.as_str() == Some(content_argument) {
                *name = Value::String(path_argument.clone());
            }
        }
    }
    Ok(())
}

fn project_workspace_bundle_schema(
    parameters: &mut Value,
    binding: &ToolInputBinding,
) -> anyhow::Result<()> {
    let ToolInputBinding::WorkspaceBundle {
        entrypoint_path_argument,
        supporting_file_paths_argument,
        entrypoint_content_argument,
        files_argument,
        entrypoint_filename,
        allowed_directories,
        max_files,
        ..
    } = binding
    else {
        anyhow::bail!("expected workspace bundle binding");
    };
    let schema = parameters
        .as_object_mut()
        .context("MCP tool input schema must be an object")?;
    {
        let properties = schema
            .get_mut("properties")
            .and_then(Value::as_object_mut)
            .context("MCP tool input schema must define object properties")?;
        for visible in [entrypoint_path_argument, supporting_file_paths_argument] {
            if properties.contains_key(visible) {
                anyhow::bail!(
                    "MCP bundle input path argument '{}' collides with an upstream property",
                    visible
                );
            }
        }
        let entrypoint_schema = properties
            .remove(entrypoint_content_argument)
            .ok_or_else(|| {
                anyhow::anyhow!(
                    "MCP bundle entrypoint content argument '{}' is not present in the upstream schema",
                    entrypoint_content_argument
                )
            })?;
        if entrypoint_schema.get("type").and_then(Value::as_str) != Some("string") {
            anyhow::bail!(
                "MCP bundle entrypoint content argument '{}' must have type string",
                entrypoint_content_argument
            );
        }
        let files_schema = properties.remove(files_argument).ok_or_else(|| {
            anyhow::anyhow!(
                "MCP bundle files argument '{}' is not present in the upstream schema",
                files_argument
            )
        })?;
        if files_schema.get("type").and_then(Value::as_str) != Some("object") {
            anyhow::bail!(
                "MCP bundle files argument '{}' must have type object",
                files_argument
            );
        }
        properties.insert(
            entrypoint_path_argument.clone(),
            json!({
                "type": "string",
                "description": format!(
                    "Path to the skill entrypoint named {entrypoint_filename} inside the sandbox workspace. Its parent directory is the skill bundle root."
                )
            }),
        );
        properties.insert(
            supporting_file_paths_argument.clone(),
            json!({
                "type": "array",
                "items": {"type": "string"},
                "uniqueItems": true,
                "maxItems": max_files.saturating_sub(1),
                "description": format!(
                    "Optional paths to UTF-8 supporting files beneath the entrypoint directory. Files must be under {}.",
                    allowed_directories.join("/, ")
                )
            }),
        );
    }
    if let Some(required) = schema.get_mut("required").and_then(Value::as_array_mut) {
        let entrypoint_was_required = required
            .iter()
            .any(|name| name.as_str() == Some(entrypoint_content_argument));
        required.retain(|name| {
            name.as_str() != Some(entrypoint_content_argument)
                && name.as_str() != Some(files_argument)
        });
        if entrypoint_was_required {
            required.push(Value::String(entrypoint_path_argument.clone()));
        }
    }
    Ok(())
}

async fn apply_workspace_text_file_binding(
    workspace_root: &Path,
    binding: &ToolInputBinding,
    arguments: &mut JsonObject,
) -> anyhow::Result<()> {
    let ToolInputBinding::WorkspaceTextFile {
        path_argument,
        content_argument,
        allowed_extensions,
        max_bytes,
        ..
    } = binding
    else {
        anyhow::bail!("expected workspace text file binding");
    };
    let Some(path_value) = arguments.remove(path_argument) else {
        return Ok(());
    };
    if arguments.contains_key(content_argument) {
        anyhow::bail!(
            "provide '{}' or '{}', not both",
            path_argument,
            content_argument
        );
    }
    let supplied_path = path_value
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("'{}' must be a file path string", path_argument))?;
    let canonical_root = canonical_workspace_root(workspace_root).await?;
    let (canonical_file, content, _) =
        read_workspace_text_file(&canonical_root, supplied_path, *max_bytes, path_argument).await?;
    let extension = canonical_file
        .extension()
        .and_then(|value| value.to_str())
        .map(str::to_ascii_lowercase)
        .unwrap_or_default();
    if !allowed_extensions
        .iter()
        .map(|allowed| normalize_extension(allowed))
        .any(|allowed| allowed == extension)
    {
        anyhow::bail!(
            "workspace file extension is not allowed; expected {}",
            allowed_extensions.join(", ")
        );
    }
    arguments.insert(content_argument.clone(), Value::String(content));
    Ok(())
}

async fn apply_workspace_bundle_binding(
    workspace_root: &Path,
    binding: &ToolInputBinding,
    arguments: &mut JsonObject,
) -> anyhow::Result<()> {
    let ToolInputBinding::WorkspaceBundle {
        entrypoint_path_argument,
        supporting_file_paths_argument,
        entrypoint_content_argument,
        files_argument,
        entrypoint_filename,
        allowed_directories,
        max_files,
        max_file_bytes,
        max_total_bytes,
        ..
    } = binding
    else {
        anyhow::bail!("expected workspace bundle binding");
    };
    let Some(entrypoint_value) = arguments.remove(entrypoint_path_argument) else {
        return Ok(());
    };
    if arguments.contains_key(entrypoint_content_argument) || arguments.contains_key(files_argument)
    {
        anyhow::bail!("workspace bundle content is supplied by the runtime");
    }
    let entrypoint_path = entrypoint_value.as_str().ok_or_else(|| {
        anyhow::anyhow!("'{}' must be a file path string", entrypoint_path_argument)
    })?;
    let supporting_paths = match arguments.remove(supporting_file_paths_argument) {
        None => Vec::new(),
        Some(Value::Array(values)) => values
            .into_iter()
            .map(|value| {
                value.as_str().map(str::to_owned).ok_or_else(|| {
                    anyhow::anyhow!(
                        "'{}' must contain only file path strings",
                        supporting_file_paths_argument
                    )
                })
            })
            .collect::<anyhow::Result<Vec<_>>>()?,
        Some(_) => {
            anyhow::bail!(
                "'{}' must be an array of file path strings",
                supporting_file_paths_argument
            )
        }
    };
    if 1 + supporting_paths.len() as u64 > *max_files {
        anyhow::bail!("workspace bundle exceeds the configured {max_files} file limit");
    }

    let canonical_root = canonical_workspace_root(workspace_root).await?;
    let (entrypoint_file, entrypoint_content, entrypoint_bytes) = read_workspace_text_file(
        &canonical_root,
        entrypoint_path,
        *max_file_bytes,
        entrypoint_path_argument,
    )
    .await?;
    if entrypoint_file.file_name().and_then(|name| name.to_str())
        != Some(entrypoint_filename.as_str())
    {
        anyhow::bail!("skill entrypoint must be named '{entrypoint_filename}'");
    }
    let bundle_root = entrypoint_file
        .parent()
        .context("skill entrypoint has no parent directory")?
        .to_path_buf();
    let mut total_bytes = entrypoint_bytes;
    let mut seen_paths = HashSet::new();
    let mut files = BTreeMap::new();
    for supplied_path in supporting_paths {
        let (canonical_file, content, file_bytes) = read_workspace_text_file(
            &canonical_root,
            &supplied_path,
            *max_file_bytes,
            supporting_file_paths_argument,
        )
        .await?;
        if canonical_file == entrypoint_file {
            anyhow::bail!("the entrypoint must not also be listed as a supporting file");
        }
        let relative = canonical_file.strip_prefix(&bundle_root).map_err(|_| {
            anyhow::anyhow!("supporting file must be beneath the skill entrypoint directory")
        })?;
        let portable_path = validated_bundle_relative_path(relative, allowed_directories)?;
        if !seen_paths.insert(portable_path.clone()) {
            anyhow::bail!("supporting file path '{portable_path}' was provided more than once");
        }
        total_bytes = total_bytes
            .checked_add(file_bytes)
            .context("workspace bundle size overflow")?;
        if total_bytes > *max_total_bytes {
            anyhow::bail!(
                "workspace bundle exceeds the configured {max_total_bytes} byte total limit"
            );
        }
        files.insert(portable_path, content);
    }
    if total_bytes > *max_total_bytes {
        anyhow::bail!("workspace bundle exceeds the configured {max_total_bytes} byte total limit");
    }

    arguments.insert(
        entrypoint_content_argument.clone(),
        Value::String(entrypoint_content),
    );
    arguments.insert(files_argument.clone(), serde_json::to_value(files)?);
    Ok(())
}

fn validate_binding(binding: &ToolInputBinding) -> anyhow::Result<()> {
    match binding {
        ToolInputBinding::WorkspaceTextFile {
            tool,
            path_argument,
            content_argument,
            allowed_extensions,
            max_bytes,
            encoding,
        } => {
            validate_argument_names(tool, &[path_argument, content_argument])?;
            if path_argument == content_argument {
                anyhow::bail!("MCP tool input binding has duplicate argument names");
            }
            if allowed_extensions.is_empty()
                || allowed_extensions
                    .iter()
                    .any(|extension| normalize_extension(extension).is_empty())
            {
                anyhow::bail!("MCP workspace text file binding requires allowed extensions");
            }
            if *max_bytes == 0 || *max_bytes > MAX_CONFIGURED_FILE_BYTES {
                anyhow::bail!(
                    "MCP workspace text file binding max_bytes must be between 1 and {MAX_CONFIGURED_FILE_BYTES}"
                );
            }
            validate_utf8_encoding(encoding)?;
        }
        ToolInputBinding::WorkspaceBundle {
            tool,
            entrypoint_path_argument,
            supporting_file_paths_argument,
            entrypoint_content_argument,
            files_argument,
            entrypoint_filename,
            allowed_directories,
            max_files,
            max_file_bytes,
            max_total_bytes,
            encoding,
        } => {
            validate_argument_names(
                tool,
                &[
                    entrypoint_path_argument,
                    supporting_file_paths_argument,
                    entrypoint_content_argument,
                    files_argument,
                ],
            )?;
            let unique_names: HashSet<&str> = [
                entrypoint_path_argument.as_str(),
                supporting_file_paths_argument.as_str(),
                entrypoint_content_argument.as_str(),
                files_argument.as_str(),
            ]
            .into_iter()
            .collect();
            if unique_names.len() != 4 {
                anyhow::bail!("MCP workspace bundle binding has duplicate argument names");
            }
            if entrypoint_filename.trim().is_empty()
                || Path::new(entrypoint_filename).components().count() != 1
            {
                anyhow::bail!("MCP workspace bundle binding requires an entrypoint filename");
            }
            if allowed_directories.is_empty()
                || allowed_directories.iter().any(|directory| {
                    directory.trim().is_empty()
                        || Path::new(directory).components().count() != 1
                        || directory == "."
                        || directory == ".."
                })
            {
                anyhow::bail!("MCP workspace bundle binding requires safe allowed directories");
            }
            if *max_files == 0 || *max_files > MAX_CONFIGURED_BUNDLE_FILES {
                anyhow::bail!(
                    "MCP workspace bundle max_files must be between 1 and {MAX_CONFIGURED_BUNDLE_FILES}"
                );
            }
            if *max_file_bytes == 0 || *max_file_bytes > MAX_CONFIGURED_FILE_BYTES {
                anyhow::bail!(
                    "MCP workspace bundle max_file_bytes must be between 1 and {MAX_CONFIGURED_FILE_BYTES}"
                );
            }
            if *max_total_bytes < *max_file_bytes || *max_total_bytes > MAX_CONFIGURED_BUNDLE_BYTES
            {
                anyhow::bail!(
                    "MCP workspace bundle max_total_bytes must be between max_file_bytes and {MAX_CONFIGURED_BUNDLE_BYTES}"
                );
            }
            validate_utf8_encoding(encoding)?;
        }
    }
    Ok(())
}

fn validate_argument_names(tool: &str, names: &[&String]) -> anyhow::Result<()> {
    if tool.trim().is_empty() || names.iter().any(|name| name.trim().is_empty()) {
        anyhow::bail!("MCP tool input binding has invalid argument names");
    }
    Ok(())
}

fn validate_utf8_encoding(encoding: &str) -> anyhow::Result<()> {
    if !matches!(encoding.to_ascii_lowercase().as_str(), "utf-8" | "utf8") {
        anyhow::bail!("MCP workspace file bindings only support UTF-8 encoding");
    }
    Ok(())
}

async fn canonical_workspace_root(workspace_root: &Path) -> anyhow::Result<PathBuf> {
    tokio::fs::canonicalize(workspace_root)
        .await
        .context("resolve sandbox workspace root")
}

async fn read_workspace_text_file(
    canonical_root: &Path,
    supplied_path: &str,
    max_bytes: u64,
    argument_name: &str,
) -> anyhow::Result<(PathBuf, String, u64)> {
    if supplied_path.trim().is_empty() {
        anyhow::bail!("'{argument_name}' must not contain an empty path");
    }
    let supplied = PathBuf::from(supplied_path);
    let candidate = if supplied.is_absolute() {
        supplied
    } else {
        canonical_root.join(supplied)
    };
    let link_metadata = tokio::fs::symlink_metadata(&candidate)
        .await
        .with_context(|| format!("resolve workspace file '{}': not found", supplied_path))?;
    if link_metadata.file_type().is_symlink() {
        anyhow::bail!("workspace file paths must not be symbolic links");
    }
    let canonical_file = tokio::fs::canonicalize(&candidate)
        .await
        .with_context(|| format!("resolve workspace file '{}': not found", supplied_path))?;
    if !canonical_file.starts_with(canonical_root) {
        anyhow::bail!("workspace file path escapes the sandbox workspace");
    }
    let metadata = tokio::fs::metadata(&canonical_file)
        .await
        .context("inspect workspace file")?;
    if !metadata.is_file() {
        anyhow::bail!("workspace file path must identify a regular file");
    }
    if metadata.len() > max_bytes {
        anyhow::bail!(
            "workspace file exceeds the configured {} byte limit",
            max_bytes
        );
    }

    let file = tokio::fs::File::open(&canonical_file)
        .await
        .context("open workspace file")?;
    let mut bytes = Vec::with_capacity(metadata.len() as usize);
    file.take(max_bytes + 1)
        .read_to_end(&mut bytes)
        .await
        .context("read workspace file")?;
    if bytes.len() as u64 > max_bytes {
        anyhow::bail!(
            "workspace file exceeds the configured {} byte limit",
            max_bytes
        );
    }
    let byte_count = bytes.len() as u64;
    let content = String::from_utf8(bytes).context("workspace file is not valid UTF-8")?;
    Ok((canonical_file, content, byte_count))
}

fn validated_bundle_relative_path(
    relative: &Path,
    allowed_directories: &[String],
) -> anyhow::Result<String> {
    let components = relative
        .components()
        .map(|component| match component {
            Component::Normal(value) => value
                .to_str()
                .map(str::to_owned)
                .context("skill bundle paths must be valid UTF-8"),
            _ => anyhow::bail!("skill bundle paths must not contain traversal components"),
        })
        .collect::<anyhow::Result<Vec<_>>>()?;
    if components.len() < 2
        || !allowed_directories
            .iter()
            .any(|directory| directory == &components[0])
    {
        anyhow::bail!(
            "supporting files must live under one of: {}/",
            allowed_directories.join("/, ")
        );
    }
    Ok(components.join("/"))
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
    use domain::ToolInputBinding;
    use rmcp::model::JsonObject;
    use serde_json::{json, Value};

    fn binding(max_bytes: u64) -> ToolInputBinding {
        ToolInputBinding::WorkspaceTextFile {
            tool: "send_email".to_string(),
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
            assert!(symlink_error.to_string().contains("symbolic links"));
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
