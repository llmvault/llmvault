import { describe, expect, it } from "vitest"
import {
  eventTurnIDs,
  replayModeForLoadedSession,
  suppressBackendEventsForLiveTurn,
  suppressBackendEventsForLiveTurns,
} from "@/app/w/(chat)/_lib/session-stream-handoff"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"

describe("session stream handoff", () => {
  it("subscribes from the active turn when backend knows it", () => {
    expect(replayModeForLoadedSession("turn-1")).toEqual({
      mode: "from_turn_id",
      turnId: "turn-1",
    })
  })

  it("uses live-only subscription when there is no active turn id", () => {
    expect(replayModeForLoadedSession()).toEqual({ mode: "none" })
  })

  it("suppresses sandbox-owned backend events for the active turn only", () => {
    const user = event("user.message.received", "turn-active")
    const oldFinal = event("final", "turn-old")
    const activeToken = event("token", "turn-active")
    const activeTool = event("tool_call", "turn-active")

    expect(
      suppressBackendEventsForLiveTurn(
        [user, oldFinal, activeToken, activeTool],
        "turn-active"
      )
    ).toEqual([user, oldFinal])
  })

  it("suppresses backend events for any retained live turn", () => {
    const turnOneToken = event("token", "turn-1")
    const turnTwoToken = event("token", "turn-2")
    const oldFinal = event("final", "turn-old")

    expect(
      suppressBackendEventsForLiveTurns(
        [turnOneToken, turnTwoToken, oldFinal],
        eventTurnIDs([turnOneToken, turnTwoToken])
      )
    ).toEqual([oldFinal])
  })
})

function event(eventType: string, turnID: string): SessionEventResponse {
  return {
    id: `${eventType}:${turnID}`,
    event_id: `${eventType}:${turnID}`,
    event_type: eventType,
    turn_id: turnID,
    sequence_number: 1,
    payload: {
      text: eventType,
    },
    event_at: "2026-06-22T12:00:00.000Z",
  } as SessionEventResponse
}
