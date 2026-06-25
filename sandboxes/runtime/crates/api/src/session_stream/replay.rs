use std::convert::Infallible;
use std::sync::Arc;

use anyhow::Result;
use async_stream::stream;
use axum::http::{HeaderName, HeaderValue};
use axum::response::sse::{Event, Sse};
use axum::response::{IntoResponse, Response};
use serde_json::json;
use tokio::sync::broadcast;
use tokio::time::Duration;

use super::{
    event_turn_id, SessionStreamBroker, SessionStreamEvent, StreamReplayMode, StreamResyncRequired,
    StreamSubscription,
};

/// A frame produced by the replay stream: either a published event or a
/// synthetic `resync` marker inserted after a `Lagged` gap.
#[derive(Debug, Clone)]
pub(super) enum StreamFrame {
    Event(SessionStreamEvent),
    Resync { skipped: u64, from_seq: Option<u64> },
    ResyncRequired(StreamResyncRequired),
}

/// Core of the SSE replay stream, decoupled from the `Sse`/`Event` wire types so
/// the reconnect/resync behavior is unit-testable. Yields history first, then
/// live events, and on `Lagged` emits a `Resync` frame followed by the exact
/// missed range from history. Stream-id subscribers stop on `done`;
/// stable session stream subscribers keep listening across turn boundaries.
pub(super) fn replay_stream(
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

fn is_terminal_turn_event(event: &SessionStreamEvent, turn_id: &str) -> bool {
    if event_turn_id(event).as_deref() != Some(turn_id) {
        return false;
    }
    matches!(
        event.event.as_str(),
        "turn_completed" | "turn_failed" | "turn_interrupted" | "done"
    )
}
