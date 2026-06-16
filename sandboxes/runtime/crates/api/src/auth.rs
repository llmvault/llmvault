use axum::{
    extract::State,
    http::{header::AUTHORIZATION, Method, Request, StatusCode},
    middleware::Next,
    response::Response,
};

use crate::browser_auth::{required_browser_scope, validate_browser_jwt, BrowserClaims};
use crate::state::ApiState;

pub async fn bearer_auth(
    State(state): State<ApiState>,
    request: Request<axum::body::Body>,
    next: Next,
) -> Result<Response, StatusCode> {
    let method = request.method().clone();
    let path = request.uri().path().to_string();
    if path == "/healthz" || method == Method::OPTIONS {
        return Ok(next.run(request).await);
    }

    let authorization_header = request
        .headers()
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .unwrap_or("");

    let runtime_secret_authorized = {
        let token = state.bearer_token.read().await;
        let expected = format!("Bearer {}", token.as_str());
        constant_time_eq(authorization_header.as_bytes(), expected.as_bytes())
    };
    if runtime_secret_authorized {
        return Ok(next.run(request).await);
    }

    if let Some(scope) = required_browser_scope(&method, &path) {
        if let Some(token) = authorization_header.strip_prefix("Bearer ") {
            let secret = state.bearer_token.read().await.clone();
            if validate_browser_jwt(token, &secret).is_ok_and(|claims| {
                browser_claims_authorized(&state, &method, &path, scope, &claims)
            }) {
                return Ok(next.run(request).await);
            }
        }
    }

    Err(StatusCode::UNAUTHORIZED)
}

fn browser_claims_authorized(
    state: &ApiState,
    method: &Method,
    path: &str,
    scope: &str,
    claims: &BrowserClaims,
) -> bool {
    if !claims.scopes.contains(scope) {
        return false;
    }
    if let Some(expected_session) = session_id_from_stream_path(method, path) {
        if expected_session != claims.session_id {
            return false;
        }
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

fn session_id_from_stream_path<'a>(method: &Method, path: &'a str) -> Option<&'a str> {
    if *method != Method::GET {
        return None;
    }
    let parts: Vec<&str> = path.trim_matches('/').split('/').collect();
    match parts.as_slice() {
        ["sessions", session_id, "stream"] if !session_id.is_empty() => Some(session_id),
        ["sessions", session_id, "streams", stream_id]
            if !session_id.is_empty() && !stream_id.is_empty() =>
        {
            Some(session_id)
        }
        _ => None,
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

    #[test]
    fn browser_scopes_do_not_cover_control_plane_routes() {
        assert_eq!(
            required_browser_scope(&Method::POST, "/sessions/s1/messages"),
            None
        );
        assert_eq!(
            required_browser_scope(&Method::POST, "/sessions/s1/questions/q1/answer"),
            None
        );
        assert_eq!(required_browser_scope(&Method::GET, "/config"), None);
    }
}
