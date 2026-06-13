use axum::{
    extract::State,
    http::{header::AUTHORIZATION, Method, Request, StatusCode},
    middleware::Next,
    response::Response,
};

use crate::state::ApiState;

pub async fn bearer_auth(
    State(state): State<ApiState>,
    request: Request<axum::body::Body>,
    next: Next,
) -> Result<Response, StatusCode> {
    let path = request.uri().path();
    if path == "/healthz" {
        return Ok(next.run(request).await);
    }
    if is_stream_get(request.method(), path) && stream_token_authorized(&state, request.uri()).await
    {
        return Ok(next.run(request).await);
    }
    let authorization_header = request
        .headers()
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .unwrap_or("");
    let authorized = {
        let token = state.bearer_token.read().await;
        let expected = format!("Bearer {}", token.as_str());
        constant_time_eq(authorization_header.as_bytes(), expected.as_bytes())
    };
    if !authorized {
        return Err(StatusCode::UNAUTHORIZED);
    }
    Ok(next.run(request).await)
}

fn is_stream_get(method: &Method, path: &str) -> bool {
    *method == Method::GET && is_session_stream_path(path)
}

fn is_session_stream_path(path: &str) -> bool {
    let parts: Vec<&str> = path.trim_matches('/').split('/').collect();
    matches!(
        parts.as_slice(),
        ["sessions", session_id, "streams", stream_id]
            if !session_id.is_empty() && !stream_id.is_empty()
    )
}

async fn stream_token_authorized(state: &ApiState, uri: &http::Uri) -> bool {
    let Some(provided) = stream_token_from_query(uri) else {
        return false;
    };
    let token = state.stream_token.read().await;
    let Some(expected) = token.as_ref() else {
        return false;
    };
    constant_time_eq(provided.as_bytes(), expected.as_bytes())
}

fn stream_token_from_query(uri: &http::Uri) -> Option<&str> {
    uri.query()?.split('&').find_map(|part| {
        let (key, value) = part.split_once('=')?;
        if key == "stream_token" && !value.is_empty() {
            Some(value)
        } else {
            None
        }
    })
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
