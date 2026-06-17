"use client"

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { Spinner, toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import ScrollToBottom from "react-scroll-to-bottom"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { Composer } from "@/app/w/(chat)/_components/composer"
import { Conversation } from "@/app/w/(chat)/_components/conversation"
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

const HISTORY_TOP_LOAD_THRESHOLD = 160
const STREAM_WATCHDOG_MS = 10 * 60 * 1000

export function SessionThreadView({
  session,
  sessionId,
}: {
  session: ChatSession
  sessionId?: string
}) {
  const { setModel } = useWorkspace()
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
  const sandboxAccessMutation = $api.useMutation(
    "post",
    "/v1/sessions/{id}/sandbox-access"
  )
  const {
    data: sandboxAccess,
    error: sandboxAccessError,
    isPending: sandboxAccessPending,
    mutate: requestSandboxAccess,
  } = sandboxAccessMutation
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

  useEffect(() => {
    refetchHistoryRef.current = sessionHistoryQuery.refetch
  }, [sessionHistoryQuery.refetch])

  useEffect(() => {
    streamingRef.current = streaming
  }, [streaming])

  useEffect(() => {
    if (!sessionId || optimisticSession) {
      return
    }
    requestSandboxAccess({ params: { path: { id: sessionId } } })
  }, [optimisticSession, requestSandboxAccess, sessionId])

  useEffect(() => {
    return () => {
      clearStreamWatchdog()
      clearStreamReconnectTimer()
    }
  }, [clearStreamReconnectTimer, clearStreamWatchdog])

  useEffect(() => {
    if (!sessionId || !historyReadyForStream) return
    if (!sandboxAccess) return
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

  const send = async (text: string, retryEventID?: string) => {
    if (streaming || interruptSession.isPending) return false
    if (optimisticSession) {
      toast.danger("This chat was not created. Start a new chat to try again.")
      return false
    }
    if (!sessionId) {
      toast.danger("Start a new chat before sending a message.")
      return false
    }

    const optimisticMessage = retryEventID
      ? null
      : optimisticUserEvent(sessionId, text)
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
        body: { text },
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

  const retryMessage = (eventID: string, text: string) => {
    void send(text, eventID)
  }

  const isBusy =
    streaming ||
    hasPendingClientEvent ||
    sendSessionMessage.isPending ||
    interruptSession.isPending
  const visibleBlocks = [...baseBlocks, ...liveBlocks]

  return (
    <div className="relative flex h-full min-w-0 flex-col">
      <ScrollToBottom
        className="min-h-0 flex-1"
        scrollViewClassName="min-h-0 [overflow-anchor:none]"
        followButtonClassName="!absolute !bottom-6 !left-1/2 !right-auto !flex !h-9 !w-9 !-translate-x-1/2 !items-center !justify-center !rounded-full !border !border-border !bg-surface !p-0 !text-muted !shadow-sm !transition-colors after:content-['↓'] hover:!bg-default hover:!text-foreground"
        initialScrollBehavior="auto"
        mode="bottom"
      >
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
      </ScrollToBottom>

      <Composer
        agent={agent}
        modelId={session.modelId}
        onModelChange={setModel}
        onSend={send}
        isStreaming={isBusy}
        onStop={stop}
      />
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

function SessionHistoryTopLoader({
  hasMore,
  isFetching,
  loadedEventCount,
  onLoadMore,
}: {
  hasMore: boolean
  isFetching: boolean
  loadedEventCount: number
  onLoadMore: () => Promise<unknown>
}) {
  const markerRef = useRef<HTMLDivElement | null>(null)
  const loadingRef = useRef(false)
  const restoreRef = useRef<{
    scrollPanel: HTMLElement
    scrollHeight: number
    scrollTop: number
    loadedEventCount: number
  } | null>(null)

  const loadMore = useCallback(() => {
    if (!hasMore || isFetching || loadingRef.current) return

    const scrollPanel = markerRef.current?.parentElement
    if (!scrollPanel) return

    restoreRef.current = {
      scrollPanel,
      scrollHeight: scrollPanel.scrollHeight,
      scrollTop: scrollPanel.scrollTop,
      loadedEventCount,
    }

    loadingRef.current = true
    void onLoadMore().catch(() => {
      restoreRef.current = null
      loadingRef.current = false
    })
  }, [hasMore, isFetching, loadedEventCount, onLoadMore])

  useEffect(() => {
    const marker = markerRef.current
    const scrollPanel = marker?.parentElement
    if (!marker || !scrollPanel || !hasMore) return

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) loadMore()
      },
      {
        root: scrollPanel,
        rootMargin: `${HISTORY_TOP_LOAD_THRESHOLD}px 0px 0px 0px`,
        threshold: 0,
      }
    )

    observer.observe(marker)
    return () => observer.disconnect()
  }, [hasMore, loadMore])

  useLayoutEffect(() => {
    const restore = restoreRef.current
    if (!restore || isFetching) return

    if (loadedEventCount <= restore.loadedEventCount) {
      restoreRef.current = null
      loadingRef.current = false
      return
    }

    const delta = restore.scrollPanel.scrollHeight - restore.scrollHeight
    restore.scrollPanel.scrollTop = restore.scrollTop + delta
    restoreRef.current = null
    loadingRef.current = false
  }, [isFetching, loadedEventCount])

  return (
    <>
      <div ref={markerRef} className="h-px" aria-hidden="true" />
      {isFetching ? (
        <div
          className="bg-surface/95 pointer-events-none absolute top-3 left-1/2 z-10 flex -translate-x-1/2 items-center justify-center rounded-full border border-border p-2 shadow-sm"
          role="status"
          aria-label="Loading older messages"
        >
          <Spinner size="sm" />
        </div>
      ) : null}
    </>
  )
}

function safeAgentById(id: string) {
  try {
    return agentById(id)
  } catch {
    return agentById(AGENTS[0].id)
  }
}
