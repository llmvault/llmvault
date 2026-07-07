import { beforeEach, describe, expect, it } from "vitest"
import {
  sessionEventsToConversationBlocks,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history"

let sequence = 0

beforeEach(() => {
  sequence = 0
})

function event(
  eventType: string,
  payload: Record<string, unknown>,
  eventAt = "2026-06-15T12:00:00.000Z"
): SessionEventResponse {
  sequence += 1
  return {
    id: `event-${sequence}`,
    event_id: `event-${sequence}`,
    event_type: eventType,
    sequence_number: sequence,
    payload,
    event_at: eventAt,
  } as SessionEventResponse
}

describe("sessionEventsToConversationBlocks automated sources", () => {
  it("threads event source and provider onto automated user blocks", () => {
    const trigger = {
      ...event("user.message.received", {
        text: "A check finished on your PR.",
        provider: "github-app",
        event_key: "check_suite.completed",
      }),
      source: "trigger",
    } as SessionEventResponse
    const scheduled = {
      ...event("user.message.received", { text: "Run the weekly digest." }),
      source: "schedule",
    } as SessionEventResponse

    const blocks = sessionEventsToConversationBlocks([trigger, scheduled])

    expect(blocks).toMatchObject([
      {
        type: "user",
        text: "A check finished on your PR.",
        source: "trigger",
        provider: "github-app",
      },
      {
        type: "user",
        text: "Run the weekly digest.",
        source: "schedule",
      },
    ])
  })

  it("leaves web-sourced user blocks without automation metadata", () => {
    const blocks = sessionEventsToConversationBlocks([
      { ...event("user.message", { text: "Hi there" }), source: "web" },
    ])

    expect(blocks[0]).toMatchObject({ type: "user", source: "web" })
    expect(blocks[0]).not.toHaveProperty("provider")
  })
})
