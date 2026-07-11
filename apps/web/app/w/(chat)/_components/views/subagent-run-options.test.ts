import { describe, expect, it } from "vitest"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"
import { subagentRunOptions } from "./subagent-run-options"

function run(
  jobId: string,
  status: SessionSubagentRun["status"],
  overrides: Partial<SessionSubagentRun> = {}
): SessionSubagentRun {
  return {
    jobId,
    agentName: "codebase-explorer",
    status,
    frames: [],
    events: [],
    updatedAt: 1,
    ...overrides,
  }
}

describe("subagentRunOptions", () => {
  it("exposes unique job ids for parallel runs", () => {
    expect(
      subagentRunOptions([
        run("job-login", "completed", {
          latestText: "Reviewed login",
          startedAt: "2026-07-11T15:31:48.841Z",
        }),
        run("job-channel", "running", {
          latestText: "Reviewing channels",
          startedAt: "2026-07-11T15:31:48.842Z",
        }),
        run("job-tests", "failed", {
          error: "Sandbox stopped",
          startedAt: "2026-07-11T15:31:48.843Z",
        }),
      ])
    ).toEqual([
      {
        id: "job-login",
        label: "codebase-explorer",
        identifier: "job-login",
        status: "Completed",
        detail: "Reviewed login",
      },
      {
        id: "job-channel",
        label: "codebase-explorer",
        identifier: "job-channel",
        status: "Running",
        detail: "Reviewing channels",
      },
      {
        id: "job-tests",
        label: "codebase-explorer",
        identifier: "job-tests",
        status: "Failed",
        detail: "Sandbox stopped",
      },
    ])
  })

  it("keeps a unique agent name uncluttered", () => {
    expect(
      subagentRunOptions([
        run("job-1", "completed", {
          agentName: "researcher",
          childSessionId: "child-1",
        }),
      ])[0]
    ).toEqual({
      id: "job-1",
      label: "researcher",
      identifier: "job-1",
      status: "Completed",
      detail: "child-1",
    })
  })

  it("uses unique job ids instead of name-based numbering", () => {
    const first = run("job-1", "completed", {
      startedAt: "2026-07-11T15:31:48.841Z",
    })
    const second = run("job-2", "running", {
      startedAt: "2026-07-11T15:31:48.842Z",
    })

    expect(
      subagentRunOptions([second, first]).map((option) => option.identifier)
    ).toEqual(["job-2", "job-1"])
  })
})
