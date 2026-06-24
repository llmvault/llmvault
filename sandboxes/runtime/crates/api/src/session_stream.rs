use std::collections::{HashMap, VecDeque};
use std::convert::Infallible;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Instant;

use anyhow::Result;
use async_stream::stream;
use axum::http::{HeaderName, HeaderValue};
use axum::response::sse::{Event, Sse};
use axum::response::{IntoResponse, Response};
use chrono::{DateTime, Utc};
use domain::{Attachment, InboundEvent, ModelConfig, SessionId};
use observability::{
    parse_model_usage, EventTimings, ObservabilityEvent, ObservabilityEventType,
    ObservabilityRecorder, ToolUsage,
};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::sync::{broadcast, Mutex};
use tokio::time::Duration;

#[derive(Clone)]
pub struct SessionMessageState {
    pub inbound_sink: tokio::sync::mpsc::Sender<InboundEvent>,
    pub broker: Arc<SessionStreamBroker>,
    pub interrupter: Option<Arc<dyn SessionInterrupter>>,
}

#[async_trait::async_trait]
pub trait SessionInterrupter: Send + Sync + 'static {
    async fn interrupt_session(&self, session_id: &SessionId) -> bool;
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SessionStreamEvent {
    pub event: String,
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub payload: Value,
}

#[derive(Default)]
pub struct SessionStreamBroker {
    streams: Mutex<HashMap<String, StreamState>>,
    session_streams: Mutex<HashMap<String, String>>,
    stream_sessions: Mutex<HashMap<String, String>>,
    active_session_streams: Mutex<HashMap<String, String>>,
    session_model_definitions: Mutex<HashMap<String, ModelConfig>>,
    observability: ObservabilityRecorder,
    counter: AtomicU64,
    turn_counter: AtomicU64,
}

/// Maximum number of events retained for replay to late/reconnecting
/// subscribers. The buffer is a ring: once full, the oldest event is evicted so
/// the newest events — crucially the turn terminal events — are always
/// replayable.
const HISTORY_CAPACITY: usize = 512;

/// How long a finished stream's state is retained before it is evicted
/// from the broker's maps. Persistent session streams are not marked done.
const STREAM_EVICTION_GRACE: Duration = Duration::from_secs(120);

/// A published event paired with a monotonically increasing per-stream sequence
/// number. The sequence lets a subscriber that fell behind (`RecvError::Lagged`)
/// resync precisely against the replay history — replaying exactly the events it
/// skipped — instead of silently dropping the gap. Derefs to the inner
/// [`SessionStreamEvent`] so `.event`/`.payload` are accessible directly.
#[derive(Clone)]
pub struct SeqEvent {
    pub seq: u64,
    pub inner: SessionStreamEvent,
}

impl std::ops::Deref for SeqEvent {
    type Target = SessionStreamEvent;
    fn deref(&self) -> &Self::Target {
        &self.inner
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum StreamReplayMode {
    All,
    None,
    AfterSeq(u64),
    FromTurnId(String),
    FromTurnIdFollow(String),
}

pub struct StreamSubscription {
    history: Vec<SeqEvent>,
    receiver: broadcast::Receiver<SeqEvent>,
    initial_last_seq: Option<u64>,
    next_seq: u64,
    resync_required: Option<StreamResyncRequired>,
    turn_filter: Option<String>,
}

#[cfg(test)]
impl StreamSubscription {
    pub(crate) fn into_parts(self) -> (Vec<SeqEvent>, broadcast::Receiver<SeqEvent>) {
        (self.history, self.receiver)
    }

    pub(crate) fn into_receiver_for_test(self) -> broadcast::Receiver<SeqEvent> {
        self.receiver
    }
}

#[derive(Debug, Clone)]
struct ResyncRequired {
    requested_after_seq: u64,
    earliest_seq: u64,
    latest_seq: u64,
}

#[derive(Debug, Clone)]
struct TurnResyncRequired {
    turn_id: String,
    reason: &'static str,
    earliest_seq: Option<u64>,
    latest_seq: Option<u64>,
}

#[derive(Debug, Clone)]
enum StreamResyncRequired {
    Cursor(ResyncRequired),
    Turn(TurnResyncRequired),
}

struct StreamState {
    sender: broadcast::Sender<SeqEvent>,
    history: VecDeque<SeqEvent>,
    turn_contexts: HashMap<String, StreamObservabilityContext>,
    /// Next sequence number to assign to a published event.
    next_seq: u64,
    /// When the terminal `done` was published, after which the stream becomes
    /// eligible for eviction once `STREAM_EVICTION_GRACE` has elapsed.
    done_at: Option<Instant>,
    persistent: bool,
}

#[derive(Debug, Clone)]
struct StreamObservabilityContext {
    trace_id: String,
    session_id: String,
    turn_id: String,
    run_id: String,
    started_at: DateTime<Utc>,
}

impl SessionStreamBroker {
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
        self.create_stream_with_persistence(false).await
    }

    async fn create_stream_with_persistence(&self, persistent: bool) -> String {
        let id = format!(
            "session-stream-{}-{}-{}",
            Utc::now().timestamp_millis(),
            self.counter.fetch_add(1, Ordering::Relaxed),
            nanoid::nanoid!(24)
        );
        let (sender, _) = broadcast::channel(256);
        let mut streams = self.streams.lock().await;
        // Opportunistically reclaim finished streams whose grace window has
        // elapsed. Done lazily here (and in publish) to avoid a background
        // sweeper task; create_stream runs once per turn so the maps stay bounded.
        evict_expired_streams(
            &mut streams,
            &self.session_streams,
            &self.stream_sessions,
            &self.active_session_streams,
        )
        .await;
        streams.insert(
            id.clone(),
            StreamState {
                sender,
                history: VecDeque::with_capacity(HISTORY_CAPACITY),
                turn_contexts: HashMap::new(),
                next_seq: 0,
                done_at: None,
                persistent,
            },
        );
        drop(streams);
        id
    }

    pub async fn get_or_create_session_stream(&self, session_id: &str) -> String {
        if let Some(stream_id) = self.session_streams.lock().await.get(session_id).cloned() {
            if self.streams.lock().await.contains_key(&stream_id) {
                return stream_id;
            }
        }
        let stream_id = self.create_stream_with_persistence(true).await;
        self.register_session(session_id, &stream_id).await;
        stream_id
    }

    pub async fn start_turn(
        &self,
        stream_id: &str,
        session_id: &str,
        text_len: usize,
    ) -> Option<(String, String)> {
        let now = Utc::now();
        let suffix = format!(
            "{}-{}-{}",
            Utc::now().timestamp_millis(),
            self.turn_counter.fetch_add(1, Ordering::Relaxed),
            nanoid::nanoid!(16)
        );
        let trace_id = format!("trace-{suffix}");
        let turn_id = format!("turn-{suffix}");
        let run_id = format!("run-{suffix}");
        let context = StreamObservabilityContext {
            trace_id: trace_id.clone(),
            session_id: session_id.to_string(),
            turn_id: turn_id.clone(),
            run_id,
            started_at: now,
        };

        let mut streams = self.streams.lock().await;
        let state = streams.get_mut(stream_id)?;
        state.turn_contexts.insert(turn_id.clone(), context.clone());
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
        let event_name = event.into();
        let mut context = None;
        let mut recorded_event = None;
        let mut streams = self.streams.lock().await;
        if let Some(state) = streams.get_mut(stream_id) {
            let seq = state.next_seq;
            state.next_seq += 1;
            let event = SessionStreamEvent {
                event: event_name.clone(),
                payload: stamp_stream_payload(payload, &event_name, stream_id, seq),
            };
            let seq_event = SeqEvent {
                seq,
                inner: event.clone(),
            };
            // Ring buffer: evict the oldest event when full so turn terminal
            // events at the tail are always retained for replay.
            if state.history.len() == HISTORY_CAPACITY {
                state.history.pop_front();
            }
            state.history.push_back(seq_event.clone());
            if event.event == "done" && !state.persistent && state.done_at.is_none() {
                // Start the eviction grace window; the state lingers long enough
                // for late subscribers to replay the terminal events.
                state.done_at = Some(Instant::now());
            }
            context = event
                .payload
                .get("turn_id")
                .and_then(Value::as_str)
                .and_then(|turn_id| state.turn_contexts.get(turn_id).cloned())
                .or_else(|| {
                    if state.turn_contexts.len() == 1 {
                        state.turn_contexts.values().next().cloned()
                    } else {
                        None
                    }
                });
            recorded_event = Some(event);
            let _ = state.sender.send(seq_event);
        }
        // Reclaim any streams whose grace window has now elapsed.
        evict_expired_streams(
            &mut streams,
            &self.session_streams,
            &self.stream_sessions,
            &self.active_session_streams,
        )
        .await;
        drop(streams);
        if let (Some(context), Some(event)) = (context.as_ref(), recorded_event.as_ref()) {
            self.record_stream_event(context, &event.event, &event.payload);
        }
    }

    pub async fn register_session(&self, session_id: &str, stream_id: &str) {
        self.session_streams
            .lock()
            .await
            .insert(session_id.to_string(), stream_id.to_string());
        self.stream_sessions
            .lock()
            .await
            .insert(stream_id.to_string(), session_id.to_string());
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

    pub async fn stream_belongs_to_session(&self, session_id: &str, stream_id: &str) -> bool {
        self.stream_sessions
            .lock()
            .await
            .get(stream_id)
            .is_some_and(|owner| owner == session_id)
    }

    pub async fn model_definition_for_session(&self, session_id: &str) -> Option<ModelConfig> {
        self.session_model_definitions
            .lock()
            .await
            .get(session_id)
            .cloned()
    }

    pub async fn set_model_definition_for_session(&self, session_id: &str, model: ModelConfig) {
        self.session_model_definitions
            .lock()
            .await
            .insert(session_id.to_string(), model);
    }

    pub async fn subscribe(
        &self,
        stream_id: &str,
        replay_mode: StreamReplayMode,
    ) -> Option<StreamSubscription> {
        let streams = self.streams.lock().await;
        let state = streams.get(stream_id)?;
        let latest_seq = state.next_seq.checked_sub(1);
        let earliest_seq = state.history.front().map(|item| item.seq);
        let mut resync_required = None;
        let mut turn_filter = None;
        let (history, initial_last_seq) = match replay_mode {
            StreamReplayMode::All => (state.history.iter().cloned().collect(), None),
            StreamReplayMode::None => (Vec::new(), latest_seq),
            StreamReplayMode::AfterSeq(after_seq) => {
                if let (Some(first), Some(latest)) = (earliest_seq, latest_seq) {
                    if after_seq < first.saturating_sub(1) {
                        resync_required = Some(StreamResyncRequired::Cursor(ResyncRequired {
                            requested_after_seq: after_seq,
                            earliest_seq: first,
                            latest_seq: latest,
                        }));
                        (Vec::new(), Some(latest))
                    } else {
                        (
                            state
                                .history
                                .iter()
                                .filter(|item| item.seq > after_seq)
                                .cloned()
                                .collect(),
                            Some(after_seq),
                        )
                    }
                } else {
                    (Vec::new(), Some(after_seq))
                }
            }
            StreamReplayMode::FromTurnId(turn_id) => {
                turn_filter = Some(turn_id.clone());
                let turn_start_seq = state
                    .history
                    .iter()
                    .find(|item| {
                        item.inner.event == "turn_started"
                            && event_turn_id(&item.inner).as_deref() == Some(turn_id.as_str())
                    })
                    .map(|item| item.seq);
                let Some(turn_start_seq) = turn_start_seq else {
                    resync_required = Some(StreamResyncRequired::Turn(TurnResyncRequired {
                        turn_id,
                        reason: "turn_not_retained",
                        earliest_seq,
                        latest_seq,
                    }));
                    return Some(StreamSubscription {
                        history: Vec::new(),
                        receiver: state.sender.subscribe(),
                        initial_last_seq: latest_seq,
                        next_seq: state.next_seq,
                        resync_required,
                        turn_filter,
                    });
                };
                let matching = state
                    .history
                    .iter()
                    .filter(|item| {
                        item.seq >= turn_start_seq
                            && event_turn_id(&item.inner).as_deref() == Some(turn_id.as_str())
                    })
                    .cloned()
                    .collect::<Vec<_>>();
                let last_seq = matching.last().map(|item| item.seq);
                (matching, last_seq)
            }
            StreamReplayMode::FromTurnIdFollow(turn_id) => {
                let turn_start_seq = state
                    .history
                    .iter()
                    .find(|item| {
                        item.inner.event == "turn_started"
                            && event_turn_id(&item.inner).as_deref() == Some(turn_id.as_str())
                    })
                    .map(|item| item.seq);
                let Some(turn_start_seq) = turn_start_seq else {
                    resync_required = Some(StreamResyncRequired::Turn(TurnResyncRequired {
                        turn_id,
                        reason: "turn_not_retained",
                        earliest_seq,
                        latest_seq,
                    }));
                    return Some(StreamSubscription {
                        history: Vec::new(),
                        receiver: state.sender.subscribe(),
                        initial_last_seq: latest_seq,
                        next_seq: state.next_seq,
                        resync_required,
                        turn_filter,
                    });
                };
                let matching = state
                    .history
                    .iter()
                    .filter(|item| item.seq >= turn_start_seq)
                    .cloned()
                    .collect::<Vec<_>>();
                let last_seq = matching.last().map(|item| item.seq);
                (matching, last_seq)
            }
        };
        Some(StreamSubscription {
            history,
            receiver: state.sender.subscribe(),
            initial_last_seq,
            next_seq: state.next_seq,
            resync_required,
            turn_filter,
        })
    }

    /// Replay-history events with sequence number strictly greater than
    /// `after_seq`, used to resync a subscriber that fell behind
    /// (`RecvError::Lagged`) by replaying exactly the events it skipped.
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
            // start_turn records the durable observability TurnStarted event;
            // the stream frame is for live browser consumers.
            "turn_started" => return,
            "thinking" | "token" => ObservabilityEventType::AssistantDelta,
            "tool_call" => ObservabilityEventType::ToolCall,
            "tool_result" => ObservabilityEventType::ToolResult,
            "model_usage" => ObservabilityEventType::ModelUsage,
            "turn_completed" | "final" => ObservabilityEventType::TurnCompleted,
            "done" => ObservabilityEventType::RunCompleted,
            "turn_failed"
            | "model_request_failed"
            | "model_stream_failed"
            | "max_turns_exhausted"
            | "error" => ObservabilityEventType::Error,
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
            | "turn_failed"
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

fn stamp_stream_payload(payload: Value, event_name: &str, stream_id: &str, sequence: u64) -> Value {
    let mut payload = match payload {
        Value::Object(map) => Value::Object(map),
        value => json!({ "value": value }),
    };
    let Some(map) = payload.as_object_mut() else {
        return payload;
    };
    map.entry("event_name".to_string())
        .or_insert_with(|| json!(event_name));
    map.entry("stream_id".to_string())
        .or_insert_with(|| json!(stream_id));
    map.entry("sequence".to_string())
        .or_insert_with(|| json!(sequence));
    map.entry("event_id".to_string())
        .or_insert_with(|| json!(format!("evt_stream_{stream_id}_{sequence}")));
    map.entry("occurred_at".to_string())
        .or_insert_with(|| json!(Utc::now()));
    map.entry("scope".to_string())
        .or_insert_with(|| json!("main"));
    payload
}

/// Reclaim per-stream state whose terminal `done` was published more
/// than `STREAM_EVICTION_GRACE` ago, and drop the session→stream mappings that
/// still point at them. Without this the broker's `streams`/`session_streams`
/// maps grow unbounded for the process lifetime. Called opportunistically from
/// `create_stream`/`publish` while the `streams` lock is held; lock order is
/// always streams → session_streams → active_session_streams to avoid deadlock.
async fn evict_expired_streams(
    streams: &mut HashMap<String, StreamState>,
    session_streams: &Mutex<HashMap<String, String>>,
    stream_sessions: &Mutex<HashMap<String, String>>,
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
    let expired_set: std::collections::HashSet<&str> = expired.iter().map(String::as_str).collect();
    // Drop session mappings that still point at an evicted stream. A session
    // re-registered onto a fresh stream keeps its newer mapping.
    let mut session = session_streams.lock().await;
    session.retain(|_, stream_id| !expired_set.contains(stream_id.as_str()));
    drop(session);
    let mut stream_owners = stream_sessions.lock().await;
    stream_owners.retain(|stream_id, _| !expired_set.contains(stream_id.as_str()));
    drop(stream_owners);
    let mut active = active_session_streams.lock().await;
    active.retain(|_, stream_id| !expired_set.contains(stream_id.as_str()));
}

#[derive(Debug, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SessionMessageRequest {
    pub text: String,
    #[serde(default = "default_session_user")]
    pub user: String,
    #[serde(default)]
    pub user_display_name: Option<String>,
    #[serde(default)]
    pub attachments: Vec<Attachment>,
    #[serde(default)]
    pub dynamic_context: Vec<String>,
    #[serde(default)]
    pub model_definition: Option<ModelConfig>,
    #[serde(default)]
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub raw: Value,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SessionMessageResponse {
    pub session_id: String,
    pub stream_id: String,
    pub stream_url: String,
    pub trace_id: String,
    pub turn_id: String,
}

impl SessionMessageState {
    pub async fn inject_message(
        &self,
        session_id: SessionId,
        request: SessionMessageRequest,
    ) -> Result<SessionMessageResponse> {
        let stream_id = self
            .broker
            .get_or_create_session_stream(session_id.as_str())
            .await;
        let (trace_id, turn_id) = self
            .broker
            .start_turn(&stream_id, session_id.as_str(), request.text.len())
            .await
            .unwrap_or_else(|| (format!("trace-{stream_id}"), format!("turn-{stream_id}")));
        self.broker
            .publish(
                &stream_id,
                "turn_started",
                json!({
                    "session_id": session_id.as_str(),
                    "trace_id": &trace_id,
                    "turn_id": &turn_id,
                    "input_text_len": request.text.len(),
                }),
            )
            .await;
        let source = request
            .raw
            .get("source")
            .and_then(Value::as_str)
            .filter(|value| matches!(*value, "session" | "web"))
            .unwrap_or("session");
        let mut raw = json!({
            "source": source,
            "session_stream_id": stream_id,
            "trace_id": trace_id,
            "turn_id": turn_id,
            "raw": request.raw.clone(),
        });
        if let Some(raw_obj) = request.raw.as_object() {
            if let Some(out) = raw.as_object_mut() {
                for key in [
                    "job_id",
                    "schedule_id",
                    "schedule_run_id",
                    "schedule_run_key",
                    "schedule_scheduled_at",
                    "schedule_started_at",
                    "schedule_is_one_shot",
                ] {
                    if let Some(value) = raw_obj.get(key) {
                        out.insert(key.to_string(), value.clone());
                    }
                }
            }
        }
        let model_definition = match request.model_definition {
            Some(model) => {
                self.broker
                    .set_model_definition_for_session(session_id.as_str(), model.clone())
                    .await;
                Some(model)
            }
            None => {
                self.broker
                    .model_definition_for_session(session_id.as_str())
                    .await
            }
        };
        let inbound = InboundEvent {
            envelope_id: stream_id.clone(),
            session_id: session_id.clone(),
            user: request.user,
            user_display_name: request.user_display_name,
            text: request.text,
            attachments: request.attachments,
            dynamic_context: request.dynamic_context,
            model_definition,
            raw,
            is_direct_message: true,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        };
        self.inbound_sink.send(inbound).await?;
        Ok(SessionMessageResponse {
            session_id: session_id.as_str().to_string(),
            stream_id: stream_id.clone(),
            stream_url: format!("/sessions/{}/stream", session_id.as_str()),
            trace_id,
            turn_id,
        })
    }
}

/// A frame produced by the replay stream: either a published event or a
/// synthetic `resync` marker inserted after a `Lagged` gap.
#[derive(Debug, Clone)]
enum StreamFrame {
    Event(SessionStreamEvent),
    Resync { skipped: u64, from_seq: Option<u64> },
    ResyncRequired(StreamResyncRequired),
}

/// Core of the SSE replay stream, decoupled from the `Sse`/`Event` wire types so
/// the reconnect/resync behavior is unit-testable. Yields history first, then
/// live events, and on `Lagged` emits a `Resync` frame followed by the exact
/// missed range from history. Stream-id subscribers stop on `done`;
/// stable session stream subscribers keep listening across turn boundaries.
fn replay_stream(
    broker: Arc<SessionStreamBroker>,
    stream_id: String,
    stop_on_done: bool,
    subscription: StreamSubscription,
) -> impl futures::Stream<Item = StreamFrame> {
    let StreamSubscription {
        history,
        mut receiver,
        initial_last_seq,
        resync_required,
        turn_filter,
        ..
    } = subscription;
    stream! {
        // Track the highest sequence number delivered to this subscriber so a
        // `Lagged` gap can be resynced precisely from history.
        let mut last_seq = initial_last_seq;
        if let Some(resync) = resync_required {
            yield StreamFrame::ResyncRequired(resync);
        }
        for item in history {
            last_seq = Some(item.seq);
            let is_done = item.inner.event == "done";
            let is_turn_terminal = turn_filter
                .as_deref()
                .is_some_and(|turn_id| is_terminal_turn_event(&item.inner, turn_id));
            yield StreamFrame::Event(item.inner);
            if stop_on_done && is_done || is_turn_terminal {
                return;
            }
        }
        loop {
            match receiver.recv().await {
                Ok(item) => {
                    last_seq = Some(item.seq);
                    if let Some(turn_id) = turn_filter.as_deref() {
                        if event_turn_id(&item.inner).as_deref() != Some(turn_id) {
                            continue;
                        }
                    }
                    let is_done = item.inner.event == "done";
                    let is_turn_terminal = turn_filter
                        .as_deref()
                        .is_some_and(|turn_id| is_terminal_turn_event(&item.inner, turn_id));
                    yield StreamFrame::Event(item.inner);
                    if stop_on_done && is_done || is_turn_terminal {
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
                    let mut turn_done = false;
                    for item in missed {
                        last_seq = Some(item.seq);
                        if let Some(turn_id) = turn_filter.as_deref() {
                            if event_turn_id(&item.inner).as_deref() != Some(turn_id) {
                                continue;
                            }
                            if is_terminal_turn_event(&item.inner, turn_id) {
                                turn_done = true;
                            }
                        }
                        if item.inner.event == "done" {
                            done = true;
                        }
                        yield StreamFrame::Event(item.inner);
                    }
                    if stop_on_done && done || turn_done {
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
        StreamFrame::ResyncRequired(resync) => match resync {
            StreamResyncRequired::Cursor(resync) => Event::default()
                .event("resync_required")
                .json_data(json!({
                    "stream_id": stream_id,
                    "requested_after_seq": resync.requested_after_seq,
                    "earliest_seq": resync.earliest_seq,
                    "latest_seq": resync.latest_seq,
                }))
                .unwrap_or_else(|_| Event::default().event("resync_required").data("{}")),
            StreamResyncRequired::Turn(resync) => Event::default()
                .event("resync_required")
                .json_data(json!({
                    "stream_id": stream_id,
                    "from_turn_id": resync.turn_id,
                    "reason": resync.reason,
                    "earliest_seq": resync.earliest_seq,
                    "latest_seq": resync.latest_seq,
                }))
                .unwrap_or_else(|_| Event::default().event("resync_required").data("{}")),
        },
    }
}

pub struct SessionStreamResponse<S> {
    stream_id: String,
    next_sequence: u64,
    sse: Sse<S>,
}

impl<S> IntoResponse for SessionStreamResponse<S>
where
    S: futures::Stream<Item = Result<Event, Infallible>> + Send + 'static,
{
    fn into_response(self) -> Response {
        let mut response = self.sse.into_response();
        let headers = response.headers_mut();
        if let Ok(value) = HeaderValue::from_str(&self.stream_id) {
            headers.insert(HeaderName::from_static("x-hivy-stream-id"), value);
        }
        if let Ok(value) = HeaderValue::from_str(&self.next_sequence.to_string()) {
            headers.insert(
                HeaderName::from_static("x-hivy-stream-next-sequence"),
                value,
            );
        }
        response
    }
}

pub async fn stream_response(
    broker: Arc<SessionStreamBroker>,
    session_id: String,
    stream_id: String,
    replay_mode: StreamReplayMode,
) -> Option<SessionStreamResponse<impl futures::Stream<Item = Result<Event, Infallible>>>> {
    if !broker
        .stream_belongs_to_session(&session_id, &stream_id)
        .await
    {
        return None;
    }
    let subscription = broker.subscribe(&stream_id, replay_mode).await?;
    let next_sequence = subscription.next_seq;
    let frames = replay_stream(broker, stream_id.clone(), true, subscription);
    let output_stream_id = stream_id.clone();
    let output = stream! {
        use futures::StreamExt;
        futures::pin_mut!(frames);
        while let Some(frame) = frames.next().await {
            yield Ok(stream_frame_to_sse(&output_stream_id, frame));
        }
    };
    Some(SessionStreamResponse {
        stream_id,
        next_sequence,
        sse: Sse::new(output).keep_alive(
            axum::response::sse::KeepAlive::new()
                .interval(Duration::from_secs(15))
                .text("keep-alive"),
        ),
    })
}

pub async fn session_stream_response(
    broker: Arc<SessionStreamBroker>,
    session_id: String,
    replay_mode: StreamReplayMode,
) -> SessionStreamResponse<impl futures::Stream<Item = Result<Event, Infallible>>> {
    let stream_id = broker.get_or_create_session_stream(&session_id).await;
    let subscription = broker
        .subscribe(&stream_id, replay_mode)
        .await
        .expect("session stream was just created");
    let next_sequence = subscription.next_seq;
    let frames = replay_stream(broker, stream_id.clone(), false, subscription);
    let output_stream_id = stream_id.clone();
    let output = stream! {
        use futures::StreamExt;
        futures::pin_mut!(frames);
        while let Some(frame) = frames.next().await {
            yield Ok(stream_frame_to_sse(&output_stream_id, frame));
        }
    };
    SessionStreamResponse {
        stream_id,
        next_sequence,
        sse: Sse::new(output).keep_alive(
            axum::response::sse::KeepAlive::new()
                .interval(Duration::from_secs(15))
                .text("keep-alive"),
        ),
    }
}

fn to_sse_event(item: SessionStreamEvent) -> Event {
    Event::default()
        .event(item.event)
        .json_data(item.payload)
        .unwrap_or_else(|_| Event::default().event("error").data("serialize event"))
}

fn event_turn_id(event: &SessionStreamEvent) -> Option<String> {
    event
        .payload
        .get("turn_id")
        .and_then(Value::as_str)
        .filter(|value| !value.trim().is_empty())
        .map(str::to_string)
}

fn is_terminal_turn_event(event: &SessionStreamEvent, turn_id: &str) -> bool {
    if event_turn_id(event).as_deref() != Some(turn_id) {
        return false;
    }
    matches!(
        event.event.as_str(),
        "turn_completed" | "turn_failed" | "turn_interrupted" | "done"
    )
}

fn default_session_user() -> String {
    "session".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use futures::StreamExt;
    use tokio::sync::mpsc;

    fn test_model(model_id: &str) -> ModelConfig {
        ModelConfig::OpenaiCompatible {
            base_url: "http://127.0.0.1:18082/v1".to_string(),
            model_id: model_id.to_string(),
            canonical_model_id: Some(model_id.to_string()),
            provider_id: Some("openrouter".to_string()),
            upstream_model_id: Some(model_id.to_string()),
            model_profile: None,
            provider_options: HashMap::new(),
            capabilities: None,
            api_key_env: "HIVY_PROXY_API_KEY".to_string(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: Some(domain::ReasoningEffort::Low),
            extra_headers: HashMap::new(),
            fallback: None,
        }
    }

    fn model_id(model: &ModelConfig) -> &str {
        match model {
            ModelConfig::OpenaiCompatible { model_id, .. } => model_id,
        }
    }

    #[tokio::test]
    async fn inject_message_uses_caller_supplied_session_id_and_stream_mapping() {
        let (tx, mut rx) = mpsc::channel(1);
        let broker = Arc::new(SessionStreamBroker::new());
        let sessions = SessionMessageState {
            inbound_sink: tx,
            broker: broker.clone(),
            interrupter: None,
        };

        let response = sessions
            .inject_message(
                SessionId::from("session-1"),
                SessionMessageRequest {
                    text: "hello".to_string(),
                    user: "user-1".to_string(),
                    user_display_name: Some("User One".to_string()),
                    attachments: Vec::new(),
                    dynamic_context: vec!["## Recent sessions\n- prior context".to_string()],
                    model_definition: None,
                    raw: json!({"source": "session", "provider": "test-provider"}),
                },
            )
            .await
            .expect("inject message");

        assert_eq!(response.session_id, "session-1");
        assert!(response.trace_id.starts_with("trace-"));
        assert!(response.turn_id.starts_with("turn-"));
        assert_eq!(response.stream_url, "/sessions/session-1/stream");
        assert_eq!(
            broker.stream_id_for_session("session-1").await,
            Some(response.stream_id.clone())
        );
        assert!(
            broker
                .stream_belongs_to_session("session-1", &response.stream_id)
                .await
        );

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(inbound.session_id.as_str(), "session-1");
        assert_eq!(inbound.user, "user-1");
        assert_eq!(inbound.text, "hello");
        assert_eq!(
            inbound.dynamic_context,
            vec!["## Recent sessions\n- prior context"]
        );
        assert_eq!(inbound.raw["session_stream_id"], response.stream_id);
        assert_eq!(inbound.raw["trace_id"], response.trace_id);
        assert_eq!(inbound.raw["turn_id"], response.turn_id);
        assert_eq!(inbound.raw["source"], "session");
        assert_eq!(inbound.raw["raw"]["provider"], "test-provider");
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
    async fn inject_message_preserves_web_source() {
        let (tx, mut rx) = mpsc::channel(1);
        let sessions = SessionMessageState {
            inbound_sink: tx,
            broker: Arc::new(SessionStreamBroker::new()),
            interrupter: None,
        };

        sessions
            .inject_message(
                SessionId::from("web-session"),
                SessionMessageRequest {
                    text: "hello from web".to_string(),
                    user: "user-1".to_string(),
                    user_display_name: Some("Ada".to_string()),
                    attachments: Vec::new(),
                    dynamic_context: Vec::new(),
                    model_definition: None,
                    raw: json!({"source": "web"}),
                },
            )
            .await
            .expect("inject web message");

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(inbound.session_id.as_str(), "web-session");
        assert_eq!(inbound.raw["source"], "web");
    }

    #[tokio::test]
    async fn inject_message_reuses_session_model_definition() {
        let (tx, mut rx) = mpsc::channel(2);
        let sessions = SessionMessageState {
            inbound_sink: tx,
            broker: Arc::new(SessionStreamBroker::new()),
            interrupter: None,
        };

        sessions
            .inject_message(
                SessionId::from("model-session"),
                SessionMessageRequest {
                    text: "first".to_string(),
                    user: "user-1".to_string(),
                    user_display_name: None,
                    attachments: Vec::new(),
                    dynamic_context: Vec::new(),
                    model_definition: Some(test_model("qwen-3.7-plus")),
                    raw: json!({"source": "session"}),
                },
            )
            .await
            .expect("first message");
        sessions
            .inject_message(
                SessionId::from("model-session"),
                SessionMessageRequest {
                    text: "second".to_string(),
                    user: "user-1".to_string(),
                    user_display_name: None,
                    attachments: Vec::new(),
                    dynamic_context: Vec::new(),
                    model_definition: None,
                    raw: json!({"source": "session"}),
                },
            )
            .await
            .expect("second message");

        let first = rx.recv().await.expect("first inbound event");
        let second = rx.recv().await.expect("second inbound event");
        assert_eq!(
            model_id(first.model_definition.as_ref().expect("first model")),
            "qwen-3.7-plus"
        );
        assert_eq!(
            model_id(second.model_definition.as_ref().expect("second model")),
            "qwen-3.7-plus"
        );
    }

    #[tokio::test]
    async fn inject_message_reuses_stable_stream_for_session_turns() {
        let (tx, mut rx) = mpsc::channel(2);
        let sessions = SessionMessageState {
            inbound_sink: tx,
            broker: Arc::new(SessionStreamBroker::new()),
            interrupter: None,
        };

        let first = sessions
            .inject_message(
                SessionId::from("session-stable"),
                SessionMessageRequest {
                    text: "first".to_string(),
                    user: "user-1".to_string(),
                    user_display_name: None,
                    attachments: Vec::new(),
                    dynamic_context: Vec::new(),
                    model_definition: None,
                    raw: json!({"source": "session"}),
                },
            )
            .await
            .expect("first message");
        let second = sessions
            .inject_message(
                SessionId::from("session-stable"),
                SessionMessageRequest {
                    text: "second".to_string(),
                    user: "user-1".to_string(),
                    user_display_name: None,
                    attachments: Vec::new(),
                    dynamic_context: Vec::new(),
                    model_definition: None,
                    raw: json!({"source": "session"}),
                },
            )
            .await
            .expect("second message");

        assert_eq!(first.stream_id, second.stream_id);
        assert_eq!(first.stream_url, "/sessions/session-stable/stream");
        assert_eq!(second.stream_url, first.stream_url);
        assert_ne!(first.trace_id, second.trace_id);
        assert_ne!(first.turn_id, second.turn_id);

        let first_inbound = rx.recv().await.expect("first inbound");
        let second_inbound = rx.recv().await.expect("second inbound");
        assert_eq!(
            first_inbound.raw["session_stream_id"],
            second_inbound.raw["session_stream_id"]
        );
        assert_ne!(first_inbound.raw["turn_id"], second_inbound.raw["turn_id"]);
    }

    #[tokio::test]
    async fn stable_session_stream_replay_does_not_stop_on_done() {
        let broker = Arc::new(SessionStreamBroker::new());
        let stream_id = broker.get_or_create_session_stream("session-stable").await;
        let subscription = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stable stream exists");
        let frames = replay_stream(broker.clone(), stream_id.clone(), false, subscription);
        futures::pin_mut!(frames);

        broker
            .publish(&stream_id, "final", json!({"text": "first"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;
        broker
            .publish(&stream_id, "final", json!({"text": "second"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        let mut names = Vec::new();
        for _ in 0..4 {
            let Some(StreamFrame::Event(event)) = frames.next().await else {
                panic!("stable stream ended before replaying multiple turns");
            };
            names.push(event.event);
        }
        assert_eq!(names, vec!["final", "done", "final", "done"]);
    }

    #[tokio::test]
    async fn session_stream_response_sets_cursor_headers() {
        let broker = Arc::new(SessionStreamBroker::new());
        let stream_id = broker.get_or_create_session_stream("session-headers").await;
        broker
            .publish(&stream_id, "token", json!({"text": "hello"}))
            .await;

        let response = session_stream_response(
            broker,
            "session-headers".to_string(),
            StreamReplayMode::None,
        )
        .await
        .into_response();

        assert_eq!(
            response.headers()["x-hivy-stream-id"],
            HeaderValue::from_str(&stream_id).unwrap()
        );
        assert_eq!(
            response.headers()["x-hivy-stream-next-sequence"],
            HeaderValue::from_static("1")
        );
    }

    #[tokio::test]
    async fn inject_message_rejects_unknown_source_to_session_default() {
        let (tx, mut rx) = mpsc::channel(1);
        let sessions = SessionMessageState {
            inbound_sink: tx,
            broker: Arc::new(SessionStreamBroker::new()),
            interrupter: None,
        };

        sessions
            .inject_message(
                SessionId::from("session-2"),
                SessionMessageRequest {
                    text: "hello".to_string(),
                    user: "user-1".to_string(),
                    user_display_name: None,
                    attachments: Vec::new(),
                    dynamic_context: Vec::new(),
                    model_definition: None,
                    raw: json!({"source": "external"}),
                },
            )
            .await
            .expect("inject message");

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(inbound.raw["source"], "session");
    }

    #[tokio::test]
    async fn broker_replays_history_to_late_subscribers() {
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker
            .publish(&stream_id, "token", json!({"text": "hello"}))
            .await;

        let (history, mut receiver) = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists")
            .into_parts();
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
        let broker = SessionStreamBroker::new();
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
        let (history, _receiver) = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists")
            .into_parts();

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
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;

        // First connection sees the first two tokens live.
        let (history0, mut first) = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists")
            .into_parts();
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
        let (history1, mut second) = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists")
            .into_parts();
        let replayed: Vec<String> = history1
            .iter()
            .filter(|e| e.event == "token")
            .map(|e| e.payload["text"].as_str().unwrap().to_string())
            .collect();
        assert_eq!(replayed, vec!["The ", "quick ", "brown "]);

        broker
            .publish(&stream_id, "token", json!({"text": "fox"}))
            .await;
        broker
            .publish(&stream_id, "final", json!({"text": "The quick brown fox"}))
            .await;
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
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;

        broker
            .publish(&stream_id, "token", json!({"text": "answer"}))
            .await;
        broker
            .publish(&stream_id, "final", json!({"text": "answer"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        // Subscribe only now — strictly after the stream is done.
        let (history, mut receiver) = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists")
            .into_parts();
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
        let broker = SessionStreamBroker::new();
        broker
            .register_session("session-conversation", "stream-1")
            .await;
        broker
            .activate_session_stream("session-conversation", "stream-1")
            .await;
        broker
            .register_session("session-conversation", "stream-2")
            .await;

        assert_eq!(
            broker.stream_id_for_session("session-conversation").await,
            Some("stream-1".to_string())
        );

        broker
            .clear_active_session_stream("session-conversation", "stream-1")
            .await;
        assert_eq!(
            broker.stream_id_for_session("session-conversation").await,
            Some("stream-2".to_string())
        );
    }

    #[tokio::test]
    async fn broker_records_observability_for_stream_events() {
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;
        let (trace_id, turn_id) = broker
            .start_turn(&stream_id, "session-conversation", 5)
            .await
            .expect("turn context");

        broker
            .publish(
                &stream_id,
                "tool_call",
                json!({
                    "turn_id": turn_id,
                    "id": "call-1",
                    "tool": "lookup",
                    "args": {"q": "hello"}
                }),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "model_usage",
                json!({
                    "turn_id": turn_id,
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
            .publish(
                &stream_id,
                "final",
                json!({"turn_id": turn_id, "text": "answer"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "done",
                json!({"session_id": "session-conversation", "turn_id": turn_id}),
            )
            .await;

        let summary = broker.observability().summarize_trace(&trace_id);
        assert!(summary.is_complete);
        assert_eq!(summary.tool_call_count, 1);
        assert_eq!(summary.model_usage.total_tokens, 5);
        assert_eq!(summary.final_text.as_deref(), Some("answer"));
    }

    #[tokio::test]
    async fn finished_stream_state_is_evicted_after_grace_on_next_activity() {
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker.register_session("session-1", &stream_id).await;
        broker
            .activate_session_stream("session-1", &stream_id)
            .await;
        broker
            .publish(&stream_id, "final", json!({"text": "ok"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        // Before the grace elapses the state is retained so a late subscriber can
        // still replay the terminal events.
        assert_eq!(broker.stream_count().await, 1);
        assert!(broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .is_some());

        // Simulate the grace window having elapsed, then trigger an opportunistic
        // sweep by creating a new (unrelated) stream.
        broker.force_expire_done(&stream_id).await;
        let _other = broker.create_stream().await;

        // The finished stream and its session mappings are reclaimed; only the
        // freshly created unrelated stream remains.
        assert_eq!(broker.stream_count().await, 1);
        assert!(broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .is_none());
        assert_eq!(broker.session_stream_count().await, 0);
        assert!(broker.stream_id_for_session("session-1").await.is_none());
    }

    #[tokio::test]
    async fn reregistered_session_keeps_newer_mapping_when_old_stream_evicted() {
        let broker = SessionStreamBroker::new();
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
        assert!(broker
            .subscribe(&old, StreamReplayMode::All)
            .await
            .is_none());
        assert_eq!(broker.stream_id_for_session("session-x").await, Some(new));
    }

    #[tokio::test]
    async fn lagged_subscriber_resyncs_skipped_range_from_history() {
        use futures::StreamExt;

        let broker = Arc::new(SessionStreamBroker::new());
        let stream_id = broker.create_stream().await;

        // Subscribe (capacity 256) but do NOT consume, then publish far more than
        // the channel capacity so the broadcast drops events for this receiver —
        // forcing a `Lagged` on the first recv.
        let subscription = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists");
        assert!(subscription.history.is_empty());
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
        let frames = replay_stream(broker.clone(), stream_id.clone(), true, subscription);
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
                        replayed_tokens.push(event.payload["text"].as_str().unwrap().to_string());
                    }
                    "final" => saw_final = true,
                    "done" => saw_done = true,
                    _ => {}
                },
                StreamFrame::ResyncRequired(_) => {
                    panic!("fresh all-history subscription must not require full resync");
                }
            }
        }

        assert!(
            saw_resync,
            "a Lagged gap must emit a resync frame, not be skipped silently"
        );
        assert!(
            saw_final,
            "the terminal final must still be replayed after resync"
        );
        assert!(
            saw_done,
            "the terminal done must still be replayed so the client stops"
        );
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
        assert_eq!(
            sorted.len(),
            replayed_tokens.len(),
            "no duplicate tokens after resync"
        );
    }

    #[tokio::test]
    async fn non_lagged_stream_emits_no_resync_frame() {
        use futures::StreamExt;

        let broker = Arc::new(SessionStreamBroker::new());
        let stream_id = broker.create_stream().await;
        broker
            .publish(&stream_id, "token", json!({"text": "hi"}))
            .await;
        broker.publish(&stream_id, "done", json!({})).await;

        let subscription = broker
            .subscribe(&stream_id, StreamReplayMode::All)
            .await
            .expect("stream exists");
        let frames = replay_stream(broker.clone(), stream_id, true, subscription);
        futures::pin_mut!(frames);
        let mut resyncs = 0;
        while let Some(frame) = frames.next().await {
            if matches!(frame, StreamFrame::Resync { .. }) {
                resyncs += 1;
            }
        }
        assert_eq!(
            resyncs, 0,
            "a healthy stream must never emit a resync frame"
        );
    }

    #[tokio::test]
    async fn subscribe_replay_none_emits_only_post_subscription_events() {
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker
            .publish(&stream_id, "token", json!({"text": "old"}))
            .await;

        let subscription = broker
            .subscribe(&stream_id, StreamReplayMode::None)
            .await
            .expect("stream exists");
        assert!(subscription.history.is_empty());
        assert_eq!(subscription.initial_last_seq, Some(0));
        assert_eq!(subscription.next_seq, 1);

        let mut receiver = subscription.receiver;
        broker
            .publish(&stream_id, "token", json!({"text": "new"}))
            .await;
        let live = receiver.recv().await.expect("live event");
        assert_eq!(live.payload["text"], "new");
    }

    #[tokio::test]
    async fn subscribe_after_seq_replays_only_newer_retained_events() {
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;
        for text in ["zero", "one", "two"] {
            broker
                .publish(&stream_id, "token", json!({"text": text}))
                .await;
        }

        let subscription = broker
            .subscribe(&stream_id, StreamReplayMode::AfterSeq(0))
            .await
            .expect("stream exists");
        let replayed: Vec<&str> = subscription
            .history
            .iter()
            .map(|event| event.payload["text"].as_str().unwrap())
            .collect();
        assert_eq!(replayed, vec!["one", "two"]);
        assert_eq!(subscription.initial_last_seq, Some(0));
        assert!(subscription.resync_required.is_none());
    }

    #[tokio::test]
    async fn subscribe_from_turn_id_replays_and_streams_only_that_turn() {
        let broker = Arc::new(SessionStreamBroker::new());
        let stream_id = broker.create_stream().await;
        broker
            .publish(
                &stream_id,
                "turn_started",
                json!({"turn_id": "turn-1", "text": "old"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "token",
                json!({"turn_id": "turn-1", "text": "old"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "turn_started",
                json!({"turn_id": "turn-2", "text": "start"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "token",
                json!({"turn_id": "turn-2", "text": "new"}),
            )
            .await;

        let subscription = broker
            .subscribe(
                &stream_id,
                StreamReplayMode::FromTurnId("turn-2".to_string()),
            )
            .await
            .expect("stream exists");
        let replayed = subscription
            .history
            .iter()
            .map(|event| {
                (
                    event.event.as_str(),
                    event.payload["turn_id"].as_str().unwrap(),
                )
            })
            .collect::<Vec<_>>();
        assert_eq!(
            replayed,
            vec![("turn_started", "turn-2"), ("token", "turn-2")]
        );
        assert!(subscription.resync_required.is_none());
        assert_eq!(subscription.turn_filter.as_deref(), Some("turn-2"));

        let frames = replay_stream(broker.clone(), stream_id.clone(), false, subscription);
        futures::pin_mut!(frames);

        let Some(StreamFrame::Event(first)) = frames.next().await else {
            panic!("expected replayed turn start");
        };
        assert_eq!(first.event, "turn_started");
        let Some(StreamFrame::Event(second)) = frames.next().await else {
            panic!("expected replayed turn token");
        };
        assert_eq!(second.payload["text"], "new");

        broker
            .publish(
                &stream_id,
                "token",
                json!({"turn_id": "turn-1", "text": "ignored"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "error",
                json!({"turn_id": "turn-2", "message": "retryable"}),
            )
            .await;
        broker
            .publish(&stream_id, "turn_completed", json!({"turn_id": "turn-2"}))
            .await;

        let Some(StreamFrame::Event(error)) =
            tokio::time::timeout(Duration::from_secs(1), frames.next())
                .await
                .expect("turn error event")
        else {
            panic!("expected turn error event");
        };
        assert_eq!(error.event, "error");
        assert_eq!(error.payload["turn_id"], "turn-2");

        let Some(StreamFrame::Event(done)) =
            tokio::time::timeout(Duration::from_secs(1), frames.next())
                .await
                .expect("turn terminal event")
        else {
            panic!("expected terminal turn event");
        };
        assert_eq!(done.event, "turn_completed");
        assert_eq!(done.payload["turn_id"], "turn-2");

        let stopped = tokio::time::timeout(Duration::from_secs(1), frames.next())
            .await
            .expect("from_turn_id stream should stop promptly");
        assert!(stopped.is_none());
    }

    #[tokio::test]
    async fn subscribe_from_turn_id_follow_replays_from_turn_and_keeps_session_streaming() {
        let broker = Arc::new(SessionStreamBroker::new());
        let stream_id = broker.create_stream().await;
        broker
            .publish(
                &stream_id,
                "turn_started",
                json!({"turn_id": "turn-1", "text": "old"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "token",
                json!({"turn_id": "turn-1", "text": "old"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "turn_started",
                json!({"turn_id": "turn-2", "text": "start"}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "token",
                json!({"turn_id": "turn-2", "text": "new"}),
            )
            .await;

        let subscription = broker
            .subscribe(
                &stream_id,
                StreamReplayMode::FromTurnIdFollow("turn-2".to_string()),
            )
            .await
            .expect("stream exists");
        let replayed = subscription
            .history
            .iter()
            .map(|event| {
                (
                    event.event.as_str(),
                    event.payload["turn_id"].as_str().unwrap(),
                )
            })
            .collect::<Vec<_>>();
        assert_eq!(
            replayed,
            vec![("turn_started", "turn-2"), ("token", "turn-2")]
        );
        assert!(subscription.resync_required.is_none());
        assert!(subscription.turn_filter.is_none());

        let frames = replay_stream(broker.clone(), stream_id.clone(), false, subscription);
        futures::pin_mut!(frames);

        let Some(StreamFrame::Event(first)) = frames.next().await else {
            panic!("expected replayed turn start");
        };
        assert_eq!(first.event, "turn_started");
        let Some(StreamFrame::Event(second)) = frames.next().await else {
            panic!("expected replayed turn token");
        };
        assert_eq!(second.payload["text"], "new");

        broker
            .publish(&stream_id, "turn_completed", json!({"turn_id": "turn-2"}))
            .await;
        let Some(StreamFrame::Event(turn_done)) =
            tokio::time::timeout(Duration::from_secs(1), frames.next())
                .await
                .expect("turn terminal event")
        else {
            panic!("expected terminal turn event");
        };
        assert_eq!(turn_done.event, "turn_completed");

        broker
            .publish(
                &stream_id,
                "turn_started",
                json!({"turn_id": "turn-3", "text": "follow-up"}),
            )
            .await;
        let Some(StreamFrame::Event(next_turn)) =
            tokio::time::timeout(Duration::from_secs(1), frames.next())
                .await
                .expect("follow-up turn event")
        else {
            panic!("expected follow-up turn event");
        };
        assert_eq!(next_turn.event, "turn_started");
        assert_eq!(next_turn.payload["turn_id"], "turn-3");
    }

    #[tokio::test]
    async fn subscribe_from_turn_id_reports_resync_when_turn_not_retained() {
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker
            .publish(&stream_id, "token", json!({"turn_id": "turn-other"}))
            .await;

        let subscription = broker
            .subscribe(
                &stream_id,
                StreamReplayMode::FromTurnId("turn-missing".to_string()),
            )
            .await
            .expect("stream exists");
        assert!(subscription.history.is_empty());
        let Some(StreamResyncRequired::Turn(resync)) = subscription.resync_required else {
            panic!("missing turn should request turn resync");
        };
        assert_eq!(resync.turn_id, "turn-missing");
        assert_eq!(resync.reason, "turn_not_retained");
        assert_eq!(resync.earliest_seq, Some(0));
        assert_eq!(resync.latest_seq, Some(0));
    }

    #[tokio::test]
    async fn subscribe_from_turn_id_rejects_partial_retained_turn() {
        let broker = SessionStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker
            .publish(
                &stream_id,
                "token",
                json!({"turn_id": "turn-partial", "text": "already mid-turn"}),
            )
            .await;

        let subscription = broker
            .subscribe(
                &stream_id,
                StreamReplayMode::FromTurnId("turn-partial".to_string()),
            )
            .await
            .expect("stream exists");
        assert!(subscription.history.is_empty());
        let Some(StreamResyncRequired::Turn(resync)) = subscription.resync_required else {
            panic!("partial turn should request turn resync");
        };
        assert_eq!(resync.turn_id, "turn-partial");
        assert_eq!(resync.reason, "turn_not_retained");
        assert_eq!(resync.earliest_seq, Some(0));
        assert_eq!(resync.latest_seq, Some(0));
    }

    #[tokio::test]
    async fn subscribe_after_expired_seq_emits_resync_required_without_old_history() {
        let broker = Arc::new(SessionStreamBroker::new());
        let stream_id = broker.create_stream().await;
        for i in 0..(HISTORY_CAPACITY + 2) {
            broker
                .publish(&stream_id, "token", json!({"text": format!("tok-{i}")}))
                .await;
        }

        let subscription = broker
            .subscribe(&stream_id, StreamReplayMode::AfterSeq(0))
            .await
            .expect("stream exists");
        assert!(subscription.history.is_empty());
        assert_eq!(
            subscription.initial_last_seq,
            Some((HISTORY_CAPACITY + 1) as u64)
        );
        assert!(subscription.resync_required.is_some());

        let frames = replay_stream(broker.clone(), stream_id.clone(), false, subscription);
        futures::pin_mut!(frames);
        let Some(StreamFrame::ResyncRequired(resync)) = frames.next().await else {
            panic!("expired cursor must produce resync_required first");
        };
        let StreamResyncRequired::Cursor(resync) = resync else {
            panic!("expired cursor should produce cursor resync");
        };
        assert_eq!(resync.requested_after_seq, 0);
        assert_eq!(resync.earliest_seq, 2);
        assert_eq!(resync.latest_seq, (HISTORY_CAPACITY + 1) as u64);
    }
}
