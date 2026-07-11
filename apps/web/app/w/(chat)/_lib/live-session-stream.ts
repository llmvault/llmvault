import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"

const displayableEvents = new Set([
  "thinking",
  "token",
  "tool_call",
  "tool_result",
  "plan_updated",
  "model_request_started",
  "model_request_completed",
  "question_requested",
  "question_answered",
  "final",
  "error",
])

export function appendLiveSessionStreamFrame(
  events: SessionEventResponse[],
  frame: GoSessionStreamFrame
): SessionEventResponse[] {
  if (!displayableEvents.has(frame.event)) {
    return clearModelRequestThinking(events)
  }
  if (frame.event === "model_request_completed") {
    return clearModelRequestThinking(events)
  }

  const next = streamFrameToSessionEvent(frame)
  if (!next) return clearModelRequestThinking(events)
  if (isRepeatedLiveEvent(events, next)) return events

  const sourceEvents = clearModelRequestThinking(events)

  if (next.event_type === "model_request_started") {
    return [...sourceEvents, modelRequestThinkingEvent(next)]
  }

  if (next.event_type === "thinking" || next.event_type === "token") {
    const last = sourceEvents.at(-1)
    if (last?.event_type === next.event_type && sameTurn(last, next)) {
      if ((next.sequence_number ?? 0) <= (last.sequence_number ?? 0)) {
        return sourceEvents
      }
      return [...sourceEvents.slice(0, -1), mergeTextEvent(last, next)]
    }
  }

  return [...sourceEvents, next]
}

function isRepeatedLiveEvent(
  events: SessionEventResponse[],
  next: SessionEventResponse
) {
  const eventID = next.event_id
  if (eventID && events.some((event) => event.event_id === eventID)) {
    return true
  }

  if (next.event_type !== "tool_call" && next.event_type !== "tool_result") {
    return false
  }
  const toolID = stringValue(payloadRecord(next), "id")
  return Boolean(
    toolID &&
      events.some(
        (event) =>
          event.event_type === next.event_type &&
          stringValue(payloadRecord(event), "id") === toolID
      )
  )
}

export function isTerminalStreamFrame(frame: GoSessionStreamFrame) {
  return (
    frame.event === "final" ||
    frame.event === "done" ||
    frame.event === "turn_completed" ||
    frame.event === "turn_failed" ||
    frame.event === "error"
  )
}

export function streamFrameToSessionEvent(
  frame: GoSessionStreamFrame
): SessionEventResponse | null {
  if (
    !frame.data ||
    typeof frame.data !== "object" ||
    Array.isArray(frame.data)
  ) {
    return null
  }
  const payload = frame.data as Record<string, unknown>
  const eventID = stringValue(payload, "event_id") || frame.id
  return {
    id: eventID || `${frame.event}-${Date.now()}`,
    session_id: stringValue(payload, "session_id") || frame.sessionId,
    event_id: eventID,
    event_type: frame.event,
    sequence_number: numberValue(payload, "sequence") ?? 0,
    payload,
    event_at: stringValue(payload, "occurred_at") || new Date().toISOString(),
  } as SessionEventResponse
}

function mergeTextEvent(
  current: SessionEventResponse,
  next: SessionEventResponse
): SessionEventResponse {
  const currentPayload = payloadRecord(current)
  const nextPayload = payloadRecord(next)
  return {
    ...current,
    sequence_number: next.sequence_number ?? current.sequence_number,
    event_at: next.event_at ?? current.event_at,
    payload: {
      ...currentPayload,
      ...nextPayload,
      text:
        stringValue(currentPayload, "text") + stringValue(nextPayload, "text"),
    },
  }
}

function sameTurn(left: SessionEventResponse, right: SessionEventResponse) {
  const leftTurn = stringValue(payloadRecord(left), "turn_id")
  const rightTurn = stringValue(payloadRecord(right), "turn_id")
  return leftTurn === rightTurn
}

function modelRequestThinkingEvent(
  event: SessionEventResponse
): SessionEventResponse {
  return {
    ...event,
    event_type: "thinking",
    payload: {
      ...payloadRecord(event),
      text: "",
      label: "Thinking...",
      model_request_pending: true,
    },
  }
}

function clearModelRequestThinking(events: SessionEventResponse[]) {
  const last = events.at(-1)
  return last && payloadRecord(last).model_request_pending === true
    ? events.slice(0, -1)
    : events
}

function payloadRecord(event: SessionEventResponse): Record<string, unknown> {
  const payload = event.payload
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return {}
  }
  return payload as Record<string, unknown>
}

function stringValue(payload: Record<string, unknown>, key: string): string {
  const value = payload[key]
  return typeof value === "string" ? value : ""
}

function numberValue(payload: Record<string, unknown>, key: string) {
  const value = payload[key]
  return typeof value === "number" ? value : undefined
}
