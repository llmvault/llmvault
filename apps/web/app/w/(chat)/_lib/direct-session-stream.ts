import { fetchEventSource } from "@microsoft/fetch-event-source"

export interface DirectSessionStreamFrame {
  sessionId: string
  event: string
  id: string
  retry?: number
  data: unknown
}

export interface DirectSessionStreamOpen {
  sessionId: string
  streamId: string | null
  nextSequence: number | null
}

export interface DirectSessionStreamCursor {
  streamId: string
  sequence: number
}

export type DirectSessionStreamReplayMode =
  | { mode: "all" }
  | { mode: "none" }
  | { mode: "after_seq"; afterSeq: number }

export function subscribeToDirectSessionStream({
  sessionId,
  directUrl,
  streamToken,
  replay,
  signal,
  onOpen,
  onEvent,
}: {
  sessionId: string
  directUrl: string
  streamToken: string
  replay?: DirectSessionStreamReplayMode
  signal: AbortSignal
  onOpen?: (meta: DirectSessionStreamOpen) => void
  onEvent?: (frame: DirectSessionStreamFrame) => void
}) {
  return fetchEventSource(directSessionStreamURL(directUrl, streamToken, replay), {
    method: "GET",
    headers: {
      Accept: "text/event-stream",
      "X-Hivy-Stream-Token": streamToken,
    },
    signal,
    openWhenHidden: true,
    async onopen(response) {
      const contentType = response.headers.get("content-type") ?? ""

      onOpen?.({
        sessionId,
        streamId: response.headers.get("x-hivy-stream-id"),
        nextSequence: parseSequenceHeader(
          response.headers.get("x-hivy-stream-next-sequence")
        ),
      })

      if (response.ok && contentType.includes("text/event-stream")) return

      let message = `Direct session stream failed with HTTP ${response.status}`
      try {
        const envelope = (await response.json()) as { error?: string }
        if (envelope.error) message = envelope.error
      } catch {
        /* non-JSON stream error */
      }
      throw new Error(message)
    },
    onmessage(event) {
      const frame = {
        sessionId,
        event: event.event || "message",
        id: event.id,
        retry: event.retry,
        data: parseStreamEventData(event.data),
      }
      onEvent?.(frame)
    },
    onerror(error) {
      throw error
    },
  })
}

export function directSessionStreamURL(
  url: string,
  streamToken: string,
  replay: DirectSessionStreamReplayMode = { mode: "all" }
) {
  const parsed = new URL(url)
  parsed.searchParams.set("stream_token", streamToken)
  parsed.searchParams.delete("replay")
  parsed.searchParams.delete("after_seq")
  if (replay.mode === "none") {
    parsed.searchParams.set("replay", "none")
  }
  if (replay.mode === "after_seq") {
    parsed.searchParams.set("after_seq", `${replay.afterSeq}`)
  }
  return parsed.toString()
}

export function directSessionStreamCursor(
  frame: DirectSessionStreamFrame
): DirectSessionStreamCursor | null {
  if (!frame.data || typeof frame.data !== "object" || Array.isArray(frame.data)) {
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

function parseStreamEventData(data: string) {
  if (!data) return null
  try {
    return JSON.parse(data) as unknown
  } catch {
    return data
  }
}

function parseSequenceHeader(value: string | null) {
  if (!value) return null
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) ? parsed : null
}
