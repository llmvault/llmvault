//! Generic "materialize files" hook for trusted MCP tool results.
//!
//! A trusted MCP server (e.g. the Hivy control-plane server) may return a
//! tool result that asks the runtime to write files into the workspace. This
//! keeps the runtime deliberately unaware of any specific feature (such as
//! skills): it only knows how to safely persist files that a trusted result
//! describes.
//!
//! Contract — the result's structured content carries a `materialize` object:
//! ```json
//! { "structuredContent": { "materialize": {
//!     "root": ".skills/my-skill",
//!     "files": { "SKILL.md": "...", "references/api.md": "..." }
//! } } }
//! ```
//! After writing, the `materialize` blob is removed and replaced with a small
//! `materialized` summary so the (potentially large) file contents are not
//! surfaced back to the model.

use std::fs;
use std::path::{Component, Path, PathBuf};

use serde_json::{json, Value};
use tracing::{info, warn};

/// If `result` came from a trusted server and carries a `materialize`
/// instruction, write the described files under `workspace_root` and replace
/// the instruction with a summary. No-op for results without the instruction.
pub fn apply_materialize(workspace_root: &Path, result: &mut Value) {
    let Some(obj) = result.as_object_mut() else {
        return;
    };
    // MCP serializes structured content as `structuredContent`; tolerate the
    // snake_case form too in case a server emits it.
    let key = if obj.contains_key("structuredContent") {
        "structuredContent"
    } else if obj.contains_key("structured_content") {
        "structured_content"
    } else {
        return;
    };
    let Some(structured) = obj.get_mut(key).and_then(Value::as_object_mut) else {
        return;
    };
    let Some(instruction) = structured.remove("materialize") else {
        return;
    };

    let summary = write_instruction(workspace_root, &instruction);
    structured.insert("materialized".to_string(), summary);
}

fn write_instruction(workspace_root: &Path, instruction: &Value) -> Value {
    let root = instruction.get("root").and_then(Value::as_str).unwrap_or("");
    let root = match validate_relative_dir(root) {
        Ok(root) => root,
        Err(error) => {
            warn!(%error, "invalid materialize root; skipping");
            return json!({ "ok": false, "error": error });
        }
    };
    let Some(files) = instruction.get("files").and_then(Value::as_object) else {
        return json!({ "ok": false, "error": "materialize.files missing" });
    };

    let base = workspace_root.join(&root);
    let mut written: Vec<String> = Vec::new();
    for (rel, content) in files {
        let Some(content) = content.as_str() else {
            continue;
        };
        match safe_join(&base, rel) {
            Ok(path) => {
                if let Err(error) = atomic_write(&path, content) {
                    warn!(%error, file = %rel, "failed to materialize file");
                } else {
                    written.push(rel.clone());
                }
            }
            Err(error) => warn!(%error, file = %rel, "rejected unsafe materialize path"),
        }
    }
    written.sort();
    info!(root = %root.display(), count = written.len(), "materialized files from tool result");
    json!({
        "ok": true,
        "root": root.to_string_lossy(),
        "files": written,
    })
}

/// Validate a workspace-relative directory: not absolute, and free of `..` or
/// other special path components.
fn validate_relative_dir(path: &str) -> Result<PathBuf, String> {
    if path.is_empty() {
        return Err("materialize.root is empty".to_string());
    }
    let rel = PathBuf::from(path);
    if rel.is_absolute() {
        return Err("materialize.root must be relative".to_string());
    }
    if !rel.components().all(|c| matches!(c, Component::Normal(_))) {
        return Err("materialize.root must not contain traversal components".to_string());
    }
    Ok(rel)
}

/// Join a relative file path onto `base`, rejecting absolute paths and any
/// traversal so the result can never escape `base`.
fn safe_join(base: &Path, rel: &str) -> Result<PathBuf, String> {
    let rel_path = PathBuf::from(rel);
    if rel_path.is_absolute() {
        return Err("file path must be relative".to_string());
    }
    if rel.is_empty() {
        return Err("file path is empty".to_string());
    }
    if !rel_path.components().all(|c| matches!(c, Component::Normal(_))) {
        return Err("file path must not contain traversal components".to_string());
    }
    Ok(base.join(rel_path))
}

fn atomic_write(path: &Path, content: &str) -> std::io::Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    let tmp = path.with_extension(format!(
        "{}tmp",
        path.extension()
            .and_then(|e| e.to_str())
            .map(|e| format!("{e}."))
            .unwrap_or_default()
    ));
    fs::write(&tmp, content)?;
    fs::rename(tmp, path)
}

#[cfg(test)]
mod tests {
    use super::apply_materialize;
    use serde_json::json;

    #[test]
    fn writes_files_and_strips_blob() {
        let dir = std::env::temp_dir().join(format!("materialize-test-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        let mut result = json!({
            "content": [{ "type": "text", "text": "loaded skill" }],
            "structuredContent": {
                "materialize": {
                    "root": ".skills/demo",
                    "files": {
                        "SKILL.md": "---\nname: demo\n---\nbody",
                        "scripts/run.sh": "echo hi"
                    }
                }
            }
        });

        apply_materialize(&dir, &mut result);

        let skill_md = std::fs::read_to_string(dir.join(".skills/demo/SKILL.md")).unwrap();
        assert!(skill_md.contains("name: demo"));
        let script = std::fs::read_to_string(dir.join(".skills/demo/scripts/run.sh")).unwrap();
        assert_eq!(script, "echo hi");

        let structured = &result["structuredContent"];
        assert!(structured.get("materialize").is_none(), "blob not stripped");
        assert_eq!(structured["materialized"]["ok"], json!(true));

        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn rejects_traversal() {
        let dir = std::env::temp_dir().join(format!("materialize-esc-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        let mut result = json!({
            "structuredContent": {
                "materialize": {
                    "root": ".skills/demo",
                    "files": { "../escape.txt": "nope" }
                }
            }
        });

        apply_materialize(&dir, &mut result);

        assert!(!dir.join("escape.txt").exists());
        assert!(!dir.parent().unwrap().join("escape.txt").exists());
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn ignores_results_without_instruction() {
        let dir = std::env::temp_dir().join("materialize-none");
        let mut result = json!({ "content": [{ "type": "text", "text": "hi" }] });
        apply_materialize(&dir, &mut result);
        assert_eq!(result["content"][0]["text"], json!("hi"));
    }
}
