import { describe, expect, it } from "vitest"
import { subagentOpenTarget } from "@/app/w/(chat)/_lib/session-subagent-open"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"
import type { SubagentConversationBlock } from "@/app/w/(chat)/_lib/static-data"

function run(overrides: Partial<SessionSubagentRun>): SessionSubagentRun {
  return {
    jobId: "job-1",
    agentName: "explorer",
    childSessionId: "child-1",
    status: "running",
    frames: [],
    events: [],
    updatedAt: 1,
    ...overrides,
  }
}

function block(
  overrides: Partial<SubagentConversationBlock>
): SubagentConversationBlock {
  return {
    type: "subagent",
    key: "tool:tool-xyz",
    agentName: "explorer",
    status: "running",
    ...overrides,
  }
}

describe("subagentOpenTarget", () => {
  it("resolves the second run when two subagents share an agentName", () => {
    const runs = [
      run({ jobId: "job-1", childSessionId: "child-1" }),
      run({ jobId: "job-2", childSessionId: "child-2" }),
    ]

    const target = subagentOpenTarget(
      block({ jobId: "job-2", childSessionId: "child-2" }),
      runs
    )

    expect(target.run?.jobId).toBe("job-2")
    expect(target.activeJobId).toBe("job-2")
  })

  it("matches a running subagent by childSessionId before its job_id resolves", () => {
    const runs = [
      run({ jobId: "job-1", childSessionId: "child-1" }),
      run({ jobId: "job-2", childSessionId: "child-2" }),
    ]

    // The card has no jobId yet (job_id has not propagated to the tool result).
    const target = subagentOpenTarget(
      block({ jobId: undefined, childSessionId: "child-2" }),
      runs
    )

    expect(target.run?.jobId).toBe("job-2")
    expect(target.activeJobId).toBe("job-2")
  })

  it("refuses to guess when agentName is ambiguous and never falls back to block.key", () => {
    const runs = [
      run({ jobId: "job-1", childSessionId: "child-1" }),
      run({ jobId: "job-2", childSessionId: "child-2" }),
    ]

    const target = subagentOpenTarget(
      block({ jobId: undefined, childSessionId: undefined, key: "tool:tool-xyz" }),
      runs
    )

    expect(target.run).toBeUndefined()
    // Must not resolve to a wrong run and must not surface the tool key.
    expect(target.activeJobId).toBeUndefined()
    expect(target.activeJobId).not.toBe("tool:tool-xyz")
  })

  it("resolves a unique agentName match as a last resort", () => {
    const runs = [
      run({ jobId: "job-1", agentName: "explorer", childSessionId: "child-1" }),
      run({ jobId: "job-2", agentName: "writer", childSessionId: "child-2" }),
    ]

    const target = subagentOpenTarget(
      block({ jobId: undefined, childSessionId: undefined, agentName: "writer" }),
      runs
    )

    expect(target.run?.jobId).toBe("job-2")
    expect(target.activeJobId).toBe("job-2")
  })
})
