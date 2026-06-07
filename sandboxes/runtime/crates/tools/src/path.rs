use std::path::{Path, PathBuf};

use globset::{Glob, GlobSet, GlobSetBuilder};

#[derive(Debug, thiserror::Error)]
#[allow(dead_code)]
pub enum PathPolicyError {
    #[error("path is required but empty")]
    Empty,
    #[error("path `{0}` is not within any allowed root")]
    OutsideAllowedRoots(String),
    #[error("path `{0}` matches a deny glob")]
    DeniedByGlob(String),
    #[error("path `{0}` could not be resolved: {1}")]
    Unresolvable(String, String),
}

pub fn expand_user_path(raw: &str) -> PathBuf {
    let trimmed = raw.trim();
    if let Some(rest) = trimmed.strip_prefix("~/") {
        if let Ok(home) = std::env::var("HOME") {
            return PathBuf::from(home).join(rest);
        }
    }
    if trimmed == "~" {
        if let Ok(home) = std::env::var("HOME") {
            return PathBuf::from(home);
        }
    }
    PathBuf::from(trimmed)
}

pub fn resolve_relative_to(base: &Path, raw: &str) -> PathBuf {
    if raw.trim().is_empty() {
        return base.to_path_buf();
    }
    let expanded = expand_user_path(raw);
    if expanded.is_absolute() {
        expanded
    } else {
        base.join(expanded)
    }
}

pub fn resolve_within_workspace(
    workspace_root: &Path,
    raw: &str,
    allowed_roots: &[String],
) -> Result<PathBuf, PathPolicyError> {
    if raw.trim().is_empty() {
        return Err(PathPolicyError::Empty);
    }
    let resolved = resolve_relative_to(workspace_root, raw);

    let mut effective_roots: Vec<PathBuf> =
        allowed_roots.iter().map(|s| expand_user_path(s)).collect();
    if effective_roots.is_empty() {
        effective_roots.push(workspace_root.to_path_buf());
    }

    let canonical = canonicalize_best_effort(&resolved);
    let canonical_roots: Vec<PathBuf> = effective_roots
        .iter()
        .map(|root| canonicalize_best_effort(root))
        .collect();

    let allowed = canonical_roots
        .iter()
        .any(|root| canonical.starts_with(root));
    if !allowed {
        return Err(PathPolicyError::OutsideAllowedRoots(
            canonical.display().to_string(),
        ));
    }
    Ok(canonical)
}

pub fn resolve_read_path(workspace_root: &Path, raw: &str) -> Result<PathBuf, PathPolicyError> {
    if raw.trim().is_empty() {
        return Err(PathPolicyError::Empty);
    }
    let resolved = resolve_relative_to(workspace_root, raw);
    Ok(canonicalize_best_effort(&resolved))
}

pub fn resolve_writable_path(
    workspace_root: &Path,
    raw: &str,
    allowed_roots: &[String],
) -> Result<PathBuf, PathPolicyError> {
    let mut effective_roots = default_writable_roots(workspace_root);
    effective_roots.extend(allowed_roots.iter().cloned());
    resolve_within_workspace(workspace_root, raw, &effective_roots)
}

fn default_writable_roots(workspace_root: &Path) -> Vec<String> {
    let mut roots = vec![
        workspace_root.display().to_string(),
        "/tmp".to_string(),
        "/var/tmp".to_string(),
    ];
    if let Ok(home) = std::env::var("HOME") {
        if !home.trim().is_empty() {
            roots.push(home);
        }
    }
    roots
}

pub fn canonicalize_best_effort(path: &Path) -> PathBuf {
    if let Ok(canonical) = std::fs::canonicalize(path) {
        return canonical;
    }

    let mut cursor = path;
    while let Some(parent) = cursor.parent() {
        if let Ok(canonical_parent) = std::fs::canonicalize(parent) {
            if let Ok(remainder) = path.strip_prefix(parent) {
                return canonical_parent.join(remainder);
            }
            return canonical_parent;
        }
        if parent == cursor {
            break;
        }
        cursor = parent;
    }
    path.to_path_buf()
}

pub fn build_glob_set(patterns: &[String]) -> GlobSet {
    let mut builder = GlobSetBuilder::new();
    for pattern in patterns {
        if let Ok(glob) = Glob::new(pattern) {
            builder.add(glob);
        }
    }
    builder.build().unwrap_or_else(|_| GlobSet::empty())
}

pub fn enforce_deny_globs(path: &Path, deny_globs: &GlobSet) -> Result<(), PathPolicyError> {
    if deny_globs.is_match(path) {
        return Err(PathPolicyError::DeniedByGlob(path.display().to_string()));
    }
    Ok(())
}
