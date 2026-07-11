import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"

interface SubagentRunOption {
  id: string
  label: string
  identifier: string
  status: string
  detail: string
}

export function subagentRunOptions(
  runs: SessionSubagentRun[]
): SubagentRunOption[] {
  return runs.map((run) => {
    const title = subagentRunTitle(run)

    return {
      id: run.jobId,
      label: title,
      identifier: run.jobId,
      status: subagentRunStatus(run),
      detail: run.error || run.latestText || run.childSessionId || run.jobId,
    }
  })
}

export function subagentRunTitle(run: SessionSubagentRun) {
  return run.agentName?.trim() || "Subagent"
}

export function subagentRunStatus(run: SessionSubagentRun) {
  if (run.status === "completed") return "Completed"
  if (run.status === "failed") return "Failed"
  return "Running"
}
