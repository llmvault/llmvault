import { describe, expect, it } from "vitest"
import {
  appendLiveSessionStreamFrame,
  isTerminalStreamFrame,
} from "@/app/w/(chat)/_lib/live-session-stream"
import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"

function frame(
  event: string,
  data: Record<string, unknown>,
  id = `${event}-frame`
): GoSessionStreamFrame {
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

  it("ignores replayed token chunks that are older than the merged live event", () => {
    const initial = appendLiveSessionStreamFrame(
      [],
      frame("token", {
        event_id: "event-token-1",
        sequence: 1,
        text: "Hello",
        turn_id: "turn_1",
      })
    )
    const merged = appendLiveSessionStreamFrame(
      initial,
      frame("token", {
        event_id: "event-token-2",
        sequence: 2,
        text: " world",
        turn_id: "turn_1",
      })
    )

    const replayed = appendLiveSessionStreamFrame(
      merged,
      frame("token", {
        event_id: "event-token-1",
        sequence: 1,
        text: "Hello",
        turn_id: "turn_1",
      })
    )

    expect(replayed).toBe(merged)
    expect(replayed[0]?.payload).toMatchObject({
      text: "Hello world",
    })
  })

  it("treats error frames as terminal", () => {
    expect(isTerminalStreamFrame(frame("error", { message: "failed" }))).toBe(
      true
    )
  })

  it("treats turn_completed frames as terminal", () => {
    expect(
      isTerminalStreamFrame(frame("turn_completed", { turn_id: "turn-1" }))
    ).toBe(true)
  })

  it("passes plan updates through as live session events", () => {
    const events = appendLiveSessionStreamFrame(
      [],
      frame("plan_updated", {
        event_id: "event-plan-1",
        sequence: 3,
        occurred_at: "2026-06-19T07:04:16.927Z",
        plan: [{ status: "in_progress", step: "1. Inspect project" }],
      })
    )

    expect(events).toHaveLength(1)
    expect(events[0]).toMatchObject({
      id: "event-plan-1",
      event_type: "plan_updated",
      sequence_number: 3,
      payload: {
        plan: [{ status: "in_progress", step: "1. Inspect project" }],
      },
    } satisfies Partial<SessionEventResponse>)
  })

  it("shows model request starts as temporary thinking events", () => {
    const started = appendLiveSessionStreamFrame(
      [],
      frame("model_request_started", {
        event_id: "event-model-started",
        sequence: 4,
        turn_id: "turn_1",
        occurred_at: "2026-06-19T07:04:17.000Z",
      })
    )

    expect(started).toMatchObject([
      {
        event_type: "thinking",
        payload: {
          label: "Thinking...",
          model_request_pending: true,
          turn_id: "turn_1",
        },
      },
    ] satisfies Partial<SessionEventResponse>[])

    const completed = appendLiveSessionStreamFrame(
      started,
      frame("model_request_completed", {
        event_id: "event-model-completed",
        sequence: 5,
        turn_id: "turn_1",
        occurred_at: "2026-06-19T07:04:18.000Z",
      })
    )

    expect(completed).toEqual([])
  })

  it("clears temporary model request thinking when another frame arrives", () => {
    const started = appendLiveSessionStreamFrame(
      [],
      frame("model_request_started", {
        event_id: "event-model-started",
        sequence: 4,
        turn_id: "turn_1",
      })
    )
    const next = appendLiveSessionStreamFrame(
      started,
      frame("token", {
        event_id: "event-token",
        sequence: 5,
        text: "Done",
        turn_id: "turn_1",
      })
    )

    expect(next.map((event) => event.event_type)).toEqual(["token"])
  })
})
