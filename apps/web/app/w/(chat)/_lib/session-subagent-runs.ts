import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"
import { streamFrameToSessionEvent } from "@/app/w/(chat)/_lib/live-session-stream"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import {
  subagentFrameKey,
  subagentStatusForFrame,
  type SubagentFrameMetadata,
  type SubagentRunStatus,
} from "@/app/w/(chat)/_lib/session-subagents"

export interface SessionSubagentRun {
  jobId: string
  agentName?: string
  parentSessionId?: string
  childSessionId?: string
  status: SubagentRunStatus
  frames: GoSessionStreamFrame[]
  events: SessionEventResponse[]
  latestText?: string
  error?: string
  startedAt?: string
  completedAt?: string
  updatedAt: number
}

const SUBAGENT_DEBUG_EVENT_FLAG = "hivy:debug-subagent-frames"

export const EMPTY_SUBAGENT_RUNS: SessionSubagentRun[] = []

export function appendSubagentRunFrame(
  runsBySessionId: Record<string, SessionSubagentRun[]>,
  sessionId: string,
  frame: GoSessionStreamFrame,
  metadata: SubagentFrameMetadata
) {
  const runs = runsBySessionId[sessionId] ?? EMPTY_SUBAGENT_RUNS
  const currentIndex = runs.findIndex((run) => run.jobId === metadata.jobId)
  const current = currentIndex >= 0 ? runs[currentIndex] : undefined
  const frameStatus = subagentStatusForFrame(frame)
  const status =
    frameStatus === "running" && current && isTerminalStatus(current.status)
      ? current.status
      : (frameStatus ?? current?.status ?? "running")
  const event = streamFrameToSessionEvent(frame)
  const frameKey = subagentFrameKey(frame)
  const frames = current?.frames.some(
    (item) => subagentFrameKey(item) === frameKey
  )
    ? (current.frames ?? [])
    : [...(current?.frames ?? []), frame]
  const events =
    event && !current?.events.some((item) => eventKey(item) === eventKey(event))
      ? [...(current?.events ?? []), event]
      : (current?.events ?? [])
  const occurredAt = timestampFromFrame(frame)
  const payload = framePayloadRecord(frame)
  const text = textFromPayload(payload)
  const error = stringValue(payload, "error")
  const completedAt =
    status === "completed" || status === "failed"
      ? (current?.completedAt ?? occurredAt)
      : current?.completedAt
  const nextRun: SessionSubagentRun = {
    jobId: metadata.jobId,
    agentName: metadata.agentName ?? current?.agentName,
    parentSessionId: metadata.parentSessionId ?? current?.parentSessionId,
    childSessionId: metadata.childSessionId ?? current?.childSessionId,
    status,
    frames,
    events,
    latestText: text || current?.latestText,
    error: error || current?.error,
    startedAt:
      current?.startedAt ??
      (frame.event === "subagent_started" || frame.event === "turn_started"
        ? occurredAt
        : undefined),
    completedAt,
    updatedAt: Date.now(),
  }
  const nextRuns =
    currentIndex >= 0
      ? runs.map((run, index) => (index === currentIndex ? nextRun : run))
      : [...runs, nextRun]

  return {
    ...runsBySessionId,
    [sessionId]: nextRuns,
  }
}

export function sessionSubagentRunsFromEvents(events: SessionEventResponse[]) {
  let runs: SessionSubagentRun[] = []
  for (const event of events) {
    const metadata = subagentEventMetadata(event)
    if (!metadata) continue
    runs = appendSubagentRunEvent(runs, event, metadata)
  }
  return runs
}

export function mergeSubagentRuns(
  ...sources: SessionSubagentRun[][]
): SessionSubagentRun[] {
  const byJob = new Map<string, SessionSubagentRun>()
  for (const runs of sources) {
    for (const run of runs) {
      const current = byJob.get(run.jobId)
      byJob.set(run.jobId, current ? mergeRun(current, run) : run)
    }
  }
  return [...byJob.values()].sort(compareRuns)
}

export function isSubagentSessionEvent(event: SessionEventResponse) {
  return subagentEventMetadata(event) !== null
}

export function dispatchSubagentFrameDebugEvent(
  sessionId: string,
  metadata: SubagentFrameMetadata,
  frame: GoSessionStreamFrame
) {
  if (!isSubagentDebugEventEnabled()) return
  window.dispatchEvent(
    new CustomEvent("hivy:subagent-frame", {
      detail: {
        sessionId,
        jobId: metadata.jobId,
        agentName: metadata.agentName,
        childSessionId: metadata.childSessionId,
        event: frame.event,
        frame,
      },
    })
  )
}

function appendSubagentRunEvent(
  runs: SessionSubagentRun[],
  event: SessionEventResponse,
  metadata: SubagentFrameMetadata
) {
  const currentIndex = runs.findIndex((run) => run.jobId === metadata.jobId)
  const current = currentIndex >= 0 ? runs[currentIndex] : undefined
  const status = mergeStatus(
    current?.status,
    subagentStatusForEventType(event.event_type ?? "")
  )
  const key = eventKey(event)
  const events = current?.events.some((item) => eventKey(item) === key)
    ? (current.events ?? [])
    : [...(current?.events ?? []), event]
  const occurredAt = timestampFromEvent(event)
  const payload = eventPayloadRecord(event)
  const error = stringValue(payload, "error")
  const completedAt =
    status === "completed" || status === "failed"
      ? (current?.completedAt ?? occurredAt)
      : current?.completedAt
  const nextRun: SessionSubagentRun = {
    jobId: metadata.jobId,
    agentName: metadata.agentName ?? current?.agentName,
    parentSessionId: metadata.parentSessionId ?? current?.parentSessionId,
    childSessionId: metadata.childSessionId ?? current?.childSessionId,
    status,
    frames: current?.frames ?? [],
    events,
    latestText: textFromPayload(payload) || current?.latestText,
    error: error || current?.error,
    startedAt:
      current?.startedAt ??
      (event.event_type === "subagent_started" ||
      event.event_type === "turn_started"
        ? occurredAt
        : undefined),
    completedAt,
    updatedAt: Math.max(current?.updatedAt ?? 0, timestampNumber(event)),
  }
  return currentIndex >= 0
    ? runs.map((run, index) => (index === currentIndex ? nextRun : run))
    : [...runs, nextRun]
}

function subagentEventMetadata(
  event: SessionEventResponse
): SubagentFrameMetadata | null {
  const payload = eventPayloadRecord(event)
  const subagent = payloadRecord(payload.subagent)
  const childSessionId =
    stringValue(subagent, "child_session_id") ||
    stringValue(payload, "child_session_id")
  const lifecycle =
    event.event_type === "subagent_started" ||
    event.event_type === "subagent_completed" ||
    event.event_type === "subagent_errored"
  const scoped = payload.scope === "subagent"
  const hasSubagentPayload = Object.keys(subagent).length > 0

  if (!lifecycle && !scoped && !hasSubagentPayload && !childSessionId) {
    return null
  }

  const jobId =
    stringValue(subagent, "job_id") ||
    stringValue(payload, "job_id") ||
    childSessionId ||
    event.event_id ||
    event.id ||
    `${event.event_type}:${event.sequence_number ?? "unknown"}`

  return {
    jobId,
    agentName:
      stringValue(subagent, "agent_name") ||
      stringValue(payload, "agent_name") ||
      stringValue(payload, "agent") ||
      undefined,
    parentSessionId:
      stringValue(subagent, "parent_session_id") ||
      stringValue(payload, "parent_session_id") ||
      event.session_id ||
      undefined,
    childSessionId: childSessionId || undefined,
  }
}

function subagentStatusForEventType(
  eventType: string
): SubagentRunStatus | undefined {
  if (eventType === "subagent_completed" || eventType === "turn_completed") {
    return "completed"
  }
  if (
    eventType === "subagent_errored" ||
    eventType === "turn_failed" ||
    eventType === "error"
  ) {
    return "failed"
  }
  if (eventType === "subagent_started" || eventType === "turn_started") {
    return "running"
  }
  return undefined
}

function mergeRun(
  current: SessionSubagentRun,
  next: SessionSubagentRun
): SessionSubagentRun {
  return {
    jobId: current.jobId,
    agentName: next.agentName ?? current.agentName,
    parentSessionId: next.parentSessionId ?? current.parentSessionId,
    childSessionId: next.childSessionId ?? current.childSessionId,
    status: mergeStatus(current.status, next.status),
    frames: mergeFrames(current.frames, next.frames),
    events: mergeEvents(current.events, next.events),
    latestText: next.latestText || current.latestText,
    error: next.error || current.error,
    startedAt: current.startedAt ?? next.startedAt,
    completedAt: current.completedAt ?? next.completedAt,
    updatedAt: Math.max(current.updatedAt, next.updatedAt),
  }
}

function mergeStatus(
  current?: SubagentRunStatus,
  next?: SubagentRunStatus
): SubagentRunStatus {
  if (!current) return next ?? "running"
  if (!next) return current
  if (next === "failed") return "failed"
  if (next === "completed") return current === "failed" ? current : next
  return isTerminalStatus(current) ? current : next
}

function mergeFrames(
  current: GoSessionStreamFrame[],
  next: GoSessionStreamFrame[]
) {
  const seen = new Set(current.map(subagentFrameKey))
  const merged = [...current]
  for (const frame of next) {
    const key = subagentFrameKey(frame)
    if (seen.has(key)) continue
    seen.add(key)
    merged.push(frame)
  }
  return merged
}

function mergeEvents(
  current: SessionEventResponse[],
  next: SessionEventResponse[]
) {
  const seen = new Set(current.map(eventKey))
  const merged = [...current]
  for (const event of next) {
    const key = eventKey(event)
    if (seen.has(key)) continue
    seen.add(key)
    merged.push(event)
  }
  return merged
}

function compareRuns(left: SessionSubagentRun, right: SessionSubagentRun) {
  const byStatus = statusSort(left.status) - statusSort(right.status)
  if (byStatus !== 0) return byStatus
  return right.updatedAt - left.updatedAt
}

function statusSort(status: SubagentRunStatus) {
  return status === "running" ? 0 : 1
}

function isSubagentDebugEventEnabled() {
  if (process.env.NEXT_PUBLIC_HIVY_DEBUG_SUBAGENT_FRAMES === "1") return true
  if (typeof window === "undefined") return false
  try {
    return window.localStorage.getItem(SUBAGENT_DEBUG_EVENT_FLAG) === "1"
  } catch {
    return false
  }
}

function isTerminalStatus(status: SubagentRunStatus) {
  return status === "completed" || status === "failed"
}

function timestampFromFrame(frame: GoSessionStreamFrame): string | undefined {
  const data = framePayloadRecord(frame)
  return (
    stringValue(data, "occurred_at") ||
    stringValue(data, "ended_at") ||
    stringValue(data, "started_at") ||
    undefined
  )
}

function timestampFromEvent(event: SessionEventResponse) {
  const payload = eventPayloadRecord(event)
  return (
    event.event_at ||
    stringValue(payload, "occurred_at") ||
    stringValue(payload, "ended_at") ||
    stringValue(payload, "started_at") ||
    undefined
  )
}

function timestampNumber(event: SessionEventResponse) {
  const timestamp = timestampFromEvent(event)
  const time = timestamp ? Date.parse(timestamp) : 0
  return Number.isNaN(time) ? 0 : time
}

function eventKey(event: SessionEventResponse) {
  return (
    event.event_id ||
    event.id ||
    `${event.event_type}:${event.sequence_number ?? ""}:${event.event_at ?? ""}`
  )
}

function framePayloadRecord(frame: GoSessionStreamFrame) {
  return payloadRecord(frame.data)
}

function eventPayloadRecord(event: SessionEventResponse) {
  return payloadRecord(event.payload)
}

function payloadRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function textFromPayload(payload: Record<string, unknown>) {
  return (
    stringValue(payload, "text") ||
    stringValue(payload, "message") ||
    stringValue(payload, "content") ||
    stringValue(payload, "markdown") ||
    stringValue(payload, "result_summary")
  )
}

function stringValue(payload: Record<string, unknown>, key: string) {
  const value = payload[key]
  return typeof value === "string" ? value.trim() : ""
}
