import { describe, expect, it } from "vitest"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import {
  isSubagentSessionEvent,
  mergeSubagentRuns,
  sessionSubagentRunsFromEvents,
} from "@/app/w/(chat)/_lib/session-subagent-runs"

describe("session subagent runs", () => {
  it("builds subagent runs from persisted history events", () => {
    const events = [
      event("subagent_started", {
        event_id: "start-1",
        job_id: "job-1",
        agent_name: "codebase-explorer",
        child_session_id: "subagent-job-1",
        occurred_at: "2026-06-26T10:00:00Z",
      }),
      event("token", {
        event_id: "token-1",
        text: "child output",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          agent_name: "codebase-explorer",
          parent_session_id: "session-1",
          child_session_id: "subagent-job-1",
        },
      }),
      event("subagent_completed", {
        event_id: "complete-1",
        job_id: "job-1",
        agent_name: "codebase-explorer",
        child_session_id: "subagent-job-1",
        result: "done",
        occurred_at: "2026-06-26T10:00:05Z",
      }),
    ]

    expect(events.every(isSubagentSessionEvent)).toBe(true)
    expect(sessionSubagentRunsFromEvents(events)).toEqual([
      expect.objectContaining({
        jobId: "job-1",
        agentName: "codebase-explorer",
        childSessionId: "subagent-job-1",
        status: "completed",
        latestText: "child output",
        events,
      }),
    ])
  })

  it("does not promote structured completion results into visible text", () => {
    const [run] = sessionSubagentRunsFromEvents([
      event("subagent_completed", {
        event_id: "complete-1",
        job_id: "job-1",
        child_session_id: "subagent-job-1",
        result: JSON.stringify({ raw_import: { files: ["README.md"] } }),
      }),
    ])

    expect(run).toMatchObject({
      jobId: "job-1",
      status: "completed",
    })
    expect(run?.latestText).toBeUndefined()
  })

  it("keeps terminal runs closed when merged with late running frames", () => {
    const completed = sessionSubagentRunsFromEvents([
      event("subagent_completed", {
        event_id: "complete-1",
        job_id: "job-1",
        child_session_id: "subagent-job-1",
        result: "done",
      }),
    ])
    const late = sessionSubagentRunsFromEvents([
      event("token", {
        event_id: "late-token-1",
        text: "late",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          child_session_id: "subagent-job-1",
        },
      }),
    ])

    const merged = mergeSubagentRuns(completed, late)

    expect(merged).toHaveLength(1)
    expect(merged[0]).toMatchObject({
      jobId: "job-1",
      status: "completed",
      latestText: "late",
    })
    expect(merged[0]?.events).toHaveLength(2)
  })

  it("does not classify ordinary parent events as subagent events", () => {
    expect(
      isSubagentSessionEvent(
        event("token", {
          event_id: "parent-token-1",
          text: "parent output",
          session_id: "session-1",
        })
      )
    ).toBe(false)
  })
})

function event(
  eventType: string,
  payload: Record<string, unknown>
): SessionEventResponse {
  return {
    id: String(payload.event_id ?? eventType),
    event_id: String(payload.event_id ?? eventType),
    event_type: eventType,
    event_at: String(payload.occurred_at ?? "2026-06-26T10:00:00Z"),
    session_id: "session-1",
    sequence_number: 1,
    payload,
  } as SessionEventResponse
}
