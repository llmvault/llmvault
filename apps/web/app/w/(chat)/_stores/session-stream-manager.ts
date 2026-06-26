"use client"

import type { QueryClient } from "@tanstack/react-query"
import {
  chatQueryKeys,
  type SessionResponse,
} from "@/app/w/(chat)/_lib/chat-cache"
import { isTerminalStreamFrame } from "@/app/w/(chat)/_lib/live-session-stream"
import {
  goSessionStreamHTTPStatus,
  isRuntimeRepoChangeFrame,
  subscribeToGoSessionStream,
  type GoSessionStreamFrame,
  type GoSessionStreamReplayMode,
} from "@/app/w/(chat)/_lib/go-session-stream"
import { getSessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"
import { useSessionRuntimeStore } from "@/app/w/(chat)/_stores/session-runtime-store"
import { isSubagentFrame } from "@/app/w/(chat)/_lib/session-subagents"
import { extractErrorMessage as errorMessage } from "@/lib/api/error"

const STREAM_WATCHDOG_MS = 0

interface StreamControllerRecord {
  abort: AbortController
  watchdog?: ReturnType<typeof setTimeout>
  reconnect?: ReturnType<typeof setTimeout>
  queryClient: QueryClient
  stopped: boolean
  forceAccessRefresh?: boolean
  authRefreshAttempts?: number
  replayKey?: string
}

interface EnsureStreamOptions {
  queryClient: QueryClient
  replay?: GoSessionStreamReplayMode
  forceAccessRefresh?: boolean
  authRefreshAttempts?: number
}

const controllers = new Map<string, StreamControllerRecord>()

export function hydrateSessionRuntimeFromResponse(
  session: SessionResponse | undefined,
  _queryClient: QueryClient
) {
  if (!session?.id) return
  useSessionRuntimeStore.getState().hydrateSession(session)
}

export function hydrateSessionListRuntime(
  sessions: SessionResponse[],
  queryClient: QueryClient
) {
  for (const session of sessions) {
    hydrateSessionRuntimeFromResponse(session, queryClient)
  }
}

export function ensureSessionStream(
  sessionId: string,
  options: EnsureStreamOptions
) {
  const nextReplayKey = replayKey(options.replay)
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
    forceAccessRefresh: options.forceAccessRefresh,
    authRefreshAttempts: options.authRefreshAttempts,
    replayKey: nextReplayKey,
  }
  controllers.set(sessionId, controller)
  resetWatchdog(sessionId, controller)
  void runSessionStream(sessionId, controller, options.replay)
}

export async function interruptSessionTurn(
  sessionId: string,
  queryClient: QueryClient,
  interruptSession: () => Promise<unknown>
) {
  stopController(sessionId)
  useSessionRuntimeStore.getState().finishStream(sessionId, {
    outcome: "stopped",
  })
  try {
    await interruptSession()
  } catch (error) {
    await refreshSessionQueries(queryClient, sessionId)
    const message = errorMessage(error, "Could not stop session")
    useSessionRuntimeStore.getState().appendStreamError(sessionId, message)
    throw new Error(message)
  }
  await refreshSessionQueries(queryClient, sessionId)
}

export function stopAllSessionStreams() {
  for (const sessionId of controllers.keys()) stopController(sessionId)
}

async function runSessionStream(
  sessionId: string,
  controller: StreamControllerRecord,
  replayOverride?: GoSessionStreamReplayMode
) {
  let attemptedReplay: GoSessionStreamReplayMode | undefined = replayOverride
  try {
    const access = await getSessionSandboxAccess(
      sessionId,
      controller.queryClient,
      {
        force: controller.forceAccessRefresh,
      }
    )
    const cursor =
      useSessionRuntimeStore.getState().cursorBySessionId[sessionId]
    const replay: GoSessionStreamReplayMode =
      replayOverride ??
      (cursor
        ? { mode: "after_seq", afterSeq: cursor.sequence }
        : { mode: "all" })
    attemptedReplay = replay

    await subscribeToGoSessionStream({
      sessionId,
      access,
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
        handleSessionStreamFrame(sessionId, controller, frame)
      },
    })
    if (controller.abort.signal.aborted || controller.stopped) return
    clearControllerIfCurrent(sessionId, controller)
    if (shouldReconnectAfterClose(attemptedReplay)) {
      reconnectSessionStream(
        sessionId,
        controller.queryClient,
        attemptedReplay
          ? replayForStreamReconnect(sessionId, attemptedReplay)
          : undefined
      )
    }
  } catch (error) {
    if (controller.abort.signal.aborted || controller.stopped) return
    const status = goSessionStreamHTTPStatus(error)
    if (
      (status === 401 || status === 403) &&
      (controller.authRefreshAttempts ?? 0) < 1
    ) {
      reconnectSessionStream(
        sessionId,
        controller.queryClient,
        replayOverride,
        {
          forceAccessRefresh: true,
          authRefreshAttempts: (controller.authRefreshAttempts ?? 0) + 1,
        }
      )
      return
    }
    if (shouldReconnectStream(error)) {
      reconnectSessionStream(
        sessionId,
        controller.queryClient,
        attemptedReplay
          ? replayForStreamReconnect(sessionId, attemptedReplay)
          : undefined
      )
      return
    }
    const message = errorMessage(error, "The live session stream failed.")
    useSessionRuntimeStore.getState().appendStreamError(sessionId, message)
    stopController(sessionId)
    await refreshSessionQueries(controller.queryClient, sessionId)
  }
}

function handleSessionStreamFrame(
  sessionId: string,
  controller: StreamControllerRecord,
  frame: GoSessionStreamFrame
) {
  resetWatchdog(sessionId, controller)
  if (frame.event === "resync_required") {
    controller.abort.abort()
    void refreshSessionQueries(controller.queryClient, sessionId).finally(() =>
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
  if (!subagentFrame && isTerminalStreamFrame(frame)) {
    const message = terminalFrameErrorMessage(frame)
    useSessionRuntimeStore.getState().finishStream(sessionId, {
      preserveError: Boolean(message),
      outcome: message ? "failed" : "completed",
    })
  }
}

function clearControllerIfCurrent(
  sessionId: string,
  controller: StreamControllerRecord
) {
  if (controllers.get(sessionId) === controller) {
    controllers.delete(sessionId)
  }
}

function reconnectSessionStream(
  sessionId: string,
  queryClient: QueryClient,
  replay?: GoSessionStreamReplayMode,
  options: {
    forceAccessRefresh?: boolean
    authRefreshAttempts?: number
  } = {}
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
    ensureSessionStream(sessionId, {
      queryClient,
      replay,
      forceAccessRefresh: options.forceAccessRefresh,
      authRefreshAttempts: options.authRefreshAttempts,
    })
  }, delay)
  controllers.set(sessionId, {
    abort: new AbortController(),
    queryClient,
    stopped: false,
    reconnect,
    forceAccessRefresh: options.forceAccessRefresh,
    authRefreshAttempts: options.authRefreshAttempts,
    replayKey: replayKey(replay),
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
  if (STREAM_WATCHDOG_MS <= 0) return
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

function terminalFrameErrorMessage(frame: GoSessionStreamFrame) {
  if (frame.event !== "error" && frame.event !== "turn_failed") return ""
  const data = frame.data
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
  const status = goSessionStreamHTTPStatus(error)
  if (status !== undefined) {
    return status !== 400 && status !== 401 && status !== 403 && status !== 404
  }
  const message = errorMessage(error, "")
  return !/\bHTTP (400|401|403|404)\b/.test(message)
}

function replayKey(replay?: GoSessionStreamReplayMode) {
  if (!replay) return "auto"
  if (replay.mode === "after_seq") return `after_seq:${replay.afterSeq}`
  if (replay.mode === "from_turn_id") return `from_turn_id:${replay.turnId}`
  if (replay.mode === "from_turn_id_follow") {
    return `from_turn_id_follow:${replay.turnId}`
  }
  return replay.mode
}

function shouldReconnectAfterClose(replay?: GoSessionStreamReplayMode) {
  return replay?.mode !== "from_turn_id"
}

function replayForStreamReconnect(
  sessionId: string,
  replay: GoSessionStreamReplayMode
): GoSessionStreamReplayMode | undefined {
  const cursor = useSessionRuntimeStore.getState().cursorBySessionId[sessionId]
  if (cursor) return undefined
  if (
    replay.mode === "from_turn_id" ||
    replay.mode === "from_turn_id_follow" ||
    replay.mode === "none"
  ) {
    return replay
  }
  return undefined
}
