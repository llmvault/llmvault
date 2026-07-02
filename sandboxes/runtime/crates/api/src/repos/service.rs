use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::path::{Component, Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, bail, Context, Result};
use domain::{WorkspaceConfig, WorkspaceRepoConfig};
use percent_encoding::percent_decode_str;
use serde_json::json;
use tokio::process::Command;
use tokio::sync::{Mutex, RwLock, Semaphore};
use tracing::{info, warn};

use crate::session_stream::SessionStreamBroker;

use super::git::{
    changed_paths, git_output, git_status, review_base_sha, snapshot_repo, RepoSnapshot,
};
use super::types::{
    ContentResponse, DiffFileSummary, DiffResponse, RepoInfo, RepoListResponse, TreeEntry,
    TreeResponse,
};

const REPOS_DIR: &str = "repos";
const DEFAULT_DIFF_CONTEXT: u32 = 3;
const MAX_DIFF_BYTES: usize = 512 * 1024;
const MAX_CONTENT_BYTES: u64 = 512 * 1024;
const MAX_LIST_DEPTH: usize = 4;
const DEFAULT_REPO_SYNC_DEPTH: u32 = 1;
const MAX_REPO_SYNC_DEPTH: u32 = 1000;
const REPO_SYNC_CONCURRENCY: usize = 2;

#[derive(Clone)]
pub struct RepoService {
    root: PathBuf,
    repos_root: PathBuf,
    state: Arc<RwLock<RepoState>>,
    broker: Arc<SessionStreamBroker>,
    active_syncs: Arc<Mutex<BTreeSet<String>>>,
    sync_semaphore: Arc<Semaphore>,
}

#[derive(Default)]
struct RepoState {
    repos: BTreeMap<String, RepoInfo>,
    snapshots: HashMap<String, RepoSnapshot>,
    sessions: BTreeSet<String>,
    sequence: u64,
}

impl RepoService {
    pub fn new(workspace_root: PathBuf, broker: Arc<SessionStreamBroker>) -> Self {
        let repos_root = workspace_root.join(REPOS_DIR);
        Self {
            root: workspace_root,
            repos_root,
            state: Arc::new(RwLock::new(RepoState::default())),
            broker,
            active_syncs: Arc::new(Mutex::new(BTreeSet::new())),
            sync_semaphore: Arc::new(Semaphore::new(REPO_SYNC_CONCURRENCY)),
        }
    }

    pub fn start(self: Arc<Self>) {
        // Poll interval for the repo change monitor. Kept modest (not sub-second)
        // because each reconcile shells out to `git status` per repo.
        const MONITOR_INTERVAL: Duration = Duration::from_secs(3);
        tokio::spawn(async move {
            loop {
                // Skip the git scan entirely when no session is registered:
                // an idle/orphaned sandbox has nobody to publish change batches
                // to, so there is no reason to burn CPU scanning repos.
                let has_sessions = !self.state.read().await.sessions.is_empty();
                if has_sessions {
                    if let Err(error) = self.reconcile_and_publish().await {
                        warn!(%error, "repo monitor reconciliation failed");
                    }
                }
                tokio::time::sleep(MONITOR_INTERVAL).await;
            }
        });
    }

    pub async fn register_session(&self, session_id: &str) {
        self.state
            .write()
            .await
            .sessions
            .insert(session_id.to_string());
    }

    pub async fn apply_workspace_config(&self, workspace: WorkspaceConfig) {
        for repo in workspace.repos {
            self.schedule_repo_sync(repo).await;
        }
    }

    async fn schedule_repo_sync(&self, repo: WorkspaceRepoConfig) {
        let key = repo_sync_key(&repo);
        if key.is_empty() {
            capture_repo_sync_error(&repo, "repo sync skipped: missing repo id");
            return;
        }
        {
            let mut active = self.active_syncs.lock().await;
            if !active.insert(key.clone()) {
                return;
            }
        }

        let repos_root = self.repos_root.clone();
        let active_syncs = self.active_syncs.clone();
        let sync_semaphore = self.sync_semaphore.clone();
        tokio::spawn(async move {
            let permit = match sync_semaphore.acquire_owned().await {
                Ok(permit) => permit,
                Err(error) => {
                    let error = format!("repo sync semaphore closed: {error}");
                    warn!(repo_id = %repo.id, repo_name = %repo.name, error = %error, "repo sync failed");
                    capture_repo_sync_error(&repo, &error);
                    active_syncs.lock().await.remove(&key);
                    return;
                }
            };
            let _permit = permit;
            if let Err(error) = sync_workspace_repo(&repos_root, &repo).await {
                let error = error.to_string();
                warn!(repo_id = %repo.id, repo_name = %repo.name, error = %error, "repo sync failed");
                capture_repo_sync_error(&repo, &error);
            }
            active_syncs.lock().await.remove(&key);
        });
    }

    pub async fn list_repos(&self) -> Result<RepoListResponse> {
        self.scan_repos().await?;
        let state = self.state.read().await;
        Ok(RepoListResponse {
            repos: state.repos.values().cloned().collect(),
        })
    }

    pub async fn tree(&self, repo_id: &str, path: &str) -> Result<TreeResponse> {
        self.scan_repos().await?;
        let repo = self.repo(repo_id).await?;
        let rel = clean_relative_path(path)?;
        let dir = self.safe_repo_path(&repo, &rel)?;
        let meta = tokio::fs::metadata(&dir).await?;
        if !meta.is_dir() {
            return Err(anyhow!("path is not a directory"));
        }
        let statuses = git_status(&self.repo_path(&repo)).await.unwrap_or_default();
        let mut read_dir = tokio::fs::read_dir(&dir).await?;
        let mut entries = Vec::new();
        while let Some(entry) = read_dir.next_entry().await? {
            let name = entry.file_name().to_string_lossy().to_string();
            if name == ".git" {
                continue;
            }
            let child_rel = join_rel(&rel, &name);
            let child_meta = entry.metadata().await?;
            entries.push(TreeEntry {
                name,
                path: child_rel.clone(),
                kind: if child_meta.is_dir() {
                    "directory"
                } else {
                    "file"
                }
                .to_string(),
                size: if child_meta.is_file() {
                    Some(child_meta.len())
                } else {
                    None
                },
                git_status: status_for_path(&statuses, &child_rel),
            });
        }
        entries.sort_by(|a, b| a.kind.cmp(&b.kind).then_with(|| a.name.cmp(&b.name)));
        Ok(TreeResponse {
            repo_id: repo.id,
            path: rel,
            entries,
        })
    }

    pub async fn content(
        &self,
        repo_id: &str,
        path: &str,
        offset: Option<usize>,
        limit: Option<usize>,
    ) -> Result<ContentResponse> {
        self.scan_repos().await?;
        let repo = self.repo(repo_id).await?;
        let rel = clean_relative_path(path)?;
        if rel.is_empty() {
            return Err(anyhow!("path is required"));
        }
        let file = self.safe_repo_path(&repo, &rel)?;
        let meta = tokio::fs::metadata(&file).await?;
        if !meta.is_file() {
            return Err(anyhow!("path is not a file"));
        }
        if meta.len() > MAX_CONTENT_BYTES && offset.is_none() && limit.is_none() {
            return Err(anyhow!("file too large; use offset and limit"));
        }
        let bytes = tokio::fs::read(&file).await?;
        let text = String::from_utf8(bytes).map_err(|_| anyhow!("file is not valid utf-8"))?;
        let total_lines = count_lines(&text);
        let total_bytes = text.len();
        let sliced = slice_lines(&text, offset, limit)?;
        let shown_lines = count_lines(&sliced);
        Ok(ContentResponse {
            repo_id: repo.id,
            path: rel,
            content: sliced,
            encoding: "utf-8".to_string(),
            truncated: offset.is_some() || limit.is_some(),
            total_lines,
            total_bytes,
            shown_lines,
            offset,
            limit,
        })
    }

    pub async fn diff(
        &self,
        repo_id: &str,
        path: Option<&str>,
        context: Option<u32>,
    ) -> Result<DiffResponse> {
        self.scan_repos().await?;
        let repo = self.repo(repo_id).await?;
        let repo_path = self.repo_path(&repo);
        let rel = path.map(clean_relative_path).transpose()?;
        if let Some(path) = &rel {
            let _ = self.safe_repo_path(&repo, path)?;
        }
        let context = context.unwrap_or(DEFAULT_DIFF_CONTEXT).min(200);
        let mut files = diff_file_summaries(&repo_path, &repo.base_sha, rel.as_deref())
            .await
            .unwrap_or_default();
        let statuses = git_status(&repo_path).await.unwrap_or_default();
        for (changed_path, status) in statuses {
            if status != "untracked" {
                continue;
            }
            if rel
                .as_deref()
                .is_some_and(|want| !path_in_scope(want, &changed_path))
            {
                continue;
            }
            files.insert(
                changed_path.clone(),
                DiffFileSummary::new(changed_path.clone(), status, None),
            );
        }
        let mut total_bytes = 0usize;
        let mut truncated = false;
        let mut diff = String::new();
        let mut files = files.into_values().collect::<Vec<_>>();
        for file in &mut files {
            let patch = if file.status == "untracked" {
                untracked_file_diff(&repo_path, &file.path, context).await
            } else {
                tracked_file_diff(&repo_path, &repo.base_sha, &file.path, context).await
            }
            .unwrap_or_default();
            file.total_bytes = patch.len();
            total_bytes += file.total_bytes;
            if file.total_bytes > MAX_DIFF_BYTES {
                file.truncated = true;
                file.patch.clear();
                file.message = Some(format!(
                    "File diff is too large to display ({} bytes exceeds {} byte limit).",
                    file.total_bytes, MAX_DIFF_BYTES
                ));
                truncated = true;
                continue;
            }
            file.patch = patch;
            diff.push_str(&file.patch);
        }
        let message = if truncated {
            Some("One or more file diffs are too large to display.".to_string())
        } else {
            None
        };
        Ok(DiffResponse {
            repo_id: repo.id,
            path: rel,
            diff,
            truncated,
            total_bytes,
            max_bytes: MAX_DIFF_BYTES,
            files,
            message,
        })
    }

    async fn reconcile_and_publish(&self) -> Result<()> {
        self.scan_repos().await?;
        let repos = {
            let state = self.state.read().await;
            state.repos.values().cloned().collect::<Vec<_>>()
        };
        for repo in repos {
            let snapshot = snapshot_repo(&self.repo_path(&repo)).await?;
            let (sequence, sessions, changed) = {
                let mut state = self.state.write().await;
                let previous = state.snapshots.insert(repo.id.clone(), snapshot.clone());
                if previous.as_ref() == Some(&snapshot) {
                    (0, Vec::new(), Vec::new())
                } else {
                    state.sequence += 1;
                    let changed = changed_paths(previous.as_ref(), &snapshot);
                    (
                        state.sequence,
                        state.sessions.iter().cloned().collect::<Vec<_>>(),
                        changed,
                    )
                }
            };
            if sequence == 0 {
                continue;
            }
            for session_id in sessions {
                let stream_id = self.broker.get_or_create_session_stream(&session_id).await;
                self.broker
                    .publish(
                        &stream_id,
                        "repo.change_batch",
                        json!({
                            "session_id": session_id,
                            "repo_id": repo.id,
                            "sequence": sequence,
                            "base_sha": repo.base_sha,
                            "head_sha": snapshot.head_sha,
                            "paths": changed,
                            "summary": {
                                "files_changed": changed.len(),
                                "insertions": 0,
                                "deletions": 0
                            }
                        }),
                    )
                    .await;
            }
        }
        Ok(())
    }

    async fn scan_repos(&self) -> Result<()> {
        let mut found = BTreeMap::new();
        if !self.repos_root.exists() {
            tokio::fs::create_dir_all(&self.repos_root).await?;
        }
        let mut stack = vec![(self.repos_root.clone(), 0usize)];
        while let Some((dir, depth)) = stack.pop() {
            if depth > MAX_LIST_DEPTH {
                continue;
            }
            let mut read_dir = tokio::fs::read_dir(&dir).await?;
            while let Some(entry) = read_dir.next_entry().await? {
                let path = entry.path();
                let meta = entry.metadata().await?;
                if !meta.is_dir() {
                    continue;
                }
                if tokio::fs::metadata(path.join(".git")).await.is_ok() {
                    if let Ok(repo) = self.info_for_repo(&path).await {
                        found.insert(repo.id.clone(), repo);
                    }
                    continue;
                }
                stack.push((path, depth + 1));
            }
        }
        let mut state = self.state.write().await;
        for (id, repo) in found {
            state.repos.insert(id, repo);
        }
        Ok(())
    }

    async fn info_for_repo(&self, path: &Path) -> Result<RepoInfo> {
        let head = git_output(path, &["rev-parse".into(), "HEAD".into()])
            .await
            .unwrap_or_default()
            .trim()
            .to_string();
        let base_sha = review_base_sha(path).await.unwrap_or_else(|| head.clone());
        let relative = path
            .strip_prefix(&self.root)
            .context("repo outside workspace")?
            .to_string_lossy()
            .replace('\\', "/");
        let id = repo_id(&relative);
        Ok(RepoInfo {
            id,
            name: path
                .file_name()
                .map(|name| name.to_string_lossy().to_string())
                .unwrap_or_else(|| relative.clone()),
            relative_path: relative,
            head_sha: head,
            base_sha,
        })
    }

    async fn repo(&self, repo_id: &str) -> Result<RepoInfo> {
        self.state
            .read()
            .await
            .repos
            .get(repo_id)
            .cloned()
            .ok_or_else(|| anyhow!("repo not found"))
    }

    fn repo_path(&self, repo: &RepoInfo) -> PathBuf {
        self.root.join(&repo.relative_path)
    }

    fn safe_repo_path(&self, repo: &RepoInfo, rel: &str) -> Result<PathBuf> {
        let repo_path = self.repo_path(repo);
        let path = repo_path.join(rel);
        let canonical_repo = repo_path.canonicalize()?;
        let canonical = if path.exists() {
            path.canonicalize()?
        } else {
            path.parent()
                .ok_or_else(|| anyhow!("invalid path"))?
                .canonicalize()?
                .join(path.file_name().ok_or_else(|| anyhow!("invalid path"))?)
        };
        if !canonical.starts_with(&canonical_repo) {
            return Err(anyhow!("path escapes repo"));
        }
        Ok(canonical)
    }
}

async fn sync_workspace_repo(repos_root: &Path, repo: &WorkspaceRepoConfig) -> Result<()> {
    validate_workspace_repo(repo)?;
    tokio::fs::create_dir_all(repos_root)
        .await
        .with_context(|| format!("create repos directory {}", repos_root.display()))?;
    let target = repos_root.join(&repo.name);
    let depth = repo
        .depth
        .unwrap_or(DEFAULT_REPO_SYNC_DEPTH)
        .clamp(1, MAX_REPO_SYNC_DEPTH);

    match tokio::fs::metadata(&target).await {
        Ok(meta) if meta.is_dir() => {
            if tokio::fs::metadata(target.join(".git")).await.is_ok() {
                info!(repo_id = %repo.id, repo_name = %repo.name, "fetching workspace repository");
                run_git(
                    &[
                        "-C".to_string(),
                        target.display().to_string(),
                        "fetch".to_string(),
                        format!("--depth={depth}"),
                        "origin".to_string(),
                    ],
                    "fetch workspace repository",
                )
                .await
            } else {
                bail!("repository target exists but is not a git checkout");
            }
        }
        Ok(_) => bail!("repository target exists but is not a directory"),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            info!(repo_id = %repo.id, repo_name = %repo.name, "cloning workspace repository");
            run_git(
                &[
                    "clone".to_string(),
                    format!("--depth={depth}"),
                    repo.clone_url.clone(),
                    target.display().to_string(),
                ],
                "clone workspace repository",
            )
            .await
        }
        Err(error) => Err(error).context("inspect repository target"),
    }
}

fn validate_workspace_repo(repo: &WorkspaceRepoConfig) -> Result<()> {
    if !is_safe_repo_directory(&repo.name) {
        bail!("invalid repository directory name");
    }
    let full_name = if repo.full_name.trim().is_empty() {
        repo.id.trim()
    } else {
        repo.full_name.trim()
    };
    if !is_safe_github_repo(full_name) {
        bail!("invalid repository full name");
    }
    if repo.clone_url.trim().is_empty() {
        bail!("missing repository clone URL");
    }
    if clone_url_contains_credentials(&repo.clone_url) {
        bail!("repository clone URL must not contain credentials");
    }
    Ok(())
}

async fn run_git(args: &[String], context: &'static str) -> Result<()> {
    let output = Command::new("git").args(args).output().await?;
    if !output.status.success() {
        bail!(
            "{} failed: {}",
            context,
            safe_git_stderr(&String::from_utf8_lossy(&output.stderr))
        );
    }
    Ok(())
}

fn safe_git_stderr(stderr: &str) -> String {
    let stderr = stderr.trim();
    if stderr.is_empty() {
        return "git exited with non-zero status".to_string();
    }
    stderr.chars().take(512).collect()
}

fn repo_sync_key(repo: &WorkspaceRepoConfig) -> String {
    if !repo.full_name.trim().is_empty() {
        return repo.full_name.trim().to_string();
    }
    repo.id.trim().to_string()
}

fn is_safe_github_repo(value: &str) -> bool {
    let parts = value.split('/').collect::<Vec<_>>();
    parts.len() == 2 && parts.iter().all(|part| is_safe_repo_directory(part))
}

fn is_safe_repo_directory(value: &str) -> bool {
    if value.is_empty() || value == "." || value == ".." {
        return false;
    }
    value
        .chars()
        .all(|ch| ch.is_ascii_alphanumeric() || ch == '-' || ch == '_' || ch == '.')
}

fn clone_url_contains_credentials(value: &str) -> bool {
    let Some(scheme_idx) = value.find("://") else {
        return false;
    };
    let authority_and_path = &value[(scheme_idx + 3)..];
    let authority_end = authority_and_path
        .find('/')
        .unwrap_or(authority_and_path.len());
    authority_and_path[..authority_end].contains('@')
}

fn capture_repo_sync_error(repo: &WorkspaceRepoConfig, error: &str) {
    sentry::with_scope(
        |scope| {
            scope.set_tag("runtime.api_context", "repo_sync");
            scope.set_tag("repo_id", repo.id.clone());
            scope.set_tag("repo_name", repo.name.clone());
            scope.set_extra("repo_full_name", repo.full_name.clone().into());
            scope.set_extra("repo_sync_error", error.to_string().into());
        },
        || sentry::capture_message("runtime repository sync failed", sentry::Level::Error),
    );
}

fn repo_id(relative: &str) -> String {
    let mut out = String::from("repo_");
    for ch in relative.chars() {
        if ch.is_ascii_alphanumeric() {
            out.push(ch.to_ascii_lowercase());
        } else {
            out.push('_');
        }
    }
    out
}

pub(super) fn clean_relative_path(raw: &str) -> Result<String> {
    let decoded = percent_decode_str(raw).decode_utf8_lossy();
    let trimmed = decoded.trim_matches('/');
    let mut parts = Vec::new();
    for component in Path::new(trimmed).components() {
        match component {
            Component::Normal(part) => parts.push(part.to_string_lossy().to_string()),
            Component::CurDir => {}
            _ => return Err(anyhow!("invalid path")),
        }
    }
    Ok(parts.join("/"))
}

fn join_rel(parent: &str, name: &str) -> String {
    if parent.is_empty() {
        name.to_string()
    } else {
        format!("{parent}/{name}")
    }
}

fn status_for_path(statuses: &BTreeMap<String, String>, path: &str) -> Option<String> {
    statuses.get(path).cloned().or_else(|| {
        statuses.iter().find_map(|(changed, status)| {
            changed
                .strip_prefix(path)
                .and_then(|tail| tail.strip_prefix('/'))
                .map(|_| status.clone())
        })
    })
}

fn count_lines(text: &str) -> usize {
    if text.is_empty() {
        0
    } else {
        text.lines().count()
    }
}

fn slice_lines(text: &str, offset: Option<usize>, limit: Option<usize>) -> Result<String> {
    if offset.is_none() && limit.is_none() {
        return Ok(text.to_string());
    }
    let lines: Vec<&str> = text.split_inclusive('\n').collect();
    let start = offset.unwrap_or(1).saturating_sub(1);
    if start >= lines.len() && !lines.is_empty() {
        return Err(anyhow!("offset beyond end of file"));
    }
    let end = limit
        .map(|limit| start + limit)
        .unwrap_or(lines.len())
        .min(lines.len());
    Ok(lines[start..end].concat())
}

async fn diff_file_summaries(
    repo_path: &Path,
    base_sha: &str,
    path: Option<&str>,
) -> Result<BTreeMap<String, DiffFileSummary>> {
    let mut args = vec![
        "diff".to_string(),
        "--name-status".to_string(),
        "--find-renames".to_string(),
        base_sha.to_string(),
        "--".to_string(),
    ];
    if let Some(path) = path {
        args.push(path.to_string());
    }
    let output = git_output(repo_path, &args).await?;
    Ok(output
        .lines()
        .filter_map(parse_diff_name_status)
        .map(|file| (file.path.clone(), file))
        .collect())
}

fn parse_diff_name_status(line: &str) -> Option<DiffFileSummary> {
    let mut parts = line.split('\t');
    let code = parts.next()?.trim();
    let status_code = code.chars().next()?;
    let status = match status_code {
        'A' => "added",
        'D' => "deleted",
        'R' => "renamed",
        'C' => "copied",
        _ => "modified",
    }
    .to_string();
    let first_path = parts.next()?.to_string();
    let (path, previous_path) = if matches!(status_code, 'R' | 'C') {
        (parts.next()?.to_string(), Some(first_path))
    } else {
        (first_path, None)
    };
    Some(DiffFileSummary {
        path,
        status,
        previous_path,
        patch: String::new(),
        truncated: false,
        total_bytes: 0,
        max_bytes: MAX_DIFF_BYTES,
        message: None,
    })
}

async fn tracked_file_diff(
    repo_path: &Path,
    base_sha: &str,
    path: &str,
    context: u32,
) -> Result<String> {
    git_output(
        repo_path,
        &[
            "diff".to_string(),
            "--find-renames".to_string(),
            format!("--unified={context}"),
            base_sha.to_string(),
            "--".to_string(),
            path.to_string(),
        ],
    )
    .await
}

async fn untracked_file_diff(repo_path: &Path, path: &str, context: u32) -> Result<String> {
    git_output(
        repo_path,
        &[
            "diff".to_string(),
            "--no-index".to_string(),
            format!("--unified={context}"),
            "/dev/null".to_string(),
            path.to_string(),
        ],
    )
    .await
}

impl DiffFileSummary {
    fn new(path: String, status: String, previous_path: Option<String>) -> Self {
        Self {
            path,
            status,
            previous_path,
            patch: String::new(),
            truncated: false,
            total_bytes: 0,
            max_bytes: MAX_DIFF_BYTES,
            message: None,
        }
    }
}

fn path_in_scope(scope: &str, path: &str) -> bool {
    scope.is_empty()
        || scope == path
        || path
            .strip_prefix(scope.trim_end_matches('/'))
            .is_some_and(|suffix| suffix.starts_with('/'))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::session_stream::StreamReplayMode;
    use tokio::process::Command;

    #[test]
    fn clean_path_rejects_traversal() {
        assert!(clean_relative_path("../secret").is_err());
        assert!(clean_relative_path("/abs").is_ok());
        assert_eq!(clean_relative_path("src/lib.rs").unwrap(), "src/lib.rs");
    }

    #[tokio::test]
    async fn repo_service_discovers_repos_and_publishes_change_batches() {
        let workspace = unique_workspace();
        let repos = workspace.join("repos");
        tokio::fs::create_dir_all(&repos).await.unwrap();
        let first = repos.join("first");
        init_repo(&first, "README.md", "hello\n").await;

        let broker = Arc::new(SessionStreamBroker::new());
        let service = RepoService::new(workspace.clone(), broker.clone());
        service.register_session("session-1").await;

        let listed = service.list_repos().await.unwrap();
        assert_eq!(listed.repos.len(), 1);
        let repo = listed.repos[0].clone();
        let tree = service.tree(&repo.id, "").await.unwrap();
        assert!(tree.entries.iter().any(|entry| entry.path == "README.md"));

        let content = service
            .content(&repo.id, "README.md", None, None)
            .await
            .unwrap();
        assert_eq!(content.content, "hello\n");

        tokio::fs::write(first.join("README.md"), "hello\nchanged\n")
            .await
            .unwrap();
        service.reconcile_and_publish().await.unwrap();
        let diff = service
            .diff(&repo.id, Some("README.md"), Some(3))
            .await
            .unwrap();
        assert!(!diff.truncated);
        assert!(diff.diff.contains("diff --git"));
        assert!(diff.diff.contains("+changed"));
        let readme_diff = diff
            .files
            .iter()
            .find(|file| file.path == "README.md")
            .expect("README.md diff should be present");
        assert_eq!(readme_diff.status, "modified");
        assert!(!readme_diff.truncated);
        assert!(readme_diff.patch.contains("+changed"));

        let stream_id = broker
            .stream_id_for_session("session-1")
            .await
            .expect("repo change should create session stream");
        let (history, _) = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists")
            .into_parts();
        assert!(history.iter().any(|event| {
            event.event == "repo.change_batch"
                && event.payload["repo_id"] == repo.id
                && event.payload["paths"][0]["path"] == "README.md"
        }));

        let second = repos.join("second");
        init_repo(&second, "lib.rs", "pub fn answer() -> i32 { 42 }\n").await;
        let listed = service.list_repos().await.unwrap();
        assert_eq!(listed.repos.len(), 2);

        let _ = tokio::fs::remove_dir_all(workspace).await;
    }

    #[tokio::test]
    async fn workspace_sync_clones_desired_repositories_into_repos_root() {
        let remote_workspace = unique_workspace();
        let source = remote_workspace.join("source");
        init_repo(&source, "README.md", "hello from remote\n").await;

        let workspace = unique_workspace();
        let broker = Arc::new(SessionStreamBroker::new());
        let service = RepoService::new(workspace.clone(), broker);
        service
            .apply_workspace_config(WorkspaceConfig {
                repos: vec![WorkspaceRepoConfig {
                    id: "usehivy/hivy".to_string(),
                    name: "hivy".to_string(),
                    full_name: "usehivy/hivy".to_string(),
                    clone_url: source.display().to_string(),
                    depth: Some(1),
                }],
            })
            .await;

        wait_for_path(&workspace.join("repos").join("hivy").join(".git")).await;
        let listed = service.list_repos().await.unwrap();
        assert!(listed.repos.iter().any(|repo| repo.name == "hivy"));

        let _ = tokio::fs::remove_dir_all(workspace).await;
        let _ = tokio::fs::remove_dir_all(remote_workspace).await;
    }

    #[tokio::test]
    async fn workspace_sync_does_not_delete_existing_non_git_path() {
        let remote_workspace = unique_workspace();
        let source = remote_workspace.join("source");
        init_repo(&source, "README.md", "hello from remote\n").await;

        let workspace = unique_workspace();
        let target = workspace.join("repos").join("hivy");
        tokio::fs::create_dir_all(&target).await.unwrap();
        tokio::fs::write(target.join("note.txt"), "local state\n")
            .await
            .unwrap();

        let broker = Arc::new(SessionStreamBroker::new());
        let service = RepoService::new(workspace.clone(), broker);
        service
            .apply_workspace_config(WorkspaceConfig {
                repos: vec![WorkspaceRepoConfig {
                    id: "usehivy/hivy".to_string(),
                    name: "hivy".to_string(),
                    full_name: "usehivy/hivy".to_string(),
                    clone_url: source.display().to_string(),
                    depth: Some(1),
                }],
            })
            .await;

        wait_for_no_active_syncs(&service).await;
        let content = tokio::fs::read_to_string(target.join("note.txt"))
            .await
            .unwrap();
        assert_eq!(content, "local state\n");

        let _ = tokio::fs::remove_dir_all(workspace).await;
        let _ = tokio::fs::remove_dir_all(remote_workspace).await;
    }

    #[tokio::test]
    async fn diff_shows_changes_on_feature_branch_relative_to_main() {
        let workspace = unique_workspace();
        let repos = workspace.join("repos");
        tokio::fs::create_dir_all(&repos).await.unwrap();
        let repo_path = repos.join("feature-repo");

        // Init repo with an initial commit on main.
        init_repo(&repo_path, "README.md", "hello\n").await;
        // Add a bare "remote" and set origin/HEAD so default_branch_sha can resolve.
        let bare = repos.join("feature-repo-remote.git");
        git(&repo_path, &["init", "--bare", bare.to_str().unwrap()]).await;
        git(
            &repo_path,
            &["remote", "add", "origin", bare.to_str().unwrap()],
        )
        .await;
        git(&repo_path, &["branch", "-M", "main"]).await;
        git(&repo_path, &["push", "origin", "main"]).await;
        git(
            &repo_path,
            &[
                "symbolic-ref",
                "refs/remotes/origin/HEAD",
                "refs/remotes/origin/main",
            ],
        )
        .await;

        let broker = Arc::new(SessionStreamBroker::new());
        let service = RepoService::new(workspace.clone(), broker.clone());

        // Create a feature branch and make a commit.
        git(&repo_path, &["checkout", "-b", "feature/test"]).await;
        git(&repo_path, &["branch", "-D", "main"]).await;
        tokio::fs::write(repo_path.join("README.md"), "hello\nchanged\n")
            .await
            .unwrap();
        git(&repo_path, &["add", "."]).await;
        git(&repo_path, &["commit", "-m", "change on feature branch"]).await;

        // Diff should show the change relative to main, even though we're
        // on a feature branch with everything committed.
        let diff = service
            .diff(&repo_id("repos/feature-repo"), Some("README.md"), Some(3))
            .await
            .unwrap();
        assert!(
            diff.diff.contains("+changed"),
            "diff should show feature branch changes relative to main, got: {}",
            diff.diff
        );

        let _ = tokio::fs::remove_dir_all(workspace).await;
    }

    #[tokio::test]
    async fn diff_uses_merge_base_when_head_is_behind_remote_default() {
        let workspace = unique_workspace();
        let repos = workspace.join("repos");
        tokio::fs::create_dir_all(&repos).await.unwrap();
        let repo_path = repos.join("older-repo");

        init_repo(&repo_path, "README.md", "hello\n").await;
        git(&repo_path, &["branch", "-M", "main"]).await;
        let base_commit = git_stdout(&repo_path, &["rev-parse", "HEAD"]).await;
        let bare = repos.join("older-repo-remote.git");
        git(&repo_path, &["init", "--bare", bare.to_str().unwrap()]).await;
        git(
            &repo_path,
            &["remote", "add", "origin", bare.to_str().unwrap()],
        )
        .await;
        git(&repo_path, &["push", "-u", "origin", "main"]).await;
        git(
            &repo_path,
            &[
                "symbolic-ref",
                "refs/remotes/origin/HEAD",
                "refs/remotes/origin/main",
            ],
        )
        .await;

        tokio::fs::write(repo_path.join("README.md"), "hello\nupstream\n")
            .await
            .unwrap();
        git(&repo_path, &["add", "."]).await;
        git(&repo_path, &["commit", "-m", "upstream change"]).await;
        git(&repo_path, &["push", "origin", "main"]).await;
        git(&repo_path, &["fetch", "origin"]).await;

        git(&repo_path, &["checkout", base_commit.trim()]).await;
        tokio::fs::write(repo_path.join("README.md"), "hello\nlocal\n")
            .await
            .unwrap();

        let broker = Arc::new(SessionStreamBroker::new());
        let service = RepoService::new(workspace.clone(), broker);
        let listed = service.list_repos().await.unwrap();
        let repo = listed
            .repos
            .iter()
            .find(|repo| repo.name == "older-repo")
            .expect("older-repo should be listed");
        assert_eq!(repo.base_sha, base_commit.trim());

        let diff = service
            .diff(&repo.id, Some("README.md"), Some(3))
            .await
            .unwrap();
        assert!(diff.diff.contains("+local"));
        assert!(
            !diff.diff.contains("upstream"),
            "diff should be from merge-base, got: {}",
            diff.diff
        );

        let _ = tokio::fs::remove_dir_all(workspace).await;
    }

    #[tokio::test]
    async fn diff_truncates_large_files_without_hiding_small_file_diffs() {
        let workspace = unique_workspace();
        let repos = workspace.join("repos");
        tokio::fs::create_dir_all(&repos).await.unwrap();
        let repo_path = repos.join("large-repo");
        init_repo(&repo_path, "models.json", "{}\n").await;

        let large_json_line = format!("{{\"models\":\"{}\"}}\n", "x".repeat(MAX_DIFF_BYTES + 1024));
        tokio::fs::write(repo_path.join("models.json"), large_json_line)
            .await
            .unwrap();
        tokio::fs::write(repo_path.join("small.txt"), "small change\n")
            .await
            .unwrap();

        let broker = Arc::new(SessionStreamBroker::new());
        let service = RepoService::new(workspace.clone(), broker);
        let diff = service
            .diff(&repo_id("repos/large-repo"), None, Some(3))
            .await
            .unwrap();

        assert!(diff.truncated);
        assert!(diff.total_bytes > diff.max_bytes);
        assert_eq!(diff.max_bytes, MAX_DIFF_BYTES);
        assert!(diff
            .message
            .as_deref()
            .is_some_and(|message| message.contains("file diffs")));
        let large = diff
            .files
            .iter()
            .find(|file| file.path == "models.json")
            .expect("large file should be listed");
        assert_eq!(large.status, "modified");
        assert!(large.truncated);
        assert!(large.patch.is_empty());
        assert!(large
            .message
            .as_deref()
            .is_some_and(|message| message.contains("too large")));
        let small = diff
            .files
            .iter()
            .find(|file| file.path == "small.txt")
            .expect("small file should be listed");
        assert_eq!(small.status, "untracked");
        assert!(!small.truncated);
        assert!(small.patch.contains("+small change"));
        assert!(diff.diff.contains("+small change"));

        let _ = tokio::fs::remove_dir_all(workspace).await;
    }

    fn unique_workspace() -> PathBuf {
        std::env::temp_dir().join(format!(
            "hivy-repos-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ))
    }

    async fn init_repo(path: &Path, file: &str, content: &str) {
        tokio::fs::create_dir_all(path).await.unwrap();
        git(path, &["init"]).await;
        git(path, &["config", "user.email", "test@example.com"]).await;
        git(path, &["config", "user.name", "Hivy Test"]).await;
        tokio::fs::write(path.join(file), content).await.unwrap();
        git(path, &["add", "."]).await;
        git(path, &["commit", "-m", "initial"]).await;
    }

    async fn git(path: &Path, args: &[&str]) {
        let output = Command::new("git")
            .args(args)
            .current_dir(path)
            .output()
            .await
            .unwrap();
        assert!(
            output.status.success(),
            "git {:?} failed: {}",
            args,
            String::from_utf8_lossy(&output.stderr)
        );
    }

    async fn git_stdout(path: &Path, args: &[&str]) -> String {
        let output = Command::new("git")
            .args(args)
            .current_dir(path)
            .output()
            .await
            .unwrap();
        assert!(
            output.status.success(),
            "git {:?} failed: {}",
            args,
            String::from_utf8_lossy(&output.stderr)
        );
        String::from_utf8_lossy(&output.stdout).trim().to_string()
    }

    async fn wait_for_path(path: &Path) {
        for _ in 0..100 {
            if tokio::fs::metadata(path).await.is_ok() {
                return;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
        panic!("path did not appear: {}", path.display());
    }

    async fn wait_for_no_active_syncs(service: &RepoService) {
        for _ in 0..100 {
            if service.active_syncs.lock().await.is_empty() {
                return;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
        panic!("repo sync job did not finish");
    }
}
