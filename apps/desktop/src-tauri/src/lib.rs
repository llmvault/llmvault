use std::{
    path::{Path, PathBuf},
    process::Stdio,
    sync::Arc,
    time::Duration,
};

use keyring::Entry;
use rand::distr::{Alphanumeric, SampleString};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tauri::{ipc::Channel, AppHandle, Manager, State};
use tokio::{process::Child, sync::Mutex};

const KEYRING_SERVICE: &str = "com.usehivy.desktop";
const RUNTIME_SECRET_ACCOUNT: &str = "runtime-bearer-token";
struct DesktopState {
    runtime: Arc<Mutex<Option<Child>>>,
    runtime_secret: String,
    runtime_base_url: String,
    cloud_url: reqwest::Url,
    http: reqwest::Client,
}

#[derive(Serialize)]
struct DesktopInfo {
    desktop: bool,
    runtime_base_url: String,
    runtime_ready: bool,
}

#[derive(Deserialize)]
struct RuntimeRequest {
    method: String,
    path: String,
    #[serde(default)]
    body: Option<Value>,
}

#[derive(Serialize)]
struct RuntimeResponse {
    status: u16,
    body: Value,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct RuntimeStreamFrame {
    session_id: String,
    event: String,
    id: String,
    data: Value,
}

fn credential(account: &str) -> Result<Entry, String> {
    Entry::new(KEYRING_SERVICE, account).map_err(|error| error.to_string())
}

fn runtime_secret() -> Result<String, String> {
    if let Ok(secret) = std::env::var("HIVY_DESKTOP_RUNTIME_SECRET") {
        if !secret.trim().is_empty() {
            return Ok(secret);
        }
    }
    let entry = credential(RUNTIME_SECRET_ACCOUNT)?;
    if let Ok(secret) = entry.get_password() {
        if !secret.trim().is_empty() {
            return Ok(secret);
        }
    }
    let secret = Alphanumeric.sample_string(&mut rand::rng(), 64);
    entry
        .set_password(&secret)
        .map_err(|error| format!("store runtime secret in OS credential manager: {error}"))?;
    Ok(secret)
}

fn cloud_url() -> Result<reqwest::Url, String> {
    let raw = std::env::var("HIVY_DESKTOP_CLOUD_URL")
        .ok()
        .or_else(|| option_env!("HIVY_DESKTOP_CLOUD_URL").map(str::to_string))
        .ok_or_else(|| "HIVY_DESKTOP_CLOUD_URL was not set when the app was built".to_string())?;
    parse_cloud_url(&raw)
}

fn parse_cloud_url(raw: &str) -> Result<reqwest::Url, String> {
    let mut url = reqwest::Url::parse(raw.trim())
        .map_err(|error| format!("parse HIVY_DESKTOP_CLOUD_URL: {error}"))?;
    let loopback_http = url.scheme() == "http"
        && url
            .host_str()
            .is_some_and(|host| matches!(host, "localhost" | "127.0.0.1" | "::1"));
    if url.scheme() != "https" && !loopback_http {
        return Err("HIVY_DESKTOP_CLOUD_URL must use HTTPS or loopback HTTP".to_string());
    }
    url.set_path("");
    url.set_query(None);
    url.set_fragment(None);
    Ok(url)
}

fn loopback_runtime_url() -> Result<String, String> {
    if let Ok(raw) = std::env::var("HIVY_DESKTOP_RUNTIME_URL") {
        let url = reqwest::Url::parse(raw.trim())
            .map_err(|error| format!("parse HIVY_DESKTOP_RUNTIME_URL: {error}"))?;
        if url.scheme() != "http"
            || !url
                .host_str()
                .is_some_and(|host| matches!(host, "localhost" | "127.0.0.1" | "::1"))
            || url.port().is_none()
        {
            return Err(
                "HIVY_DESKTOP_RUNTIME_URL must be loopback HTTP with an explicit port".to_string(),
            );
        }
        return Ok(url.as_str().trim_end_matches('/').to_string());
    }
    let listener = std::net::TcpListener::bind("127.0.0.1:0")
        .map_err(|error| format!("reserve desktop runtime port: {error}"))?;
    let port = listener
        .local_addr()
        .map_err(|error| format!("read desktop runtime port: {error}"))?
        .port();
    drop(listener);
    Ok(format!("http://127.0.0.1:{port}"))
}

fn runtime_binary(app: &AppHandle) -> Result<PathBuf, String> {
    if let Ok(path) = std::env::var("HIVY_DESKTOP_RUNTIME_BINARY") {
        let path = PathBuf::from(path);
        if path.is_file() {
            return Ok(path);
        }
    }

    if let Ok(resource_dir) = app.path().resource_dir() {
        for packaged in [
            resource_dir.join("hivy-sandboxes-runtime"),
            resource_dir.join("resources/hivy-sandboxes-runtime"),
        ] {
            if packaged.is_file() {
                return Ok(packaged);
            }
        }
    }

    let manifest_dir = Path::new(env!("CARGO_MANIFEST_DIR"));
    let development =
        manifest_dir.join("../../../sandboxes/runtime/target/debug/hivy-sandboxes-runtime");
    if development.is_file() {
        return development
            .canonicalize()
            .map_err(|error| format!("resolve development runtime: {error}"));
    }

    Err("Hivy runtime binary was not found; run pnpm prepare:runtime".to_string())
}

fn spawn_runtime(app: &AppHandle, state: &DesktopState) -> Result<Child, String> {
    let binary = runtime_binary(app)?;
    let app_data = app
        .path()
        .app_data_dir()
        .map_err(|error| format!("resolve desktop data directory: {error}"))?;
    let workspace = app_data.join("workspace");
    std::fs::create_dir_all(&workspace)
        .map_err(|error| format!("create desktop workspace: {error}"))?;

    let database = app_data.join("runtime.db");
    let bind = state
        .runtime_base_url
        .strip_prefix("http://")
        .ok_or_else(|| "invalid local runtime URL".to_string())?;
    let mut command = tokio::process::Command::new(binary);
    command
        .env("HIVY_RUNTIME_MODE", "desktop")
        .env("HIVY_RUNTIME_BIND_ADDR", &bind)
        .env("HIVY_RUNTIME_SECRET", &state.runtime_secret)
        .env("HIVY_DB_PATH", database)
        .env("HIVY_WORKSPACE_ROOT", workspace)
        .env("HIVY_RUNTIME_CORS_MODE", "runtime")
        .stdin(Stdio::null())
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .kill_on_drop(true);
    command
        .spawn()
        .map_err(|error| format!("start Hivy desktop runtime: {error}"))
}

async fn ensure_runtime(app: &AppHandle, state: &DesktopState) -> Result<(), String> {
    let mut runtime = state.runtime.lock().await;
    if let Some(child) = runtime.as_mut() {
        match child.try_wait() {
            Ok(None) => return Ok(()),
            Ok(Some(_)) => *runtime = None,
            Err(error) => return Err(format!("inspect Hivy desktop runtime: {error}")),
        }
    }
    *runtime = Some(spawn_runtime(app, state)?);
    Ok(())
}

async fn runtime_ready(state: &DesktopState) -> bool {
    state
        .http
        .get(format!("{}/healthz", state.runtime_base_url))
        .timeout(Duration::from_secs(1))
        .send()
        .await
        .is_ok_and(|response| response.status().is_success())
}

async fn wait_for_runtime(state: &DesktopState) -> bool {
    for _ in 0..50 {
        if runtime_ready(state).await {
            return true;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    false
}

#[tauri::command]
fn desktop_cloud_url(state: State<'_, DesktopState>) -> String {
    state.cloud_url.as_str().trim_end_matches('/').to_string()
}

fn trusted_cloud_origin(
    webview: &tauri::WebviewWindow,
    state: &DesktopState,
) -> Result<(), String> {
    let current = webview
        .url()
        .map_err(|error| format!("read desktop page URL: {error}"))?;
    let expected = &state.cloud_url;
    if current.scheme() == expected.scheme()
        && current.host_str() == expected.host_str()
        && current.port_or_known_default() == expected.port_or_known_default()
    {
        Ok(())
    } else {
        Err("desktop bridge is unavailable for this page origin".to_string())
    }
}

#[tauri::command]
async fn desktop_info(
    app: AppHandle,
    webview: tauri::WebviewWindow,
    state: State<'_, DesktopState>,
) -> Result<DesktopInfo, String> {
    trusted_cloud_origin(&webview, &state)?;
    ensure_runtime(&app, &state).await?;
    Ok(DesktopInfo {
        desktop: true,
        runtime_base_url: state.runtime_base_url.clone(),
        runtime_ready: wait_for_runtime(&state).await,
    })
}

#[tauri::command]
async fn runtime_request(
    app: AppHandle,
    webview: tauri::WebviewWindow,
    request: RuntimeRequest,
    state: State<'_, DesktopState>,
) -> Result<RuntimeResponse, String> {
    trusted_cloud_origin(&webview, &state)?;
    if !request.path.starts_with('/') || request.path.starts_with("//") {
        return Err("runtime path must be an absolute local path".to_string());
    }
    if !allowed_runtime_request(&request.method, &request.path) {
        return Err("runtime operation is not available to desktop web content".to_string());
    }
    ensure_runtime(&app, &state).await?;
    let method = reqwest::Method::from_bytes(request.method.as_bytes())
        .map_err(|_| "invalid runtime request method".to_string())?;
    let mut outgoing = state
        .http
        .request(
            method,
            format!("{}{}", state.runtime_base_url, request.path),
        )
        .bearer_auth(&state.runtime_secret);
    if let Some(body) = request.body {
        outgoing = outgoing.json(&body);
    }
    let response = outgoing
        .send()
        .await
        .map_err(|error| format!("local runtime request failed: {error}"))?;
    let status = response.status().as_u16();
    let bytes = response
        .bytes()
        .await
        .map_err(|error| format!("read local runtime response: {error}"))?;
    let body = if bytes.is_empty() {
        Value::Null
    } else {
        serde_json::from_slice(&bytes)
            .unwrap_or_else(|_| Value::String(String::from_utf8_lossy(&bytes).into_owned()))
    };
    Ok(RuntimeResponse { status, body })
}

#[tauri::command]
async fn runtime_session_stream(
    app: AppHandle,
    webview: tauri::WebviewWindow,
    session_id: String,
    turn_id: String,
    on_event: Channel<RuntimeStreamFrame>,
    state: State<'_, DesktopState>,
) -> Result<(), String> {
    trusted_cloud_origin(&webview, &state)?;
    if !safe_runtime_id(&session_id) || !safe_runtime_id(&turn_id) {
        return Err("invalid local runtime session stream identifier".to_string());
    }
    ensure_runtime(&app, &state).await?;
    let mut response = state
        .http
        .get(format!(
            "{}/sessions/{}/stream?from_turn_id={}&follow=false",
            state.runtime_base_url, session_id, turn_id
        ))
        .bearer_auth(&state.runtime_secret)
        .timeout(Duration::from_secs(6 * 60 * 60))
        .send()
        .await
        .map_err(|error| format!("open local runtime session stream: {error}"))?;
    if !response.status().is_success() {
        return Err(format!(
            "local runtime session stream returned {}",
            response.status()
        ));
    }
    let content_type = response
        .headers()
        .get(reqwest::header::CONTENT_TYPE)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default();
    if !content_type
        .to_ascii_lowercase()
        .contains("text/event-stream")
    {
        return Err("local runtime returned an invalid session stream".to_string());
    }

    let mut buffer = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|error| format!("read local runtime session stream: {error}"))?
    {
        buffer.extend_from_slice(&chunk);
        for frame in drain_runtime_sse_frames(&mut buffer, &session_id)? {
            on_event
                .send(frame)
                .map_err(|error| format!("deliver local runtime session event: {error}"))?;
        }
    }
    Ok(())
}

fn drain_runtime_sse_frames(
    buffer: &mut Vec<u8>,
    session_id: &str,
) -> Result<Vec<RuntimeStreamFrame>, String> {
    let mut frames = Vec::new();
    while let Some(end) = buffer.windows(2).position(|window| window == b"\n\n") {
        let raw = buffer.drain(..end + 2).collect::<Vec<_>>();
        let text = std::str::from_utf8(&raw[..end])
            .map_err(|error| format!("decode local runtime session event: {error}"))?;
        if let Some(frame) = parse_runtime_sse_frame(text, session_id) {
            frames.push(frame);
        }
    }
    Ok(frames)
}

fn parse_runtime_sse_frame(raw: &str, session_id: &str) -> Option<RuntimeStreamFrame> {
    let mut event = String::new();
    let mut id = String::new();
    let mut data = Vec::new();
    for line in raw.lines() {
        let line = line.trim_end_matches('\r');
        if let Some(value) = line.strip_prefix("event:") {
            event = value.trim_start().to_string();
        } else if let Some(value) = line.strip_prefix("id:") {
            id = value.trim_start().to_string();
        } else if let Some(value) = line.strip_prefix("data:") {
            data.push(value.trim_start());
        }
    }
    if event.is_empty() && data.is_empty() {
        return None;
    }
    let raw_data = data.join("\n");
    let data = serde_json::from_str(&raw_data).unwrap_or_else(|_| Value::String(raw_data));
    Some(RuntimeStreamFrame {
        session_id: session_id.to_string(),
        event: if event.is_empty() {
            "message".to_string()
        } else {
            event
        },
        id,
        data,
    })
}

fn safe_runtime_id(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_'))
}

fn allowed_runtime_request(method: &str, path: &str) -> bool {
    if path.contains('#') || path.contains("..") {
        return false;
    }
    let (route_path, query) = path
        .split_once('?')
        .map_or((path, None), |(route, query)| (route, Some(query)));
    if query.is_some() && method != "GET" {
        return false;
    }
    let segments = route_path
        .split('/')
        .filter(|segment| !segment.is_empty())
        .collect::<Vec<_>>();
    match (method, segments.as_slice()) {
        ("PUT", ["desktop", "agents", agent_id, "config"]) => safe_runtime_id(agent_id),
        ("POST", ["desktop", "agents", agent_id, "sessions", session_id, "messages"]) => {
            safe_runtime_id(agent_id) && safe_runtime_id(session_id)
        }
        ("GET", ["repos"]) => true,
        ("GET", ["repos", repo_id, operation]) => {
            safe_runtime_id(repo_id) && matches!(*operation, "tree" | "content" | "diff")
        }
        _ => false,
    }
}

pub fn run() {
    let secret = runtime_secret().expect("initialize OS-protected runtime credential");
    let cloud_url = cloud_url().expect("resolve Hivy desktop cloud URL");
    let state = DesktopState {
        runtime: Arc::new(Mutex::new(None)),
        runtime_secret: secret,
        runtime_base_url: loopback_runtime_url().expect("reserve desktop runtime port"),
        cloud_url,
        http: reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(3))
            .timeout(Duration::from_secs(120))
            .build()
            .expect("build local runtime HTTP client"),
    };

    tauri::Builder::default()
        .manage(state)
        .invoke_handler(tauri::generate_handler![
            desktop_cloud_url,
            desktop_info,
            runtime_request,
            runtime_session_stream
        ])
        .setup(|app| {
            let handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                let state = handle.state::<DesktopState>();
                if let Err(error) = ensure_runtime(&handle, &state).await {
                    eprintln!("{error}");
                }
            });
            Ok(())
        })
        .on_window_event(|window, event| {
            if matches!(event, tauri::WindowEvent::Destroyed) {
                let handle = window.app_handle().clone();
                tauri::async_runtime::spawn(async move {
                    if let Some(mut child) =
                        handle.state::<DesktopState>().runtime.lock().await.take()
                    {
                        let _ = child.kill().await;
                    }
                });
            }
        })
        .run(tauri::generate_context!())
        .expect("run Hivy desktop app");
}

#[cfg(test)]
mod tests {
    use super::{allowed_runtime_request, drain_runtime_sse_frames, parse_cloud_url};

    #[test]
    fn desktop_bridge_only_allows_required_runtime_operations() {
        assert!(allowed_runtime_request(
            "PUT",
            "/desktop/agents/agent-1/config"
        ));
        assert!(allowed_runtime_request(
            "POST",
            "/desktop/agents/agent-1/sessions/session-1/messages"
        ));
        assert!(allowed_runtime_request("GET", "/repos"));
        assert!(allowed_runtime_request(
            "GET",
            "/repos/repo-1/content?path=src%2Flib.rs"
        ));
        assert!(!allowed_runtime_request("POST", "/control/commands"));
        assert!(!allowed_runtime_request("GET", "/config"));
        assert!(!allowed_runtime_request("POST", "/repos?path=src"));
        assert!(!allowed_runtime_request(
            "POST",
            "/desktop/agents/../sessions/session-1/messages"
        ));
    }

    #[test]
    fn cloud_origin_requires_https_except_for_loopback_development() {
        assert!(parse_cloud_url("https://hivy.example/path").is_ok());
        assert!(parse_cloud_url("http://localhost:30112").is_ok());
        assert!(parse_cloud_url("http://127.0.0.1:30112").is_ok());
        assert!(parse_cloud_url("http://hivy.example").is_err());
        assert!(parse_cloud_url("file:///tmp/hivy").is_err());
    }

    #[test]
    fn runtime_sse_frames_stream_across_chunk_boundaries() {
        let mut buffer = b"event: token\nid: 4\ndata: {\"text\":\"hel".to_vec();
        assert!(drain_runtime_sse_frames(&mut buffer, "session-1")
            .expect("partial frame")
            .is_empty());
        buffer.extend_from_slice(
            b"lo\",\"sequence\":4}\n\nevent: turn_completed\ndata: {\"sequence\":5}\n\n",
        );
        let frames = drain_runtime_sse_frames(&mut buffer, "session-1").expect("frames");
        assert_eq!(frames.len(), 2);
        assert_eq!(frames[0].event, "token");
        assert_eq!(frames[0].id, "4");
        assert_eq!(frames[0].data["text"], "hello");
        assert_eq!(frames[1].event, "turn_completed");
        assert!(buffer.is_empty());
    }
}
