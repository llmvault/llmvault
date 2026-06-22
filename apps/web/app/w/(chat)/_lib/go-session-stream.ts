import { fetchEventSource } from "@microsoft/fetch-event-source"

export interface GoSessionStreamFrame {
  sessionId: string
  event: string
  id: string
  retry?: number
  data: unknown
}

export interface GoSessionStreamOpen {
  sessionId: string
  streamId: string | null
  nextSequence: number | null
}

export interface GoSessionStreamCursor {
  streamId: string
  sequence: number
}

export const RUNTIME_REPO_CHANGE_EVENT = "repo.change_batch"

export type GoSessionStreamReplayMode =
  | { mode: "all" }
  | { mode: "none" }
  | { mode: "after_seq"; afterSeq: number }

export function subscribeToGoSessionStream({
  sessionId,
  replay,
  signal,
  onOpen,
  onEvent,
}: {
  sessionId: string
  replay?: GoSessionStreamReplayMode
  signal: AbortSignal
  onOpen?: (meta: GoSessionStreamOpen) => void
  onEvent?: (frame: GoSessionStreamFrame) => void
}) {
  return fetchEventSource(goSessionStreamURL(sessionId, replay), {
    method: "GET",
    headers: { Accept: "text/event-stream" },
    credentials: "include",
    signal,
    openWhenHidden: true,
    async onopen(response) {
      const contentType = response.headers.get("content-type") ?? ""
      onOpen?.({ sessionId, streamId: null, nextSequence: null })
      if (response.ok && contentType.includes("text/event-stream")) return

      let message = `Session stream failed with HTTP ${response.status}`
      try {
        const envelope = (await response.json()) as { error?: string }
        if (envelope.error) message = envelope.error
      } catch {
        /* non-JSON stream error */
      }
      throw new Error(message)
    },
    onmessage(event) {
      const frame = goSessionStreamFrame({
        sessionId,
        event: event.event || "message",
        id: event.id,
        retry: event.retry,
        data: parseStreamEventData(event.data),
      })
      if (frame) onEvent?.(frame)
    },
    onerror(error) {
      throw error
    },
  })
}

export function goSessionStreamURL(
  sessionId: string,
  replay: GoSessionStreamReplayMode = { mode: "all" }
) {
  const origin =
    typeof window !== "undefined" ? window.location.origin : "http://localhost"
  const parsed = new URL(
    `/api/proxy/v1/sessions/${sessionId}/stream`,
    origin
  )
  if (replay.mode === "after_seq") {
    parsed.searchParams.set("after_seq", `${replay.afterSeq}`)
  }
  return parsed.toString()
}

export function goSessionStreamCursor(
  frame: GoSessionStreamFrame
): GoSessionStreamCursor | null {
  if (
    !frame.data ||
    typeof frame.data !== "object" ||
    Array.isArray(frame.data)
  ) {
    return null
  }
  const record = frame.data as Record<string, unknown>
  const streamId = record.stream_id
  const sequence = record.sequence
  if (typeof streamId !== "string" || typeof sequence !== "number") {
    return null
  }
  return { streamId, sequence }
}

export function isRuntimeRepoChangeFrame(frame: GoSessionStreamFrame) {
  return frame.event === RUNTIME_REPO_CHANGE_EVENT
}

function parseStreamEventData(data: string) {
  if (!data) return null
  try {
    return JSON.parse(data) as unknown
  } catch {
    return data
  }
}

function goSessionStreamFrame(frame: GoSessionStreamFrame) {
  if (
    frame.event !== "session.preview" &&
    frame.event !== "session.event" &&
    frame.event !== "session.control"
  ) {
    return frame
  }
  if (!isRecord(frame.data)) return null

  if (frame.event === "session.control") {
    const type = stringValue(frame.data, "type")
    return {
      ...frame,
      event: type === "resync" ? "resync_required" : "control",
    }
  }

  if (frame.event === "session.preview") {
    const eventType = stringValue(frame.data, "event_type")
    if (!eventType) return null
    const payload = recordValue(frame.data, "payload")
    const runtimeSeq = numberValue(frame.data, "runtime_seq")
    return {
      ...frame,
      event: eventType,
      id: stringValue(frame.data, "event_id") || frame.id,
      data: {
        ...payload,
        session_id: stringValue(frame.data, "session_id") || frame.sessionId,
        event_id: stringValue(frame.data, "event_id") || frame.id,
        turn_id: stringValue(frame.data, "turn_id"),
        span_id: stringValue(frame.data, "span_id"),
        durability: stringValue(frame.data, "durability") || "preview",
        runtime_seq: runtimeSeq,
        sequence: runtimeSeq,
        occurred_at: stringValue(frame.data, "occurred_at"),
      },
    }
  }

  const eventType = stringValue(frame.data, "event_type")
  if (!eventType) return null
  const payload = recordValue(frame.data, "payload")
  const sequence = numberValue(frame.data, "sequence_number")
  return {
    ...frame,
    event: eventType,
    id: stringValue(frame.data, "id") || stringValue(frame.data, "event_id"),
    data: {
      ...payload,
      session_id: stringValue(frame.data, "session_id") || frame.sessionId,
      event_id: stringValue(frame.data, "event_id") || frame.id,
      turn_id: stringValue(frame.data, "turn_id"),
      span_id: stringValue(frame.data, "span_id"),
      durability: stringValue(frame.data, "durability") || "durable",
      runtime_seq: numberValue(frame.data, "runtime_seq"),
      sequence,
      sequence_number: sequence,
      occurred_at: stringValue(frame.data, "event_at"),
    },
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value)
}

function recordValue(record: Record<string, unknown>, key: string) {
  const value = record[key]
  return isRecord(value) ? value : {}
}

function stringValue(record: Record<string, unknown>, key: string) {
  const value = record[key]
  return typeof value === "string" ? value : ""
}

function numberValue(record: Record<string, unknown>, key: string) {
  const value = record[key]
  return typeof value === "number" ? value : undefined
}
