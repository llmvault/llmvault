use std::collections::{hash_map::DefaultHasher, BTreeMap, BTreeSet, HashMap};
use std::hash::{Hash, Hasher};
use std::path::{Component, Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use anyhow::{anyhow, bail, Context, Result};
use futures::{stream, StreamExt};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::sync::RwLock;
use tracing::{info, warn};

const CANVAS_DIR: &str = "canvas";
const PROJECTS_DIR: &str = "projects";
const ARTIFACTS_DIR: &str = "artifacts";
const SNAPSHOT_DOWNLOAD_CONCURRENCY: usize = 8;
const WATCH_INTERVAL: Duration = Duration::from_secs(1);

#[derive(Clone)]
pub struct CanvasRuntimeService {
    canvas_root: PathBuf,
    control_plane_url: Option<String>,
    agent_id: Option<String>,
    runtime_secret: Option<String>,
    org_id: Option<String>,
    sandbox_id: Option<String>,
    http: reqwest::Client,
    broker: Arc<api::SessionStreamBroker>,
    sessions: Arc<RwLock<BTreeSet<String>>>,
    state: Arc<RwLock<CanvasState>>,
}

#[derive(Default, Clone)]
struct CanvasState {
    files: BTreeMap<String, CanvasFileEntry>,
    sequence: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct CanvasFileEntry {
    key: String,
    path: PathBuf,
    project_slug: String,
    artifact_slug: String,
    artifact_path: String,
    snapshot: CanvasFileSnapshot,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct CanvasFileSnapshot {
    size_bytes: u64,
    modified_ms: u128,
    content_hash: u64,
}

#[derive(Default)]
struct SyncGroup {
    changed: Vec<CanvasFileEntry>,
    deleted: Vec<String>,
}

#[derive(Debug, Deserialize)]
struct SnapshotResponse {
    #[serde(default)]
    projects: Vec<SnapshotProject>,
    #[serde(default)]
    artifacts: Vec<SnapshotArtifact>,
}

#[derive(Debug, Deserialize)]
struct SnapshotProject {
    #[serde(default)]
    id: Option<String>,
    #[serde(default)]
    slug: Option<String>,
    #[serde(default)]
    artifacts: Vec<SnapshotArtifact>,
}

#[derive(Debug, Deserialize)]
struct SnapshotArtifact {
    #[serde(default)]
    id: Option<String>,
    #[serde(default)]
    slug: Option<String>,
    #[serde(default)]
    project_slug: Option<String>,
    #[serde(default)]
    files: Vec<SnapshotFile>,
}

#[derive(Debug, Deserialize)]
struct SnapshotFile {
    path: String,
    #[serde(default)]
    download_url: Option<String>,
    #[serde(default)]
    url: Option<String>,
    #[serde(default)]
    content_url: Option<String>,
    #[serde(default)]
    content: Option<String>,
}

#[derive(Clone)]
struct SnapshotDownload {
    project_slug: String,
    artifact_slug: String,
    path: String,
    url: Option<String>,
    content: Option<String>,
}

#[derive(Debug, Serialize)]
struct SyncFilePayload {
    path: String,
    encoding: &'static str,
    content: String,
    size_bytes: u64,
    content_hash: String,
}

impl CanvasRuntimeService {
    pub fn new(
        workspace_root: PathBuf,
        runtime_env: &HashMap<String, String>,
        broker: Arc<api::SessionStreamBroker>,
        sessions: Arc<RwLock<BTreeSet<String>>>,
    ) -> Self {
        let control_plane_url = non_empty_env(runtime_env, "HIVY_CONTROL_PLANE_URL")
            .map(|value| value.trim_end_matches('/').to_string());
        Self {
            canvas_root: workspace_root.join(CANVAS_DIR),
            control_plane_url,
            agent_id: non_empty_env(runtime_env, "HIVY_AGENT_ID"),
            runtime_secret: non_empty_env(runtime_env, "HIVY_RUNTIME_SECRET"),
            org_id: non_empty_env(runtime_env, "HIVY_ORG_ID"),
            sandbox_id: non_empty_env(runtime_env, "HIVY_SANDBOX_ID"),
            http: reqwest::Client::builder()
                .timeout(Duration::from_secs(30))
                .build()
                .unwrap_or_else(|_| reqwest::Client::new()),
            broker,
            sessions,
            state: Arc::new(RwLock::new(CanvasState::default())),
        }
    }

    pub fn start(self: Arc<Self>) {
        tokio::spawn(async move {
            self.run().await;
        });
    }

    async fn run(self: Arc<Self>) {
        if let Err(error) = tokio::fs::create_dir_all(&self.canvas_root).await {
            warn!(canvas_root = %self.canvas_root.display(), %error, "canvas root setup failed");
            return;
        }
        if let Err(error) = self.clone().hydrate_from_control_plane().await {
            warn!(%error, "canvas hydration failed");
        }
        match self.scan_canvas_files().await {
            Ok(files) => {
                self.state.write().await.files = files;
                info!(canvas_root = %self.canvas_root.display(), "canvas watcher started");
            }
            Err(error) => warn!(%error, "canvas baseline scan failed"),
        }
        loop {
            tokio::time::sleep(WATCH_INTERVAL).await;
            if let Err(error) = self.reconcile_local_changes().await {
                warn!(%error, "canvas watcher reconciliation failed");
            }
        }
    }

    async fn hydrate_from_control_plane(self: Arc<Self>) -> Result<()> {
        let (base_url, agent_id, runtime_secret) = self.control_plane_context()?;
        let url = format!("{base_url}/internal/agents/{agent_id}/canvas/snapshot");
        let response = self
            .http
            .get(url)
            .bearer_auth(runtime_secret)
            .send()
            .await
            .context("canvas snapshot request failed")?
            .error_for_status()
            .context("canvas snapshot request returned an error")?
            .json::<SnapshotResponse>()
            .await
            .context("canvas snapshot response was invalid")?;
        let downloads = response.downloads();
        if downloads.is_empty() {
            return Ok(());
        }
        stream::iter(downloads)
            .for_each_concurrent(SNAPSHOT_DOWNLOAD_CONCURRENCY, |download| {
                let service = self.clone();
                async move {
                    if let Err(error) = service.download_snapshot_file(download).await {
                        warn!(%error, "canvas snapshot file download failed");
                    }
                }
            })
            .await;
        Ok(())
    }

    async fn download_snapshot_file(&self, download: SnapshotDownload) -> Result<()> {
        let destination = artifact_file_path(
            &self.canvas_root,
            &download.project_slug,
            &download.artifact_slug,
            &download.path,
        )?;
        if let Some(parent) = destination.parent() {
            tokio::fs::create_dir_all(parent).await?;
        }
        if let Some(content) = download.content {
            tokio::fs::write(destination, content).await?;
            return Ok(());
        }
        let url = download
            .url
            .ok_or_else(|| anyhow!("canvas snapshot file has no download URL"))?;
        let bytes = self
            .http
            .get(url)
            .send()
            .await
            .context("download request failed")?
            .error_for_status()
            .context("download request returned an error")?
            .bytes()
            .await
            .context("download response body failed")?;
        tokio::fs::write(destination, bytes).await?;
        Ok(())
    }

    async fn reconcile_local_changes(&self) -> Result<()> {
        let current = self.scan_canvas_files().await?;
        let previous = self.state.read().await.files.clone();
        let groups = changed_groups(&previous, &current);
        if groups.is_empty() {
            return Ok(());
        }
        let sequence = {
            let mut state = self.state.write().await;
            state.sequence += 1;
            state.sequence
        };
        let mut all_synced = true;
        for ((project_slug, artifact_slug), group) in groups {
            let current_entries = current
                .values()
                .filter(|entry| {
                    entry.project_slug == project_slug && entry.artifact_slug == artifact_slug
                })
                .cloned()
                .collect::<Vec<_>>();
            let changed_paths = group
                .changed
                .iter()
                .map(|entry| entry.artifact_path.clone())
                .collect::<Vec<_>>();
            let result = self
                .sync_artifact_group(&project_slug, &artifact_slug, &current_entries, &group)
                .await;
            let status = if result.is_ok() { "synced" } else { "failed" };
            if let Err(error) = &result {
                all_synced = false;
                warn!(%project_slug, %artifact_slug, %error, "canvas artifact sync failed");
            }
            self.publish_sync_event(
                sequence,
                &project_slug,
                &artifact_slug,
                &changed_paths,
                &group.deleted,
                status,
            )
            .await;
        }
        if all_synced {
            self.state.write().await.files = current;
        }
        Ok(())
    }

    async fn sync_artifact_group(
        &self,
        project_slug: &str,
        artifact_slug: &str,
        current_entries: &[CanvasFileEntry],
        group: &SyncGroup,
    ) -> Result<()> {
        let (base_url, agent_id, runtime_secret) = self.control_plane_context()?;
        let artifact_root = self
            .canvas_root
            .join(PROJECTS_DIR)
            .join(project_slug)
            .join(ARTIFACTS_DIR)
            .join(artifact_slug);
        let project_manifest = read_json_file(
            &self
                .canvas_root
                .join(PROJECTS_DIR)
                .join(project_slug)
                .join("project.json"),
        )
        .await;
        let artifact_manifest = read_json_file(&artifact_root.join("artifact.json")).await;
        let files = sync_file_payloads(current_entries).await?;
        let payload = json!({
            "source": "runtime_watcher",
            "org_id": self.org_id,
            "sandbox_id": self.sandbox_id,
            "project": {
                "slug": project_slug,
                "name": project_manifest
                    .as_ref()
                    .and_then(|value| value.get("name"))
                    .and_then(Value::as_str)
                    .unwrap_or(project_slug),
                "manifest": project_manifest.unwrap_or_else(|| json!({ "slug": project_slug }))
            },
            "artifact": {
                "slug": artifact_slug,
                "name": artifact_manifest
                    .as_ref()
                    .and_then(|value| value.get("name"))
                    .and_then(Value::as_str)
                    .unwrap_or(artifact_slug),
                "type": artifact_manifest
                    .as_ref()
                    .and_then(|value| value.get("type"))
                    .and_then(Value::as_str)
                    .unwrap_or("web_page"),
                "manifest": artifact_manifest.unwrap_or_else(|| json!({ "slug": artifact_slug }))
            },
            "files": files,
            "deleted_paths": group.deleted
        });
        let url = format!("{base_url}/internal/agents/{agent_id}/canvas/artifacts/sync");
        self.http
            .post(url)
            .bearer_auth(runtime_secret)
            .json(&payload)
            .send()
            .await
            .context("canvas sync request failed")?
            .error_for_status()
            .context("canvas sync request returned an error")?;
        Ok(())
    }

    async fn publish_sync_event(
        &self,
        sequence: u64,
        project_slug: &str,
        artifact_slug: &str,
        changed_paths: &[String],
        deleted_paths: &[String],
        status: &str,
    ) {
        let sessions = self
            .sessions
            .read()
            .await
            .iter()
            .cloned()
            .collect::<Vec<_>>();
        for session_id in sessions {
            let stream_id = self.broker.get_or_create_session_stream(&session_id).await;
            self.broker
                .publish(
                    &stream_id,
                    "canvas.sync",
                    json!({
                        "session_id": session_id,
                        "sequence": sequence,
                        "status": status,
                        "project_slug": project_slug,
                        "artifact_slug": artifact_slug,
                        "changed_paths": changed_paths,
                        "deleted_paths": deleted_paths
                    }),
                )
                .await;
        }
    }

    async fn scan_canvas_files(&self) -> Result<BTreeMap<String, CanvasFileEntry>> {
        scan_canvas_files(&self.canvas_root).await
    }

    fn control_plane_context(&self) -> Result<(&str, &str, &str)> {
        let base_url = self
            .control_plane_url
            .as_deref()
            .ok_or_else(|| anyhow!("HIVY_CONTROL_PLANE_URL is missing"))?;
        let agent_id = self
            .agent_id
            .as_deref()
            .ok_or_else(|| anyhow!("HIVY_AGENT_ID is missing"))?;
        let runtime_secret = self
            .runtime_secret
            .as_deref()
            .ok_or_else(|| anyhow!("HIVY_RUNTIME_SECRET is missing"))?;
        Ok((base_url, agent_id, runtime_secret))
    }
}

impl SnapshotResponse {
    fn downloads(self) -> Vec<SnapshotDownload> {
        let mut downloads = Vec::new();
        for artifact in self.artifacts {
            if let Some(project_slug) = non_empty_option(artifact.project_slug.clone()) {
                downloads.extend(artifact.downloads(project_slug));
            }
        }
        for project in self.projects {
            let Some(project_slug) =
                non_empty_option(project.slug.clone()).or_else(|| non_empty_option(project.id))
            else {
                continue;
            };
            for artifact in project.artifacts {
                downloads.extend(artifact.downloads(project_slug.clone()));
            }
        }
        downloads
    }
}

impl SnapshotArtifact {
    fn downloads(self, project_slug: String) -> Vec<SnapshotDownload> {
        let Some(artifact_slug) =
            non_empty_option(self.slug.clone()).or_else(|| non_empty_option(self.id))
        else {
            return Vec::new();
        };
        self.files
            .into_iter()
            .map(|file| SnapshotDownload {
                project_slug: project_slug.clone(),
                artifact_slug: artifact_slug.clone(),
                path: file.path,
                url: file
                    .download_url
                    .or(file.url)
                    .or(file.content_url)
                    .and_then(non_empty_string),
                content: file.content,
            })
            .collect()
    }
}

async fn scan_canvas_files(canvas_root: &Path) -> Result<BTreeMap<String, CanvasFileEntry>> {
    if tokio::fs::metadata(canvas_root).await.is_err() {
        return Ok(BTreeMap::new());
    }
    let mut files = BTreeMap::new();
    let mut stack = vec![canvas_root.to_path_buf()];
    while let Some(dir) = stack.pop() {
        let mut entries = match tokio::fs::read_dir(&dir).await {
            Ok(entries) => entries,
            Err(error) => {
                warn!(path = %dir.display(), %error, "canvas scan skipped unreadable directory");
                continue;
            }
        };
        while let Some(entry) = entries.next_entry().await? {
            let path = entry.path();
            let metadata = entry.metadata().await?;
            if metadata.is_dir() {
                stack.push(path);
                continue;
            }
            if !metadata.is_file() {
                continue;
            }
            let rel = path
                .strip_prefix(canvas_root)
                .map_err(|_| anyhow!("canvas file escaped root"))?;
            let rel_key = forward_slash_path(rel);
            let Some((project_slug, artifact_slug, artifact_path)) = artifact_parts(&rel_key)
            else {
                continue;
            };
            let bytes = match tokio::fs::read(&path).await {
                Ok(bytes) => bytes,
                Err(error) => {
                    warn!(path = %path.display(), %error, "canvas scan skipped unreadable file");
                    continue;
                }
            };
            files.insert(
                rel_key.clone(),
                CanvasFileEntry {
                    key: rel_key,
                    path,
                    project_slug,
                    artifact_slug,
                    artifact_path,
                    snapshot: CanvasFileSnapshot {
                        size_bytes: metadata.len(),
                        modified_ms: modified_ms(&metadata),
                        content_hash: content_hash(&bytes),
                    },
                },
            );
        }
    }
    Ok(files)
}

fn changed_groups(
    previous: &BTreeMap<String, CanvasFileEntry>,
    current: &BTreeMap<String, CanvasFileEntry>,
) -> BTreeMap<(String, String), SyncGroup> {
    let mut groups = BTreeMap::new();
    for (key, entry) in current {
        if previous
            .get(key)
            .is_some_and(|old| old.snapshot == entry.snapshot)
        {
            continue;
        }
        groups
            .entry((entry.project_slug.clone(), entry.artifact_slug.clone()))
            .or_insert_with(SyncGroup::default)
            .changed
            .push(entry.clone());
    }
    for (key, entry) in previous {
        if current.contains_key(key) {
            continue;
        }
        groups
            .entry((entry.project_slug.clone(), entry.artifact_slug.clone()))
            .or_insert_with(SyncGroup::default)
            .deleted
            .push(entry.artifact_path.clone());
    }
    groups
}

async fn sync_file_payloads(entries: &[CanvasFileEntry]) -> Result<Vec<SyncFilePayload>> {
    let mut files = Vec::with_capacity(entries.len());
    for entry in entries {
        let bytes = tokio::fs::read(&entry.path).await?;
        let content_hash = format!("{:016x}", content_hash(&bytes));
        let content = String::from_utf8(bytes).context("canvas artifact files must be utf-8")?;
        files.push(SyncFilePayload {
            path: entry.artifact_path.clone(),
            encoding: "utf-8",
            content,
            size_bytes: entry.snapshot.size_bytes,
            content_hash,
        });
    }
    Ok(files)
}

async fn read_json_file(path: &Path) -> Option<Value> {
    let bytes = tokio::fs::read(path).await.ok()?;
    serde_json::from_slice(&bytes).ok()
}

fn artifact_file_path(
    canvas_root: &Path,
    project_slug: &str,
    artifact_slug: &str,
    file_path: &str,
) -> Result<PathBuf> {
    validate_segment(project_slug)?;
    validate_segment(artifact_slug)?;
    let rel = clean_relative_path(file_path)?;
    if rel.as_os_str().is_empty() {
        bail!("canvas artifact file path is required");
    }
    Ok(canvas_root
        .join(PROJECTS_DIR)
        .join(project_slug)
        .join(ARTIFACTS_DIR)
        .join(artifact_slug)
        .join(rel))
}

fn artifact_parts(relative_key: &str) -> Option<(String, String, String)> {
    let parts = relative_key.split('/').collect::<Vec<_>>();
    if parts.len() < 5 || parts[0] != PROJECTS_DIR || parts[2] != ARTIFACTS_DIR {
        return None;
    }
    let project_slug = parts[1].to_string();
    let artifact_slug = parts[3].to_string();
    if validate_segment(&project_slug).is_err() || validate_segment(&artifact_slug).is_err() {
        return None;
    }
    let artifact_path = parts[4..].join("/");
    if artifact_path.is_empty() {
        return None;
    }
    Some((project_slug, artifact_slug, artifact_path))
}

fn clean_relative_path(raw: &str) -> Result<PathBuf> {
    let trimmed = raw.trim_matches('/');
    let mut out = PathBuf::new();
    for component in Path::new(trimmed).components() {
        match component {
            Component::Normal(part) => out.push(part),
            Component::CurDir => {}
            _ => bail!("invalid canvas file path"),
        }
    }
    Ok(out)
}

fn validate_segment(segment: &str) -> Result<()> {
    if segment.is_empty()
        || segment == "."
        || segment == ".."
        || segment.contains('/')
        || segment.contains('\\')
    {
        bail!("invalid canvas path segment");
    }
    Ok(())
}

fn forward_slash_path(path: &Path) -> String {
    path.components()
        .filter_map(|component| match component {
            Component::Normal(part) => Some(part.to_string_lossy().to_string()),
            _ => None,
        })
        .collect::<Vec<_>>()
        .join("/")
}

fn modified_ms(metadata: &std::fs::Metadata) -> u128 {
    metadata
        .modified()
        .unwrap_or(SystemTime::UNIX_EPOCH)
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis()
}

fn content_hash(bytes: &[u8]) -> u64 {
    let mut hasher = DefaultHasher::new();
    bytes.hash(&mut hasher);
    hasher.finish()
}

fn non_empty_env(env: &HashMap<String, String>, key: &str) -> Option<String> {
    env.get(key)
        .and_then(|value| non_empty_string(value.clone()))
}

fn non_empty_option(value: Option<String>) -> Option<String> {
    value.and_then(non_empty_string)
}

fn non_empty_string(value: String) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn unique_temp_dir() -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "hivy-canvas-runtime-test-{}-{}",
            std::process::id(),
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir_all(&dir).unwrap();
        dir
    }

    #[tokio::test]
    async fn scan_canvas_files_tracks_artifact_files() {
        let root = unique_temp_dir();
        let canvas = root.join(CANVAS_DIR);
        let artifact = canvas
            .join(PROJECTS_DIR)
            .join("homepage")
            .join(ARTIFACTS_DIR)
            .join("variant-a");
        tokio::fs::create_dir_all(&artifact).await.unwrap();
        tokio::fs::write(artifact.join("index.html"), "<main>Hello</main>")
            .await
            .unwrap();

        let files = scan_canvas_files(&canvas).await.unwrap();

        let entry = files
            .get("projects/homepage/artifacts/variant-a/index.html")
            .unwrap();
        assert_eq!(entry.project_slug, "homepage");
        assert_eq!(entry.artifact_slug, "variant-a");
        assert_eq!(entry.artifact_path, "index.html");

        let _ = std::fs::remove_dir_all(root);
    }

    #[test]
    fn artifact_file_path_rejects_traversal() {
        let root = PathBuf::from("/workspace/canvas");

        assert!(artifact_file_path(&root, "project", "artifact", "../secret").is_err());
        assert!(artifact_file_path(&root, "project/escape", "artifact", "index.html").is_err());
        assert_eq!(
            artifact_file_path(&root, "project", "artifact", "slides/one.html").unwrap(),
            root.join("projects/project/artifacts/artifact/slides/one.html")
        );
    }

    #[test]
    fn snapshot_response_accepts_nested_and_flat_artifacts() {
        let snapshot: SnapshotResponse = serde_json::from_value(json!({
            "projects": [{
                "slug": "homepage",
                "artifacts": [{
                    "slug": "variant-a",
                    "files": [{ "path": "index.html", "download_url": "http://example.test/a" }]
                }]
            }],
            "artifacts": [{
                "slug": "standalone",
                "project_slug": "homepage",
                "files": [{ "path": "index.html", "url": "http://example.test/b" }]
            }]
        }))
        .unwrap();

        let downloads = snapshot.downloads();

        assert_eq!(downloads.len(), 2);
        assert_eq!(downloads[0].project_slug, "homepage");
        assert_eq!(downloads[0].artifact_slug, "standalone");
        assert_eq!(downloads[1].artifact_slug, "variant-a");
    }
}
