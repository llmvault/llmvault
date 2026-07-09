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

  it("does not split a turn's work when an automated message arrives mid-turn", () => {
    const started = event(
      "turn_started",
      { turn_id: "turn-a" },
      "2026-06-15T11:39:58.000Z"
    )
    const token = {
      ...event(
        "token",
        { text: "Investigating the failure.", turn_id: "turn-a" },
        "2026-06-15T11:40:05.000Z"
      ),
      sequence_number: 4120,
    } as SessionEventResponse
    const work = {
      ...event(
        "tool_result",
        {
          id: "tool-1",
          tool: "bash",
          result: { command: "pytest", output: "ok", exit_code: 0 },
          turn_id: "turn-a",
        },
        "2026-06-15T11:41:10.000Z"
      ),
      sequence_number: 4180,
    } as SessionEventResponse
    const trigger = {
      ...event(
        "user.message.received",
        { text: "A check finished on your PR." },
        "2026-06-15T11:42:31.000Z"
      ),
      source: "trigger",
      sequence_number: 0,
    } as SessionEventResponse
    const final = {
      ...event(
        "final",
        { text: "The check is green.", turn_id: "turn-a" },
        "2026-06-15T11:42:35.000Z"
      ),
      sequence_number: 4240,
    } as SessionEventResponse
    const completed = {
      ...event(
        "turn_completed",
        { turn_id: "turn-a" },
        "2026-06-15T11:42:35.000Z"
      ),
      sequence_number: 4241,
    } as SessionEventResponse

    const blocks = sessionEventsToConversationBlocks([
      started,
      token,
      work,
      trigger,
      final,
      completed,
    ])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        duration: "2m 37s",
        blocks: [
          { type: "assistant", text: "Investigating the failure." },
          { type: "tool", label: "Ran pytest" },
        ],
      },
      { type: "assistant", text: "The check is green." },
      {
        type: "user",
        text: "A check finished on your PR.",
        source: "trigger",
      },
    ])
  })

  it("keeps splitting a turn's work when a web message arrives mid-turn", () => {
    const first = event(
      "token",
      { text: "First pass.", turn_id: "turn-a" },
      "2026-06-15T11:40:00.000Z"
    )
    const webUser = {
      ...event(
        "user.message",
        { text: "Actually check this too." },
        "2026-06-15T11:40:30.000Z"
      ),
      source: "web",
    } as SessionEventResponse
    const second = event(
      "token",
      { text: "Second pass.", turn_id: "turn-a" },
      "2026-06-15T11:41:00.000Z"
    )

    const blocks = sessionEventsToConversationBlocks([first, webUser, second])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        blocks: [{ type: "assistant", text: "First pass." }],
      },
      { type: "user", text: "Actually check this too.", source: "web" },
      {
        type: "agent_work",
        blocks: [{ type: "assistant", text: "Second pass." }],
      },
    ])
  })
})
