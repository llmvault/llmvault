use std::collections::HashMap;
use std::io::Write;
use std::sync::Arc;
use std::time::Duration;

use chrono::{DateTime, Utc};
use domain::{OutboundChannelKind, OutboundChannelSpec, OutboundEvent};
use flate2::write::GzEncoder;
use flate2::Compression;
use reqwest::Client as HttpClient;
use serde_json::{json, Value};
use tokio::sync::Mutex;
use tracing::warn;

use crate::webhook::{compute_signature, filter_matches};
use crate::{DatabaseEventQueue, OutboundError, Result};

const HTTP_TIMEOUT_SECONDS: u64 = 30;
const MAX_BATCH_EVENTS: usize = 1000;
const MAX_BATCH_BYTES: usize = 5 * 1024 * 1024;
const MAX_COALESCED_TEXT_BYTES: usize = 128 * 1024;
const MAX_COALESCED_DELTAS: u64 = 1500;

pub struct StreamBatcher {
    sinks: Vec<BatchWebhookSink>,
    state: Mutex<BatchState>,
    http: HttpClient,
    /// Durable fallback for batches that fail every sink delivery. Failed spans
    /// are requeued here so the SQLite outbox persists them (and Go re-derives
    /// the span) instead of being silently dropped from persistence.
    requeue: Option<Arc<DatabaseEventQueue>>,
}

#[derive(Clone)]
struct BatchWebhookSink {
    name: String,
    url: String,
    secret: String,
    extra_headers: HashMap<String, String>,
    event_filter: Option<Vec<String>>,
}

#[derive(Default)]
struct BatchState {
    coalescing: HashMap<String, CoalescedStream>,
    pending: Vec<OutboundEvent>,
    pending_bytes: usize,
}

struct CoalescedStream {
    event_type: String,
    session_id: String,
    source: String,
    stream_id: Option<String>,
    trace_id: Option<String>,
    turn_id: Option<String>,
    scope: String,
    subagent: Option<Value>,
    sequence_start: u64,
    sequence_end: u64,
    delta_count: u64,
    text: String,
    started_at: DateTime<Utc>,
    ended_at: DateTime<Utc>,
}

impl StreamBatcher {
    pub fn from_specs(
        specs: &[OutboundChannelSpec],
        runtime_env: &HashMap<String, String>,
    ) -> Result<Option<Arc<Self>>> {
        let mut sinks = Vec::new();
        for spec in specs {
            let OutboundChannelKind::Webhook {
                url,
                secret_env,
                extra_headers,
            } = &spec.kind;
            let secret = match runtime_env.get(secret_env) {
                Some(secret) => secret.clone(),
                None => {
                    warn!(
                        name = %spec.name,
                        "skipping stream batch webhook because secret env var is not set"
                    );
                    continue;
                }
            };
            sinks.push(BatchWebhookSink {
                name: spec.name.clone(),
                url: batch_url(url),
                secret,
                extra_headers: extra_headers.clone(),
                event_filter: spec.event_filter.clone(),
            });
        }
        if sinks.is_empty() {
            return Ok(None);
        }
        let http = HttpClient::builder()
            .timeout(Duration::from_secs(HTTP_TIMEOUT_SECONDS))
            .build()
            .map_err(|e| OutboundError::Delivery(format!("http client: {e}")))?;
        Ok(Some(Arc::new(Self {
            sinks,
            state: Mutex::new(BatchState::default()),
            http,
            requeue: None,
        })))
    }

    /// Attach a durable queue that failed batches are requeued into instead of
    /// being dropped. Returns a new `StreamBatcher` sharing the same sinks.
    pub fn with_requeue(self: Arc<Self>, requeue: Arc<DatabaseEventQueue>) -> Arc<Self> {
        Arc::new(Self {
            sinks: self.sinks.clone(),
            state: Mutex::new(BatchState::default()),
            http: self.http.clone(),
            requeue: Some(requeue),
        })
    }

    pub async fn emit(&self, event: OutboundEvent) -> Result<bool> {
        if !is_stream_delta(&event.event_type) {
            return Ok(false);
        }
        if !self
            .sinks
            .iter()
            .any(|sink| sink.accepts(&event.event_type))
        {
            return Ok(false);
        }
        let send_now = is_coalesced_stream_delta_event(&event);
        let mut state = self.state.lock().await;
        if send_now {
            if let Some(session_id) = string_field(&event.payload, "session_id") {
                state.finish_session(&session_id);
            }
            state.push_event(event);
        } else {
            state.add_stream_delta(event);
        }
        let mut events = Vec::new();
        if send_now
            || state.pending.len() >= MAX_BATCH_EVENTS
            || state.pending_bytes >= MAX_BATCH_BYTES
        {
            state.finish_all();
            events = state.drain_pending();
        }
        drop(state);
        self.spawn_send(events);
        Ok(true)
    }

    pub async fn flush_before_event(&self, event: &OutboundEvent) -> Result<()> {
        if is_stream_delta_event(event) {
            return Ok(());
        }
        if string_field(&event.payload, "session_id").is_none() {
            return Ok(());
        }
        let mut state = self.state.lock().await;
        state.finish_streams_before_event(event);
        if !state.pending.is_empty() {
            self.flush_locked(&mut state).await?;
        }
        Ok(())
    }

    pub async fn flush_before_event_background(&self, event: &OutboundEvent) {
        if is_stream_delta_event(event) || string_field(&event.payload, "session_id").is_none() {
            return;
        }
        let mut state = self.state.lock().await;
        state.finish_streams_before_event(event);
        let events = state.drain_pending();
        drop(state);
        self.spawn_send(events);
    }

    pub async fn flush_session(&self, session_id: &str) -> Result<()> {
        let mut state = self.state.lock().await;
        state.finish_session(session_id);
        if !state.pending.is_empty() {
            self.flush_locked(&mut state).await?;
        }
        Ok(())
    }

    pub async fn flush_session_background(&self, session_id: &str) {
        let mut state = self.state.lock().await;
        state.finish_session(session_id);
        let events = state.drain_pending();
        drop(state);
        self.spawn_send(events);
    }

    async fn flush_locked(&self, state: &mut BatchState) -> Result<()> {
        state.finish_all();
        let events = state.drain_pending();
        send_batch_events(&self.http, &self.sinks, self.requeue.as_ref(), events).await
    }

    fn spawn_send(&self, events: Vec<OutboundEvent>) {
        if events.is_empty() {
            return;
        }
        let http = self.http.clone();
        let sinks = self.sinks.clone();
        let requeue = self.requeue.clone();
        tokio::spawn(async move {
            if let Err(error) = send_batch_events(&http, &sinks, requeue.as_ref(), events).await {
                warn!(%error, "stream batch background flush failed");
            }
        });
    }
}

/// Deliver a drained batch to every sink, serialized one sink at a time.
///
/// A failure on one sink never aborts delivery to the remaining sinks. Events
/// that no sink accepted-and-delivered successfully are requeued into the
/// durable database queue (when configured) so they are persisted rather than
/// dropped; otherwise the aggregate error is returned to the caller.
async fn send_batch_events(
    http: &HttpClient,
    sinks: &[BatchWebhookSink],
    requeue: Option<&Arc<DatabaseEventQueue>>,
    events: Vec<OutboundEvent>,
) -> Result<()> {
    if events.is_empty() {
        return Ok(());
    }
    // Track, per event index, whether at least one accepting sink delivered it.
    // An event with no accepting sink is considered handled (nothing to requeue).
    let mut delivered = vec![true; events.len()];
    for (index, event) in events.iter().enumerate() {
        if sinks.iter().any(|sink| sink.accepts(&event.event_type)) {
            delivered[index] = false;
        }
    }

    let mut errors: Vec<String> = Vec::new();
    for sink in sinks {
        let accepted_indices = events
            .iter()
            .enumerate()
            .filter(|(_, event)| sink.accepts(&event.event_type))
            .map(|(index, _)| index)
            .collect::<Vec<_>>();
        if accepted_indices.is_empty() {
            continue;
        }
        match send_to_sink(http, sink, &accepted_indices, &events).await {
            Ok(()) => {
                for index in accepted_indices {
                    delivered[index] = true;
                }
            }
            Err(error) => {
                warn!(channel = %sink.name, %error, "stream batch sink delivery failed");
                errors.push(error.to_string());
            }
        }
    }

    // Requeue any event that every accepting sink failed to deliver, so it is
    // persisted durably rather than dropped.
    let undelivered = events
        .into_iter()
        .zip(delivered)
        .filter_map(|(event, ok)| (!ok).then_some(event))
        .collect::<Vec<_>>();

    if let Some(requeue) = requeue {
        if !undelivered.is_empty() {
            let count = undelivered.len();
            for event in &undelivered {
                if let Err(error) = requeue.enqueue(event).await {
                    warn!(%error, "requeue of failed stream batch event into database queue failed");
                }
            }
            warn!(
                requeued = count,
                "requeued failed stream batch events into the durable database queue"
            );
        }
        // Failures were absorbed (requeued, or delivered by another sink).
        return Ok(());
    }

    // No durable fallback: surface the failure so the caller is aware a sink is
    // down, even when another sink delivered the same events.
    if errors.is_empty() {
        Ok(())
    } else {
        Err(OutboundError::Delivery(format!(
            "stream batch delivery failed for {} sink(s): {}",
            errors.len(),
            errors.join("; ")
        )))
    }
}

async fn send_to_sink(
    http: &HttpClient,
    sink: &BatchWebhookSink,
    indices: &[usize],
    events: &[OutboundEvent],
) -> Result<()> {
    let mut ndjson = Vec::new();
    for &index in indices {
        serde_json::to_writer(&mut ndjson, &events[index])
            .map_err(|e| OutboundError::Delivery(format!("serialize batch event: {e}")))?;
        ndjson.push(b'\n');
    }
    let body = gzip_bytes(&ndjson)?;
    let signature = compute_signature(&sink.secret, &body);
    let mut request = http
        .post(&sink.url)
        .header("X-Hivy-Signature", format!("sha256={signature}"))
        .header(reqwest::header::CONTENT_TYPE, "application/x-ndjson")
        .header(reqwest::header::CONTENT_ENCODING, "gzip");
    for (header_name, header_value) in &sink.extra_headers {
        request = request.header(header_name.as_str(), header_value.as_str());
    }
    let response = request
        .body(body)
        .send()
        .await
        .map_err(|e| OutboundError::Delivery(format!("send batch {}: {e}", sink.url)))?;
    let status = response.status();
    if !status.is_success() {
        let body = response.text().await.unwrap_or_default();
        warn!(channel = %sink.name, %status, body = %body, "webhook batch non-2xx");
        return Err(OutboundError::Delivery(format!(
            "{} returned {status}",
            sink.url
        )));
    }
    Ok(())
}

impl BatchWebhookSink {
    fn accepts(&self, event_type: &str) -> bool {
        match self.event_filter.as_ref() {
            None => true,
            Some(filters) if filters.is_empty() => true,
            Some(filters) => filters.iter().any(|f| filter_matches(f, event_type)),
        }
    }
}

impl BatchState {
    fn add_stream_delta(&mut self, event: OutboundEvent) {
        let Some(session_id) = string_field(&event.payload, "session_id") else {
            self.push_event(event);
            return;
        };
        let text = event
            .payload
            .get("text")
            .and_then(Value::as_str)
            .or_else(|| {
                event
                    .payload
                    .get("agent_event")
                    .and_then(|v| v.get("text"))
                    .and_then(Value::as_str)
            })
            .unwrap_or("");
        if text.is_empty() {
            self.push_event(event);
            return;
        }
        let sequence = event
            .payload
            .get("sequence")
            .and_then(Value::as_u64)
            .unwrap_or(0);
        let source = string_field(&event.payload, "source").unwrap_or_else(|| "manual".to_string());
        let stream_id = string_field(&event.payload, "stream_id");
        let trace_id = string_field(&event.payload, "trace_id");
        let turn_id = string_field(&event.payload, "turn_id");
        let scope = string_field(&event.payload, "scope").unwrap_or_else(|| "main".to_string());
        let subagent = event.payload.get("subagent").cloned();
        let key = format!(
            "{}:{}:{}:{session_id}",
            event.event_type,
            scope,
            turn_id.as_deref().unwrap_or("")
        );
        self.finish_other_session_streams(&session_id, &key);
        if self
            .coalescing
            .get(&key)
            .is_some_and(|entry| sequence == 0 || sequence != entry.sequence_end + 1)
        {
            self.finish_key(&key);
        }
        let should_finish = {
            let entry = self
                .coalescing
                .entry(key.clone())
                .or_insert_with(|| CoalescedStream {
                    event_type: event.event_type.clone(),
                    session_id,
                    source,
                    stream_id,
                    trace_id,
                    turn_id,
                    scope,
                    subagent,
                    sequence_start: sequence,
                    sequence_end: sequence,
                    delta_count: 0,
                    text: String::new(),
                    started_at: event.at,
                    ended_at: event.at,
                });
            entry.sequence_end = sequence;
            entry.delta_count += 1;
            entry.text.push_str(text);
            entry.ended_at = event.at;
            entry.delta_count >= MAX_COALESCED_DELTAS
                || entry.text.len() >= MAX_COALESCED_TEXT_BYTES
        };
        if should_finish {
            self.finish_key(&key);
        }
    }

    fn finish_streams_before_event(&mut self, event: &OutboundEvent) {
        if is_stream_delta(&event.event_type) {
            return;
        }
        let Some(session_id) = string_field(&event.payload, "session_id") else {
            return;
        };
        self.finish_session(&session_id);
    }

    fn finish_other_session_streams(&mut self, session_id: &str, keep_key: &str) {
        let suffix = format!(":{session_id}");
        let keys = self
            .coalescing
            .keys()
            .filter(|key| key.ends_with(&suffix) && key.as_str() != keep_key)
            .cloned()
            .collect::<Vec<_>>();
        for key in keys {
            self.finish_key(&key);
        }
    }

    fn finish_session(&mut self, session_id: &str) {
        let keys = self
            .coalescing
            .keys()
            .filter(|key| key.ends_with(&format!(":{session_id}")))
            .cloned()
            .collect::<Vec<_>>();
        for key in keys {
            self.finish_key(&key);
        }
    }

    fn finish_all(&mut self) {
        let keys = self.coalescing.keys().cloned().collect::<Vec<_>>();
        for key in keys {
            self.finish_key(&key);
        }
    }

    fn finish_key(&mut self, key: &str) {
        let Some(entry) = self.coalescing.remove(key) else {
            return;
        };
        self.push_event(entry.into_event());
    }

    fn push_event(&mut self, event: OutboundEvent) {
        self.pending_bytes += serde_json::to_vec(&event).map(|v| v.len() + 1).unwrap_or(0);
        self.pending.push(event);
    }

    fn drain_pending(&mut self) -> Vec<OutboundEvent> {
        let events = std::mem::take(&mut self.pending);
        self.pending_bytes = 0;
        events
    }
}

impl CoalescedStream {
    fn into_event(self) -> OutboundEvent {
        let mut payload = json!({
            "event_name": &self.event_type,
            "session_id": &self.session_id,
            "source": &self.source,
            "sequence": self.sequence_end,
            "event_id": format!(
                "evt_rt_{}_{}_{}",
                self.session_id,
                self.event_type,
                self.sequence_end
            ),
            "occurred_at": self.ended_at,
            "coalesced": true,
            "delta_count": self.delta_count,
            "sequence_start": self.sequence_start,
            "sequence_end": self.sequence_end,
            "started_at": self.started_at,
            "ended_at": self.ended_at,
            "text": self.text,
            "scope": self.scope,
        });
        if let Some(map) = payload.as_object_mut() {
            if let Some(stream_id) = self.stream_id {
                map.insert("stream_id".to_string(), json!(stream_id));
            }
            if let Some(trace_id) = self.trace_id {
                map.insert("trace_id".to_string(), json!(trace_id));
            }
            if let Some(turn_id) = self.turn_id {
                map.insert("turn_id".to_string(), json!(turn_id));
            }
            if let Some(subagent) = self.subagent {
                map.insert("subagent".to_string(), subagent);
            }
        }
        OutboundEvent {
            event_type: self.event_type,
            payload,
            at: self.ended_at,
        }
    }
}

fn gzip_bytes(input: &[u8]) -> Result<Vec<u8>> {
    let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
    encoder
        .write_all(input)
        .map_err(|e| OutboundError::Delivery(format!("gzip batch: {e}")))?;
    encoder
        .finish()
        .map_err(|e| OutboundError::Delivery(format!("finish gzip batch: {e}")))
}

fn batch_url(url: &str) -> String {
    format!("{}/batch", url.trim_end_matches('/'))
}

fn is_stream_delta(event_type: &str) -> bool {
    matches!(event_type, "token" | "thinking")
}

fn is_stream_delta_event(event: &OutboundEvent) -> bool {
    is_stream_delta(&event.event_type)
        && !event
            .payload
            .get("coalesced")
            .and_then(Value::as_bool)
            .unwrap_or(false)
}

fn is_coalesced_stream_delta_event(event: &OutboundEvent) -> bool {
    is_stream_delta(&event.event_type)
        && event
            .payload
            .get("coalesced")
            .and_then(Value::as_bool)
            .unwrap_or(false)
}

fn string_field(value: &Value, key: &str) -> Option<String> {
    value
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .map(ToString::to_string)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;

    #[test]
    fn coalesces_many_token_deltas_into_one_business_event() {
        let mut state = BatchState::default();
        for sequence in 1..=1500 {
            state.add_stream_delta(OutboundEvent::new(
                "token",
                json!({
                    "session_id": "http-thread-1",
                    "source": "http",
                    "sequence": sequence,
                    "agent_event": {"text": "x"},
                }),
            ));
        }
        state.finish_all();
        assert_eq!(state.pending.len(), 1);
        let event = &state.pending[0];
        assert_eq!(event.event_type, "token");
        assert_eq!(event.payload["coalesced"], true);
        assert_eq!(event.payload["delta_count"], 1500);
        assert_eq!(event.payload["sequence_start"], 1);
        assert_eq!(event.payload["sequence_end"], 1500);
        assert_eq!(event.payload["text"].as_str().unwrap().len(), 1500);
    }

    #[test]
    fn flush_session_does_not_flush_other_sessions() {
        let mut state = BatchState::default();
        for session in ["a", "b"] {
            state.add_stream_delta(OutboundEvent::new(
                "thinking",
                json!({
                    "session_id": session,
                    "source": "http",
                    "sequence": 1,
                    "agent_event": {"text": session},
                }),
            ));
        }
        state.finish_session("a");
        assert_eq!(state.pending.len(), 1);
        assert_eq!(state.pending[0].payload["session_id"], "a");
        assert_eq!(state.coalescing.len(), 1);
        assert!(state
            .coalescing
            .keys()
            .collect::<HashSet<_>>()
            .iter()
            .any(|k| k.ends_with(":b")));
    }

    #[test]
    fn flushes_stream_span_before_non_stream_event() {
        let mut state = BatchState::default();
        state.add_stream_delta(OutboundEvent::new(
            "thinking",
            json!({
                "session_id": "http-thread-1",
                "source": "http",
                "sequence": 3,
                "agent_event": {"text": "thinking"},
            }),
        ));
        state.finish_streams_before_event(&OutboundEvent::new(
            "tool_call",
            json!({
                "session_id": "http-thread-1",
                "source": "http",
                "sequence": 4,
                "agent_event": {"kind": "tool_call"},
            }),
        ));

        assert_eq!(state.pending.len(), 1);
        assert_eq!(state.pending[0].event_type, "thinking");
        assert_eq!(state.pending[0].payload["sequence_start"], 3);
        assert_eq!(state.pending[0].payload["sequence_end"], 3);
        assert!(state.coalescing.is_empty());
    }

    #[test]
    fn splits_stream_spans_on_kind_change_and_sequence_gap() {
        let mut state = BatchState::default();
        for (event_type, sequence, text) in [
            ("thinking", 3, "think-a"),
            ("thinking", 4, "think-b"),
            ("token", 5, "token-a"),
            ("token", 8, "token-b"),
        ] {
            state.add_stream_delta(OutboundEvent::new(
                event_type,
                json!({
                    "session_id": "http-thread-1",
                    "source": "http",
                    "sequence": sequence,
                    "agent_event": {"text": text},
                }),
            ));
        }
        state.finish_all();

        assert_eq!(state.pending.len(), 3);
        assert_eq!(state.pending[0].event_type, "thinking");
        assert_eq!(state.pending[0].payload["sequence_start"], 3);
        assert_eq!(state.pending[0].payload["sequence_end"], 4);
        assert_eq!(state.pending[1].event_type, "token");
        assert_eq!(state.pending[1].payload["sequence_start"], 5);
        assert_eq!(state.pending[1].payload["sequence_end"], 5);
        assert_eq!(state.pending[2].event_type, "token");
        assert_eq!(state.pending[2].payload["sequence_start"], 8);
        assert_eq!(state.pending[2].payload["sequence_end"], 8);
    }

    use std::sync::atomic::{AtomicU64, Ordering};
    use std::sync::Arc as StdArc;

    use storage::init_sqlite_store;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;
    use tokio::sync::Mutex as TokioMutex;

    static BATCH_DB_COUNTER: AtomicU64 = AtomicU64::new(0);

    /// Spawn a one-shot HTTP server that always replies with `status`, recording
    /// each received request body. Returns the bound URL and a shared store of
    /// the bodies it observed.
    async fn spawn_recording_server(
        status_line: &'static str,
    ) -> (String, StdArc<TokioMutex<Vec<Vec<u8>>>>) {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("bind");
        let addr = listener.local_addr().expect("addr");
        let bodies: StdArc<TokioMutex<Vec<Vec<u8>>>> = StdArc::new(TokioMutex::new(Vec::new()));
        let bodies_clone = bodies.clone();
        tokio::spawn(async move {
            loop {
                let Ok((mut socket, _)) = listener.accept().await else {
                    return;
                };
                let bodies = bodies_clone.clone();
                tokio::spawn(async move {
                    let mut buf = vec![0u8; 64 * 1024];
                    if let Ok(n) = socket.read(&mut buf).await {
                        bodies.lock().await.push(buf[..n].to_vec());
                    }
                    let response = format!("HTTP/1.1 {status_line}\r\nContent-Length: 0\r\n\r\n");
                    let _ = socket.write_all(response.as_bytes()).await;
                    let _ = socket.flush().await;
                });
            }
        });
        (format!("http://{addr}"), bodies)
    }

    fn test_sink(name: &str, url: &str) -> BatchWebhookSink {
        BatchWebhookSink {
            name: name.to_string(),
            url: url.to_string(),
            secret: "secret".to_string(),
            extra_headers: HashMap::new(),
            event_filter: None,
        }
    }

    fn token_event(sequence: u64, text: &str) -> OutboundEvent {
        OutboundEvent::new(
            "token",
            json!({
                "session_id": "http-1",
                "source": "http",
                "sequence": sequence,
                "agent_event": {"text": text},
            }),
        )
    }

    async fn setup_queue() -> (storage::SqliteStore, StdArc<DatabaseEventQueue>) {
        let db_path = std::env::temp_dir().join(format!(
            "outbound-batch-{}-{}.db",
            std::process::id(),
            BATCH_DB_COUNTER.fetch_add(1, Ordering::Relaxed)
        ));
        let store = init_sqlite_store(&db_path, None).await.expect("init store");
        let queue = DatabaseEventQueue::new(store.writer());
        (store, queue)
    }

    async fn event_log_count(store: &storage::SqliteStore) -> i64 {
        sqlx::query_scalar("SELECT COUNT(*) FROM events_log")
            .fetch_one(store.read_pool().as_ref())
            .await
            .expect("count events_log")
    }

    #[tokio::test]
    async fn continues_to_remaining_sinks_when_one_sink_fails() {
        let (failing_url, _) = spawn_recording_server("500 Internal Server Error").await;
        let (ok_url, ok_bodies) = spawn_recording_server("200 OK").await;
        let http = HttpClient::new();
        let sinks = vec![
            test_sink("failing", &batch_url(&failing_url)),
            test_sink("ok", &batch_url(&ok_url)),
        ];
        let events = vec![token_event(1, "hello")];

        // No requeue configured: an aggregate error is returned because a sink
        // failed, but the healthy sink must still have received the batch.
        let result = send_batch_events(&http, &sinks, None, events).await;
        assert!(result.is_err(), "a failed sink should surface an error");

        let bodies = ok_bodies.lock().await;
        assert_eq!(
            bodies.len(),
            1,
            "the healthy sink must still receive the batch after the first sink failed"
        );
    }

    #[tokio::test]
    async fn requeues_failed_batch_into_database_queue() {
        let (failing_url, _) = spawn_recording_server("503 Service Unavailable").await;
        let (store, queue) = setup_queue().await;
        let http = HttpClient::new();
        let sinks = vec![test_sink("failing", &batch_url(&failing_url))];
        let events = vec![token_event(1, "alpha"), token_event(2, "beta")];

        // With a requeue configured, the failed events must be persisted into the
        // durable database queue instead of being dropped.
        let result = send_batch_events(&http, &sinks, Some(&queue), events).await;
        assert!(
            result.is_ok(),
            "requeue should absorb the failure instead of erroring"
        );

        queue.flush().await.expect("flush queue");
        assert_eq!(
            event_log_count(&store).await,
            2,
            "both failed events must be requeued into the durable outbox"
        );
    }

    #[tokio::test]
    async fn delivered_events_are_not_requeued() {
        let (ok_url, _) = spawn_recording_server("200 OK").await;
        let (store, queue) = setup_queue().await;
        let http = HttpClient::new();
        let sinks = vec![test_sink("ok", &batch_url(&ok_url))];
        let events = vec![token_event(1, "delivered")];

        send_batch_events(&http, &sinks, Some(&queue), events)
            .await
            .expect("delivery succeeds");

        queue.flush().await.expect("flush queue");
        assert_eq!(
            event_log_count(&store).await,
            0,
            "successfully delivered events must not be requeued"
        );
    }

    #[tokio::test]
    async fn partial_sink_success_only_requeues_events_no_sink_delivered() {
        // Two sinks with disjoint filters; the sink that accepts thinking events
        // fails, the sink that accepts tokens succeeds. Only the thinking event
        // (which no sink delivered) should be requeued.
        let (failing_url, _) = spawn_recording_server("500 Internal Server Error").await;
        let (ok_url, _) = spawn_recording_server("200 OK").await;
        let (store, queue) = setup_queue().await;
        let http = HttpClient::new();
        let sinks = vec![
            BatchWebhookSink {
                event_filter: Some(vec!["thinking".to_string()]),
                ..test_sink("thinking-failing", &batch_url(&failing_url))
            },
            BatchWebhookSink {
                event_filter: Some(vec!["token".to_string()]),
                ..test_sink("token-ok", &batch_url(&ok_url))
            },
        ];
        let events = vec![
            OutboundEvent::new(
                "thinking",
                json!({"session_id": "http-1", "agent_event": {"text": "t"}}),
            ),
            token_event(1, "tok"),
        ];

        send_batch_events(&http, &sinks, Some(&queue), events)
            .await
            .expect("requeue absorbs the failure");

        queue.flush().await.expect("flush queue");
        assert_eq!(
            event_log_count(&store).await,
            1,
            "only the undelivered thinking event should be requeued"
        );
    }
}
