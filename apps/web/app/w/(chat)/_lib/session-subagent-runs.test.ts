import { describe, expect, it } from "vitest"
import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import {
  appendSubagentRunFrame,
  isSubagentSessionEvent,
  mergeSubagentRuns,
  sessionSubagentRunsFromEvents,
  type SessionSubagentRun,
} from "@/app/w/(chat)/_lib/session-subagent-runs"
import { subagentFrameMetadata } from "@/app/w/(chat)/_lib/session-subagents"

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

  it("dedupes to one run when the same subagent is seen live then on reload without a job_id", () => {
    // Live frames observe the subagent by child_session_id only (job_id absent).
    const liveRuns = runsFromFrames([
      frame("subagent_started", {
        event_id: "live-start-1",
        scope: "subagent",
        subagent: {
          child_session_id: "child-1",
          agent_name: "explorer",
        },
      }),
      frame("token", {
        event_id: "live-token-1",
        text: "live output",
        scope: "subagent",
        subagent: { child_session_id: "child-1" },
      }),
    ])

    // History reload of the same subagent, also keyed by child_session_id.
    const historyRuns = sessionSubagentRunsFromEvents([
      event("subagent_completed", {
        event_id: "history-complete-1",
        child_session_id: "child-1",
        agent_name: "explorer",
        result: "done",
      }),
    ])

    expect(liveRuns).toHaveLength(1)
    expect(historyRuns).toHaveLength(1)
    expect(liveRuns[0]?.jobId).toBe("child-1")
    expect(historyRuns[0]?.jobId).toBe("child-1")

    const merged = mergeSubagentRuns(liveRuns, historyRuns)
    expect(merged).toHaveLength(1)
    expect(merged[0]).toMatchObject({ jobId: "child-1", status: "completed" })
  })

  it("ignores malformed subagent events without a unique run id", () => {
    const runs = runsFromFrames([
      frame("subagent_started", {
        event_id: "f1",
        scope: "subagent",
        subagent: { agent_name: "explorer", parent_session_id: "session-1" },
      }),
      frame("token", {
        event_id: "f2",
        text: "one",
        scope: "subagent",
        subagent: { agent_name: "explorer", parent_session_id: "session-1" },
      }),
      frame("token", {
        event_id: "f3",
        text: "two",
        scope: "subagent",
        subagent: { agent_name: "explorer", parent_session_id: "session-1" },
      }),
    ])

    expect(runs).toEqual([])

    const historyRuns = sessionSubagentRunsFromEvents([
      event("token", {
        event_id: "h1",
        text: "reload",
        scope: "subagent",
        session_id: "session-1",
        subagent: { agent_name: "explorer" },
      }),
    ])
    expect(historyRuns).toEqual([])
  })

  it("never assigns a clock/Date.now()-based run id", () => {
    const before = Date.now()
    const runs = [
      ...runsFromFrames([
        frame("subagent_started", {
          event_id: "f1",
          scope: "subagent",
          subagent: { agent_name: "explorer", parent_session_id: "session-1" },
        }),
      ]),
      ...sessionSubagentRunsFromEvents([
        event("token", {
          event_id: "h1",
          text: "x",
          scope: "subagent",
          child_session_id: "child-1",
          subagent: {},
        }),
      ]),
    ]
    const after = Date.now()

    for (const run of runs) {
      // No 13-digit millisecond timestamp embedded anywhere in the id.
      expect(run.jobId).not.toMatch(/\d{13}/)
      // And it is not any concrete clock value produced during this test.
      for (let ts = before; ts <= after; ts += 1) {
        expect(run.jobId).not.toContain(String(ts))
      }
    }
  })

  it("reconstructs run.events from persisted scoped DETAIL events without a lifecycle event", () => {
    const events = [
      event("token", {
        event_id: "detail-1",
        text: "child thinking",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          agent_name: "explorer",
          child_session_id: "child-1",
        },
      }),
      event("tool_call", {
        event_id: "detail-2",
        tool: "bash",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          child_session_id: "child-1",
        },
      }),
    ]

    const runs = sessionSubagentRunsFromEvents(events)
    expect(runs).toHaveLength(1)
    expect(runs[0]?.jobId).toBe("job-1")
    expect(runs[0]?.events).toHaveLength(2)
    expect(runs[0]?.events).toEqual(events)
  })
})

function frame(
  eventType: string,
  data: Record<string, unknown>
): GoSessionStreamFrame {
  return {
    sessionId: "session-1",
    event: eventType,
    id: String(data.event_id ?? eventType),
    data,
  }
}

function runsFromFrames(frames: GoSessionStreamFrame[]): SessionSubagentRun[] {
  let bySession: Record<string, SessionSubagentRun[]> = {}
  for (const item of frames) {
    const metadata = subagentFrameMetadata(item)
    if (!metadata) continue
    bySession = appendSubagentRunFrame(bySession, "session-1", item, metadata)
  }
  return bySession["session-1"] ?? []
}

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
