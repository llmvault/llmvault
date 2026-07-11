interface SubagentIdentitySource {
  jobId?: string | null
  childSessionId?: string | null
}

/**
 * Resolves the stable jobId used to group every frame/event that belongs to a
 * single subagent run. The SAME subagent must resolve to the SAME id whether it
 * is observed live (stream frames) or reconstructed from persisted history
 * events — otherwise `mergeSubagentRuns` keeps duplicate ("phantom") runs.
 *
 * Resolution order (deterministic, never clock/counter based):
 *   1. job_id            — authoritative subagent identity
 *   2. child_session_id  — stable per-run identity present on subagent events
 */
export function subagentIdentityJobId(source: SubagentIdentitySource): string {
  const jobId = clean(source.jobId)
  if (jobId) return jobId

  const childSessionId = clean(source.childSessionId)
  if (childSessionId) return childSessionId

  return ""
}

function clean(value: string | null | undefined): string {
  return typeof value === "string" ? value.trim() : ""
}
