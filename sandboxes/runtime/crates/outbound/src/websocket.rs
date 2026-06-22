use std::collections::HashMap;
use std::time::Duration;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::OutboundEvent;
use futures_util::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::net::TcpStream;
use tokio::sync::Mutex;
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::http::{HeaderName, HeaderValue};
use tokio_tungstenite::tungstenite::protocol::Message;
use tokio_tungstenite::{connect_async, MaybeTlsStream, WebSocketStream};
use tracing::warn;

use crate::webhook::filter_matches;
use crate::{OutboundChannel, OutboundError, Result};

const ACK_TIMEOUT: Duration = Duration::from_secs(15);

type WsStream = WebSocketStream<MaybeTlsStream<TcpStream>>;

pub struct WebSocketChannel {
    name: String,
    url: String,
    secret: String,
    extra_headers: HashMap<String, String>,
    event_filter: Option<Vec<String>>,
    connection: Mutex<Option<WsStream>>,
}

impl WebSocketChannel {
    pub fn new(
        name: impl Into<String>,
        url: impl Into<String>,
        secret: impl Into<String>,
        extra_headers: HashMap<String, String>,
        event_filter: Option<Vec<String>>,
    ) -> Result<Self> {
        Ok(Self {
            name: name.into(),
            url: url.into(),
            secret: secret.into(),
            extra_headers,
            event_filter,
            connection: Mutex::new(None),
        })
    }

    async fn connect(&self) -> Result<WsStream> {
        let mut request = self
            .url
            .as_str()
            .into_client_request()
            .map_err(|e| OutboundError::Delivery(format!("websocket request: {e}")))?;
        request.headers_mut().insert(
            "Authorization",
            HeaderValue::from_str(&format!("Bearer {}", self.secret))
                .map_err(|e| OutboundError::Delivery(format!("authorization header: {e}")))?,
        );
        for (name, value) in &self.extra_headers {
            let header_name = HeaderName::from_bytes(name.as_bytes())
                .map_err(|e| OutboundError::Delivery(format!("header name {name}: {e}")))?;
            request.headers_mut().insert(
                header_name,
                HeaderValue::from_str(value)
                    .map_err(|e| OutboundError::Delivery(format!("header {name}: {e}")))?,
            );
        }
        let (stream, _) = connect_async(request)
            .await
            .map_err(|e| OutboundError::Delivery(format!("connect {}: {e}", self.url)))?;
        Ok(stream)
    }
}

#[async_trait]
impl OutboundChannel for WebSocketChannel {
    fn name(&self) -> &str {
        &self.name
    }

    fn kind(&self) -> &'static str {
        "websocket"
    }

    fn accepts(&self, event_type: &str) -> bool {
        match self.event_filter.as_ref() {
            None => true,
            Some(filters) if filters.is_empty() => true,
            Some(filters) => filters.iter().any(|f| filter_matches(f, event_type)),
        }
    }

    async fn deliver(&self, event: &OutboundEvent) -> Result<()> {
        let frame = RuntimeStreamFrame::from_outbound(event)?;
        let runtime_seq = frame.event.runtime_seq;
        let body = serde_json::to_string(&frame)
            .map_err(|e| OutboundError::Delivery(format!("serialize websocket event: {e}")))?;

        let mut guard = self.connection.lock().await;
        let mut stream = match guard.take() {
            Some(stream) => stream,
            None => self.connect().await?,
        };
        if let Err(error) = stream.send(Message::Text(body.clone())).await {
            warn!(channel = %self.name, %error, "websocket send failed; reconnecting");
            stream = self.connect().await?;
            stream
                .send(Message::Text(body))
                .await
                .map_err(|e| OutboundError::Delivery(format!("send {}: {e}", self.url)))?;
        }

        let ack = match timeout(ACK_TIMEOUT, read_ack(&mut stream, runtime_seq)).await {
            Ok(result) => result,
            Err(_) => Err(OutboundError::Delivery(format!(
                "websocket ack timeout for runtime_seq {runtime_seq}"
            ))),
        };
        if ack.is_err() {
            *guard = None;
        } else {
            *guard = Some(stream);
        }
        ack
    }
}

async fn read_ack(stream: &mut WsStream, runtime_seq: i64) -> Result<()> {
    while let Some(message) = stream.next().await {
        let message = message.map_err(|e| OutboundError::Delivery(format!("read ack: {e}")))?;
        let Message::Text(text) = message else {
            continue;
        };
        let ack: RuntimeIngressAck = serde_json::from_str(&text)
            .map_err(|e| OutboundError::Delivery(format!("decode ack: {e}")))?;
        for result in ack.results {
            if result.runtime_seq == runtime_seq {
                if ack.r#type == "ack"
                    && (result.status == "accepted" || result.status == "duplicate")
                {
                    return Ok(());
                }
                return Err(OutboundError::Delivery(format!(
                    "runtime_seq {runtime_seq} rejected: {} {}",
                    result.status,
                    result.error.unwrap_or_default()
                )));
            }
        }
    }
    Err(OutboundError::Delivery(
        "websocket closed before ack".to_string(),
    ))
}

#[derive(Debug, Serialize)]
struct RuntimeStreamFrame {
    event: RuntimeStreamEvent,
}

#[derive(Debug, Serialize)]
struct RuntimeStreamEvent {
    session_id: String,
    sandbox_id: Option<String>,
    agent_id: Option<String>,
    org_id: Option<String>,
    turn_id: Option<String>,
    stream_id: Option<String>,
    runtime_seq: i64,
    event_id: Option<String>,
    event_type: String,
    durability: String,
    span_id: Option<String>,
    source: Option<String>,
    payload: Value,
    occurred_at: DateTime<Utc>,
}

impl RuntimeStreamFrame {
    fn from_outbound(event: &OutboundEvent) -> Result<Self> {
        let payload = event.payload.clone();
        let session_id = string_field(&payload, "session_id").ok_or_else(|| {
            OutboundError::Delivery("runtime event missing session_id".to_string())
        })?;
        let runtime_seq = integer_field(&payload, "runtime_seq").ok_or_else(|| {
            OutboundError::Delivery(format!(
                "runtime event for {session_id} missing runtime_seq"
            ))
        })?;
        Ok(Self {
            event: RuntimeStreamEvent {
                session_id,
                sandbox_id: string_field(&payload, "sandbox_id"),
                agent_id: string_field(&payload, "agent_id"),
                org_id: string_field(&payload, "org_id"),
                turn_id: string_field(&payload, "turn_id"),
                stream_id: string_field(&payload, "stream_id"),
                runtime_seq,
                event_id: string_field(&payload, "event_id"),
                event_type: event.event_type.clone(),
                durability: string_field(&payload, "durability")
                    .unwrap_or_else(|| "durable".to_string()),
                span_id: string_field(&payload, "span_id"),
                source: string_field(&payload, "source"),
                payload,
                occurred_at: event.at,
            },
        })
    }
}

#[derive(Debug, Deserialize)]
struct RuntimeIngressAck {
    #[serde(rename = "type")]
    r#type: String,
    results: Vec<RuntimeIngressAckResult>,
}

#[derive(Debug, Deserialize)]
struct RuntimeIngressAckResult {
    runtime_seq: i64,
    status: String,
    #[serde(default)]
    error: Option<String>,
}

fn string_field(payload: &Value, key: &str) -> Option<String> {
    payload
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
}

fn integer_field(payload: &Value, key: &str) -> Option<i64> {
    payload.get(key).and_then(|value| {
        value
            .as_i64()
            .or_else(|| value.as_u64().and_then(|number| i64::try_from(number).ok()))
    })
}
