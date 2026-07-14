//! Legacy MCP HTTP+SSE client transport.
//!
//! MCP replaced this transport with Streamable HTTP in protocol version
//! 2025-03-26, but existing servers still expose a GET event stream that
//! announces a separate POST endpoint. rmcp 1.x no longer ships that legacy
//! client, so this adapter implements the protocol over rmcp's Transport API.

use std::collections::HashMap;
use std::future::Future;

use futures::StreamExt;
use http::{HeaderName, HeaderValue};
use rmcp::service::{RxJsonRpcMessage, TxJsonRpcMessage};
use rmcp::transport::Transport;
use rmcp::RoleClient;
use sse_stream::SseStream;
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tracing::{debug, warn};

use crate::ssrf::PinnedHttpTarget;

#[derive(Debug, thiserror::Error)]
pub enum LegacySseError {
    #[error("invalid legacy MCP SSE URL: {0}")]
    InvalidUrl(#[from] url::ParseError),
    #[error("legacy MCP SSE request failed: {0}")]
    Request(#[from] reqwest::Error),
    #[error("legacy MCP SSE endpoint returned HTTP {status}: {body}")]
    Http { status: u16, body: String },
    #[error("legacy MCP SSE endpoint did not return text/event-stream")]
    InvalidContentType,
    #[error("legacy MCP SSE stream ended before announcing a POST endpoint")]
    MissingEndpoint,
    #[error("legacy MCP SSE server announced a cross-origin POST endpoint")]
    CrossOriginEndpoint,
}

pub struct LegacySseClientTransport {
    client: reqwest::Client,
    post_url: reqwest::Url,
    headers: reqwest::header::HeaderMap,
    receiver: mpsc::Receiver<RxJsonRpcMessage<RoleClient>>,
    reader_task: JoinHandle<()>,
}

impl LegacySseClientTransport {
    pub async fn connect(
        target: PinnedHttpTarget,
        headers: HashMap<HeaderName, HeaderValue>,
    ) -> Result<Self, LegacySseError> {
        let event_url = target.url;
        let headers: reqwest::header::HeaderMap = headers.into_iter().collect();
        let client = target.client;
        let response = client
            .get(event_url.clone())
            .headers(headers.clone())
            .header(reqwest::header::ACCEPT, "text/event-stream")
            .send()
            .await?;
        if !response.status().is_success() {
            let status = response.status().as_u16();
            let body = response.text().await.unwrap_or_default();
            return Err(LegacySseError::Http {
                status,
                body: truncate(&body, 512),
            });
        }
        let is_event_stream = response
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|value| {
                value
                    .split(';')
                    .next()
                    .is_some_and(|mime| mime.trim().eq_ignore_ascii_case("text/event-stream"))
            });
        if !is_event_stream {
            return Err(LegacySseError::InvalidContentType);
        }

        let mut events = Box::pin(SseStream::from_byte_stream(response.bytes_stream()));
        let post_url = loop {
            let Some(event) = events.next().await else {
                return Err(LegacySseError::MissingEndpoint);
            };
            let event = event.map_err(|error| LegacySseError::Http {
                status: 200,
                body: truncate(&error.to_string(), 512),
            })?;
            if event.event.as_deref() != Some("endpoint") {
                continue;
            }
            let Some(endpoint) = event.data.as_deref() else {
                continue;
            };
            let post_url = event_url.join(endpoint)?;
            if !same_origin(&event_url, &post_url) {
                return Err(LegacySseError::CrossOriginEndpoint);
            }
            break post_url;
        };

        let (sender, receiver) = mpsc::channel(64);
        let reader_task = tokio::spawn(async move {
            while let Some(event) = events.next().await {
                let event = match event {
                    Ok(event) => event,
                    Err(read_error) => {
                        warn!(error = %read_error, "legacy MCP SSE stream failed");
                        break;
                    }
                };
                match event.event.as_deref() {
                    Some("endpoint") => {
                        debug!("legacy MCP SSE server re-announced its endpoint");
                    }
                    Some("message") | None => {
                        let Some(data) = event.data else {
                            continue;
                        };
                        match serde_json::from_str::<RxJsonRpcMessage<RoleClient>>(&data) {
                            Ok(message) => {
                                if sender.send(message).await.is_err() {
                                    break;
                                }
                            }
                            Err(parse_error) => {
                                warn!(error = %parse_error, "ignored malformed legacy MCP SSE message");
                            }
                        }
                    }
                    Some(event_name) => {
                        debug!(event = event_name, "ignored unknown legacy MCP SSE event");
                    }
                }
            }
        });

        Ok(Self {
            client,
            post_url,
            headers,
            receiver,
            reader_task,
        })
    }
}

impl Transport<RoleClient> for LegacySseClientTransport {
    type Error = LegacySseError;

    fn send(
        &mut self,
        item: TxJsonRpcMessage<RoleClient>,
    ) -> impl Future<Output = Result<(), Self::Error>> + Send + 'static {
        let client = self.client.clone();
        let post_url = self.post_url.clone();
        let headers = self.headers.clone();
        async move {
            let response = client
                .post(post_url)
                .headers(headers)
                .header(reqwest::header::CONTENT_TYPE, "application/json")
                .json(&item)
                .send()
                .await?;
            if response.status().is_success() {
                return Ok(());
            }
            let status = response.status().as_u16();
            let body = response.text().await.unwrap_or_default();
            Err(LegacySseError::Http {
                status,
                body: truncate(&body, 512),
            })
        }
    }

    fn receive(&mut self) -> impl Future<Output = Option<RxJsonRpcMessage<RoleClient>>> + Send {
        self.receiver.recv()
    }

    async fn close(&mut self) -> Result<(), Self::Error> {
        self.reader_task.abort();
        Ok(())
    }
}

fn same_origin(left: &reqwest::Url, right: &reqwest::Url) -> bool {
    left.scheme() == right.scheme()
        && left.host_str() == right.host_str()
        && left.port_or_known_default() == right.port_or_known_default()
}

fn truncate(value: &str, max_chars: usize) -> String {
    value.chars().take(max_chars).collect()
}

#[cfg(test)]
mod tests {
    use super::same_origin;

    #[test]
    fn endpoint_origin_must_match_event_stream() {
        let base = reqwest::Url::parse("https://mcp.example.test/sse").unwrap();
        assert!(same_origin(
            &base,
            &reqwest::Url::parse("https://mcp.example.test/messages?id=1").unwrap()
        ));
        assert!(!same_origin(
            &base,
            &reqwest::Url::parse("https://evil.example.test/messages?id=1").unwrap()
        ));
    }
}
