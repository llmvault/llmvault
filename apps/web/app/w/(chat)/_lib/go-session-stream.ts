import { fetchEventSource } from "@microsoft/fetch-event-source"

export interface GoSessionStreamFrame {
  sessionId: string
  event: string
  id: string
  retry?: number
  data: unknown
}

interface GoSessionStreamOpen {
  sessionId: string
  streamId: string | null
  nextSequence: number | null
}

export interface GoSessionStreamCursor {
  streamId: string
  sequence: number
}

const RUNTIME_REPO_CHANGE_EVENT = "repo.change_batch"
const STREAM_ID_HEADER = "x-hivy-stream-id"
const NEXT_SEQUENCE_HEADER = "x-hivy-stream-next-sequence"

export type GoSessionStreamReplayMode =
  | { mode: "all" }
  | { mode: "none" }
  | { mode: "after_seq"; afterSeq: number }
  | { mode: "from_turn_id"; turnId: string }
  | { mode: "from_turn_id_follow"; turnId: string }

export class GoSessionStreamHTTPError extends Error {
  constructor(
    readonly status: number,
    message: string
  ) {
    super(message)
    this.name = "GoSessionStreamHTTPError"
  }
}

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
    // SSE stream through the authenticated Next.js API proxy.
    headers: { Accept: "text/event-stream" },
    credentials: "include",
    signal,
    openWhenHidden: true,
    async onopen(response) {
      const contentType = response.headers.get("content-type") ?? ""
      if (response.ok && contentType.includes("text/event-stream")) {
        onOpen?.({
          sessionId,
          streamId: response.headers.get(STREAM_ID_HEADER),
          nextSequence: numberHeader(response.headers, NEXT_SEQUENCE_HEADER),
        })
        return
      }

      let message = `Session stream failed with HTTP ${response.status}`
      try {
        const raw = await response.text()
        const detail = streamErrorDetail(raw)
        if (detail) message = `${message}: ${detail}`
      } catch {
        /* non-text stream error */
      }
      throw new GoSessionStreamHTTPError(response.status, message)
    },
    onmessage(event) {
      const data = parseStreamEventData(event.data)
      const frame = {
        sessionId,
        event: event.event || "message",
        id: event.id || eventIDFromData(data),
        retry: event.retry,
        data,
      }
      onEvent?.(frame)
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
  const path = `/api/proxy/v1/sessions/${encodeURIComponent(sessionId)}/stream`
  const search = new URLSearchParams()
  if (replay.mode === "none") {
    search.set("replay", "none")
  }
  if (replay.mode === "after_seq") {
    search.set("after_seq", `${replay.afterSeq}`)
  }
  if (replay.mode === "from_turn_id" || replay.mode === "from_turn_id_follow") {
    search.set("from_turn_id", replay.turnId)
  }
  if (replay.mode === "from_turn_id_follow") {
    search.set("follow", "true")
  }
  const query = search.toString()
  return query ? `${path}?${query}` : path
}

export function goSessionStreamHTTPStatus(error: unknown) {
  return error instanceof GoSessionStreamHTTPError ? error.status : undefined
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

function eventIDFromData(data: unknown) {
  if (!data || typeof data !== "object" || Array.isArray(data)) return ""
  const value = (data as Record<string, unknown>).event_id
  return typeof value === "string" ? value : ""
}

function numberHeader(headers: Headers, key: string) {
  const value = headers.get(key)
  if (!value) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function streamErrorDetail(raw: string) {
  const trimmed = raw.trim()
  if (!trimmed) return ""
  try {
    const parsed = JSON.parse(trimmed) as Record<string, unknown>
    const value = parsed.error ?? parsed.message
    return typeof value === "string" && value.trim() ? value.trim() : trimmed
  } catch {
    return trimmed
  }
}
