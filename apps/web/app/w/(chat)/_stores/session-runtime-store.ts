"use client"

import { create } from "zustand"
import { subscribeWithSelector } from "zustand/middleware"
import {
  goSessionStreamCursor,
  type GoSessionStreamCursor,
  type GoSessionStreamFrame,
} from "@/app/w/(chat)/_lib/go-session-stream"
import {
  appendLiveSessionStreamFrame,
  isTerminalStreamFrame,
} from "@/app/w/(chat)/_lib/live-session-stream"
import {
  appendSubagentRunFrame,
  dispatchSubagentFrameDebugEvent,
  EMPTY_SUBAGENT_RUNS,
  type SessionSubagentRun,
} from "@/app/w/(chat)/_lib/session-subagent-runs"
import { subagentFrameMetadata } from "@/app/w/(chat)/_lib/session-subagents"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import type { components } from "@/lib/api/schema"

export type SessionRuntimeStatus =
  | "idle"
  | "queued"
  | "streaming"
  | "waiting_for_user"
  | "stopped"
  | "failed"

export type SessionLastTurnOutcome = "completed" | "stopped" | "failed"

export interface PendingInputRequest {
  requestId: string
  prompt?: string
  options?: unknown
  turnId?: string
  eventId?: string
  requestedAt: string
}

export interface SessionRuntimeSummary {
  status: SessionRuntimeStatus
  lastOutcome?: SessionLastTurnOutcome
  error?: string
  pendingInput?: PendingInputRequest
  updatedAt: number
}

interface SessionRuntimeStoreState {
  statusBySessionId: Record<string, SessionRuntimeSummary>
  liveEventsBySessionId: Record<string, SessionEventResponse[]>
  subagentRunsBySessionId: Record<string, SessionSubagentRun[]>
  cursorBySessionId: Record<string, GoSessionStreamCursor | undefined>
  reconnectAttemptsBySessionId: Record<string, number>
  hydrateSession: (session: SessionResponse | undefined) => void
  setStatus: (
    sessionId: string,
    status: SessionRuntimeStatus,
    patch?: Partial<Omit<SessionRuntimeSummary, "status" | "updatedAt">>
  ) => void
  setLiveEvents: (sessionId: string, events: SessionEventResponse[]) => void
  reconcileLiveEvents: (
    sessionId: string,
    historyEvents: SessionEventResponse[]
  ) => void
  appendStreamError: (sessionId: string, message: string) => void
  applyStreamFrame: (sessionId: string, frame: GoSessionStreamFrame) => void
  finishStream: (
    sessionId: string,
    options?: {
      preserveError?: boolean
      outcome?: SessionLastTurnOutcome
      clearLiveEvents?: boolean
    }
  ) => void
  clearSessionRuntime: (sessionId: string) => void
  setReconnectAttempt: (sessionId: string, attempt: number) => void
}

type SessionResponse = components["schemas"]["sessionResponse"]

const EMPTY_EVENTS: SessionEventResponse[] = []
export const IDLE_RUNTIME_SUMMARY: SessionRuntimeSummary = {
  status: "idle",
  updatedAt: 0,
}

export const useSessionRuntimeStore = create<SessionRuntimeStoreState>()(
  subscribeWithSelector((setState, getState) => ({
    statusBySessionId: {},
    liveEventsBySessionId: {},
    subagentRunsBySessionId: {},
    cursorBySessionId: {},
    reconnectAttemptsBySessionId: {},
    hydrateSession(session) {
      const sessionId = session?.id
      if (!sessionId) return
      const status = sessionRuntimeStatusFromResponse(session)
      const lastOutcome = sessionLastOutcomeFromResponse(session)
      const existing = getState().statusBySessionId[sessionId]
      if (
        existing &&
        existing.status !== "idle" &&
        existing.status !== "stopped" &&
        existing.status !== "failed"
      ) {
        return
      }
      getState().setStatus(sessionId, status, {
        lastOutcome,
        error: existing?.error,
        pendingInput: existing?.pendingInput,
      })
    },
    setStatus(sessionId, status, patch = {}) {
      setState((state) => ({
        statusBySessionId: {
          ...state.statusBySessionId,
          [sessionId]: {
            ...state.statusBySessionId[sessionId],
            ...patch,
            status,
            updatedAt: Date.now(),
          },
        },
      }))
    },
    setLiveEvents(sessionId, events) {
      setState((state) => ({
        liveEventsBySessionId: {
          ...state.liveEventsBySessionId,
          [sessionId]: events,
        },
      }))
    },
    reconcileLiveEvents(sessionId, historyEvents) {
      if (historyEvents.length === 0) return
      setState((state) => {
        const current = state.liveEventsBySessionId[sessionId] ?? EMPTY_EVENTS
        if (current.length === 0) return state
        const historyByKey = historyEventMap(historyEvents)
        const next = current.filter(
          (event) => !historyContainsLiveEvent(historyByKey, event)
        )
        if (next.length === current.length) return state
        return {
          liveEventsBySessionId: {
            ...state.liveEventsBySessionId,
            [sessionId]: next,
          },
        }
      })
    },
    appendStreamError(sessionId, message) {
      setState((state) => {
        const current = state.liveEventsBySessionId[sessionId] ?? EMPTY_EVENTS
        return {
          liveEventsBySessionId: {
            ...state.liveEventsBySessionId,
            [sessionId]: [
              ...current.filter((event) => event.event_type !== "error"),
              streamErrorEvent(sessionId, message),
            ],
          },
          statusBySessionId: {
            ...state.statusBySessionId,
            [sessionId]: {
              ...state.statusBySessionId[sessionId],
              status: "failed",
              lastOutcome: "failed",
              error: message,
              updatedAt: Date.now(),
            },
          },
        }
      })
    },
    applyStreamFrame(sessionId, frame) {
      const nextCursor = goSessionStreamCursor(frame)
      const subagentMetadata = subagentFrameMetadata(frame)
      setState((state) => {
        const cursorPatch =
          nextCursor &&
          shouldAdvanceCursor(state.cursorBySessionId[sessionId], nextCursor)
            ? {
                cursorBySessionId: {
                  ...state.cursorBySessionId,
                  [sessionId]: nextCursor,
                },
              }
            : {}
        if (subagentMetadata) {
          return {
            ...cursorPatch,
            subagentRunsBySessionId: appendSubagentRunFrame(
              state.subagentRunsBySessionId,
              sessionId,
              frame,
              subagentMetadata
            ),
          }
        }

        const summary = summaryForFrame(
          state.statusBySessionId[sessionId],
          frame
        )
        const currentEvents =
          state.liveEventsBySessionId[sessionId] ?? EMPTY_EVENTS
        const sourceEvents = shouldReplaceOptimisticWork(frame.event)
          ? currentEvents.filter((event) => !isPendingClientEvent(event))
          : currentEvents
        const nextEvents = appendLiveSessionStreamFrame(sourceEvents, frame)
        return {
          ...cursorPatch,
          statusBySessionId: {
            ...state.statusBySessionId,
            [sessionId]: summary,
          },
          liveEventsBySessionId:
            nextEvents === currentEvents
              ? state.liveEventsBySessionId
              : {
                  ...state.liveEventsBySessionId,
                  [sessionId]: nextEvents,
                },
        }
      })
      if (subagentMetadata) {
        dispatchSubagentFrameDebugEvent(sessionId, subagentMetadata, frame)
      }
    },
    finishStream(sessionId, options = {}) {
      setState((state) => {
        const current = state.liveEventsBySessionId[sessionId] ?? EMPTY_EVENTS
        const events = options.clearLiveEvents
          ? EMPTY_EVENTS
          : options.preserveError
            ? current.filter((event) => event.event_type === "error")
            : current
        return {
          statusBySessionId: {
            ...state.statusBySessionId,
            [sessionId]: {
              ...state.statusBySessionId[sessionId],
              status:
                options.outcome === "failed"
                  ? "failed"
                  : options.outcome === "stopped"
                    ? "stopped"
                    : "idle",
              lastOutcome: options.outcome ?? "completed",
              pendingInput: undefined,
              updatedAt: Date.now(),
            },
          },
          liveEventsBySessionId: {
            ...state.liveEventsBySessionId,
            [sessionId]: events,
          },
          reconnectAttemptsBySessionId: {
            ...state.reconnectAttemptsBySessionId,
            [sessionId]: 0,
          },
        }
      })
    },
    clearSessionRuntime(sessionId) {
      setState((state) => {
        const { [sessionId]: _status, ...statusBySessionId } =
          state.statusBySessionId
        const { [sessionId]: _events, ...liveEventsBySessionId } =
          state.liveEventsBySessionId
        const { [sessionId]: _subagents, ...subagentRunsBySessionId } =
          state.subagentRunsBySessionId
        const { [sessionId]: _cursor, ...cursorBySessionId } =
          state.cursorBySessionId
        const { [sessionId]: _attempts, ...reconnectAttemptsBySessionId } =
          state.reconnectAttemptsBySessionId
        return {
          statusBySessionId,
          liveEventsBySessionId,
          subagentRunsBySessionId,
          cursorBySessionId,
          reconnectAttemptsBySessionId,
        }
      })
    },
    setReconnectAttempt(sessionId, attempt) {
      setState((state) => ({
        reconnectAttemptsBySessionId: {
          ...state.reconnectAttemptsBySessionId,
          [sessionId]: attempt,
        },
      }))
    },
  }))
)

export function useSessionRuntimeSummary(sessionId?: string) {
  return useSessionRuntimeStore((state) =>
    sessionId
      ? (state.statusBySessionId[sessionId] ?? IDLE_RUNTIME_SUMMARY)
      : IDLE_RUNTIME_SUMMARY
  )
}

export function useSessionRuntimeStatus(sessionId?: string) {
  return useSessionRuntimeStore((state) =>
    sessionId ? (state.statusBySessionId[sessionId]?.status ?? "idle") : "idle"
  )
}

export function useSessionLiveEvents(sessionId?: string) {
  return useSessionRuntimeStore((state) =>
    sessionId
      ? (state.liveEventsBySessionId[sessionId] ?? EMPTY_EVENTS)
      : EMPTY_EVENTS
  )
}

export function useSessionSubagentRuns(sessionId?: string) {
  return useSessionRuntimeStore((state) =>
    sessionId
      ? (state.subagentRunsBySessionId[sessionId] ?? EMPTY_SUBAGENT_RUNS)
      : EMPTY_SUBAGENT_RUNS
  )
}

export function sessionRuntimeSummary(sessionId?: string) {
  if (!sessionId) return IDLE_RUNTIME_SUMMARY
  return (
    useSessionRuntimeStore.getState().statusBySessionId[sessionId] ??
    IDLE_RUNTIME_SUMMARY
  )
}

export function sessionRuntimeStatusFromResponse(
  session: SessionResponse
): SessionRuntimeStatus {
  const turnStatus = stringValue(session, "agent_turn_status")
  if (turnStatus === "active") return "streaming"
  if (turnStatus === "queued") return "queued"
  if (turnStatus === "waiting_for_user") return "waiting_for_user"
  const outcome = sessionLastOutcomeFromResponse(session)
  if (outcome === "stopped") return "stopped"
  if (outcome === "failed") return "failed"
  return "idle"
}

function sessionLastOutcomeFromResponse(
  session: SessionResponse
): SessionLastTurnOutcome | undefined {
  const value = stringValue(session, "last_turn_outcome")
  return value === "completed" || value === "stopped" || value === "failed"
    ? value
    : undefined
}

function summaryForFrame(
  current: SessionRuntimeSummary | undefined,
  frame: GoSessionStreamFrame
): SessionRuntimeSummary {
  const pendingInput = pendingInputFromFrame(frame)
  if (pendingInput) {
    return {
      ...current,
      status: "waiting_for_user",
      pendingInput,
      updatedAt: Date.now(),
    }
  }
  if (frame.event === "question_answered") {
    return {
      ...current,
      status: "streaming",
      pendingInput: undefined,
      updatedAt: Date.now(),
    }
  }
  if (isTerminalStreamFrame(frame)) {
    const message = terminalFrameErrorMessage(frame)
    if (message) {
      return {
        ...current,
        status: "failed",
        lastOutcome: "failed",
        error: message,
        pendingInput: undefined,
        updatedAt: Date.now(),
      }
    }
    return {
      ...current,
      status: "idle",
      lastOutcome: "completed",
      pendingInput: undefined,
      updatedAt: Date.now(),
    }
  }
  if (isActiveStreamFrame(frame.event)) {
    return {
      ...current,
      status: "streaming",
      error: undefined,
      updatedAt: Date.now(),
    }
  }
  return {
    ...(current ?? IDLE_RUNTIME_SUMMARY),
    updatedAt: current?.updatedAt ?? Date.now(),
  }
}

function pendingInputFromFrame(
  frame: GoSessionStreamFrame
): PendingInputRequest | undefined {
  if (frame.event !== "question_requested") return undefined
  const data = payloadRecord(frame.data)
  const requestId =
    stringValue(data, "request_id") ||
    stringValue(data, "question_id") ||
    stringValue(data, "id") ||
    frame.id
  return {
    requestId,
    prompt:
      stringValue(data, "prompt") ||
      stringValue(data, "question") ||
      stringValue(data, "message") ||
      stringValue(data, "text") ||
      undefined,
    options: data.options,
    turnId: stringValue(data, "turn_id") || undefined,
    eventId: stringValue(data, "event_id") || frame.id || undefined,
    requestedAt:
      stringValue(data, "occurred_at") ||
      stringValue(data, "requested_at") ||
      new Date().toISOString(),
  }
}

function shouldAdvanceCursor(
  current: GoSessionStreamCursor | undefined,
  next: GoSessionStreamCursor
) {
  return (
    !current ||
    current.streamId !== next.streamId ||
    next.sequence > current.sequence
  )
}

function shouldReplaceOptimisticWork(event: string) {
  return (
    event === "thinking" ||
    event === "token" ||
    event === "tool_call" ||
    event === "tool_result" ||
    event === "final" ||
    event === "error"
  )
}

function isActiveStreamFrame(event: string) {
  return (
    event === "turn_started" ||
    event === "thinking" ||
    event === "token" ||
    event === "tool_call" ||
    event === "tool_result" ||
    event === "plan_updated" ||
    event === "final"
  )
}

function isPendingClientEvent(event: SessionEventResponse) {
  const payload = payloadRecord(event.payload)
  return payload.client_status === "pending"
}

function historyEventMap(events: SessionEventResponse[]) {
  const byKey = new Map<string, SessionEventResponse>()
  for (const event of events) {
    for (const key of eventKeys(event)) {
      byKey.set(key, event)
    }
  }
  return byKey
}

function historyContainsLiveEvent(
  historyByKey: Map<string, SessionEventResponse>,
  live: SessionEventResponse
) {
  for (const key of eventKeys(live)) {
    const history = historyByKey.get(key)
    if (!history) continue
    if (isMergedTextEvent(live)) {
      return (
        eventText(history).length >= eventText(live).length ||
        (history.sequence_number ?? 0) >= (live.sequence_number ?? 0)
      )
    }
    return true
  }
  return false
}

function eventKeys(event: SessionEventResponse) {
  return [event.id, event.event_id].filter((key): key is string => Boolean(key))
}

function isMergedTextEvent(event: SessionEventResponse) {
  return event.event_type === "thinking" || event.event_type === "token"
}

function eventText(event: SessionEventResponse) {
  const payload = payloadRecord(event.payload)
  return (
    stringValue(payload, "text") ||
    stringValue(payload, "message") ||
    stringValue(payload, "content") ||
    stringValue(payload, "markdown") ||
    stringValue(payload, "result_summary")
  )
}

function terminalFrameErrorMessage(frame: GoSessionStreamFrame): string {
  if (frame.event !== "error" && frame.event !== "turn_failed") return ""
  const fallback =
    frame.event === "turn_failed"
      ? "The agent turn failed."
      : "The live session stream failed."
  const record = payloadRecord(frame.data)
  if (record.interrupted === true) return ""
  const value = record.error ?? record.message ?? record.text
  if (
    typeof value === "string" &&
    value.trim().toLowerCase() === "interrupted by user"
  ) {
    return ""
  }
  return typeof value === "string" && value.trim() ? value : fallback
}

function streamErrorEvent(
  sessionId: string,
  message: string
): SessionEventResponse {
  const now = new Date().toISOString()
  const id = `stream-error:${Date.now()}`
  return {
    id,
    session_id: sessionId,
    event_id: id,
    event_type: "error",
    sequence_number: Number.MAX_SAFE_INTEGER,
    payload: {
      message,
    },
    event_at: now,
  } as SessionEventResponse
}

function payloadRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function stringValue(record: Record<string, unknown>, key: string): string
function stringValue(session: SessionResponse, key: string): string
function stringValue(
  value: Record<string, unknown> | SessionResponse,
  key: string
) {
  const record = value as Record<string, unknown>
  const raw = record[key]
  return typeof raw === "string" ? raw : ""
}
