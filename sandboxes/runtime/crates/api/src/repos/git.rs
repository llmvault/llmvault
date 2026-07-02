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
            // `normal` reports untracked directories without descending into
            // them (e.g. node_modules is a single entry, not a full recursive
            // stat walk). `all` made the once-per-second repo monitor scan the
            // entire working tree, pinning a CPU core. `normal` still detects
            // new/changed tracked files and top-level untracked directories.
            "--untracked-files=normal".into(),
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

/// Returns the review base SHA for the current checkout.
///
/// For repos with an origin default branch, this is the merge-base of HEAD and
/// the remote default branch. That keeps feature branches and detached older
/// commits diffed against the point where they diverged instead of against the
/// latest remote tip. Returns `None` if no default branch can be determined.
pub(super) async fn review_base_sha(repo_path: &Path) -> Option<String> {
    let remote_head = remote_default_branch(repo_path).await?;
    let merge_base = git_output(
        repo_path,
        &["merge-base".into(), "HEAD".into(), remote_head.clone()],
    )
    .await
    .ok()
    .map(|value| value.trim().to_string())
    .filter(|value| !value.is_empty());
    if merge_base.is_some() {
        return merge_base;
    }
    git_output(repo_path, &["rev-parse".into(), remote_head])
        .await
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}

/// Returns the local remote-tracking ref for the remote default branch
/// (e.g. `origin/main`).
/// Tries `git symbolic-ref refs/remotes/origin/HEAD` first (set during
/// clone, no network needed). Returns `None` if no default branch can be
/// determined (e.g. a fresh local repo with no remote, or a remote without
/// an `origin/HEAD` symbolic ref).
async fn remote_default_branch(repo_path: &Path) -> Option<String> {
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

    Some(remote_head)
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
