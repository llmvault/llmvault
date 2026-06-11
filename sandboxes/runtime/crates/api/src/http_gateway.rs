use std::collections::{HashMap, VecDeque};
use std::convert::Infallible;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Instant;

use anyhow::Result;
use async_stream::stream;
use axum::response::sse::{Event, Sse};
use chrono::{DateTime, Utc};
use domain::{Attachment, InboundEvent, MessageHandle, SessionId};
use observability::{
    parse_model_usage, EventTimings, ObservabilityEvent, ObservabilityEventType,
    ObservabilityRecorder, ToolUsage,
};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::sync::{broadcast, Mutex};
use tokio::time::Duration;

#[derive(Clone)]
pub struct HttpGatewayState {
    pub inbound_sink: tokio::sync::mpsc::Sender<InboundEvent>,
    pub broker: Arc<HttpStreamBroker>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HttpStreamEvent {
    pub event: String,
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub payload: Value,
}

#[derive(Default)]
pub struct HttpStreamBroker {
    streams: Mutex<HashMap<String, StreamState>>,
    session_streams: Mutex<HashMap<String, String>>,
    active_session_streams: Mutex<HashMap<String, String>>,
    observability: ObservabilityRecorder,
    counter: AtomicU64,
}

/// Maximum number of events retained for replay to late/reconnecting
/// subscribers. The buffer is a ring: once full, the oldest event is evicted so
/// the newest events — crucially the terminal `final`/`done` — are always
/// replayable.
const HISTORY_CAPACITY: usize = 512;

/// How long a finished (`done`) stream's state is retained before it is evicted
/// from the broker's maps. The grace window lets a reconnecting/late subscriber
/// (Slack asynq task, Go proxy retry, browser reload) still replay the terminal
/// `final`/`done` from history. After it, the per-stream state and its session
/// mapping are reclaimed so the maps do not grow without bound (P2-1).
const STREAM_EVICTION_GRACE: Duration = Duration::from_secs(120);

/// A published event paired with a monotonically increasing per-stream sequence
/// number. The sequence lets a subscriber that fell behind (`RecvError::Lagged`)
/// resync precisely against the replay history — replaying exactly the events it
/// skipped — instead of silently dropping the gap (P2-2). Derefs to the inner
/// [`HttpStreamEvent`] so `.event`/`.payload` are accessible directly.
#[derive(Clone)]
pub struct SeqEvent {
    pub seq: u64,
    pub inner: HttpStreamEvent,
}

impl std::ops::Deref for SeqEvent {
    type Target = HttpStreamEvent;
    fn deref(&self) -> &Self::Target {
        &self.inner
    }
}

struct StreamState {
    sender: broadcast::Sender<SeqEvent>,
    history: VecDeque<SeqEvent>,
    context: Option<StreamObservabilityContext>,
    /// Next sequence number to assign to a published event.
    next_seq: u64,
    /// When the terminal `done` was published, after which the stream becomes
    /// eligible for eviction once `STREAM_EVICTION_GRACE` has elapsed.
    done_at: Option<Instant>,
}

#[derive(Debug, Clone)]
struct StreamObservabilityContext {
    trace_id: String,
    session_id: String,
    turn_id: String,
    run_id: String,
    started_at: DateTime<Utc>,
}

impl HttpStreamBroker {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn observability(&self) -> ObservabilityRecorder {
        self.observability.clone()
    }

    #[cfg(test)]
    async fn stream_count(&self) -> usize {
        self.streams.lock().await.len()
    }

    #[cfg(test)]
    async fn session_stream_count(&self) -> usize {
        self.session_streams.lock().await.len()
    }

    /// Test-only: backdate a finished stream's eviction grace so the next
    /// opportunistic sweep reclaims it without waiting the real grace window.
    #[cfg(test)]
    async fn force_expire_done(&self, stream_id: &str) {
        if let Some(state) = self.streams.lock().await.get_mut(stream_id) {
            state.done_at = Some(Instant::now() - STREAM_EVICTION_GRACE - Duration::from_secs(1));
        }
    }

    pub async fn create_stream(&self) -> String {
        let id = format!(
            "http-stream-{}-{}",
            Utc::now().timestamp_millis(),
            self.counter.fetch_add(1, Ordering::Relaxed)
        );
        let (sender, _) = broadcast::channel(256);
        let mut streams = self.streams.lock().await;
        // Opportunistically reclaim finished streams whose grace window has
        // elapsed (P2-1). Done lazily here (and in publish) to avoid a background
        // sweeper task; create_stream runs once per turn so the maps stay bounded.
        evict_expired_streams(&mut streams, &self.session_streams, &self.active_session_streams)
            .await;
        streams.insert(
            id.clone(),
            StreamState {
                sender,
                history: VecDeque::with_capacity(HISTORY_CAPACITY),
                context: None,
                next_seq: 0,
                done_at: None,
            },
        );
        drop(streams);
        id
    }

    pub async fn start_turn(
        &self,
        stream_id: &str,
        session_id: &str,
        text_len: usize,
    ) -> Option<(String, String)> {
        let now = Utc::now();
        let trace_id = format!("trace-{stream_id}");
        let turn_id = format!("turn-{stream_id}");
        let run_id = format!("run-{stream_id}");
        let context = StreamObservabilityContext {
            trace_id: trace_id.clone(),
            session_id: session_id.to_string(),
            turn_id: turn_id.clone(),
            run_id,
            started_at: now,
        };

        let mut streams = self.streams.lock().await;
        let state = streams.get_mut(stream_id)?;
        state.context = Some(context.clone());
        drop(streams);

        self.record_context_event(
            &context,
            ObservabilityEventType::RunStarted,
            json!({ "stream_id": stream_id }),
        );
        self.record_context_event(
            &context,
            ObservabilityEventType::TurnStarted,
            json!({
                "stream_id": stream_id,
                "input_text_len": text_len,
            }),
        );
        Some((trace_id, turn_id))
    }

    pub async fn publish(&self, stream_id: &str, event: impl Into<String>, payload: Value) {
        let event = HttpStreamEvent {
            event: event.into(),
            payload,
        };
        let mut context = None;
        let mut streams = self.streams.lock().await;
        if let Some(state) = streams.get_mut(stream_id) {
            let seq = state.next_seq;
            state.next_seq += 1;
            let seq_event = SeqEvent {
                seq,
                inner: event.clone(),
            };
            // Ring buffer: evict the oldest event when full so terminal events
            // (final/done) at the tail are always retained for replay.
            if state.history.len() == HISTORY_CAPACITY {
                state.history.pop_front();
            }
            state.history.push_back(seq_event.clone());
            if event.event == "done" && state.done_at.is_none() {
                // Start the eviction grace window; the state lingers long enough
                // for late subscribers to replay the terminal events (P2-1).
                state.done_at = Some(Instant::now());
            }
            context = state.context.clone();
            let _ = state.sender.send(seq_event);
        }
        // Reclaim any streams whose grace window has now elapsed.
        evict_expired_streams(&mut streams, &self.session_streams, &self.active_session_streams)
            .await;
        drop(streams);
        if let Some(context) = context.as_ref() {
            self.record_stream_event(context, &event.event, &event.payload);
        }
    }

    pub async fn register_session(&self, session_id: &str, stream_id: &str) {
        self.session_streams
            .lock()
            .await
            .insert(session_id.to_string(), stream_id.to_string());
    }

    pub async fn activate_session_stream(&self, session_id: &str, stream_id: &str) {
        self.active_session_streams
            .lock()
            .await
            .insert(session_id.to_string(), stream_id.to_string());
    }

    pub async fn clear_active_session_stream(&self, session_id: &str, stream_id: &str) {
        let mut active = self.active_session_streams.lock().await;
        if active
            .get(session_id)
            .is_some_and(|current| current == stream_id)
        {
            active.remove(session_id);
        }
    }

    pub async fn stream_id_for_session(&self, session_id: &str) -> Option<String> {
        if let Some(stream_id) = self.active_session_streams.lock().await.get(session_id) {
            return Some(stream_id.clone());
        }
        self.session_streams.lock().await.get(session_id).cloned()
    }

    pub async fn subscribe(
        &self,
        stream_id: &str,
    ) -> Option<(Vec<SeqEvent>, broadcast::Receiver<SeqEvent>)> {
        let streams = self.streams.lock().await;
        let state = streams.get(stream_id)?;
        let history = state.history.iter().cloned().collect();
        Some((history, state.sender.subscribe()))
    }

    /// Replay-history events with sequence number strictly greater than
    /// `after_seq`, used to resync a subscriber that fell behind
    /// (`RecvError::Lagged`) by replaying exactly the events it skipped (P2-2).
    /// Returns `None` if the stream no longer exists.
    pub async fn history_after(&self, stream_id: &str, after_seq: u64) -> Option<Vec<SeqEvent>> {
        let streams = self.streams.lock().await;
        let state = streams.get(stream_id)?;
        Some(
            state
                .history
                .iter()
                .filter(|item| item.seq > after_seq)
                .cloned()
                .collect(),
        )
    }

    /// Full retained replay-history snapshot for a stream. Used to resync a
    /// subscriber that lagged before delivering any event. Returns `None` if the
    /// stream no longer exists.
    pub async fn history_snapshot(&self, stream_id: &str) -> Option<Vec<SeqEvent>> {
        let streams = self.streams.lock().await;
        let state = streams.get(stream_id)?;
        Some(state.history.iter().cloned().collect())
    }

    fn record_stream_event(
        &self,
        context: &StreamObservabilityContext,
        stream_event: &str,
        payload: &Value,
    ) {
        let event_type = match stream_event {
            "turn_started" => ObservabilityEventType::TurnStarted,
            "thinking" | "token" => ObservabilityEventType::AssistantDelta,
            "tool_call" => ObservabilityEventType::ToolCall,
            "tool_result" => ObservabilityEventType::ToolResult,
            "model_usage" => ObservabilityEventType::ModelUsage,
            "turn_completed" | "final" => ObservabilityEventType::TurnCompleted,
            "done" => ObservabilityEventType::RunCompleted,
            "model_request_failed" | "model_stream_failed" | "max_turns_exhausted" | "error" => {
                ObservabilityEventType::Error
            }
            _ => return,
        };

        let mut event = ObservabilityEvent::new(
            context.trace_id.clone(),
            context.session_id.clone(),
            context.turn_id.clone(),
            event_type,
        );
        event.run_id = Some(context.run_id.clone());
        event.payload = payload.clone();

        match stream_event {
            "tool_call" => {
                event.tool = Some(ToolUsage {
                    call_id: payload
                        .get("id")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    tool_name: payload
                        .get("tool")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    status: "started".to_string(),
                });
            }
            "tool_result" => {
                event.tool = Some(ToolUsage {
                    call_id: payload
                        .get("id")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    tool_name: payload
                        .get("tool")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    status: "completed".to_string(),
                });
            }
            "model_usage" => {
                event.model = Some(parse_model_usage(payload));
            }
            "turn_completed"
            | "final"
            | "done"
            | "error"
            | "model_request_failed"
            | "model_stream_failed"
            | "max_turns_exhausted" => {
                let completed_at = event.occurred_at;
                event.timings = EventTimings {
                    started_at: Some(context.started_at),
                    completed_at: Some(completed_at),
                    duration_ms: (completed_at - context.started_at)
                        .num_milliseconds()
                        .try_into()
                        .ok(),
                    ..EventTimings::default()
                };
            }
            _ => {}
        }

        self.observability.append(event);
    }

    fn record_context_event(
        &self,
        context: &StreamObservabilityContext,
        event_type: ObservabilityEventType,
        payload: Value,
    ) {
        let mut event = ObservabilityEvent::new(
            context.trace_id.clone(),
            context.session_id.clone(),
            context.turn_id.clone(),
            event_type,
        );
        event.run_id = Some(context.run_id.clone());
        event.payload = payload;
        if matches!(
            event.event_type,
            ObservabilityEventType::RunStarted | ObservabilityEventType::TurnStarted
        ) {
            event.timings.started_at = Some(context.started_at);
        }
        self.observability.append(event);
    }
}

/// Reclaim per-stream state for streams whose terminal `done` was published more
/// than `STREAM_EVICTION_GRACE` ago, and drop the session→stream mappings that
/// still point at them. Without this the broker's `streams`/`session_streams`
/// maps grow unbounded for the process lifetime (P2-1). Called opportunistically
/// from `create_stream`/`publish` while the `streams` lock is held; lock order is
/// always streams → session_streams → active_session_streams to avoid deadlock.
async fn evict_expired_streams(
    streams: &mut HashMap<String, StreamState>,
    session_streams: &Mutex<HashMap<String, String>>,
    active_session_streams: &Mutex<HashMap<String, String>>,
) {
    let now = Instant::now();
    let expired: Vec<String> = streams
        .iter()
        .filter(|(_, state)| {
            state
                .done_at
                .is_some_and(|done_at| now.duration_since(done_at) >= STREAM_EVICTION_GRACE)
        })
        .map(|(id, _)| id.clone())
        .collect();
    if expired.is_empty() {
        return;
    }
    for id in &expired {
        streams.remove(id);
    }
    let expired_set: std::collections::HashSet<&str> =
        expired.iter().map(String::as_str).collect();
    // Drop session mappings that still point at an evicted stream. A session
    // re-registered onto a fresh stream keeps its newer mapping.
    let mut session = session_streams.lock().await;
    session.retain(|_, stream_id| !expired_set.contains(stream_id.as_str()));
    drop(session);
    let mut active = active_session_streams.lock().await;
    active.retain(|_, stream_id| !expired_set.contains(stream_id.as_str()));
}

#[derive(Debug, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HttpMessageRequest {
    pub text: String,
    #[serde(default)]
    pub conversation_id: Option<String>,
    #[serde(default = "default_http_user")]
    pub user: String,
    #[serde(default)]
    pub user_display_name: Option<String>,
    #[serde(default)]
    pub attachments: Vec<Attachment>,
    #[serde(default)]
    pub dynamic_context: Vec<String>,
    #[serde(default)]
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub raw: Value,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HttpMessageResponse {
    pub session_id: String,
    pub stream_id: String,
    pub stream_url: String,
    pub response_stream_id: String,
    pub response_stream_url: String,
    pub trace_id: String,
    pub turn_id: String,
}

impl HttpGatewayState {
    pub async fn inject_message(&self, request: HttpMessageRequest) -> Result<HttpMessageResponse> {
        let stream_id = self.broker.create_stream().await;
        let response_stream_id = self.broker.create_stream().await;
        let conversation_id = request
            .conversation_id
            .clone()
            .unwrap_or_else(|| stream_id.clone());
        let session_id = http_session_id_for_conversation(&conversation_id);
        self.broker
            .register_session(session_id.as_str(), &stream_id)
            .await;
        let (trace_id, turn_id) = self
            .broker
            .start_turn(&stream_id, session_id.as_str(), request.text.len())
            .await
            .unwrap_or_else(|| (format!("trace-{stream_id}"), format!("turn-{stream_id}")));
        let source = request
            .raw
            .get("source")
            .and_then(Value::as_str)
            .filter(|value| matches!(*value, "gateway" | "web"))
            .unwrap_or("http");
        let raw = json!({
            "source": source,
            "http_stream_id": stream_id,
            "http_response_stream_id": response_stream_id,
            "trace_id": trace_id,
            "turn_id": turn_id,
            "conversation_id": conversation_id,
            "raw": request.raw,
        });
        let inbound = InboundEvent {
            envelope_id: stream_id.clone(),
            session_id: session_id.clone(),
            user: request.user,
            user_display_name: request.user_display_name,
            text: request.text,
            attachments: request.attachments,
            dynamic_context: request.dynamic_context,
            raw,
            inbound_handle: MessageHandle {
                channel: "http".to_string(),
                ts: stream_id.clone(),
            },
            is_direct_message: true,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        };
        self.inbound_sink.send(inbound).await?;
        Ok(HttpMessageResponse {
            session_id: session_id.as_str().to_string(),
            stream_id: stream_id.clone(),
            stream_url: format!("/gateway/http/streams/{stream_id}"),
            response_stream_id: response_stream_id.clone(),
            response_stream_url: format!("/gateway/http/response-streams/{response_stream_id}"),
            trace_id,
            turn_id,
        })
    }
}

fn http_session_id_for_conversation(conversation_id: &str) -> SessionId {
    if conversation_id.starts_with("http-") {
        SessionId::from(conversation_id.to_string())
    } else {
        SessionId::from(format!("http-{conversation_id}"))
    }
}

/// A frame produced by the replay stream: either a published event or a
/// synthetic `resync` marker inserted after a `Lagged` gap.
#[derive(Debug, Clone)]
enum StreamFrame {
    Event(HttpStreamEvent),
    Resync { skipped: u64, from_seq: Option<u64> },
}

/// Core of the SSE replay stream, decoupled from the `Sse`/`Event` wire types so
/// the reconnect/resync behavior is unit-testable. Yields history first, then
/// live events, and on `Lagged` emits a `Resync` frame followed by the exact
/// missed range from history (P2-2). Stops on `done`.
fn replay_stream(
    broker: Arc<HttpStreamBroker>,
    stream_id: String,
    history: Vec<SeqEvent>,
    mut receiver: broadcast::Receiver<SeqEvent>,
) -> impl futures::Stream<Item = StreamFrame> {
    stream! {
        // Track the highest sequence number delivered to this subscriber so a
        // `Lagged` gap can be resynced precisely from history (P2-2).
        let mut last_seq: Option<u64> = None;
        for item in history {
            last_seq = Some(item.seq);
            let is_done = item.inner.event == "done";
            yield StreamFrame::Event(item.inner);
            if is_done {
                return;
            }
        }
        loop {
            match receiver.recv().await {
                Ok(item) => {
                    last_seq = Some(item.seq);
                    let is_done = item.inner.event == "done";
                    yield StreamFrame::Event(item.inner);
                    if is_done {
                        break;
                    }
                }
                Err(broadcast::error::RecvError::Lagged(skipped)) => {
                    // The consumer fell more than the channel capacity behind and
                    // the broadcast dropped `skipped` events. Rather than silently
                    // leaving a mid-stream gap, resync from the replay history:
                    // emit a `resync` marker, then replay every buffered event the
                    // subscriber has not yet seen (seq > last_seq), in order.
                    yield StreamFrame::Resync { skipped, from_seq: last_seq };
                    // Replay every buffered event newer than the last one we
                    // delivered. If nothing was delivered yet, replay the whole
                    // retained history.
                    let missed = match last_seq {
                        Some(s) => broker.history_after(&stream_id, s).await,
                        None => broker.history_snapshot(&stream_id).await,
                    }
                    .unwrap_or_default();
                    let mut done = false;
                    for item in missed {
                        last_seq = Some(item.seq);
                        if item.inner.event == "done" {
                            done = true;
                        }
                        yield StreamFrame::Event(item.inner);
                    }
                    if done {
                        break;
                    }
                    continue;
                }
                Err(broadcast::error::RecvError::Closed) => break,
            }
        }
    }
}

fn stream_frame_to_sse(stream_id: &str, frame: StreamFrame) -> Event {
    match frame {
        StreamFrame::Event(event) => to_sse_event(event),
        StreamFrame::Resync { skipped, from_seq } => Event::default()
            .event("resync")
            .json_data(json!({
                "stream_id": stream_id,
                "skipped": skipped,
                "from_seq": from_seq,
            }))
            .unwrap_or_else(|_| Event::default().event("resync").data("{}")),
    }
}

pub async fn stream_response(
    broker: Arc<HttpStreamBroker>,
    stream_id: String,
) -> Option<Sse<impl futures::Stream<Item = Result<Event, Infallible>>>> {
    let (history, receiver) = broker.subscribe(&stream_id).await?;
    let frames = replay_stream(broker, stream_id.clone(), history, receiver);
    let output = stream! {
        use futures::StreamExt;
        futures::pin_mut!(frames);
        while let Some(frame) = frames.next().await {
            yield Ok(stream_frame_to_sse(&stream_id, frame));
        }
    };
    Some(
        Sse::new(output).keep_alive(
            axum::response::sse::KeepAlive::new()
                .interval(Duration::from_secs(15))
                .text("keep-alive"),
        ),
    )
}

fn to_sse_event(item: HttpStreamEvent) -> Event {
    Event::default()
        .event(item.event)
        .json_data(item.payload)
        .unwrap_or_else(|_| Event::default().event("error").data("serialize event"))
}

fn default_http_user() -> String {
    "http".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::sync::mpsc;

    #[test]
    fn http_session_id_for_conversation_is_idempotent() {
        assert_eq!(
            http_session_id_for_conversation("gateway-conversation").as_str(),
            "http-gateway-conversation"
        );
        assert_eq!(
            http_session_id_for_conversation("http-gateway-conversation").as_str(),
            "http-gateway-conversation"
        );
    }

    #[tokio::test]
    async fn inject_message_creates_inbound_event_and_stream_mapping() {
        let (tx, mut rx) = mpsc::channel(1);
        let broker = Arc::new(HttpStreamBroker::new());
        let gateway = HttpGatewayState {
            inbound_sink: tx,
            broker: broker.clone(),
        };

        let response = gateway
            .inject_message(HttpMessageRequest {
                text: "hello".to_string(),
                conversation_id: Some("conversation-1".to_string()),
                user: "user-1".to_string(),
                user_display_name: Some("User One".to_string()),
                attachments: Vec::new(),
                dynamic_context: vec!["## Recent sessions\n- prior context".to_string()],
                raw: json!({"source": "test"}),
            })
            .await
            .expect("inject message");

        assert_eq!(response.session_id, "http-conversation-1");
        assert!(response.trace_id.starts_with("trace-http-stream-"));
        assert!(response.turn_id.starts_with("turn-http-stream-"));
        assert!(response.stream_url.ends_with(&response.stream_id));
        assert!(response
            .response_stream_url
            .ends_with(&response.response_stream_id));
        assert_ne!(response.stream_id, response.response_stream_id);
        assert_eq!(
            broker.stream_id_for_session("http-conversation-1").await,
            Some(response.stream_id.clone())
        );

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(inbound.session_id.as_str(), "http-conversation-1");
        assert_eq!(inbound.user, "user-1");
        assert_eq!(inbound.text, "hello");
        assert_eq!(
            inbound.dynamic_context,
            vec!["## Recent sessions\n- prior context"]
        );
        assert_eq!(inbound.raw["http_stream_id"], response.stream_id);
        assert_eq!(
            inbound.raw["http_response_stream_id"],
            response.response_stream_id
        );
        assert_eq!(inbound.raw["trace_id"], response.trace_id);
        assert_eq!(inbound.raw["turn_id"], response.turn_id);
        assert!(inbound.is_direct_message);
        assert!(inbound.is_directly_addressed);

        let events = broker.observability().list_by_trace(&response.trace_id);
        assert_eq!(events.len(), 2);
        assert!(matches!(
            events[0].event_type,
            ObservabilityEventType::RunStarted
        ));
        assert!(matches!(
            events[1].event_type,
            ObservabilityEventType::TurnStarted
        ));
    }

    #[tokio::test]
    async fn inject_message_preserves_gateway_source_and_metadata() {
        let (tx, mut rx) = mpsc::channel(1);
        let gateway = HttpGatewayState {
            inbound_sink: tx,
            broker: Arc::new(HttpStreamBroker::new()),
        };

        let response = gateway
            .inject_message(HttpMessageRequest {
                text: "hello from slack".to_string(),
                conversation_id: Some("gateway-conversation".to_string()),
                user: "U123".to_string(),
                user_display_name: Some("Ada".to_string()),
                attachments: Vec::new(),
                dynamic_context: Vec::new(),
                raw: json!({
                    "source": "gateway",
                    "provider": "fake-slack",
                    "route_id": "route-1",
                    "thread_key": "fake-slack:T123:C123:100.000",
                    "channel_id": "C123",
                    "thread_id": "100.000"
                }),
            })
            .await
            .expect("inject gateway message");

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(response.session_id, "http-gateway-conversation");
        assert_eq!(inbound.raw["source"], "gateway");
        assert_eq!(inbound.raw["conversation_id"], "gateway-conversation");
        assert_eq!(inbound.raw["raw"]["provider"], "fake-slack");
        assert_eq!(inbound.raw["raw"]["channel_id"], "C123");
        assert_eq!(inbound.raw["raw"]["thread_id"], "100.000");
    }

    #[tokio::test]
    async fn inject_message_preserves_web_source() {
        let (tx, mut rx) = mpsc::channel(1);
        let gateway = HttpGatewayState {
            inbound_sink: tx,
            broker: Arc::new(HttpStreamBroker::new()),
        };

        gateway
            .inject_message(HttpMessageRequest {
                text: "hello from web".to_string(),
                conversation_id: Some("web-conversation".to_string()),
                user: "user-1".to_string(),
                user_display_name: Some("Ada".to_string()),
                attachments: Vec::new(),
                dynamic_context: Vec::new(),
                raw: json!({"source": "web"}),
            })
            .await
            .expect("inject web message");

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(inbound.raw["source"], "web");
        assert_eq!(inbound.raw["conversation_id"], "web-conversation");
    }

    #[tokio::test]
    async fn broker_replays_history_to_late_subscribers() {
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker
            .publish(&stream_id, "token", json!({"text": "hello"}))
            .await;

        let (history, mut receiver) = broker.subscribe(&stream_id).await.expect("stream exists");
        assert_eq!(history.len(), 1);
        assert_eq!(history[0].event, "token");
        assert_eq!(history[0].payload["text"], "hello");

        broker
            .publish(&stream_id, "final", json!({"text": "done"}))
            .await;
        let live = receiver.recv().await.expect("live event");
        assert_eq!(live.event, "final");
        assert_eq!(live.payload["text"], "done");
    }

    #[tokio::test]
    async fn late_subscriber_after_history_overflow_still_sees_terminal_events() {
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;

        // A token-heavy turn: publish far more than the history capacity, then
        // the terminal events. A FIRST-512 buffer would drop these terminals.
        let total_tokens = HISTORY_CAPACITY + 200;
        for i in 0..total_tokens {
            broker
                .publish(&stream_id, "token", json!({"text": format!("tok-{i}")}))
                .await;
        }
        broker
            .publish(&stream_id, "final", json!({"text": "the answer"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        // A subscriber arriving only now (reconnect / Slack asynq task / Go
        // proxy retry) must replay the NEWEST events, including final + done.
        let (history, _receiver) = broker.subscribe(&stream_id).await.expect("stream exists");

        assert_eq!(
            history.len(),
            HISTORY_CAPACITY,
            "history is bounded to the ring capacity"
        );

        let final_event = history
            .iter()
            .find(|e| e.event == "final")
            .expect("terminal final event must be replayable");
        assert_eq!(final_event.payload["text"], "the answer");
        assert!(
            history.iter().any(|e| e.event == "done"),
            "terminal done event must be replayable so the client stops waiting"
        );

        // The two terminal events are the very last in the replay, in order.
        assert_eq!(history[history.len() - 2].event, "final");
        assert_eq!(history[history.len() - 1].event, "done");

        // The oldest tokens were evicted; the earliest retained token is recent.
        let first_token_text = history
            .iter()
            .find(|e| e.event == "token")
            .expect("some tokens retained")
            .payload["text"]
            .as_str()
            .unwrap()
            .to_string();
        assert_ne!(
            first_token_text, "tok-0",
            "the oldest tokens must have been evicted by the ring buffer"
        );
    }

    #[tokio::test]
    async fn reconnect_mid_stream_replays_tail_then_continues_live() {
        // Models the Go proxy / web client reconnecting partway through a turn:
        // the first subscriber reads some tokens, the connection drops, and a
        // NEW subscriber resubscribes. It must replay the buffered tail (every
        // event so far, including the ones the first subscriber missed) and then
        // keep receiving the live remainder up to the terminal `done`.
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;

        // First connection sees the first two tokens live.
        let (history0, mut first) = broker.subscribe(&stream_id).await.expect("stream exists");
        assert!(history0.is_empty(), "no history before any publish");
        broker
            .publish(&stream_id, "token", json!({"text": "The "}))
            .await;
        broker
            .publish(&stream_id, "token", json!({"text": "quick "}))
            .await;
        assert_eq!(first.recv().await.unwrap().payload["text"], "The ");
        assert_eq!(first.recv().await.unwrap().payload["text"], "quick ");

        // Connection drops mid-turn.
        drop(first);

        // More events are published while nobody is connected.
        broker
            .publish(&stream_id, "token", json!({"text": "brown "}))
            .await;

        // Reconnect: history must contain the WHOLE turn so far (the tail the new
        // subscriber missed is replayable), then live events continue.
        let (history1, mut second) = broker.subscribe(&stream_id).await.expect("stream exists");
        let replayed: Vec<String> = history1
            .iter()
            .filter(|e| e.event == "token")
            .map(|e| e.payload["text"].as_str().unwrap().to_string())
            .collect();
        assert_eq!(replayed, vec!["The ", "quick ", "brown "]);

        broker
            .publish(&stream_id, "token", json!({"text": "fox"}))
            .await;
        broker.publish(&stream_id, "final", json!({"text": "The quick brown fox"})).await;
        broker.publish(&stream_id, "done", json!({})).await;

        assert_eq!(second.recv().await.unwrap().payload["text"], "fox");
        assert_eq!(second.recv().await.unwrap().event, "final");
        assert_eq!(second.recv().await.unwrap().event, "done");
    }

    #[tokio::test]
    async fn subscriber_after_done_replays_terminal_events_and_does_not_hang() {
        // A subscriber that arrives only AFTER the turn fully completed (Slack
        // asynq task scheduled late, a Go proxy retry, a browser reload) must
        // still receive the terminal `final`/`done` from history so it stops
        // waiting instead of blocking on recv() forever.
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;

        broker
            .publish(&stream_id, "token", json!({"text": "answer"}))
            .await;
        broker
            .publish(&stream_id, "final", json!({"text": "answer"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        // Subscribe only now — strictly after the stream is done.
        let (history, mut receiver) = broker.subscribe(&stream_id).await.expect("stream exists");
        assert_eq!(history.last().map(|e| e.event.as_str()), Some("done"));
        assert!(
            history.iter().any(|e| e.event == "final"),
            "terminal final must be replayable to a post-done subscriber"
        );

        // Crucially, the client must be able to STOP from the replayed `done`
        // alone: the live broadcast channel yields nothing more (the sender is
        // still held by the broker, so it is not Closed), so a client that
        // waited on recv() instead of honoring the history `done` would hang.
        // This asserts the terminal signal lives in history, not the live tail.
        let live = tokio::time::timeout(Duration::from_millis(150), receiver.recv()).await;
        assert!(
            live.is_err(),
            "no further live events after done; the terminal signal must come from replayed history"
        );
    }

    #[tokio::test]
    async fn active_session_stream_overrides_latest_registered_stream_temporarily() {
        let broker = HttpStreamBroker::new();
        broker
            .register_session("http-conversation", "stream-1")
            .await;
        broker
            .activate_session_stream("http-conversation", "stream-1")
            .await;
        broker
            .register_session("http-conversation", "stream-2")
            .await;

        assert_eq!(
            broker.stream_id_for_session("http-conversation").await,
            Some("stream-1".to_string())
        );

        broker
            .clear_active_session_stream("http-conversation", "stream-1")
            .await;
        assert_eq!(
            broker.stream_id_for_session("http-conversation").await,
            Some("stream-2".to_string())
        );
    }

    #[tokio::test]
    async fn broker_records_observability_for_stream_events() {
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;
        let (trace_id, _) = broker
            .start_turn(&stream_id, "http-conversation", 5)
            .await
            .expect("turn context");

        broker
            .publish(
                &stream_id,
                "tool_call",
                json!({"id": "call-1", "tool": "lookup", "args": {"q": "hello"}}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "model_usage",
                json!({
                    "provider": "fake",
                    "model": "fake-model",
                    "usage": {
                        "prompt_tokens": 2,
                        "completion_tokens": 3,
                        "total_tokens": 5
                    }
                }),
            )
            .await;
        broker
            .publish(&stream_id, "final", json!({"text": "answer"}))
            .await;
        broker
            .publish(
                &stream_id,
                "done",
                json!({"session_id": "http-conversation"}),
            )
            .await;

        let summary = broker.observability().summarize_trace(&trace_id);
        assert!(summary.is_complete);
        assert_eq!(summary.tool_call_count, 1);
        assert_eq!(summary.model_usage.total_tokens, 5);
        assert_eq!(summary.final_text.as_deref(), Some("answer"));
    }

    // ---- P2-1: stream/session map eviction after done + grace ----

    #[tokio::test]
    async fn finished_stream_state_is_evicted_after_grace_on_next_activity() {
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker
            .register_session("http-session-1", &stream_id)
            .await;
        broker
            .activate_session_stream("http-session-1", &stream_id)
            .await;
        broker
            .publish(&stream_id, "final", json!({"text": "ok"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        // Before the grace elapses the state is retained so a late subscriber can
        // still replay the terminal events.
        assert_eq!(broker.stream_count().await, 1);
        assert!(broker.subscribe(&stream_id).await.is_some());

        // Simulate the grace window having elapsed, then trigger an opportunistic
        // sweep by creating a new (unrelated) stream.
        broker.force_expire_done(&stream_id).await;
        let _other = broker.create_stream().await;

        // The finished stream and its session mappings are reclaimed; only the
        // freshly created unrelated stream remains.
        assert_eq!(broker.stream_count().await, 1);
        assert!(broker.subscribe(&stream_id).await.is_none());
        assert_eq!(broker.session_stream_count().await, 0);
        assert!(broker.stream_id_for_session("http-session-1").await.is_none());
    }

    #[tokio::test]
    async fn reregistered_session_keeps_newer_mapping_when_old_stream_evicted() {
        let broker = HttpStreamBroker::new();
        let old = broker.create_stream().await;
        broker.register_session("session-x", &old).await;
        broker.publish(&old, "done", json!({})).await;

        // The session moves on to a fresh stream (a follow-up turn).
        let new = broker.create_stream().await;
        broker.register_session("session-x", &new).await;

        broker.force_expire_done(&old).await;
        let _trigger = broker.create_stream().await;

        // The old finished stream is evicted, but the session mapping now points
        // at the newer stream and must be preserved.
        assert!(broker.subscribe(&old).await.is_none());
        assert_eq!(
            broker.stream_id_for_session("session-x").await,
            Some(new)
        );
    }

    // ---- P2-2: Lagged resync replays the skipped range from history ----

    #[tokio::test]
    async fn lagged_subscriber_resyncs_skipped_range_from_history() {
        use futures::StreamExt;

        let broker = Arc::new(HttpStreamBroker::new());
        let stream_id = broker.create_stream().await;

        // Subscribe (capacity 256) but do NOT consume, then publish far more than
        // the channel capacity so the broadcast drops events for this receiver —
        // forcing a `Lagged` on the first recv.
        let (history, receiver) = broker.subscribe(&stream_id).await.expect("stream exists");
        assert!(history.is_empty());
        let total = 400usize; // > broadcast capacity (256), < HISTORY_CAPACITY (512)
        for i in 0..total {
            broker
                .publish(&stream_id, "token", json!({"text": format!("tok-{i}")}))
                .await;
        }
        broker
            .publish(&stream_id, "final", json!({"text": "the answer"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        // Drive the replay core directly so we can inspect the frame sequence.
        let frames = replay_stream(broker.clone(), stream_id.clone(), history, receiver);
        futures::pin_mut!(frames);
        let mut saw_resync = false;
        let mut replayed_tokens: Vec<String> = Vec::new();
        let mut saw_final = false;
        let mut saw_done = false;
        while let Some(frame) = frames.next().await {
            match frame {
                StreamFrame::Resync { skipped, .. } => {
                    saw_resync = true;
                    assert!(skipped > 0, "Lagged must report a positive skip count");
                }
                StreamFrame::Event(event) => match event.event.as_str() {
                    "token" => {
                        replayed_tokens
                            .push(event.payload["text"].as_str().unwrap().to_string());
                    }
                    "final" => saw_final = true,
                    "done" => saw_done = true,
                    _ => {}
                },
            }
        }

        assert!(saw_resync, "a Lagged gap must emit a resync frame, not be skipped silently");
        assert!(saw_final, "the terminal final must still be replayed after resync");
        assert!(saw_done, "the terminal done must still be replayed so the client stops");
        // The resync replays the retained history tail (the ring keeps the newest
        // HISTORY_CAPACITY events) including the most recent tokens.
        assert!(
            replayed_tokens.iter().any(|t| t == "tok-399"),
            "the newest token before the terminal must be replayed"
        );
        // Tokens are replayed in order, each exactly once (no duplication of the
        // already-delivered prefix — nothing had been delivered before the lag).
        let mut sorted = replayed_tokens.clone();
        sorted.dedup();
        assert_eq!(sorted.len(), replayed_tokens.len(), "no duplicate tokens after resync");
    }

    #[tokio::test]
    async fn non_lagged_stream_emits_no_resync_frame() {
        use futures::StreamExt;

        let broker = Arc::new(HttpStreamBroker::new());
        let stream_id = broker.create_stream().await;
        broker
            .publish(&stream_id, "token", json!({"text": "hi"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        let (history, receiver) = broker.subscribe(&stream_id).await.expect("stream exists");
        let frames = replay_stream(broker.clone(), stream_id, history, receiver);
        futures::pin_mut!(frames);
        let mut resyncs = 0;
        while let Some(frame) = frames.next().await {
            if matches!(frame, StreamFrame::Resync { .. }) {
                resyncs += 1;
            }
        }
        assert_eq!(resyncs, 0, "a healthy stream must never emit a resync frame");
    }
}
