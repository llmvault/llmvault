import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"

export type SubagentRunStatus = "running" | "completed" | "failed"

export interface SubagentFrameMetadata {
  jobId: string
  agentName?: string
  parentSessionId?: string
  childSessionId?: string
}

export function isSubagentFrame(frame: GoSessionStreamFrame) {
  return subagentFrameMetadata(frame) !== null
}

export function subagentFrameMetadata(
  frame: GoSessionStreamFrame
): SubagentFrameMetadata | null {
  const data = payloadRecord(frame.data)
  const subagent = payloadRecord(data.subagent)
  const childSessionId =
    stringValue(subagent, "child_session_id") ||
    stringValue(data, "child_session_id")
  const scoped = data.scope === "subagent"
  const hasSubagentPayload = Object.keys(subagent).length > 0

  if (!scoped && !hasSubagentPayload && !childSessionId) return null

  const jobId =
    stringValue(subagent, "job_id") ||
    stringValue(data, "job_id") ||
    childSessionId ||
    frame.id ||
    `${frame.event}:${Date.now()}`

  return {
    jobId,
    agentName:
      stringValue(subagent, "agent_name") ||
      stringValue(data, "agent_name") ||
      undefined,
    parentSessionId:
      stringValue(subagent, "parent_session_id") ||
      stringValue(data, "parent_session_id") ||
      undefined,
    childSessionId: childSessionId || undefined,
  }
}

export function subagentStatusForFrame(
  frame: GoSessionStreamFrame
): SubagentRunStatus | undefined {
  if (
    frame.event === "subagent_completed" ||
    frame.event === "turn_completed"
  ) {
    return "completed"
  }
  if (
    frame.event === "subagent_errored" ||
    frame.event === "turn_failed" ||
    frame.event === "error"
  ) {
    return "failed"
  }
  if (
    frame.event === "subagent_started" ||
    frame.event === "turn_started" ||
    isSubagentFrame(frame)
  ) {
    return "running"
  }
  return undefined
}

export function subagentFrameKey(frame: GoSessionStreamFrame) {
  const data = payloadRecord(frame.data)
  const eventID = stringValue(data, "event_id") || frame.id
  if (eventID) return eventID
  const sequence = data.sequence
  return `${frame.event}:${typeof sequence === "number" ? sequence : "unknown"}`
}

function payloadRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function stringValue(payload: Record<string, unknown>, key: string) {
  const value = payload[key]
  return typeof value === "string" ? value.trim() : ""
}
