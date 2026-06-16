use std::collections::HashSet;

use anyhow::{anyhow, Result};
use axum::http::Method;
use base64::prelude::{Engine as _, BASE64_URL_SAFE_NO_PAD};
use chrono::Utc;
use hmac::{Hmac, Mac};
use serde::Deserialize;
use sha2::Sha256;

type HmacSha256 = Hmac<Sha256>;

const JWT_AUDIENCE: &str = "hivy-runtime";

#[derive(Debug, Clone)]
pub struct BrowserClaims {
    pub session_id: String,
    pub sandbox_id: String,
    pub scopes: HashSet<String>,
}

#[derive(Debug, Deserialize)]
struct RawClaims {
    aud: Audience,
    exp: i64,
    #[serde(default)]
    nbf: Option<i64>,
    session_id: String,
    sandbox_id: String,
    scopes: Vec<String>,
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum Audience {
    One(String),
    Many(Vec<String>),
}

impl Audience {
    fn contains(&self, want: &str) -> bool {
        match self {
            Audience::One(value) => value == want,
            Audience::Many(values) => values.iter().any(|value| value == want),
        }
    }
}

pub fn validate_browser_jwt(token: &str, secret: &str) -> Result<BrowserClaims> {
    let mut parts = token.split('.');
    let header = parts.next().ok_or_else(|| anyhow!("missing jwt header"))?;
    let payload = parts.next().ok_or_else(|| anyhow!("missing jwt payload"))?;
    let signature = parts
        .next()
        .ok_or_else(|| anyhow!("missing jwt signature"))?;
    if parts.next().is_some() {
        return Err(anyhow!("too many jwt segments"));
    }

    let header_bytes = BASE64_URL_SAFE_NO_PAD.decode(header)?;
    let header_json: serde_json::Value = serde_json::from_slice(&header_bytes)?;
    if header_json.get("alg").and_then(|value| value.as_str()) != Some("HS256") {
        return Err(anyhow!("unsupported jwt alg"));
    }

    let signing_input = format!("{header}.{payload}");
    let mut mac = HmacSha256::new_from_slice(secret.as_bytes())?;
    mac.update(signing_input.as_bytes());
    let expected = mac.finalize().into_bytes();
    let provided = BASE64_URL_SAFE_NO_PAD.decode(signature)?;
    if provided.len() != expected.len() || !constant_time_eq(&provided, &expected) {
        return Err(anyhow!("invalid jwt signature"));
    }

    let payload_bytes = BASE64_URL_SAFE_NO_PAD.decode(payload)?;
    let claims: RawClaims = serde_json::from_slice(&payload_bytes)?;
    if !claims.aud.contains(JWT_AUDIENCE) {
        return Err(anyhow!("invalid jwt audience"));
    }
    let now = Utc::now().timestamp();
    if claims.exp <= now {
        return Err(anyhow!("jwt expired"));
    }
    if claims.nbf.is_some_and(|nbf| nbf > now) {
        return Err(anyhow!("jwt not yet valid"));
    }
    if claims.session_id.trim().is_empty() || claims.sandbox_id.trim().is_empty() {
        return Err(anyhow!("missing runtime claims"));
    }
    Ok(BrowserClaims {
        session_id: claims.session_id,
        sandbox_id: claims.sandbox_id,
        scopes: claims.scopes.into_iter().collect(),
    })
}

pub fn required_browser_scope(method: &Method, path: &str) -> Option<&'static str> {
    if *method == Method::GET && is_session_stream_path(path) {
        return Some("stream:read");
    }
    if *method == Method::GET && is_repo_path(path) {
        return Some("repo:read");
    }
    None
}

fn is_session_stream_path(path: &str) -> bool {
    let parts: Vec<&str> = path.trim_matches('/').split('/').collect();
    matches!(
        parts.as_slice(),
        ["sessions", session_id, "stream"] if !session_id.is_empty()
    ) || matches!(
        parts.as_slice(),
        ["sessions", session_id, "streams", stream_id]
            if !session_id.is_empty() && !stream_id.is_empty()
    )
}

fn is_repo_path(path: &str) -> bool {
    let parts: Vec<&str> = path.trim_matches('/').split('/').collect();
    matches!(parts.as_slice(), ["repos"])
        || matches!(parts.as_slice(), ["repos", repo_id, tail @ ..] if !repo_id.is_empty() && !tail.is_empty())
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
    use base64::prelude::BASE64_URL_SAFE_NO_PAD;
    use serde_json::json;

    fn sign(payload: serde_json::Value, secret: &str) -> String {
        let header = BASE64_URL_SAFE_NO_PAD.encode(br#"{"alg":"HS256","typ":"JWT"}"#);
        let payload = BASE64_URL_SAFE_NO_PAD.encode(serde_json::to_vec(&payload).unwrap());
        let input = format!("{header}.{payload}");
        let mut mac = HmacSha256::new_from_slice(secret.as_bytes()).unwrap();
        mac.update(input.as_bytes());
        let sig = BASE64_URL_SAFE_NO_PAD.encode(mac.finalize().into_bytes());
        format!("{input}.{sig}")
    }

    #[test]
    fn validates_scoped_browser_jwt() {
        let token = sign(
            json!({
                "aud": "hivy-runtime",
                "exp": Utc::now().timestamp() + 60,
                "session_id": "s1",
                "sandbox_id": "sb1",
                "scopes": ["stream:read", "repo:read"]
            }),
            "secret",
        );
        let claims = validate_browser_jwt(&token, "secret").unwrap();
        assert_eq!(claims.session_id, "s1");
        assert!(claims.scopes.contains("repo:read"));
    }

    #[test]
    fn rejects_expired_jwt() {
        let token = sign(
            json!({
                "aud": "hivy-runtime",
                "exp": Utc::now().timestamp() - 1,
                "session_id": "s1",
                "sandbox_id": "sb1",
                "scopes": ["repo:read"]
            }),
            "secret",
        );
        assert!(validate_browser_jwt(&token, "secret").is_err());
    }

    #[test]
    fn maps_safe_routes_to_scopes() {
        assert_eq!(
            required_browser_scope(&Method::GET, "/sessions/s1/stream"),
            Some("stream:read")
        );
        assert_eq!(
            required_browser_scope(&Method::GET, "/repos"),
            Some("repo:read")
        );
        assert_eq!(
            required_browser_scope(&Method::POST, "/sessions/s1/messages"),
            None
        );
    }
}
