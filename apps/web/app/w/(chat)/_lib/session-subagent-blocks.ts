import type {
  ConversationBlock,
  SubagentConversationBlock,
  ToolConversationBlock,
} from "@/app/w/(chat)/_lib/static-data"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"

export interface SessionSubagentBlockOptions {
  subagentRuns?: SessionSubagentRun[]
  activeSubagentJobId?: string | null
}

export function isSubagentToolBlock(block: ToolConversationBlock) {
  return block.detail?.tool === "subagent_task"
}

export function subagentBlockFromTool(
  block: ToolConversationBlock,
  options: SessionSubagentBlockOptions
): SubagentConversationBlock {
  const detail = block.detail
  const run = subagentRunForTool(detail, options.subagentRuns ?? [])
  const jobId = detail?.jobId || run?.jobId
  const childSessionId = detail?.childSessionId || run?.childSessionId
  const agentName =
    detail?.agentName || detail?.input || run?.agentName || "Subagent"
  const toolStatus = subagentStatusFromTool(block)
  const status = run ? subagentStatusRanked(run.status, toolStatus) : toolStatus
  return {
    type: "subagent",
    key: jobId ? `subagent:${jobId}` : block.key,
    jobId,
    agentName,
    goal: detail?.goal || detail?.query || undefined,
    childSessionId,
    status,
    preview:
      run?.latestText ||
      detail?.goal ||
      detail?.query ||
      childSessionId ||
      "Subagent task",
    error: run?.error || detail?.error,
    selected: Boolean(jobId && jobId === options.activeSubagentJobId),
  }
}

export function mergeSubagentBlocks(
  current: ConversationBlock,
  next: SubagentConversationBlock,
  options: SessionSubagentBlockOptions
): ConversationBlock {
  if (current.type !== "subagent") return next
  const jobId = next.jobId || current.jobId
  const run = subagentRunForJob(jobId, options.subagentRuns ?? [])
  const status =
    run?.status ?? subagentStatusRanked(current.status, next.status)
  return {
    ...current,
    ...next,
    key: jobId ? `subagent:${jobId}` : (current.key ?? next.key),
    jobId,
    childSessionId: next.childSessionId || current.childSessionId,
    agentName: next.agentName || current.agentName,
    goal: next.goal || current.goal,
    status,
    preview: run?.latestText || next.preview || current.preview,
    error: run?.error || next.error || current.error,
    selected: Boolean(jobId && jobId === options.activeSubagentJobId),
  }
}

function subagentRunForTool(
  detail: ToolConversationBlock["detail"],
  runs: SessionSubagentRun[]
) {
  const jobRun = subagentRunForJob(detail?.jobId, runs)
  if (jobRun) return jobRun
  const childSessionId = detail?.childSessionId?.trim()
  if (childSessionId) {
    const match = runs.find((run) => run.childSessionId === childSessionId)
    if (match) return match
  }
  const agentName = (detail?.agentName || detail?.input || "").trim()
  if (!agentName) return undefined
  const matches = runs.filter((run) => run.agentName === agentName)
  return matches.length === 1 ? matches[0] : undefined
}

function subagentRunForJob(
  jobId: string | undefined,
  runs: SessionSubagentRun[]
) {
  if (!jobId) return undefined
  return runs.find((run) => run.jobId === jobId)
}

function subagentStatusFromTool(
  block: ToolConversationBlock
): SubagentConversationBlock["status"] {
  const status = block.detail?.status
  if (status === "errored") return "failed"
  if (status === "completed") return "completed"
  return "running"
}

function subagentStatusRanked(
  current: SubagentConversationBlock["status"],
  next: SubagentConversationBlock["status"]
) {
  if (next === "failed" || current === "failed") return "failed"
  if (next === "completed" || current === "completed") return "completed"
  return "running"
}
