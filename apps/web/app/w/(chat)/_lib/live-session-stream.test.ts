import { describe, expect, it } from "vitest"
import {
  appendLiveSessionStreamFrame,
  isTerminalStreamFrame,
} from "@/app/w/(chat)/_lib/live-session-stream"
import type { DirectSessionStreamFrame } from "@/app/w/(chat)/_lib/direct-session-stream"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"

function frame(
  event: string,
  data: Record<string, unknown>,
  id = `${event}-frame`
): DirectSessionStreamFrame {
  return {
    sessionId: "session_1",
    event,
    id,
    data,
  }
}

describe("live session stream", () => {
  it("keeps the first token event identity while merging streamed chunks", () => {
    const initial = appendLiveSessionStreamFrame(
      [],
      frame(
        "token",
        {
          event_id: "event-token-1",
          sequence: 1,
          text: "Hello",
          turn_id: "turn_1",
          occurred_at: "2026-06-15T12:00:00.000Z",
        },
        "event-token-1"
      )
    )

    const merged = appendLiveSessionStreamFrame(
      initial,
      frame(
        "token",
        {
          event_id: "event-token-2",
          sequence: 2,
          text: " world",
          turn_id: "turn_1",
          occurred_at: "2026-06-15T12:00:01.000Z",
        },
        "event-token-2"
      )
    )

    expect(merged).toHaveLength(1)
    expect(merged[0]).toMatchObject({
      id: "event-token-1",
      event_id: "event-token-1",
      sequence_number: 2,
      payload: {
        text: "Hello world",
      },
    } satisfies Partial<SessionEventResponse>)
  })

  it("treats error frames as terminal", () => {
    expect(isTerminalStreamFrame(frame("error", { message: "failed" }))).toBe(
      true
    )
  })
})
