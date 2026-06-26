import { afterEach, describe, expect, it, vi } from "vitest"
import {
  fetchSubagentSessionEvents,
  hydrateCompletedSubagentRun,
} from "@/app/w/(chat)/_lib/session-subagent-events"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"
import { useSessionRuntimeStore } from "@/app/w/(chat)/_stores/session-runtime-store"

const originalFetch = global.fetch

afterEach(() => {
  global.fetch = originalFetch
  useSessionRuntimeStore.setState({
    statusBySessionId: {},
    liveEventsBySessionId: {},
    subagentRunsBySessionId: {},
    usageBySessionId: {},
    usageEventKeysBySessionId: {},
    cursorBySessionId: {},
    reconnectAttemptsBySessionId: {},
  })
  vi.restoreAllMocks()
})

describe("fetchSubagentSessionEvents", () => {
  it("fetches every page for a child subagent session", async () => {
    const first = event("event-1", "first")
    const second = event("event-2", "second")
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({
          data: [first],
          has_more: true,
          next_cursor: "cursor-2",
        })
      )
      .mockResolvedValueOnce(
        jsonResponse({
          data: [second],
          has_more: false,
        })
      )
    global.fetch = fetchMock as unknown as typeof fetch

    const events = await fetchSubagentSessionEvents("parent session", "child/a")

    expect(events).toEqual([first, second])
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/proxy/v1/sessions/parent%20session/subagents/child%2Fa/events?limit=100",
      expect.objectContaining({
        headers: { Accept: "application/json" },
      })
    )
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/proxy/v1/sessions/parent%20session/subagents/child%2Fa/events?limit=100&cursor=cursor-2",
      expect.any(Object)
    )
  })

  it("keeps running subagents on stream state without fetching history", async () => {
    const fetchMock = vi.fn()
    global.fetch = fetchMock as unknown as typeof fetch

    await hydrateCompletedSubagentRun({
      parentSessionId: "parent-session",
      run: subagentRun("running"),
    })

    expect(fetchMock).not.toHaveBeenCalled()
    expect(
      useSessionRuntimeStore.getState().subagentRunsBySessionId[
        "parent-session"
      ]
    ).toBeUndefined()
  })

  it("hydrates completed subagents from durable history", async () => {
    const durableEvent = event("durable-event-1", "from db")
    global.fetch = vi.fn().mockResolvedValue(
      jsonResponse({
        data: [durableEvent],
        has_more: false,
      })
    ) as unknown as typeof fetch

    await hydrateCompletedSubagentRun({
      parentSessionId: "parent-session",
      run: subagentRun("completed"),
    })

    expect(
      useSessionRuntimeStore.getState().subagentRunsBySessionId[
        "parent-session"
      ]?.[0]
    ).toMatchObject({
      jobId: "job-1",
      childSessionId: "child-1",
      status: "completed",
      events: [durableEvent],
    })
  })
})

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function event(id: string, text: string): SessionEventResponse {
  return {
    id,
    event_id: id,
    event_type: "token",
    event_at: "2026-06-26T14:00:00.000Z",
    payload: { text },
  } as SessionEventResponse
}

function subagentRun(status: SessionSubagentRun["status"]): SessionSubagentRun {
  return {
    jobId: "job-1",
    agentName: "codebase-reader",
    childSessionId: "child-1",
    status,
    frames: [],
    events: status === "running" ? [event("live-event-1", "from stream")] : [],
    updatedAt: 1,
  }
}
