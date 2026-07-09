import { describe, expect, it } from "vitest"
import { orderActiveTurnAfterLatestUserMessage } from "@/app/w/(chat)/_lib/session-history-order"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history-event-utils"

let sequence = 0

function event(
  eventType: string,
  payload: Record<string, unknown>,
  eventAt: string,
  extra: Partial<SessionEventResponse> = {}
): SessionEventResponse {
  sequence += 1
  return {
    id: `event-${sequence}`,
    event_id: `event-${sequence}`,
    event_type: eventType,
    sequence_number: sequence,
    payload,
    event_at: eventAt,
    ...extra,
  } as SessionEventResponse
}

describe("orderActiveTurnAfterLatestUserMessage", () => {
  it("hoists the active turn after a user message that is newer than the turn start", () => {
    const thinking = event(
      "thinking",
      { text: "Reading the code", turn_id: "turn-active" },
      "2026-06-15T12:00:00.000Z"
    )
    const user = event(
      "user.message",
      { text: "what about this one?" },
      "2026-06-15T12:00:05.000Z",
      { source: "web" }
    )

    const ordered = orderActiveTurnAfterLatestUserMessage(
      [thinking, user],
      "turn-active",
      "2026-06-15T12:00:00.000Z"
    )

    expect(ordered.map((entry) => entry.event_type)).toEqual([
      "user.message",
      "thinking",
    ])
  })

  it("does not hoist when the active turn started after the latest user message", () => {
    const user = event(
      "user.message",
      { text: "kick things off" },
      "2026-06-15T12:00:00.000Z",
      { source: "web" }
    )
    const finalA = event(
      "final",
      { text: "Completed A", turn_id: "turn-a" },
      "2026-06-15T12:01:00.000Z"
    )
    const finalB = event(
      "final",
      { text: "Completed B", turn_id: "turn-b" },
      "2026-06-15T12:02:00.000Z"
    )
    const streaming = event(
      "token",
      { text: "Working now", turn_id: "turn-active" },
      "2026-06-15T12:03:00.000Z"
    )

    const ordered = orderActiveTurnAfterLatestUserMessage(
      [user, finalA, finalB, streaming],
      "turn-active",
      "2026-06-15T12:02:30.000Z"
    )

    expect(ordered.map((entry) => entry.event_type)).toEqual([
      "user.message",
      "final",
      "final",
      "token",
    ])
    expect(ordered.at(-1)).toBe(streaming)
  })

  it("falls back to hoisting when the turn start timestamp is missing", () => {
    const user = event(
      "user.message",
      { text: "kick things off" },
      "2026-06-15T12:00:00.000Z",
      { source: "web" }
    )
    const finalA = event(
      "final",
      { text: "Completed A", turn_id: "turn-a" },
      "2026-06-15T12:01:00.000Z"
    )
    const streaming = event(
      "token",
      { text: "Working now", turn_id: "turn-active" },
      "2026-06-15T12:03:00.000Z"
    )

    const ordered = orderActiveTurnAfterLatestUserMessage(
      [user, finalA, streaming],
      "turn-active"
    )

    expect(ordered.map((entry) => entry.event_type)).toEqual([
      "user.message",
      "token",
      "final",
    ])
  })
})
