use std::collections::BTreeMap;
use std::path::Path;

use anyhow::{anyhow, Result};
use serde_json::json;
use tokio::process::Command;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(super) struct RepoSnapshot {
    pub head_sha: String,
    pub paths: BTreeMap<String, String>,
}

pub(super) async fn snapshot_repo(repo_path: &Path) -> Result<RepoSnapshot> {
    Ok(RepoSnapshot {
        head_sha: git_output(repo_path, &["rev-parse".into(), "HEAD".into()])
            .await
            .unwrap_or_default()
            .trim()
            .to_string(),
        paths: git_status(repo_path).await.unwrap_or_default(),
    })
}

pub(super) async fn git_status(repo_path: &Path) -> Result<BTreeMap<String, String>> {
    let output = git_output(
        repo_path,
        &[
            "status".into(),
            "--porcelain=v1".into(),
            "-z".into(),
            "--untracked-files=all".into(),
        ],
    )
    .await?;
    let mut statuses = BTreeMap::new();
    for item in output.split('\0').filter(|item| !item.is_empty()) {
        if item.len() < 4 {
            continue;
        }
        let code = &item[..2];
        let path = item[3..].to_string();
        let status = if code.contains("??") {
            "untracked"
        } else if code.contains('D') {
            "deleted"
        } else if code.contains('A') {
            "added"
        } else if code.contains('R') {
            "renamed"
        } else {
            "modified"
        };
        statuses.insert(path, status.to_string());
    }
    Ok(statuses)
}

/// Returns the SHA of the remote default branch (e.g. `origin/main`).
/// Tries `git symbolic-ref refs/remotes/origin/HEAD` first (set during
/// clone, no network needed). Returns `None` if no default branch can be
/// determined (e.g. a fresh local repo with no remote, or a remote without
/// an `origin/HEAD` symbolic ref).
pub(super) async fn default_branch_sha(repo_path: &Path) -> Option<String> {
    // Try the remote HEAD symbolic ref first; this is set automatically
    // when cloning and doesn't require network access.
    let remote_head = git_output(
        repo_path,
        &[
            "symbolic-ref".into(),
            "--short".into(),
            "refs/remotes/origin/HEAD".into(),
        ],
    )
    .await
    .ok()?
    .trim()
    .to_string();

    if remote_head.is_empty() {
        return None;
    }

    // Resolve the remote-tracking ref, even when no local main branch exists.
    let sha = git_output(repo_path, &["rev-parse".into(), remote_head])
        .await
        .ok()?
        .trim()
        .to_string();

    if sha.is_empty() {
        None
    } else {
        Some(sha)
    }
}

pub(super) async fn git_output(repo_path: &Path, args: &[String]) -> Result<String> {
    let output = Command::new("git")
        .args(args)
        .current_dir(repo_path)
        .output()
        .await?;
    if !output.status.success() && args.first().is_some_and(|arg| arg != "diff") {
        return Err(anyhow!(
            "git {:?} failed: {}",
            args,
            String::from_utf8_lossy(&output.stderr)
        ));
    }
    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

pub(super) fn changed_paths(
    previous: Option<&RepoSnapshot>,
    next: &RepoSnapshot,
) -> Vec<serde_json::Value> {
    let mut paths = BTreeMap::new();
    if previous.map_or(true, |prev| prev.head_sha != next.head_sha) {
        for (path, status) in &next.paths {
            paths.insert(path.clone(), status.clone());
        }
    }
    if let Some(previous) = previous {
        for (path, status) in &next.paths {
            if previous.paths.get(path) != Some(status) {
                paths.insert(path.clone(), status.clone());
            }
        }
        for path in previous.paths.keys() {
            if !next.paths.contains_key(path) {
                paths.insert(path.clone(), "clean".to_string());
            }
        }
    } else {
        for (path, status) in &next.paths {
            paths.insert(path.clone(), status.clone());
        }
    }
    paths
        .into_iter()
        .map(|(path, status)| json!({ "path": path, "status": status }))
        .collect()
}
