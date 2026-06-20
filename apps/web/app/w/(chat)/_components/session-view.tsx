"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import ScrollToBottom from "react-scroll-to-bottom"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { Composer } from "@/app/w/(chat)/_components/composer"
import { Conversation } from "@/app/w/(chat)/_components/conversation"
import { SessionHistorySkeleton } from "@/app/w/(chat)/_components/session-history-skeleton"
import { SessionHistoryTopLoader } from "@/app/w/(chat)/_components/session-history-top-loader"
import { SessionPlanCard } from "@/app/w/(chat)/_components/session-plan-card"
import {
  useWorkspace,
  type ChatSession,
} from "@/app/w/(chat)/_components/shell"
import { AGENTS, agentById } from "@/app/w/(chat)/_lib/agents"
import {
  CHAT_QUERY_STALE_TIME_MS,
  SESSION_EVENTS_INFINITE_KEY,
  SESSION_HISTORY_PAGE_LIMIT,
  appendSessionEvents,
  markOptimisticEventFailed,
  markOptimisticEventPending,
  optimisticThinkingEvent,
  optimisticUserEvent,
  replaceOptimisticEvent,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  directSessionStreamCursor,
  isRuntimeRepoChangeFrame,
  subscribeToDirectSessionStream,
  type DirectSessionStreamCursor,
  type DirectSessionStreamFrame,
  type DirectSessionStreamReplayMode,
} from "@/app/w/(chat)/_lib/direct-session-stream"
import {
  appendLiveSessionStreamFrame,
  isTerminalStreamFrame,
} from "@/app/w/(chat)/_lib/live-session-stream"
import {
  sessionHistoryPagesToEvents,
  sessionEventsToConversationBlocks,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history"
import type { ImageAttachmentMetadata } from "@/app/w/(chat)/_lib/image-attachments"
import {
  codeLineCommentReferenceToPayload,
  type CodeLineCommentPayload,
  type CodeLineCommentReference,
} from "@/app/w/(chat)/_lib/code-line-comments"
import { latestSessionPlan } from "@/app/w/(chat)/_lib/session-plan"
import { reviewDiffsQueryKey } from "@/app/w/(chat)/_lib/review-diffs-query"

const STREAM_WATCHDOG_MS = 10 * 60 * 1000

export function SessionThreadView({
  session,
  sessionId,
}: {
  session: ChatSession
  sessionId?: string
}) {
  const { setModel, sandboxAccess, sandboxAccessError, sandboxAccessPending } =
    useWorkspace()
  const queryClient = useQueryClient()
  const agent = safeAgentById(session.agentId)
  const [liveEvents, setLiveEvents] = useState<SessionEventResponse[]>([])
  const [streaming, setStreaming] = useState(false)
  const [streamReconnectVersion, setStreamReconnectVersion] = useState(0)
  const streamingRef = useRef(false)
  const streamWatchdogRef = useRef<number | null>(null)
  const streamReconnectTimerRef = useRef<number | null>(null)
  const streamReconnectAttemptsRef = useRef(0)
  const streamCursorRef = useRef<DirectSessionStreamCursor | null>(null)
  const optimisticSession = sessionId?.startsWith("tmp_") ?? false
  const sendSessionMessage = $api.useMutation(
    "post",
    "/v1/sessions/{id}/messages"
  )
  const interruptSession = $api.useMutation(
    "post",
    "/v1/sessions/{id}/interrupt"
  )
  const sessionHistoryQuery = $api.useInfiniteQuery(
    "get",
    "/v1/sessions/{id}/events",
    {
      _hivyQueryKey: SESSION_EVENTS_INFINITE_KEY,
      params: {
        path: { id: sessionId ?? "" },
        query: { limit: SESSION_HISTORY_PAGE_LIMIT },
      },
    },
    {
      enabled: Boolean(sessionId) && !optimisticSession,
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const refetchHistoryRef = useRef(sessionHistoryQuery.refetch)
  const historyPages = sessionHistoryQuery.data?.pages
  const historyEvents = useMemo(
    () => sessionHistoryPagesToEvents(historyPages ?? []),
    [historyPages]
  )
  const hasPendingClientEvent = useMemo(
    () => historyEvents.some(isPendingClientEvent),
    [historyEvents]
  )
  const historyBlocks = useMemo(
    () =>
      historyPages?.length
        ? sessionEventsToConversationBlocks(historyEvents)
        : null,
    [historyEvents, historyPages?.length]
  )
  const baseBlocks = historyBlocks ?? []
  const liveBlocks = useMemo(
    () => sessionEventsToConversationBlocks(liveEvents, { mode: "live" }),
    [liveEvents]
  )
  const latestPlan = useMemo(
    () => latestSessionPlan([...historyEvents, ...liveEvents]),
    [historyEvents, liveEvents]
  )
  const fetchNextHistoryPage = sessionHistoryQuery.fetchNextPage
  const loadNextHistoryPage = useCallback(
    () => fetchNextHistoryPage(),
    [fetchNextHistoryPage]
  )
  const historyReadyForStream =
    optimisticSession ||
    sessionHistoryQuery.isSuccess ||
    sessionHistoryQuery.isError

  const clearStreamWatchdog = useCallback(() => {
    if (!streamWatchdogRef.current) return
    window.clearTimeout(streamWatchdogRef.current)
    streamWatchdogRef.current = null
  }, [])

  const clearStreamReconnectTimer = useCallback(() => {
    if (!streamReconnectTimerRef.current) return
    window.clearTimeout(streamReconnectTimerRef.current)
    streamReconnectTimerRef.current = null
  }, [])

  const appendStreamError = useCallback(
    (message: string) => {
      if (!sessionId) return
      setLiveEvents((current) => [
        ...current.filter((event) => event.event_type !== "error"),
        streamErrorEvent(sessionId, message),
      ])
    },
    [sessionId]
  )

  const finishLiveStream = useCallback(
    (options: { preserveError?: boolean; refetch?: boolean } = {}) => {
      streamingRef.current = false
      clearStreamWatchdog()
      clearStreamReconnectTimer()
      streamReconnectAttemptsRef.current = 0
      setStreaming(false)
      if (options.refetch === false) {
        setLiveEvents((current) =>
          options.preserveError
            ? current.filter((event) => event.event_type === "error")
            : []
        )
        return
      }
      void refetchHistoryRef.current().then((result) => {
        if (result.error) return
        setLiveEvents((current) =>
          options.preserveError
            ? current.filter((event) => event.event_type === "error")
            : []
        )
      })
    },
    [clearStreamReconnectTimer, clearStreamWatchdog]
  )

  const failLiveStream = useCallback(
    (message: string) => {
      window.queueMicrotask(() => {
        if (!streamingRef.current) return
        appendStreamError(message)
        finishLiveStream({ preserveError: true })
      })
    },
    [appendStreamError, finishLiveStream]
  )

  const scheduleStreamWatchdog = useCallback(() => {
    if (!sessionId) return
    clearStreamWatchdog()
    streamWatchdogRef.current = window.setTimeout(() => {
      if (!streamingRef.current) return
      appendStreamError(
        "The live session stream stopped responding. History has been refreshed."
      )
      finishLiveStream({ preserveError: true })
    }, STREAM_WATCHDOG_MS)
  }, [appendStreamError, clearStreamWatchdog, finishLiveStream, sessionId])

  const startLiveStream = useCallback(() => {
    streamingRef.current = true
    setStreaming(true)
    scheduleStreamWatchdog()
  }, [scheduleStreamWatchdog])

  const scheduleStreamReconnect = useCallback(() => {
    clearStreamReconnectTimer()
    const attempt = streamReconnectAttemptsRef.current + 1
    streamReconnectAttemptsRef.current = attempt
    const delay = Math.min(2000, attempt * 400)
    streamReconnectTimerRef.current = window.setTimeout(() => {
      setStreamReconnectVersion((current) => current + 1)
    }, delay)
  }, [clearStreamReconnectTimer])

  const handleStreamResyncRequired = useCallback(() => {
    clearStreamReconnectTimer()
    streamReconnectAttemptsRef.current = 0
    streamCursorRef.current = null
    setLiveEvents([])
    void refetchHistoryRef.current().finally(() => {
      setStreamReconnectVersion((current) => current + 1)
    })
  }, [clearStreamReconnectTimer])

  const invalidateReviewDiffs = useCallback(() => {
    if (!sessionId) return
    void queryClient.invalidateQueries({
      queryKey: reviewDiffsQueryKey(sessionId, sandboxAccess),
    })
  }, [queryClient, sandboxAccess, sessionId])

  useEffect(() => {
    refetchHistoryRef.current = sessionHistoryQuery.refetch
  }, [sessionHistoryQuery.refetch])

  useEffect(() => {
    streamingRef.current = streaming
  }, [streaming])

  useEffect(() => {
    return () => {
      clearStreamWatchdog()
      clearStreamReconnectTimer()
    }
  }, [clearStreamReconnectTimer, clearStreamWatchdog])

  useEffect(() => {
    if (!sessionId || !historyReadyForStream) return
    if (!sandboxAccess) return
    if (sandboxAccess.session_id && sandboxAccess.session_id !== sessionId) {
      return
    }
    const sandboxBaseUrl = sandboxAccess.sandbox_base_url
    const sandboxToken = sandboxAccess.token
    if (!sandboxBaseUrl || !sandboxToken) {
      failLiveStream("The live session stream is not available.")
      return
    }
    const directUrl = `${sandboxBaseUrl.replace(/\/+$/, "")}/sessions/${sessionId}/stream`

    const cursor = streamCursorRef.current
    const replay: DirectSessionStreamReplayMode = cursor
      ? { mode: "after_seq", afterSeq: cursor.sequence }
      : sessionHistoryQuery.isSuccess
        ? { mode: "none" }
        : { mode: "all" }

    const controller = new AbortController()
    void subscribeToDirectSessionStream({
      sessionId,
      directUrl,
      token: sandboxToken,
      replay,
      signal: controller.signal,
      onOpen: ({ streamId: openedStreamId, nextSequence }) => {
        streamReconnectAttemptsRef.current = 0
        if (
          replay.mode !== "none" ||
          !openedStreamId ||
          nextSequence === null ||
          nextSequence <= 0
        ) {
          return
        }
        streamCursorRef.current = {
          streamId: openedStreamId,
          sequence: nextSequence - 1,
        }
      },
      onEvent: (frame) => {
        if (frame.event === "resync_required") {
          controller.abort()
          handleStreamResyncRequired()
          return
        }

        const nextCursor = directSessionStreamCursor(frame)
        if (nextCursor) {
          const currentCursor = streamCursorRef.current
          if (
            !currentCursor ||
            currentCursor.streamId !== nextCursor.streamId ||
            nextCursor.sequence > currentCursor.sequence
          ) {
            streamCursorRef.current = nextCursor
          }
        }

        if (isTerminalStreamFrame(frame)) {
          const message = terminalFrameErrorMessage(frame)
          if (message) appendStreamError(message)
          finishLiveStream({ preserveError: Boolean(message) })
          return
        }

        if (isRuntimeRepoChangeFrame(frame)) {
          invalidateReviewDiffs()
        }

        setLiveEvents((current) =>
          appendLiveSessionStreamFrame(
            shouldReplaceOptimisticWork(frame.event)
              ? current.filter((event) => !isPendingClientEvent(event))
              : current,
            frame
          )
        )
        if (isActiveStreamFrame(frame.event)) startLiveStream()
      },
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return
      if (!streamingRef.current) return
      if (shouldReconnectStream(error)) {
        scheduleStreamReconnect()
        return
      }
      appendStreamError(errorMessage(error, "The live session stream failed."))
      finishLiveStream({ preserveError: true })
    })

    return () => {
      controller.abort()
    }
  }, [
    appendStreamError,
    failLiveStream,
    finishLiveStream,
    handleStreamResyncRequired,
    historyReadyForStream,
    invalidateReviewDiffs,
    scheduleStreamReconnect,
    sessionId,
    sessionHistoryQuery.isSuccess,
    startLiveStream,
    streamReconnectVersion,
    sandboxAccess,
  ])

  useEffect(() => {
    if (sandboxAccessError && streamingRef.current) {
      failLiveStream(
        extractErrorMessage(
          sandboxAccessError,
          "Sandbox access is not available."
        )
      )
    }
  }, [failLiveStream, sandboxAccessError])

  useEffect(() => {
    if (!streaming || sandboxAccessPending) return
    if (sandboxAccess?.sandbox_base_url && sandboxAccess.token) {
      return
    }
    failLiveStream("The live session stream is not available.")
  }, [
    failLiveStream,
    sandboxAccess?.sandbox_base_url,
    sandboxAccess?.token,
    sandboxAccessPending,
    streaming,
  ])

  const send = async (
    text: string,
    options: {
      retryEventID?: string
      attachments?: ImageAttachmentMetadata[]
      codeLineComments?: CodeLineCommentPayload[]
    } = {}
  ) => {
    const retryEventID = options.retryEventID
    const attachments = options.attachments ?? []
    const codeLineComments = options.codeLineComments ?? []
    if (streaming || interruptSession.isPending) return false
    if (optimisticSession) {
      toast.danger("This chat was not created. Start a new chat to try again.")
      return false
    }
    if (!sessionId) {
      toast.danger("Start a new chat before sending a message.")
      return false
    }

    const raw = {
      ...(attachments.length ? { attachments } : {}),
      ...(codeLineComments.length
        ? { code_line_comments: codeLineComments }
        : {}),
    }
    const optimisticMessage = retryEventID
      ? null
      : optimisticUserEvent(
          sessionId,
          text,
          undefined,
          attachments,
          codeLineComments
        )
    const optimisticEventID =
      retryEventID ?? optimisticMessage?.event_id ?? optimisticMessage?.id ?? ""
    const optimisticThinking = optimisticThinkingEvent(sessionId)
    if (retryEventID) {
      markOptimisticEventPending(queryClient, sessionId, retryEventID)
    } else if (optimisticMessage) {
      appendSessionEvents(queryClient, sessionId, [optimisticMessage])
    }
    setLiveEvents([optimisticThinking])
    startLiveStream()
    try {
      const response = await sendSessionMessage.mutateAsync({
        params: { path: { id: sessionId } },
        body: {
          text,
          raw: Object.keys(raw).length ? raw : undefined,
        },
      })
      if (response.event) {
        replaceOptimisticEvent(
          queryClient,
          sessionId,
          optimisticEventID,
          response.event
        )
      }
      startLiveStream()
      return true
    } catch (error) {
      const message = extractErrorMessage(error, "Could not send message")
      markOptimisticEventFailed(
        queryClient,
        sessionId,
        optimisticEventID,
        message
      )
      finishLiveStream({ refetch: false })
      toast.danger(message)
      return false
    }
  }

  const stop = () => {
    finishLiveStream({ refetch: false })
    if (!sessionId || optimisticSession) return
    void interruptSession
      .mutateAsync({
        params: { path: { id: sessionId } },
      })
      .then(() => refetchHistoryRef.current())
      .catch((error) => {
        toast.danger(extractErrorMessage(error, "Could not stop session"))
        void refetchHistoryRef.current()
      })
  }

  const retryMessage = (
    eventID: string,
    text: string,
    codeLineComments?: CodeLineCommentReference[]
  ) => {
    void send(text, {
      retryEventID: eventID,
      codeLineComments: codeLineComments?.map(
        codeLineCommentReferenceToPayload
      ),
    })
  }

  const isBusy =
    streaming ||
    hasPendingClientEvent ||
    sendSessionMessage.isPending ||
    interruptSession.isPending
  const visibleBlocks = [...baseBlocks, ...liveBlocks]
  const showHistorySkeleton =
    !optimisticSession && sessionHistoryQuery.isPending && !historyPages?.length
  const followButtonClassName = `!absolute ${
    latestPlan ? "!bottom-20" : "!bottom-6"
  } !left-1/2 !right-auto !flex !h-9 !w-9 !-translate-x-1/2 !items-center !justify-center !rounded-full !border !border-border !bg-surface !p-0 !text-muted !shadow-sm !transition-colors after:content-['↓'] hover:!bg-default hover:!text-foreground`

  return (
    <div className="relative flex h-full min-w-0 flex-col">
      <ScrollToBottom
        className="min-h-0 flex-1"
        scrollViewClassName="min-h-0 [overflow-anchor:none]"
        followButtonClassName={followButtonClassName}
        initialScrollBehavior="auto"
        mode="bottom"
      >
        {showHistorySkeleton ? (
          <SessionHistorySkeleton />
        ) : (
          <>
            <SessionHistoryTopLoader
              hasMore={Boolean(sessionHistoryQuery.hasNextPage)}
              isFetching={sessionHistoryQuery.isFetchingNextPage}
              loadedEventCount={historyEvents.length}
              onLoadMore={loadNextHistoryPage}
            />
            <Conversation
              blocks={visibleBlocks}
              onRetryMessage={optimisticSession ? undefined : retryMessage}
            />
          </>
        )}
      </ScrollToBottom>

      <div className="relative z-20 shrink-0">
        {latestPlan ? (
          <SessionPlanCard plan={latestPlan} turnActive={streaming} />
        ) : null}
        <Composer
          agent={agent}
          agentId={session.agentId}
          modelId={session.modelId}
          onModelChange={setModel}
          onSend={(text, attachments, codeLineComments) =>
            send(text, { attachments, codeLineComments })
          }
          isStreaming={isBusy}
          onStop={stop}
        />
      </div>
    </div>
  )
}

function isPendingClientEvent(event: SessionEventResponse) {
  const payload = event.payload
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return false
  }
  return (payload as Record<string, unknown>).client_status === "pending"
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
    event === "thinking" ||
    event === "token" ||
    event === "tool_call" ||
    event === "tool_result" ||
    event === "plan_updated" ||
    event === "final"
  )
}

function terminalFrameErrorMessage(frame: DirectSessionStreamFrame): string {
  if (frame.event !== "error" && frame.event !== "turn_failed") return ""
  const fallback =
    frame.event === "turn_failed"
      ? "The agent turn failed."
      : "The live session stream failed."
  if (
    !frame.data ||
    typeof frame.data !== "object" ||
    Array.isArray(frame.data)
  ) {
    return fallback
  }
  const record = frame.data as Record<string, unknown>
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

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message.trim()
    ? error.message
    : fallback
}

function shouldReconnectStream(error: unknown) {
  const message = errorMessage(error, "")
  return !/\bHTTP (400|401|403|404)\b/.test(message)
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

function safeAgentById(id: string) {
  try {
    return agentById(id)
  } catch {
    return agentById(AGENTS[0].id)
  }
}
