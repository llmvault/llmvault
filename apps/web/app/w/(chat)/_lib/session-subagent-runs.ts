import type { DirectSessionStreamFrame } from "@/app/w/(chat)/_lib/direct-session-stream"
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
  frames: DirectSessionStreamFrame[]
  events: SessionEventResponse[]
  startedAt?: string
  completedAt?: string
  updatedAt: number
}

const SUBAGENT_DEBUG_EVENT_FLAG = "hivy:debug-subagent-frames"

export const EMPTY_SUBAGENT_RUNS: SessionSubagentRun[] = []

export function appendSubagentRunFrame(
  runsBySessionId: Record<string, SessionSubagentRun[]>,
  sessionId: string,
  frame: DirectSessionStreamFrame,
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

export function dispatchSubagentFrameDebugEvent(
  sessionId: string,
  metadata: SubagentFrameMetadata,
  frame: DirectSessionStreamFrame
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

function timestampFromFrame(
  frame: DirectSessionStreamFrame
): string | undefined {
  const data = payloadRecord(frame.data)
  return (
    stringValue(data, "occurred_at") ||
    stringValue(data, "ended_at") ||
    stringValue(data, "started_at") ||
    undefined
  )
}

function eventKey(event: SessionEventResponse) {
  return (
    event.event_id ||
    event.id ||
    `${event.event_type}:${event.sequence_number ?? ""}:${event.event_at ?? ""}`
  )
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
