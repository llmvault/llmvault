use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use anyhow::{anyhow, Context, Result};
use async_trait::async_trait;
use domain::{DriveDownloadConfig, DriveUploadConfig};
use reqwest::header::{CONTENT_LENGTH, CONTENT_TYPE};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio_util::io::ReaderStream;

use crate::path::{build_glob_set, enforce_deny_globs, resolve_read_path, resolve_writable_path};
use crate::{schema_for, JsonTool, ToolDefinition};

const ENV_DRIVE_URL: &str = "HIVY_DRIVE_UPLOAD_URL";
const ENV_DRIVE_BEARER: &str = "HIVY_DRIVE_UPLOAD_BEARER";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct DriveUploadArgs {
    /// Folder in the agent drive. Use an empty string for the drive root.
    #[serde(default)]
    pub folder: String,
    /// Existing local sandbox file to upload.
    pub file_path: String,
    /// Optional destination filename. Defaults to the local file name.
    #[serde(default)]
    pub filename: Option<String>,
    /// Optional media type. The control plane will infer it when omitted.
    #[serde(default)]
    pub content_type: Option<String>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct DriveDownloadArgs {
    /// Exact asset_id returned by drive_search.
    pub asset_id: String,
    /// Local sandbox destination path.
    pub destination_path: String,
}

pub struct DriveUploadTool {
    config: DriveUploadConfig,
    workspace_root: PathBuf,
    runtime_env: Arc<HashMap<String, String>>,
}
pub struct DriveDownloadTool {
    config: DriveDownloadConfig,
    workspace_root: PathBuf,
    runtime_env: Arc<HashMap<String, String>>,
}

impl DriveUploadTool {
    pub fn new(
        config: DriveUploadConfig,
        workspace_root: PathBuf,
        runtime_env: Arc<HashMap<String, String>>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            runtime_env,
        }
    }
    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }
    async fn execute(&self, args: Value) -> Result<Value> {
        let args: DriveUploadArgs =
            serde_json::from_value(args).map_err(|e| anyhow!("invalid arguments: {e}"))?;
        let source = resolve_read_path(&self.workspace_root, &args.file_path)
            .map_err(|e| anyhow!(e.to_string()))?;
        enforce_deny_globs(&source, &build_glob_set(&self.config.deny_globs))
            .map_err(|e| anyhow!(e.to_string()))?;
        let metadata = tokio::fs::metadata(&source)
            .await
            .context("inspect source file")?;
        if !metadata.is_file() {
            return Err(anyhow!("file_path must refer to a regular file"));
        }
        if metadata.len() == 0 {
            return Err(anyhow!("empty files are not allowed"));
        }
        if metadata.len() > self.config.max_file_size_bytes {
            return Err(anyhow!(
                "file exceeds max_file_size_bytes ({} > {})",
                metadata.len(),
                self.config.max_file_size_bytes
            ));
        }
        let folder = normalized_folder(&args.folder)?;
        let filename = args
            .filename
            .as_deref()
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .map(str::to_string)
            .unwrap_or_else(|| {
                source
                    .file_name()
                    .and_then(|name| name.to_str())
                    .unwrap_or("upload")
                    .to_string()
            });
        validate_filename(&filename)?;
        let base_url = required_env(&self.runtime_env, ENV_DRIVE_URL)?;
        let bearer = required_env(&self.runtime_env, ENV_DRIVE_BEARER)?;
        let relative = if folder.is_empty() {
            filename.clone()
        } else {
            format!("{folder}/{filename}")
        };
        let file = tokio::fs::File::open(&source)
            .await
            .context("open source file")?;
        let response = reqwest::Client::new()
            .put(format!("{}/{}", base_url.trim_end_matches('/'), relative))
            .bearer_auth(bearer)
            .header(
                CONTENT_TYPE,
                args.content_type
                    .unwrap_or_else(|| "application/octet-stream".into()),
            )
            .header(CONTENT_LENGTH, metadata.len())
            .body(reqwest::Body::wrap_stream(ReaderStream::new(file)))
            .send()
            .await
            .context("upload drive file")?;
        parse_json_response(response, "upload drive file").await
    }
}

impl DriveDownloadTool {
    pub fn new(
        config: DriveDownloadConfig,
        workspace_root: PathBuf,
        runtime_env: Arc<HashMap<String, String>>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            runtime_env,
        }
    }
    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }
    async fn execute(&self, args: Value) -> Result<Value> {
        let args: DriveDownloadArgs =
            serde_json::from_value(args).map_err(|e| anyhow!("invalid arguments: {e}"))?;
        if args.asset_id.trim().is_empty() {
            return Err(anyhow!("asset_id is required"));
        }
        let destination = resolve_writable_path(
            &self.workspace_root,
            &args.destination_path,
            &self.config.allowed_roots,
        )
        .map_err(|e| anyhow!(e.to_string()))?;
        enforce_deny_globs(&destination, &build_glob_set(&self.config.deny_globs))
            .map_err(|e| anyhow!(e.to_string()))?;
        if destination.exists() {
            return Err(anyhow!("destination_path already exists"));
        }
        let base_url = required_env(&self.runtime_env, ENV_DRIVE_URL)?;
        let bearer = required_env(&self.runtime_env, ENV_DRIVE_BEARER)?;
        let response = reqwest::Client::new()
            .get(format!(
                "{}/assets/{}",
                base_url.trim_end_matches('/'),
                args.asset_id.trim()
            ))
            .bearer_auth(bearer)
            .send()
            .await
            .context("download drive asset")?;
        let status = response.status();
        if !status.is_success() {
            return Err(anyhow!("download drive asset: status {status}"));
        }
        if let Some(length) = response.content_length() {
            if length > self.config.max_file_size_bytes {
                return Err(anyhow!(
                    "asset exceeds max_file_size_bytes ({} > {})",
                    length,
                    self.config.max_file_size_bytes
                ));
            }
        }
        if let Some(parent) = destination.parent() {
            tokio::fs::create_dir_all(parent)
                .await
                .context("create destination directory")?;
        }
        let mut file = tokio::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&destination)
            .await
            .context("create destination file")?;
        let mut stream = response.bytes_stream();
        let mut written = 0u64;
        use futures::StreamExt;
        use tokio::io::AsyncWriteExt;
        while let Some(chunk) = stream.next().await {
            let chunk = chunk.context("read download body")?;
            written += chunk.len() as u64;
            if written > self.config.max_file_size_bytes {
                let _ = tokio::fs::remove_file(&destination).await;
                return Err(anyhow!("asset exceeds max_file_size_bytes"));
            }
            file.write_all(&chunk)
                .await
                .context("write destination file")?;
        }
        file.flush().await.context("flush destination file")?;
        Ok(
            json!({"asset_id": args.asset_id, "destination_path": destination.display().to_string(), "bytes_written": written}),
        )
    }
}

#[async_trait]
impl JsonTool for DriveUploadTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition { name: "drive_upload".into(), description: "Upload an existing sandbox file to the agent drive. The MCP server cannot access sandbox files, so this runtime tool streams the file through the authenticated control plane. Use drive_search to find existing assets.".into(), parameters: schema_for::<DriveUploadArgs>() }
    }
    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
    fn errors_are_safe(&self) -> bool {
        true
    }
}

#[async_trait]
impl JsonTool for DriveDownloadTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition { name: "drive_download".into(), description: "Download an exact asset_id from this agent's drive into a new sandbox file. Use drive_search first; existing destination paths are refused to prevent accidental overwrite.".into(), parameters: schema_for::<DriveDownloadArgs>() }
    }
    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
    fn errors_are_safe(&self) -> bool {
        true
    }
}

async fn parse_json_response(response: reqwest::Response, operation: &str) -> Result<Value> {
    let status = response.status();
    let body = response
        .text()
        .await
        .with_context(|| format!("{operation}: read response body"))?;
    if !status.is_success() {
        return Err(anyhow!("{operation}: status {status}"));
    }
    serde_json::from_str(&body).with_context(|| format!("{operation}: decode response"))
}
fn required_env<'a>(env: &'a HashMap<String, String>, key: &str) -> Result<&'a str> {
    env.get(key)
        .map(|value| value.trim())
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("{key} is required for agent drive"))
}
fn normalized_folder(value: &str) -> Result<String> {
    let value = value.trim().trim_matches('/');
    if value.is_empty() {
        return Ok(String::new());
    }
    if value
        .split('/')
        .any(|part| part.is_empty() || part == "." || part == "..")
    {
        return Err(anyhow!(
            "folder must be a relative path without . or .. segments"
        ));
    }
    Ok(value.to_string())
}
fn validate_filename(value: &str) -> Result<()> {
    if value.is_empty()
        || value == "."
        || value == ".."
        || value.contains('/')
        || value.contains('\\')
    {
        return Err(anyhow!("filename must be a single file name"));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::sync::Arc;

    use domain::{DriveDownloadConfig, DriveUploadConfig};
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;

    use super::{DriveDownloadTool, DriveUploadTool};

    fn test_dir(label: &str) -> std::path::PathBuf {
        std::env::temp_dir().join(format!(
            "hivy-drive-{label}-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system clock")
                .as_nanos()
        ))
    }

    async fn server_once(response: String) -> (String, tokio::task::JoinHandle<String>) {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("bind");
        let address = listener.local_addr().expect("address");
        let handle = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let mut bytes = Vec::new();
            let mut chunk = [0u8; 4096];
            loop {
                let read = stream.read(&mut chunk).await.expect("read request");
                if read == 0 {
                    break;
                }
                bytes.extend_from_slice(&chunk[..read]);
                let header_end = bytes.windows(4).position(|value| value == b"\r\n\r\n");
                if let Some(header_end) = header_end {
                    let headers = String::from_utf8_lossy(&bytes[..header_end + 4]);
                    let length = headers
                        .lines()
                        .find_map(|line| {
                            line.strip_prefix("content-length: ")
                                .or_else(|| line.strip_prefix("Content-Length: "))
                        })
                        .and_then(|value| value.trim().parse::<usize>().ok())
                        .unwrap_or(0);
                    if bytes.len() >= header_end + 4 + length {
                        break;
                    }
                }
            }
            stream
                .write_all(response.as_bytes())
                .await
                .expect("write response");
            String::from_utf8(bytes).expect("utf8 request")
        });
        (format!("http://{address}"), handle)
    }

    #[tokio::test]
    async fn upload_streams_local_file_to_authenticated_control_plane() {
        let root = test_dir("upload");
        tokio::fs::create_dir_all(&root).await.expect("root");
        let source = root.join("report.txt");
        tokio::fs::write(&source, "quarterly results")
            .await
            .expect("source");
        let (base_url, server) = server_once("HTTP/1.1 201 Created\r\nContent-Type: application/json\r\nConnection: close\r\n\r\n{\"id\":\"asset-1\",\"path\":\"reports\",\"filename\":\"report.txt\",\"bytes\":17}".to_string()).await;
        let tool = DriveUploadTool::new(
            DriveUploadConfig {
                max_file_size_bytes: 1024,
                deny_globs: vec![],
            },
            root.clone(),
            Arc::new(HashMap::from([
                ("HIVY_DRIVE_UPLOAD_URL".into(), base_url),
                ("HIVY_DRIVE_UPLOAD_BEARER".into(), "sandbox-secret".into()),
            ])),
        );
        let result = tool.execute(serde_json::json!({"folder":"reports", "file_path":"report.txt", "content_type":"text/plain"})).await.expect("upload");
        assert_eq!(result["id"], "asset-1");
        let request = server.await.expect("server");
        assert!(request.starts_with("PUT /reports/report.txt HTTP/1.1"));
        assert!(
            request.contains("authorization: Bearer sandbox-secret")
                || request.contains("Authorization: Bearer sandbox-secret")
        );
        assert!(request.ends_with("quarterly results"));
        tokio::fs::remove_dir_all(root).await.expect("cleanup");
    }

    #[tokio::test]
    async fn download_writes_new_file_and_refuses_overwrite() {
        let root = test_dir("download");
        tokio::fs::create_dir_all(&root).await.expect("root");
        let response = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 16\r\n\r\ndrive file body!".to_string();
        let (base_url, server) = server_once(response).await;
        let tool = DriveDownloadTool::new(
            DriveDownloadConfig {
                allowed_roots: vec![],
                max_file_size_bytes: 1024,
                deny_globs: vec![],
            },
            root.clone(),
            Arc::new(HashMap::from([
                ("HIVY_DRIVE_UPLOAD_URL".into(), base_url),
                ("HIVY_DRIVE_UPLOAD_BEARER".into(), "sandbox-secret".into()),
            ])),
        );
        let result = tool.execute(serde_json::json!({"asset_id":"asset-1", "destination_path":"downloads/report.txt"})).await.expect("download");
        assert_eq!(result["bytes_written"], 16);
        assert_eq!(
            tokio::fs::read_to_string(root.join("downloads/report.txt"))
                .await
                .expect("downloaded file"),
            "drive file body!"
        );
        let request = server.await.expect("server");
        assert!(request.starts_with("GET /assets/asset-1 HTTP/1.1"));
        assert!(
            request.contains("authorization: Bearer sandbox-secret")
                || request.contains("Authorization: Bearer sandbox-secret")
        );
        assert!(tool
            .execute(
                serde_json::json!({"asset_id":"asset-1", "destination_path":"downloads/report.txt"})
            )
            .await
            .expect_err("existing target must fail")
            .to_string()
            .contains("already exists"));
        tokio::fs::remove_dir_all(root).await.expect("cleanup");
    }
}
