use axum::{
    extract::State,
    http::{header::AUTHORIZATION, HeaderName, Method, Request, StatusCode},
    middleware::Next,
    response::Response,
};

use crate::state::ApiState;

const STREAM_TOKEN_HEADER: HeaderName = HeaderName::from_static("x-hivy-stream-token");

pub async fn bearer_auth(
    State(state): State<ApiState>,
    request: Request<axum::body::Body>,
    next: Next,
) -> Result<Response, StatusCode> {
    let method = request.method().clone();
    let path = request.uri().path().to_string();
    if path == "/healthz" {
        return Ok(next.run(request).await);
    }
    if method == Method::OPTIONS {
        return Ok(next.run(request).await);
    }
    let stream_token = stream_token_from_query(request.uri())
        .or_else(|| stream_token_from_header(&request))
        .map(ToString::to_string);
    if is_stream_token_route(&method, &path)
        && stream_token_authorized(&state, stream_token.as_deref()).await
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

fn is_stream_token_route(method: &Method, path: &str) -> bool {
    (*method == Method::GET && is_session_stream_path(path))
        || (*method == Method::POST && is_question_answer_path(path))
}

fn is_session_stream_path(path: &str) -> bool {
    let parts: Vec<&str> = path.trim_matches('/').split('/').collect();
    matches!(
        parts.as_slice(),
        ["sessions", session_id, "streams", stream_id]
            if !session_id.is_empty() && !stream_id.is_empty()
    )
}

fn is_question_answer_path(path: &str) -> bool {
    let parts: Vec<&str> = path.trim_matches('/').split('/').collect();
    matches!(
        parts.as_slice(),
        ["sessions", session_id, "questions", question_request_id, "answer"]
            if !session_id.is_empty() && !question_request_id.is_empty()
    )
}

async fn stream_token_authorized(state: &ApiState, provided: Option<&str>) -> bool {
    let Some(provided) = provided else {
        return false;
    };
    let token = state.stream_token.read().await;
    let Some(expected) = token.as_ref() else {
        return false;
    };
    constant_time_eq(provided.as_bytes(), expected.as_bytes())
}

fn stream_token_from_header(request: &Request<axum::body::Body>) -> Option<&str> {
    request
        .headers()
        .get(STREAM_TOKEN_HEADER)
        .and_then(|value| value.to_str().ok())
        .filter(|value| !value.is_empty())
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stream_token_routes_are_limited_to_streams_and_question_answers() {
        assert!(is_stream_token_route(
            &Method::GET,
            "/sessions/s1/streams/stream-1"
        ));
        assert!(is_stream_token_route(
            &Method::POST,
            "/sessions/s1/questions/question-request-1/answer"
        ));
        assert!(!is_stream_token_route(
            &Method::POST,
            "/sessions/s1/messages"
        ));
        assert!(!is_stream_token_route(&Method::GET, "/config"));
    }
}
