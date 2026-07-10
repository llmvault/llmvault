use std::path::{Path, PathBuf};
use std::sync::Arc;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::ApplyPatchConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::diff::unified_diff;
use crate::lsp::LspService;
use crate::mutation_queue::with_file_lock;
use crate::path::{build_glob_set, enforce_deny_globs, resolve_writable_path, PathPolicyError};
use crate::{schema_for, JsonTool, ToolDefinition};

const TOOL_NAME: &str = "apply_patch";
const TOOL_DESCRIPTION: &str =
    "Apply a multi-file patch using the opencode/Codex patch format. The patch \
     should start with *** Begin Patch and end with *** End Patch. Supports Add \
     File, Update File, Delete File, and Move to. For a new file, use exactly:\n\
     *** Begin Patch\n*** Add File: notes.txt\n+line one\n+line two\n*** End Patch\n\
     Prefix every added file content line with +. Use this for coordinated edits \
     across files; use edit_file for one or two exact replacements.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct ApplyPatchArgs {
    /// Patch text in the *** Begin Patch format.
    pub patch: String,
}

pub struct ApplyPatchTool {
    config: ApplyPatchConfig,
    workspace_root: PathBuf,
    lsp: Option<LspService>,
}

#[derive(Debug)]
enum PatchOp {
    Add {
        path: String,
        content: String,
    },
    Delete {
        path: String,
    },
    Update {
        path: String,
        move_to: Option<String>,
        hunks: Vec<PatchHunk>,
    },
}

#[derive(Debug)]
struct PatchHunk {
    old_text: String,
    new_text: String,
}

#[derive(Debug)]
struct PreparedOp {
    path: PathBuf,
    move_to: Option<PathBuf>,
    before: Option<Vec<u8>>,
    after: Option<Vec<u8>>,
}

impl ApplyPatchTool {
    pub fn new(config: ApplyPatchConfig, workspace_root: PathBuf) -> Self {
        Self {
            config,
            workspace_root,
            lsp: None,
        }
    }

    pub fn with_lsp_service(mut self, lsp: LspService) -> Self {
        self.lsp = Some(lsp);
        self
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: ApplyPatchArgs =
            serde_json::from_value(args).map_err(|error| anyhow!("invalid arguments: {error}"))?;
        let ops = parse_patch(&parsed.patch)?;
        if ops.is_empty() {
            return Err(anyhow!("patch did not contain any file operations"));
        }

        let prepared = self.prepare_ops(ops).await?;
        let mut applied = Vec::new();
        for op in &prepared {
            let path_for_lock = op.path.clone();
            let op = PreparedOp {
                path: op.path.clone(),
                move_to: op.move_to.clone(),
                before: op.before.clone(),
                after: op.after.clone(),
            };
            let result = with_file_lock(&path_for_lock, move || {
                let op = PreparedOp {
                    path: op.path.clone(),
                    move_to: op.move_to.clone(),
                    before: op.before.clone(),
                    after: op.after.clone(),
                };
                async move { apply_prepared_op(op).await }
            })
            .await;
            result.map_err(|error| {
                anyhow!(
                    "patch failed while applying {} after {} operation(s): {error}",
                    path_for_lock.display(),
                    applied.len()
                )
            })?;
            applied.push(path_for_lock.display().to_string());
        }

        if let Some(lsp) = &self.lsp {
            for op in &prepared {
                if op.after.is_some() {
                    lsp.touch_file(op.move_to.as_ref().unwrap_or(&op.path))
                        .await;
                }
            }
        }

        let diffs: Vec<Value> = prepared
            .iter()
            .map(|op| {
                let before = op
                    .before
                    .as_deref()
                    .map(String::from_utf8_lossy)
                    .map(|text| text.to_string())
                    .unwrap_or_default();
                let after = op
                    .after
                    .as_deref()
                    .map(String::from_utf8_lossy)
                    .map(|text| text.to_string())
                    .unwrap_or_default();
                json!({
                    "path": op.path.display().to_string(),
                    "move_to": op.move_to.as_ref().map(|path| path.display().to_string()),
                    "diff": unified_diff(&before, &after, &op.path.display().to_string()),
                })
            })
            .collect();

        Ok(json!({
            "applied": applied,
            "operation_count": prepared.len(),
            "diffs": diffs,
        }))
    }

    async fn prepare_ops(&self, ops: Vec<PatchOp>) -> Result<Vec<PreparedOp>> {
        let deny_globs = build_glob_set(&self.config.deny_globs);
        let mut prepared = Vec::with_capacity(ops.len());
        for op in ops {
            match op {
                PatchOp::Add { path, content } => {
                    let resolved = self.resolve_target(&path, &deny_globs)?;
                    if tokio::fs::try_exists(&resolved).await.unwrap_or(false) {
                        return Err(anyhow!("add target already exists: {}", resolved.display()));
                    }
                    let bytes = content.into_bytes();
                    self.enforce_size(bytes.len())?;
                    prepared.push(PreparedOp {
                        path: resolved,
                        move_to: None,
                        before: None,
                        after: Some(bytes),
                    });
                }
                PatchOp::Delete { path } => {
                    let resolved = self.resolve_target(&path, &deny_globs)?;
                    let before = tokio::fs::read(&resolved)
                        .await
                        .map_err(|error| anyhow!("read {}: {error}", resolved.display()))?;
                    prepared.push(PreparedOp {
                        path: resolved,
                        move_to: None,
                        before: Some(before),
                        after: None,
                    });
                }
                PatchOp::Update {
                    path,
                    move_to,
                    hunks,
                } => {
                    let resolved = self.resolve_target(&path, &deny_globs)?;
                    let target = match move_to {
                        Some(move_to) => Some(self.resolve_target(&move_to, &deny_globs)?),
                        None => None,
                    };
                    let before = tokio::fs::read(&resolved)
                        .await
                        .map_err(|error| anyhow!("read {}: {error}", resolved.display()))?;
                    let after = apply_update_hunks(&resolved, &before, &hunks)?;
                    self.enforce_size(after.len())?;
                    if let Some(target) = &target {
                        if tokio::fs::try_exists(target).await.unwrap_or(false) {
                            return Err(anyhow!(
                                "move target already exists: {}",
                                target.display()
                            ));
                        }
                    }
                    prepared.push(PreparedOp {
                        path: resolved,
                        move_to: target,
                        before: Some(before),
                        after: Some(after),
                    });
                }
            }
        }
        Ok(prepared)
    }

    fn resolve_target(&self, raw: &str, deny_globs: &globset::GlobSet) -> Result<PathBuf> {
        let resolved = resolve_writable_path(&self.workspace_root, raw, &self.config.allowed_roots)
            .map_err(map_path_error)?;
        enforce_deny_globs(&resolved, deny_globs).map_err(map_path_error)?;
        Ok(resolved)
    }

    fn enforce_size(&self, bytes: usize) -> Result<()> {
        let max = self.config.max_file_size_bytes as usize;
        if bytes > max {
            return Err(anyhow!(
                "patched content size {} exceeds max_file_size_bytes ({})",
                bytes,
                max
            ));
        }
        Ok(())
    }
}

#[async_trait]
impl JsonTool for ApplyPatchTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: TOOL_NAME.to_string(),
            description: TOOL_DESCRIPTION.to_string(),
            parameters: schema_for::<ApplyPatchArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }

    fn errors_are_safe(&self) -> bool {
        true
    }
}

async fn apply_prepared_op(op: PreparedOp) -> std::io::Result<()> {
    match (op.after, op.move_to) {
        (Some(after), Some(move_to)) => {
            if let Some(parent) = move_to.parent() {
                tokio::fs::create_dir_all(parent).await?;
            }
            tokio::fs::write(&move_to, after).await?;
            tokio::fs::remove_file(&op.path).await?;
        }
        (Some(after), None) => {
            if let Some(parent) = op.path.parent() {
                tokio::fs::create_dir_all(parent).await?;
            }
            tokio::fs::write(&op.path, after).await?;
        }
        (None, _) => {
            tokio::fs::remove_file(&op.path).await?;
        }
    }
    Ok(())
}

fn parse_patch(input: &str) -> Result<Vec<PatchOp>> {
    let normalized = extract_patch(input)?;
    let lines: Vec<&str> = normalized.lines().collect();
    if lines.first().map(|line| line.trim()) != Some("*** Begin Patch") {
        return Err(anyhow!("patch must start with *** Begin Patch"));
    }
    if lines.last().map(|line| line.trim()) != Some("*** End Patch") {
        return Err(anyhow!("patch must end with *** End Patch"));
    }

    let mut ops = Vec::new();
    let mut index = 1;
    while index + 1 < lines.len() {
        let line = lines[index];
        if let Some(path) = line.strip_prefix("*** Add File: ") {
            index += 1;
            let mut content = String::new();
            while index + 1 < lines.len() && !lines[index].starts_with("*** ") {
                let rest = lines[index].strip_prefix('+').unwrap_or(lines[index]);
                content.push_str(rest);
                content.push('\n');
                index += 1;
            }
            ops.push(PatchOp::Add {
                path: path.trim().to_string(),
                content,
            });
            continue;
        }
        if let Some(path) = line.strip_prefix("*** Delete File: ") {
            ops.push(PatchOp::Delete {
                path: path.trim().to_string(),
            });
            index += 1;
            continue;
        }
        if let Some(path) = line.strip_prefix("*** Update File: ") {
            index += 1;
            let mut move_to = None;
            if index + 1 < lines.len() {
                if let Some(target) = lines[index].strip_prefix("*** Move to: ") {
                    move_to = Some(target.trim().to_string());
                    index += 1;
                }
            }
            let mut hunks = Vec::new();
            let mut old_text = String::new();
            let mut new_text = String::new();
            while index + 1 < lines.len() && !lines[index].starts_with("*** ") {
                let patch_line = lines[index];
                if patch_line.starts_with("@@") {
                    if !old_text.is_empty() || !new_text.is_empty() {
                        hunks.push(PatchHunk { old_text, new_text });
                        old_text = String::new();
                        new_text = String::new();
                    }
                    index += 1;
                    continue;
                }
                if patch_line == "*** End of File" {
                    index += 1;
                    continue;
                }
                if patch_line.is_empty() {
                    old_text.push('\n');
                    new_text.push('\n');
                    index += 1;
                    continue;
                }
                let (prefix, rest) = patch_line.split_at(1);
                match prefix {
                    " " => {
                        old_text.push_str(rest);
                        old_text.push('\n');
                        new_text.push_str(rest);
                        new_text.push('\n');
                    }
                    "-" => {
                        old_text.push_str(rest);
                        old_text.push('\n');
                    }
                    "+" => {
                        new_text.push_str(rest);
                        new_text.push('\n');
                    }
                    _ => {
                        return Err(anyhow!(
                            "update hunk lines must start with space, -, +, or @@"
                        ))
                    }
                }
                index += 1;
            }
            if !old_text.is_empty() || !new_text.is_empty() {
                hunks.push(PatchHunk { old_text, new_text });
            }
            if hunks.is_empty() {
                return Err(anyhow!(
                    "update file {} did not contain any hunks",
                    path.trim()
                ));
            }
            ops.push(PatchOp::Update {
                path: path.trim().to_string(),
                move_to,
                hunks,
            });
            continue;
        }
        if line.trim().is_empty() {
            index += 1;
            continue;
        }
        return Err(anyhow!("unexpected patch line: {line}"));
    }
    Ok(ops)
}

fn extract_patch(input: &str) -> Result<String> {
    let normalized = input.replace("\r\n", "\n").replace('\r', "\n");
    let Some(start) = normalized.find("*** Begin Patch") else {
        return Err(anyhow!("patch must contain *** Begin Patch"));
    };
    let after_start = &normalized[start..];
    let Some(end_relative) = after_start.find("*** End Patch") else {
        return Err(anyhow!("patch must contain *** End Patch"));
    };
    Ok(after_start[..end_relative + "*** End Patch".len()].to_string())
}

fn apply_update_hunks(path: &Path, before: &[u8], hunks: &[PatchHunk]) -> Result<Vec<u8>> {
    let original = String::from_utf8(before.to_vec())
        .map_err(|_| anyhow!("{} is not valid UTF-8", path.display()))?;
    let (bom, without_bom) = strip_utf8_bom(&original);
    let line_ending = if without_bom.contains("\r\n") {
        "\r\n"
    } else {
        "\n"
    };
    let mut text = without_bom.replace("\r\n", "\n");
    for hunk in hunks {
        let before_count = text.matches(&hunk.old_text).count();
        if before_count == 0 {
            let trimmed_old = hunk.old_text.trim_end_matches('\n');
            if !trimmed_old.is_empty() && text.matches(trimmed_old).count() == 1 {
                text = text.replacen(trimmed_old, hunk.new_text.trim_end_matches('\n'), 1);
                continue;
            }
            return Err(anyhow!(
                "patch hunk did not match exactly once in {}",
                path.display()
            ));
        }
        if before_count > 1 {
            return Err(anyhow!(
                "patch hunk matched multiple locations in {}",
                path.display()
            ));
        }
        text = text.replacen(&hunk.old_text, &hunk.new_text, 1);
    }
    let restored = if line_ending == "\n" {
        text
    } else {
        text.replace('\n', line_ending)
    };
    Ok(format!("{bom}{restored}").into_bytes())
}

fn strip_utf8_bom(text: &str) -> (&str, &str) {
    if let Some(rest) = text.strip_prefix('\u{FEFF}') {
        ("\u{FEFF}", rest)
    } else {
        ("", text)
    }
}

fn map_path_error(error: PathPolicyError) -> anyhow::Error {
    anyhow!(error.to_string())
}
