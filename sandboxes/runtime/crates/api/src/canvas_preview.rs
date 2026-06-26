use std::path::{Component, Path as FsPath, PathBuf};

use axum::{
    body::Body,
    extract::{Path, Query, State},
    http::{
        header::{AUTHORIZATION, CONTENT_TYPE},
        HeaderMap, HeaderValue, StatusCode,
    },
    response::Response,
};
use serde::Deserialize;

use crate::browser_auth::{validate_browser_jwt, BrowserClaims};
use crate::state::ApiState;

#[derive(Debug, Default, Deserialize)]
pub struct CanvasPreviewQuery {
    #[serde(default)]
    token: Option<String>,
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/canvas/preview/{path}",
    params(
        ("path" = String, Path, description = "Canvas file path relative to /workspace/canvas"),
        ("token" = Option<String>, Query, description = "Read-only browser token for iframe previews")
    ),
    responses(
        (status = 200, description = "Canvas artifact file"),
        (status = 400, description = "Invalid path"),
        (status = 401, description = "Unauthorized"),
        (status = 404, description = "File not found")
    ),
    security(("bearer" = []))
))]
pub async fn preview_canvas_file(
    State(state): State<ApiState>,
    Path(path): Path<String>,
    Query(query): Query<CanvasPreviewQuery>,
    headers: HeaderMap,
) -> Result<Response, (StatusCode, String)> {
    if !preview_authorized(&state, &headers, &query).await {
        return Err((StatusCode::UNAUTHORIZED, "unauthorized".to_string()));
    }
    let path = resolve_preview_path(&state.workspace_root, &path).await?;
    let bytes = tokio::fs::read(&path)
        .await
        .map_err(|_| (StatusCode::NOT_FOUND, "file not found".to_string()))?;
    let mut response = Response::new(Body::from(bytes));
    response.headers_mut().insert(
        CONTENT_TYPE,
        HeaderValue::from_static(content_type_for_path(&path)),
    );
    Ok(response)
}

async fn preview_authorized(
    state: &ApiState,
    headers: &HeaderMap,
    query: &CanvasPreviewQuery,
) -> bool {
    let Some(header) = headers
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
    else {
        return authorize_browser_query_token(state, query).await;
    };
    let secret = state.bearer_token.read().await.clone();
    let expected = format!("Bearer {secret}");
    if constant_time_eq(header.as_bytes(), expected.as_bytes()) {
        return true;
    }
    if let Some(token) = header.strip_prefix("Bearer ") {
        return authorize_browser_token(state, token, &secret).await;
    }
    authorize_browser_query_token(state, query).await
}

async fn authorize_browser_query_token(state: &ApiState, query: &CanvasPreviewQuery) -> bool {
    let Some(token) = query
        .token
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return false;
    };
    let secret = state.bearer_token.read().await.clone();
    authorize_browser_token(state, token, &secret).await
}

async fn authorize_browser_token(state: &ApiState, token: &str, secret: &str) -> bool {
    let Ok(claims) = validate_browser_jwt(token, secret) else {
        return false;
    };
    if !browser_claims_authorized(state, &claims) {
        return false;
    }
    state
        .canvas_sessions
        .write()
        .await
        .insert(claims.session_id.clone());
    true
}

fn browser_claims_authorized(state: &ApiState, claims: &BrowserClaims) -> bool {
    if !claims.scopes.contains("sandbox:read") {
        return false;
    }
    let env = state.config_store.runtime_env();
    match env
        .get("HIVY_SANDBOX_ID")
        .filter(|value| !value.trim().is_empty())
    {
        Some(expected) => expected == &claims.sandbox_id,
        None => true,
    }
}

async fn resolve_preview_path(
    workspace_root: &FsPath,
    requested: &str,
) -> Result<PathBuf, (StatusCode, String)> {
    let rel = clean_relative_path(requested)?;
    if rel.as_os_str().is_empty() {
        return Err((StatusCode::BAD_REQUEST, "path is required".to_string()));
    }
    let canvas_root = workspace_root.join("canvas");
    let root = tokio::fs::canonicalize(&canvas_root)
        .await
        .map_err(|_| (StatusCode::NOT_FOUND, "canvas root not found".to_string()))?;
    let mut path = root.join(rel);
    if tokio::fs::metadata(&path)
        .await
        .is_ok_and(|metadata| metadata.is_dir())
    {
        path = path.join("index.html");
    }
    let path = tokio::fs::canonicalize(&path)
        .await
        .map_err(|_| (StatusCode::NOT_FOUND, "file not found".to_string()))?;
    if !path.starts_with(root) {
        return Err((StatusCode::BAD_REQUEST, "invalid path".to_string()));
    }
    let metadata = tokio::fs::metadata(&path)
        .await
        .map_err(|_| (StatusCode::NOT_FOUND, "file not found".to_string()))?;
    if !metadata.is_file() {
        return Err((StatusCode::NOT_FOUND, "file not found".to_string()));
    }
    Ok(path)
}

fn clean_relative_path(raw: &str) -> Result<PathBuf, (StatusCode, String)> {
    let trimmed = raw.trim_matches('/');
    let mut out = PathBuf::new();
    for component in FsPath::new(trimmed).components() {
        match component {
            Component::Normal(part) => out.push(part),
            Component::CurDir => {}
            _ => return Err((StatusCode::BAD_REQUEST, "invalid path".to_string())),
        }
    }
    Ok(out)
}

fn content_type_for_path(path: &FsPath) -> &'static str {
    match path
        .extension()
        .and_then(|value| value.to_str())
        .unwrap_or_default()
        .to_ascii_lowercase()
        .as_str()
    {
        "html" | "htm" => "text/html; charset=utf-8",
        "css" => "text/css; charset=utf-8",
        "js" | "mjs" => "text/javascript; charset=utf-8",
        "json" => "application/json; charset=utf-8",
        "svg" => "image/svg+xml",
        "png" => "image/png",
        "jpg" | "jpeg" => "image/jpeg",
        "gif" => "image/gif",
        "webp" => "image/webp",
        "ico" => "image/x-icon",
        "txt" => "text/plain; charset=utf-8",
        _ => "application/octet-stream",
    }
}

fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    let mut diff: u8 = 0;
    for (left, right) in a.iter().zip(b.iter()) {
        diff |= left ^ right;
    }
    diff == 0
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn unique_temp_dir() -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "hivy-canvas-preview-test-{}-{}",
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
    async fn resolve_preview_path_serves_files_inside_canvas_root() {
        let root = unique_temp_dir();
        let file = root.join("canvas/projects/homepage/artifacts/variant-a/index.html");
        tokio::fs::create_dir_all(file.parent().unwrap())
            .await
            .unwrap();
        tokio::fs::write(&file, "<html></html>").await.unwrap();

        let resolved =
            resolve_preview_path(&root, "projects/homepage/artifacts/variant-a/index.html")
                .await
                .unwrap();

        assert_eq!(resolved, tokio::fs::canonicalize(&file).await.unwrap());
        let _ = std::fs::remove_dir_all(root);
    }

    #[tokio::test]
    async fn resolve_preview_path_rejects_traversal() {
        let root = unique_temp_dir();
        tokio::fs::create_dir_all(root.join("canvas"))
            .await
            .unwrap();

        let result = resolve_preview_path(&root, "../secret.txt").await;

        assert_eq!(result.unwrap_err().0, StatusCode::BAD_REQUEST);
        let _ = std::fs::remove_dir_all(root);
    }

    #[test]
    fn content_type_matches_web_artifact_files() {
        assert_eq!(
            content_type_for_path(FsPath::new("index.html")),
            "text/html; charset=utf-8"
        );
        assert_eq!(
            content_type_for_path(FsPath::new("app.js")),
            "text/javascript; charset=utf-8"
        );
    }
}
