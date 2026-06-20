use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::path::{Component, Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use percent_encoding::percent_decode_str;
use serde_json::json;
use tokio::sync::RwLock;
use tracing::warn;

use crate::session_stream::SessionStreamBroker;

use super::git::{
    changed_paths, default_branch_sha, git_output, git_status, snapshot_repo, RepoSnapshot,
};
use super::types::{
    ContentResponse, DiffResponse, RepoInfo, RepoListResponse, TreeEntry, TreeResponse,
};

const REPOS_DIR: &str = "repos";
const DEFAULT_DIFF_CONTEXT: u32 = 3;
const MAX_CONTENT_BYTES: u64 = 512 * 1024;
const MAX_LIST_DEPTH: usize = 4;

#[derive(Clone)]
pub struct RepoService {
    root: PathBuf,
    repos_root: PathBuf,
    state: Arc<RwLock<RepoState>>,
    broker: Arc<SessionStreamBroker>,
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
        }
    }

    pub fn start(self: Arc<Self>) {
        tokio::spawn(async move {
            loop {
                if let Err(error) = self.reconcile_and_publish().await {
                    warn!(%error, "repo monitor reconciliation failed");
                }
                tokio::time::sleep(Duration::from_secs(1)).await;
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
        let mut args = vec![
            "diff".to_string(),
            format!("--unified={context}"),
            repo.base_sha.clone(),
            "--".to_string(),
        ];
        if let Some(path) = &rel {
            args.push(path.clone());
        }
        let mut diff = git_output(&repo_path, &args).await.unwrap_or_default();
        let statuses = git_status(&repo_path).await.unwrap_or_default();
        for (changed_path, status) in statuses {
            if status != "untracked" {
                continue;
            }
            if rel.as_ref().is_some_and(|want| want != &changed_path) {
                continue;
            }
            let file = self.safe_repo_path(&repo, &changed_path)?;
            if tokio::fs::metadata(&file)
                .await
                .is_ok_and(|meta| meta.is_file())
            {
                diff.push_str(
                    &git_output(
                        &repo_path,
                        &[
                            "diff".to_string(),
                            "--no-index".to_string(),
                            format!("--unified={context}"),
                            "/dev/null".to_string(),
                            changed_path,
                        ],
                    )
                    .await
                    .unwrap_or_default(),
                );
            }
        }
        Ok(DiffResponse {
            repo_id: repo.id,
            path: rel,
            diff,
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
        let base_sha = default_branch_sha(path)
            .await
            .unwrap_or_else(|| head.clone());
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
        assert!(diff.diff.contains("diff --git"));
        assert!(diff.diff.contains("+changed"));

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
}
