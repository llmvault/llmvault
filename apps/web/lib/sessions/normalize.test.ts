import { describe, expect, it } from "vitest"
import {
  type EmployeeSessionEvent,
  appendLiveThinkingEvent,
  cleanPersistedAssistantText,
  liveAssistantEventType,
  normalizeSessionEvents,
  reconcileLiveFinalEvent,
  eventText,
} from "./normalize"

let seq = 0
function ev(
  event_type: string,
  payload: Record<string, unknown> = {}
): EmployeeSessionEvent {
  seq += 1
  return {
    id: `e${seq}`,
    event_type,
    sequence_number: seq,
    payload,
  }
}

function tokens(...texts: string[]) {
  return texts.map((text) => ev("agent.stream.token", { text }))
}

function assistantTexts(events: EmployeeSessionEvent[]) {
  return events
    .filter((event) => normalizeAssistantKind(event))
    .map((event) => eventText(event))
}

function normalizeAssistantKind(event: EmployeeSessionEvent) {
  return (
    event.event_type === "agent.message.sent" ||
    event.event_type === liveAssistantEventType ||
    event.event_type === "response_completed"
  )
}

// ---------------------------------------------------------------------------
// P2-3: multi-span turns must keep every assistant span as its own full bubble.
// ---------------------------------------------------------------------------
describe("normalizeSessionEvents — multi-span turns (P2-3)", () => {
  it("keeps each assistant span's full text instead of collapsing to the last token", () => {
    // One turn: span 1 streams "AB", a tool runs, span 2 streams "CD".
    const events = [
      ...tokens("A", "B"),
      ev("agent.message.sent", { text: "AB" }),
      ev("tool_call", { id: "t1", tool: "search" }),
      ev("tool_result", { id: "t1", result: { ok: true } }),
      ...tokens("C", "D"),
      ev("agent.message.sent", { text: "CD" }),
    ]
    const normalized = normalizeSessionEvents(events)
    const texts = assistantTexts(normalized)
    // Before the fix, span 1 collapsed to just "B" (its last token) because the
    // per-turn token list was deduped against the whole turn.
    expect(texts).toEqual(["AB", "CD"])
  })

  it("renders the tool span between the two assistant spans", () => {
    const events = [
      ...tokens("Hello "),
      ev("agent.message.sent", { text: "Hello " }),
      ev("tool_call", { id: "t1", tool: "fetch" }),
      ev("tool_result", { id: "t1", result: "done" }),
      ...tokens("world"),
      ev("agent.message.sent", { text: "world" }),
    ]
    const normalized = normalizeSessionEvents(events)
    const types = normalized.map((event) => event.event_type)
    expect(types).toEqual([
      "agent.message.sent",
      "tool_call",
      "agent.message.sent",
    ])
  })

  it("does not strip a single-span assistant message that equals its tokens", () => {
    const events = [
      ...tokens("The ", "quick ", "fox"),
      ev("agent.message.sent", { text: "The quick fox" }),
    ]
    const normalized = normalizeSessionEvents(events)
    expect(assistantTexts(normalized)).toEqual(["The quick fox"])
  })
})

// ---------------------------------------------------------------------------
// P2-5: live thinking deltas coalesce into one event instead of one-per-frame.
// ---------------------------------------------------------------------------
describe("appendLiveThinkingEvent — coalescing (P2-5)", () => {
  it("merges consecutive thinking deltas into a single trailing event", () => {
    let events: EmployeeSessionEvent[] = []
    events = appendLiveThinkingEvent(events, ev("thinking", { text: "Let " }))
    events = appendLiveThinkingEvent(events, ev("thinking", { text: "me " }))
    events = appendLiveThinkingEvent(events, ev("thinking", { text: "think" }))
    const thinking = events.filter((event) => event.event_type === "thinking")
    expect(thinking).toHaveLength(1)
    expect(eventText(thinking[0])).toBe("Let me think")
  })

  it("starts a new thinking event after a non-thinking event breaks the run", () => {
    let events: EmployeeSessionEvent[] = []
    events = appendLiveThinkingEvent(events, ev("thinking", { text: "first" }))
    events = [...events, ev("agent.message.sent", { text: "answer" })]
    events = appendLiveThinkingEvent(events, ev("thinking", { text: "second" }))
    const thinking = events.filter((event) => event.event_type === "thinking")
    expect(thinking.map((event) => eventText(event))).toEqual([
      "first",
      "second",
    ])
  })

  it("ignores empty deltas", () => {
    const events = appendLiveThinkingEvent([], ev("thinking", { text: "" }))
    expect(events).toEqual([])
  })
})

describe("cleanPersistedAssistantText", () => {
  it("returns the full text when it equals the token concatenation", () => {
    expect(cleanPersistedAssistantText("AB", ["A", "B"])).toBe("AB")
  })

  it("strips a duplicated leading concatenation", () => {
    // Persisted text accidentally carries the whole streamed text twice.
    expect(cleanPersistedAssistantText("ABtail", ["A", "B"])).toBe("tail")
  })

  it("passes through when there are fewer than two tokens", () => {
    expect(cleanPersistedAssistantText("only", ["only"])).toBe("only")
  })
})

// ---------------------------------------------------------------------------
// P2-4: a `final` event must not duplicate the assistant bubble when the turn
// ends on a non-assistant event and the final text is already rendered.
// ---------------------------------------------------------------------------
describe("reconcileLiveFinalEvent — no duplicate bubble (P2-4)", () => {
  const sessionID = "s1"

  it("returns the timeline unchanged when the turn ends on a tool call and final equals rendered", () => {
    // Streamed assistant bubble, then a tool call (the trailing entry), then a
    // `final` event repeating the already-rendered answer.
    const events: EmployeeSessionEvent[] = [
      {
        id: "a1",
        event_type: liveAssistantEventType,
        payload: { text: "All done.", status: "completed" },
      },
      {
        id: "tool1",
        event_type: "tool_call",
        payload: { id: "t1", tool: "noop", status: "completed" },
      },
    ]
    const result = reconcileLiveFinalEvent(events, sessionID, "All done.", 5)
    // Before the fix this appended a second "All done." assistant bubble.
    expect(result).toBe(events)
    const assistant = result.filter(
      (event) => event.event_type === liveAssistantEventType
    )
    expect(assistant).toHaveLength(1)
  })

  it("completes the trailing streamed assistant bubble in place when final matches it", () => {
    // Realistic flow: tokens already filled the bubble with the full streamed
    // text, then `final` repeats it. The bubble is marked completed, not
    // duplicated, and its text is preserved.
    const events: EmployeeSessionEvent[] = [
      {
        id: "a1",
        event_type: liveAssistantEventType,
        payload: { text: "Partial answer", status: "streaming" },
      },
    ]
    const result = reconcileLiveFinalEvent(
      events,
      sessionID,
      "Partial answer",
      5
    )
    const assistant = result.filter(
      (event) => event.event_type === liveAssistantEventType
    )
    expect(assistant).toHaveLength(1)
    expect(eventText(assistant[0])).toBe("Partial answer")
    expect((assistant[0].payload as { status?: string }).status).toBe(
      "completed"
    )
  })

  it("appends a fresh bubble when the turn ends on a tool call and final carries NEW text", () => {
    const events: EmployeeSessionEvent[] = [
      {
        id: "a1",
        event_type: liveAssistantEventType,
        payload: { text: "Working", status: "completed" },
      },
      {
        id: "tool1",
        event_type: "tool_call",
        payload: { id: "t1", tool: "noop", status: "completed" },
      },
    ]
    const result = reconcileLiveFinalEvent(
      events,
      sessionID,
      "Working — here is the result.",
      9
    )
    const assistant = result.filter(
      (event) => event.event_type === liveAssistantEventType
    )
    expect(assistant).toHaveLength(2)
    expect(eventText(assistant[1])).toContain("result")
  })
})
