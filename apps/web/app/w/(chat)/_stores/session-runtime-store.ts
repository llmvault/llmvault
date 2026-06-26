"use client"

import { create } from "zustand"
import { subscribeWithSelector } from "zustand/middleware"
import {
  goSessionStreamCursor,
  type GoSessionStreamCursor,
  type GoSessionStreamFrame,
} from "@/app/w/(chat)/_lib/go-session-stream"
import { appendLiveSessionStreamFrame } from "@/app/w/(chat)/_lib/live-session-stream"
import {
  sessionUsageSummaryFromSnapshot,
  type SessionUsageSnapshot,
  type SessionUsageSummary,
} from "@/app/w/(chat)/_lib/session-usage"
import {
  isModelRequestMarker,
  reconcileLiveSessionEvents,
  sessionLastOutcomeFromResponse,
  sessionRuntimeStatusFromResponse,
  sessionServerUpdatedAt,
  shouldAdvanceCursor,
  shouldHydrateSessionRuntime,
  streamErrorEvent,
  summaryForFrame,
  usagePatchForFrame,
} from "@/app/w/(chat)/_lib/session-runtime-helpers"
import {
  IDLE_RUNTIME_SUMMARY,
  type SessionLastTurnOutcome,
  type SessionResponse,
  type SessionRuntimeStatus,
  type SessionRuntimeSummary,
} from "@/app/w/(chat)/_lib/session-runtime-types"
import {
  appendSubagentRunFrame,
  dispatchSubagentFrameDebugEvent,
  EMPTY_SUBAGENT_RUNS,
  mergeSubagentRuns as mergeSubagentRunLists,
  type SessionSubagentRun,
} from "@/app/w/(chat)/_lib/session-subagent-runs"
import { subagentFrameMetadata } from "@/app/w/(chat)/_lib/session-subagents"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"

export { IDLE_RUNTIME_SUMMARY, sessionRuntimeStatusFromResponse }
export type {
  PendingInputRequest,
  SessionLastTurnOutcome,
  SessionRuntimeStatus,
  SessionRuntimeSummary,
} from "@/app/w/(chat)/_lib/session-runtime-types"

interface SessionRuntimeStoreState {
  statusBySessionId: Record<string, SessionRuntimeSummary>
  liveEventsBySessionId: Record<string, SessionEventResponse[]>
  subagentRunsBySessionId: Record<string, SessionSubagentRun[]>
  usageBySessionId: Record<string, SessionUsageSummary | undefined>
  usageEventKeysBySessionId: Record<string, Record<string, boolean> | undefined>
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
  hydrateSessionUsage: (
    sessionId: string,
    snapshot: SessionUsageSnapshot
  ) => void
  applyStreamFrame: (sessionId: string, frame: GoSessionStreamFrame) => void
  mergeSubagentRuns: (sessionId: string, runs: SessionSubagentRun[]) => void
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

const EMPTY_EVENTS: SessionEventResponse[] = []

export const useSessionRuntimeStore = create<SessionRuntimeStoreState>()(
  subscribeWithSelector((setState, getState) => ({
    statusBySessionId: {},
    liveEventsBySessionId: {},
    subagentRunsBySessionId: {},
    usageBySessionId: {},
    usageEventKeysBySessionId: {},
    cursorBySessionId: {},
    reconnectAttemptsBySessionId: {},
    hydrateSession(session) {
      const sessionId = session?.id
      if (!sessionId) return
      const status = sessionRuntimeStatusFromResponse(session)
      const lastOutcome = sessionLastOutcomeFromResponse(session)
      const serverUpdatedAt = sessionServerUpdatedAt(session)
      const existing = getState().statusBySessionId[sessionId]
      if (
        !shouldHydrateSessionRuntime(
          existing,
          status,
          lastOutcome,
          serverUpdatedAt
        )
      ) {
        return
      }
      getState().setStatus(sessionId, status, {
        lastOutcome,
        error: existing?.error,
        pendingInput: existing?.pendingInput,
        serverUpdatedAt,
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
        const next = reconcileLiveSessionEvents(current, historyEvents)
        if (next === current) return state
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
    hydrateSessionUsage(sessionId, snapshot) {
      const incoming = sessionUsageSummaryFromSnapshot(snapshot)
      setState((state) => {
        const existing = state.usageBySessionId[sessionId]
        const usage =
          existing && existing.costUsd > incoming.costUsd ? existing : incoming
        return {
          usageBySessionId: {
            ...state.usageBySessionId,
            [sessionId]: usage,
          },
        }
      })
    },
    applyStreamFrame(sessionId, frame) {
      const nextCursor = goSessionStreamCursor(frame)
      const subagentMetadata = subagentFrameMetadata(frame)
      setState((state) => {
        const usagePatch = usagePatchForFrame(state, sessionId, frame)
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
            ...usagePatch,
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
        const nextEvents = appendLiveSessionStreamFrame(currentEvents, frame)
        return {
          ...usagePatch,
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
    mergeSubagentRuns(sessionId, runs) {
      if (runs.length === 0) return
      setState((state) => ({
        subagentRunsBySessionId: {
          ...state.subagentRunsBySessionId,
          [sessionId]: mergeSubagentRunLists(
            state.subagentRunsBySessionId[sessionId] ?? EMPTY_SUBAGENT_RUNS,
            runs
          ),
        },
      }))
    },
    finishStream(sessionId, options = {}) {
      setState((state) => {
        const current = state.liveEventsBySessionId[sessionId] ?? EMPTY_EVENTS
        const events = options.clearLiveEvents
          ? EMPTY_EVENTS
          : options.preserveError
            ? current.filter((event) => event.event_type === "error")
            : current.filter((event) => !isModelRequestMarker(event))
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
        const { [sessionId]: _usage, ...usageBySessionId } =
          state.usageBySessionId
        const { [sessionId]: _usageKeys, ...usageEventKeysBySessionId } =
          state.usageEventKeysBySessionId
        const { [sessionId]: _cursor, ...cursorBySessionId } =
          state.cursorBySessionId
        const { [sessionId]: _attempts, ...reconnectAttemptsBySessionId } =
          state.reconnectAttemptsBySessionId
        return {
          statusBySessionId,
          liveEventsBySessionId,
          subagentRunsBySessionId,
          usageBySessionId,
          usageEventKeysBySessionId,
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

export function useSessionUsageSummary(sessionId?: string) {
  return useSessionRuntimeStore((state) =>
    sessionId ? state.usageBySessionId[sessionId] : undefined
  )
}

export function sessionRuntimeSummary(sessionId?: string) {
  if (!sessionId) return IDLE_RUNTIME_SUMMARY
  return (
    useSessionRuntimeStore.getState().statusBySessionId[sessionId] ??
    IDLE_RUNTIME_SUMMARY
  )
}
