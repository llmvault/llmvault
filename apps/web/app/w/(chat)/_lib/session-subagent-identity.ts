interface SubagentIdentitySource {
  jobId?: string | null
  childSessionId?: string | null
  agentName?: string | null
  parentSessionId?: string | null
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
 *   3. deterministic synthetic derived from parent_session_id + agent_name
 *
 * The synthetic tail is a last resort for malformed events that carry neither a
 * job id nor a child session id. It is derived only from durable fields shared
 * by the live and history paths so both agree, and it never uses Date.now() or
 * a per-frame/per-event counter (which would mint a new run for every frame).
 */
export function subagentIdentityJobId(source: SubagentIdentitySource): string {
  const jobId = clean(source.jobId)
  if (jobId) return jobId

  const childSessionId = clean(source.childSessionId)
  if (childSessionId) return childSessionId

  const parentSessionId = clean(source.parentSessionId)
  const agentName = clean(source.agentName)
  if (parentSessionId || agentName) {
    return `subagent:${parentSessionId}:${agentName}`
  }

  return "subagent:unknown"
}

function clean(value: string | null | undefined): string {
  return typeof value === "string" ? value.trim() : ""
}
