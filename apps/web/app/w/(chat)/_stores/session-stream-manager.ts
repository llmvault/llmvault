"use client"

import type { QueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api/client"
import {
  chatQueryKeys,
  type SessionEventResponse,
  type SessionResponse,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  isRuntimeRepoChangeFrame,
  subscribeToDirectSessionStream,
  type DirectSessionStreamReplayMode,
} from "@/app/w/(chat)/_lib/direct-session-stream"
import {
  clearSessionSandboxAccess,
  getSessionSandboxAccess,
} from "@/app/w/(chat)/_lib/session-sandbox-access"
import {
  sessionRuntimeStatusFromResponse,
  useSessionRuntimeStore,
} from "@/app/w/(chat)/_stores/session-runtime-store"
import { isSubagentFrame } from "@/app/w/(chat)/_lib/session-subagents"

const STREAM_WATCHDOG_MS = 10 * 60 * 1000

interface StreamControllerRecord {
  abort: AbortController
  watchdog?: ReturnType<typeof setTimeout>
  reconnect?: ReturnType<typeof setTimeout>
  queryClient: QueryClient
  stopped: boolean
}

interface EnsureStreamOptions {
  queryClient: QueryClient
  replay?: DirectSessionStreamReplayMode
}

const controllers = new Map<string, StreamControllerRecord>()

export function hydrateSessionRuntimeFromResponse(
  session: SessionResponse | undefined,
  queryClient: QueryClient
) {
  if (!session?.id) return
  useSessionRuntimeStore.getState().hydrateSession(session)
  const status = sessionRuntimeStatusFromResponse(session)
  if (
    status === "streaming" ||
    status === "queued" ||
    status === "waiting_for_user"
  ) {
    ensureSessionStream(session.id, { queryClient })
  }
}

export function hydrateSessionListRuntime(
  sessions: SessionResponse[],
  queryClient: QueryClient
) {
  for (const session of sessions) {
    hydrateSessionRuntimeFromResponse(session, queryClient)
  }
}

export function beginOptimisticSessionTurn(
  sessionId: string,
  events: SessionEventResponse[]
) {
  const runtime = useSessionRuntimeStore.getState()
  runtime.setLiveEvents(sessionId, events)
  runtime.setStatus(sessionId, "streaming", {
    error: undefined,
    lastOutcome: undefined,
    pendingInput: undefined,
  })
}

export function ensureSessionStream(
  sessionId: string,
  options: EnsureStreamOptions
) {
  const existing = controllers.get(sessionId)
  if (existing && !existing.stopped && !existing.reconnect) {
    existing.queryClient = options.queryClient
    resetWatchdog(sessionId, existing)
    return
  }
  if (existing?.reconnect) {
    clearTimeout(existing.reconnect)
    controllers.delete(sessionId)
  }
  const abort = new AbortController()
  const controller: StreamControllerRecord = {
    abort,
    queryClient: options.queryClient,
    stopped: false,
  }
  controllers.set(sessionId, controller)
  resetWatchdog(sessionId, controller)
  void runSessionStream(sessionId, controller, options.replay)
}

export async function interruptSessionTurn(
  sessionId: string,
  queryClient: QueryClient
) {
  stopController(sessionId)
  useSessionRuntimeStore.getState().finishStream(sessionId, {
    outcome: "stopped",
  })
  const { error } = await api.POST("/v1/sessions/{id}/interrupt", {
    params: { path: { id: sessionId } },
  })
  await refreshSessionQueries(queryClient, sessionId)
  if (error) {
    const message =
      typeof error === "object" &&
      error &&
      "error" in error &&
      typeof error.error === "string"
        ? error.error
        : "Could not stop session"
    useSessionRuntimeStore.getState().appendStreamError(sessionId, message)
    throw new Error(message)
  }
}

export async function respondToPendingInput({
  sessionId,
  requestId,
  text,
  optionId,
  queryClient,
}: {
  sessionId: string
  requestId: string
  text?: string
  optionId?: string
  queryClient: QueryClient
}) {
  const { error } = await api.POST("/v1/sessions/{id}/input-responses", {
    params: { path: { id: sessionId } },
    body: {
      request_id: requestId,
      text,
      option_id: optionId,
    },
  })
  if (error) {
    const message =
      typeof error === "object" &&
      error &&
      "error" in error &&
      typeof error.error === "string"
        ? error.error
        : "Could not send response"
    throw new Error(message)
  }
  useSessionRuntimeStore.getState().setStatus(sessionId, "streaming", {
    pendingInput: undefined,
  })
  ensureSessionStream(sessionId, { queryClient })
}

export function stopAllSessionStreams() {
  for (const sessionId of controllers.keys()) stopController(sessionId)
}

async function runSessionStream(
  sessionId: string,
  controller: StreamControllerRecord,
  replayOverride?: DirectSessionStreamReplayMode
) {
  try {
    const access = await getSessionSandboxAccess(sessionId)
    if (controller.abort.signal.aborted || controller.stopped) return
    const directUrl = `${access.sandbox_base_url.replace(/\/+$/, "")}/sessions/${sessionId}/stream`
    const cursor =
      useSessionRuntimeStore.getState().cursorBySessionId[sessionId]
    const replay: DirectSessionStreamReplayMode =
      replayOverride ??
      (cursor
        ? { mode: "after_seq", afterSeq: cursor.sequence }
        : { mode: "all" })

    await subscribeToDirectSessionStream({
      sessionId,
      directUrl,
      token: access.token,
      replay,
      signal: controller.abort.signal,
      onOpen: ({ streamId, nextSequence }) => {
        resetReconnect(sessionId)
        resetWatchdog(sessionId, controller)
        if (
          replay.mode === "none" &&
          streamId &&
          nextSequence !== null &&
          nextSequence > 0
        ) {
          useSessionRuntimeStore.setState((state) => ({
            cursorBySessionId: {
              ...state.cursorBySessionId,
              [sessionId]: {
                streamId,
                sequence: nextSequence - 1,
              },
            },
          }))
        }
      },
      onEvent: (frame) => {
        resetWatchdog(sessionId, controller)
        if (frame.event === "resync_required") {
          controller.abort.abort()
          void refreshSessionQueries(controller.queryClient, sessionId).finally(
            () =>
              reconnectSessionStream(sessionId, controller.queryClient, {
                mode: "all",
              })
          )
          return
        }
        if (isRuntimeRepoChangeFrame(frame)) {
          void controller.queryClient.invalidateQueries({
            queryKey: ["sandbox-runtime-review-diffs", sessionId],
          })
        }
        const subagentFrame = isSubagentFrame(frame)
        useSessionRuntimeStore.getState().applyStreamFrame(sessionId, frame)
        if (!subagentFrame && isTerminalFrame(frame.event)) {
          stopController(sessionId)
          void refreshSessionQueries(controller.queryClient, sessionId).then(
            () => {
              const message = terminalFrameErrorMessage(frame.data)
              useSessionRuntimeStore.getState().finishStream(sessionId, {
                preserveError: Boolean(message),
                outcome: message ? "failed" : "completed",
              })
            }
          )
        }
      },
    })
  } catch (error) {
    if (controller.abort.signal.aborted || controller.stopped) return
    if (shouldReconnectStream(error)) {
      clearSessionSandboxAccess(sessionId)
      reconnectSessionStream(sessionId, controller.queryClient)
      return
    }
    const message = errorMessage(error, "The live session stream failed.")
    useSessionRuntimeStore.getState().appendStreamError(sessionId, message)
    stopController(sessionId)
    await refreshSessionQueries(controller.queryClient, sessionId)
  }
}

function reconnectSessionStream(
  sessionId: string,
  queryClient: QueryClient,
  replay?: DirectSessionStreamReplayMode
) {
  const currentAttempt =
    useSessionRuntimeStore.getState().reconnectAttemptsBySessionId[sessionId] ??
    0
  const nextAttempt = currentAttempt + 1
  useSessionRuntimeStore.getState().setReconnectAttempt(sessionId, nextAttempt)
  const delay = Math.min(2000, nextAttempt * 400)
  stopController(sessionId, { keepStatus: true })
  const reconnect = setTimeout(() => {
    controllers.delete(sessionId)
    ensureSessionStream(sessionId, { queryClient, replay })
  }, delay)
  controllers.set(sessionId, {
    abort: new AbortController(),
    queryClient,
    stopped: false,
    reconnect,
  })
}

function resetReconnect(sessionId: string) {
  const controller = controllers.get(sessionId)
  if (controller?.reconnect) {
    clearTimeout(controller.reconnect)
    controller.reconnect = undefined
  }
  useSessionRuntimeStore.getState().setReconnectAttempt(sessionId, 0)
}

function resetWatchdog(sessionId: string, controller: StreamControllerRecord) {
  if (controller.watchdog) clearTimeout(controller.watchdog)
  controller.watchdog = setTimeout(() => {
    if (controller.abort.signal.aborted || controller.stopped) return
    useSessionRuntimeStore
      .getState()
      .appendStreamError(
        sessionId,
        "The live session stream stopped responding. History has been refreshed."
      )
    stopController(sessionId)
    void refreshSessionQueries(controller.queryClient, sessionId)
  }, STREAM_WATCHDOG_MS)
}

function stopController(
  sessionId: string,
  options: { keepStatus?: boolean } = {}
) {
  const controller = controllers.get(sessionId)
  if (!controller) return
  controller.stopped = true
  controller.abort.abort()
  if (controller.watchdog) clearTimeout(controller.watchdog)
  if (controller.reconnect) clearTimeout(controller.reconnect)
  controllers.delete(sessionId)
  if (!options.keepStatus) {
    useSessionRuntimeStore.getState().setReconnectAttempt(sessionId, 0)
  }
}

async function refreshSessionQueries(
  queryClient: QueryClient,
  sessionId: string
) {
  await Promise.all([
    queryClient.invalidateQueries({
      queryKey: chatQueryKeys.session(sessionId),
    }),
    queryClient.invalidateQueries({
      queryKey: chatQueryKeys.sessionEvents(sessionId),
    }),
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/channels/{id}/sessions"],
    }),
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/sessions"] }),
  ])
}

function isTerminalFrame(event: string) {
  return (
    event === "done" ||
    event === "turn_completed" ||
    event === "turn_failed" ||
    event === "error"
  )
}

function terminalFrameErrorMessage(data: unknown) {
  if (!data || typeof data !== "object" || Array.isArray(data)) return ""
  const record = data as Record<string, unknown>
  if (record.interrupted === true) return ""
  const value = record.error ?? record.message ?? record.text
  if (
    typeof value === "string" &&
    value.trim().toLowerCase() === "interrupted by user"
  ) {
    return ""
  }
  return typeof value === "string" && value.trim() ? value : ""
}

function shouldReconnectStream(error: unknown) {
  const message = errorMessage(error, "")
  return !/\bHTTP (400|401|403|404)\b/.test(message)
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message.trim()
    ? error.message
    : fallback
}
