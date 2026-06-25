use std::collections::HashMap;
use std::path::Path;

use anyhow::{anyhow, Context, Result};
use reqwest::header::CONTENT_TYPE;
use serde::Deserialize;
use serde_json::{json, Value};

use crate::file_read_snapshot;

const ENV_CONTROL_PLANE_URL: &str = "HIVY_CONTROL_PLANE_URL";
const ENV_DRIVE_UPLOAD_BEARER: &str = "HIVY_DRIVE_UPLOAD_BEARER";
const ENV_DRIVE_UPLOAD_URL: &str = "HIVY_DRIVE_UPLOAD_URL";
const ENV_AGENT_ID: &str = "HIVY_AGENT_ID";
const ENV_RUNTIME_SECRET: &str = "HIVY_RUNTIME_SECRET";

#[derive(Debug, Deserialize)]
struct UploadResponse {
    id: String,
    asset_url: String,
    filename: String,
    content_type: String,
    bytes: i64,
}

#[derive(Debug, Deserialize)]
struct DescribeResponse {
    drive_asset_id: String,
    asset_url: String,
    filename: String,
    content_type: String,
    category: String,
    confidence: f64,
    analysis: Value,
    rendered_description: String,
}

pub async fn describe_image_file(
    runtime_env: &HashMap<String, String>,
    path: &Path,
    mime_type: &str,
    bytes: &[u8],
) -> Result<Value> {
    let client = reqwest::Client::new();
    let upload = upload_image(&client, runtime_env, path, mime_type, bytes).await?;
    let described = describe_uploaded_image(&client, runtime_env, &upload.id).await?;
    Ok(json!({
        "path": path.display().to_string(),
        "mime_type": mime_type,
        "content": described.rendered_description,
        "description": described.rendered_description,
        "drive_asset_id": described.drive_asset_id,
        "asset_url": described.asset_url,
        "filename": described.filename,
        "content_type": described.content_type,
        "category": described.category,
        "confidence": described.confidence,
        "analysis": described.analysis,
        "uploaded_asset": {
            "drive_asset_id": upload.id,
            "asset_url": upload.asset_url,
            "filename": upload.filename,
            "content_type": upload.content_type,
            "bytes": upload.bytes,
        },
        "truncated": false,
        "truncated_by": "none",
    }))
}

async fn upload_image(
    client: &reqwest::Client,
    runtime_env: &HashMap<String, String>,
    path: &Path,
    mime_type: &str,
    bytes: &[u8],
) -> Result<UploadResponse> {
    let base_url = required_env(runtime_env, ENV_DRIVE_UPLOAD_URL)?;
    let bearer = required_env(runtime_env, ENV_DRIVE_UPLOAD_BEARER)?;
    let upload_url = format!(
        "{}/read-file-images/{}",
        base_url.trim_end_matches('/'),
        upload_filename(path, bytes)
    );
    let response = client
        .put(upload_url)
        .bearer_auth(bearer)
        .header(CONTENT_TYPE, mime_type)
        .body(bytes.to_vec())
        .send()
        .await
        .context("upload image for description")?;
    parse_json_response(response, "upload image for description").await
}

async fn describe_uploaded_image(
    client: &reqwest::Client,
    runtime_env: &HashMap<String, String>,
    asset_id: &str,
) -> Result<DescribeResponse> {
    let control_plane = required_env(runtime_env, ENV_CONTROL_PLANE_URL)?;
    let agent_id = required_env(runtime_env, ENV_AGENT_ID)?;
    let secret = required_env(runtime_env, ENV_RUNTIME_SECRET)?;
    let url = format!(
        "{}/internal/agents/{}/images/describe",
        control_plane.trim_end_matches('/'),
        agent_id
    );
    let response = client
        .post(url)
        .bearer_auth(secret)
        .json(&json!({
            "drive_asset_id": asset_id,
            "detail_level": "high",
        }))
        .send()
        .await
        .context("describe uploaded image")?;
    parse_json_response(response, "describe uploaded image").await
}

async fn parse_json_response<T: for<'de> Deserialize<'de>>(
    response: reqwest::Response,
    operation: &str,
) -> Result<T> {
    let status = response.status();
    let body = response
        .text()
        .await
        .with_context(|| format!("{operation}: read response body"))?;
    if !status.is_success() {
        return Err(anyhow!("{operation}: status {status}: {body}"));
    }
    serde_json::from_str(&body).with_context(|| format!("{operation}: decode response"))
}

fn required_env<'a>(runtime_env: &'a HashMap<String, String>, key: &str) -> Result<&'a str> {
    runtime_env
        .get(key)
        .map(|value| value.trim())
        .filter(|value| !value.is_empty())
        .ok_or_else(|| anyhow!("{key} is required to describe image files"))
}

fn upload_filename(path: &Path, bytes: &[u8]) -> String {
    let raw_name = path
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("image");
    let safe_name = sanitize_filename(raw_name);
    format!("{:016x}-{safe_name}", file_read_snapshot(bytes).hash)
}

fn sanitize_filename(raw: &str) -> String {
    let sanitized: String = raw
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || matches!(ch, '.' | '-' | '_') {
                ch
            } else {
                '_'
            }
        })
        .collect();
    let trimmed = sanitized.trim_matches('_');
    if trimmed.is_empty() {
        "image".to_string()
    } else {
        trimmed.to_string()
    }
}
