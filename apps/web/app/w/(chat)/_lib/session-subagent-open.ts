import type { SubagentConversationBlock } from "@/app/w/(chat)/_lib/static-data"
import { subagentIdentityJobId } from "@/app/w/(chat)/_lib/session-subagent-identity"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"

export function subagentOpenTarget(
  block: SubagentConversationBlock,
  runs: SessionSubagentRun[]
) {
  const run = subagentRunForBlock(block, runs) ?? subagentRunFromBlock(block)
  // Prefer the resolved run's id; otherwise derive a matchable id from the
  // block. Never fall back to `block.key` ("tool:<id>"), which can never equal
  // any run.jobId and would strand the panel on a "waiting" state forever.
  return {
    activeJobId: run?.jobId || block.jobId || block.childSessionId,
    run,
  }
}

function subagentRunForBlock(
  block: SubagentConversationBlock,
  runs: SessionSubagentRun[]
) {
  // Stable ids (jobId, then childSessionId) are the primary keys.
  if (block.jobId) {
    const match = runs.find((run) => run.jobId === block.jobId)
    if (match) return match
  }
  if (block.childSessionId) {
    const match = runs.find(
      (run) => run.childSessionId === block.childSessionId
    )
    if (match) return match
  }
  return undefined
}

function subagentRunFromBlock(
  block: SubagentConversationBlock
): SessionSubagentRun | undefined {
  if (!block.jobId && !block.childSessionId) return undefined
  const jobId = subagentIdentityJobId({
    jobId: block.jobId,
    childSessionId: block.childSessionId,
  })
  return {
    jobId,
    agentName: block.agentName,
    childSessionId: block.childSessionId,
    status: block.status,
    frames: [],
    events: [],
    latestText: block.preview,
    error: block.error,
    updatedAt: Date.now(),
  }
}
