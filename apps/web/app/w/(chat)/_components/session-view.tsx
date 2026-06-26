"use client"

import { useCallback, useEffect, useMemo } from "react"
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
import { RequestUserInputBlock } from "@/app/w/(chat)/_components/request-user-input-block"
import type { ChatSession } from "@/app/w/(chat)/_components/shell"
import {
  CHAT_QUERY_STALE_TIME_MS,
  SESSION_EVENTS_INFINITE_KEY,
  SESSION_HISTORY_PAGE_LIMIT,
  patchSessionInChatCaches,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  sessionHistoryPagesToEvents,
  sessionEventsToConversationBlocks,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history"
import {
  eventTurnIDs,
  replayModeForLoadedSession,
  suppressBackendEventsForLiveTurn,
  suppressBackendEventsForLiveTurns,
  terminalOutcomeForTurnEvents,
} from "@/app/w/(chat)/_lib/session-stream-handoff"
import {
  imageAttachmentIDs,
  type ImageAttachmentMetadata,
} from "@/app/w/(chat)/_lib/image-attachments"
import type { CanvasDesignTarget } from "@/app/w/(chat)/_lib/canvas-design-links"
import type { PreviewBrowserTarget } from "@/app/w/(chat)/_lib/preview-browser-links"
import { type CodeLineCommentPayload } from "@/app/w/(chat)/_lib/code-line-comments"
import { latestSessionPlan } from "@/app/w/(chat)/_lib/session-plan"
import {
  eventMatchesInputRequest,
  latestInputRequestBlock,
} from "@/app/w/(chat)/_lib/session-input-requests"
import { fetchSessionUsage } from "@/app/w/(chat)/_lib/session-usage"
import {
  ensureSessionStream,
  hydrateSessionRuntimeFromResponse,
  interruptSessionTurn,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import {
  useSessionLiveEvents,
  useSessionRuntimeSummary,
  useSessionRuntimeStore,
  type SessionRuntimeStatus,
} from "@/app/w/(chat)/_stores/session-runtime-store"
import { useSessionWorkspaceStore } from "@/app/w/(chat)/_stores/session-workspace-store"

const ENABLE_DIRECT_SESSION_STREAM = true

export function SessionThreadView({
  session,
  sessionId,
}: {
  session: ChatSession
  sessionId?: string
}) {
  const queryClient = useQueryClient()
  const liveEvents = useSessionLiveEvents(sessionId)
  const openCanvasTarget = useSessionWorkspaceStore(
    (state) => state.openCanvasTarget
  )
  const openBrowserURL = useSessionWorkspaceStore(
    (state) => state.openBrowserURL
  )
  const renderedLiveEvents = useMemo(
    () => (ENABLE_DIRECT_SESSION_STREAM ? liveEvents : []),
    [liveEvents]
  )
  const runtimeSummary = useSessionRuntimeSummary(sessionId)
  const runtimeStatus = runtimeSummary.status
  const turnActive = isTurnActive(runtimeStatus)
  const temporarySession = sessionId?.startsWith("tmp_") ?? false
  const sendSessionMessage = $api.useMutation(
    "post",
    "/v1/sessions/{id}/messages"
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
      enabled: Boolean(sessionId) && !temporarySession,
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const historyPages = sessionHistoryQuery.data?.pages
  const historyEvents = useMemo(
    () => sessionHistoryPagesToEvents(historyPages ?? []),
    [historyPages]
  )
  const activeBackendTurnID =
    turnActive && session.agentTurnID?.trim()
      ? session.agentTurnID.trim()
      : undefined
  const liveTurnIDs = useMemo(
    () => eventTurnIDs(renderedLiveEvents),
    [renderedLiveEvents]
  )
  const renderedHistoryEvents = useMemo(
    () =>
      suppressBackendEventsForLiveTurns(historyEvents, [
        activeBackendTurnID,
        ...liveTurnIDs,
      ]),
    [activeBackendTurnID, historyEvents, liveTurnIDs]
  )
  const reconcileHistoryEvents = useMemo(
    () => suppressBackendEventsForLiveTurn(historyEvents, activeBackendTurnID),
    [activeBackendTurnID, historyEvents]
  )
  const durableActiveTurnOutcome = useMemo(
    () => terminalOutcomeForTurnEvents(historyEvents, activeBackendTurnID),
    [activeBackendTurnID, historyEvents]
  )
  const combinedEvents = useMemo(
    () =>
      sessionHistoryPagesToEvents([
        { data: renderedHistoryEvents },
        { data: renderedLiveEvents },
      ]),
    [renderedHistoryEvents, renderedLiveEvents]
  )
  const activeInputRequest = useMemo(
    () =>
      runtimeStatus === "waiting_for_user"
        ? latestInputRequestBlock(combinedEvents)
        : undefined,
    [combinedEvents, runtimeStatus]
  )
  const visibleConversationEvents = useMemo(
    () =>
      activeInputRequest
        ? combinedEvents.filter(
            (event) =>
              !eventMatchesInputRequest(
                event,
                activeInputRequest.questionRequestId
              )
          )
        : combinedEvents,
    [activeInputRequest, combinedEvents]
  )
  const activeTurnID = turnActive
    ? activeBackendTurnID || latestTurnID(renderedLiveEvents)
    : undefined
  const visibleBlocks = useMemo(
    () =>
      sessionEventsToConversationBlocks(visibleConversationEvents, {
        activeTurnID,
        activeTurnStartedAt: session.agentTurnStartedAt,
      }),
    [activeTurnID, session.agentTurnStartedAt, visibleConversationEvents]
  )
  const latestPlan = useMemo(
    () => latestSessionPlan(combinedEvents),
    [combinedEvents]
  )
  const fetchNextHistoryPage = sessionHistoryQuery.fetchNextPage
  const loadNextHistoryPage = useCallback(
    () => fetchNextHistoryPage(),
    [fetchNextHistoryPage]
  )
  const historyLoadedForStream = sessionHistoryQuery.isSuccess
  const sessionReadyForStream = session.loaded !== false

  useEffect(() => {
    if (!sessionId || temporarySession) return
    const controller = new AbortController()
    void fetchSessionUsage(sessionId, controller.signal)
      .then((usage) => {
        useSessionRuntimeStore.getState().hydrateSessionUsage(sessionId, usage)
      })
      .catch((error) => {
        if (error instanceof DOMException && error.name === "AbortError") {
          return
        }
      })
    return () => controller.abort()
  }, [sessionId, temporarySession])

  useEffect(() => {
    if (!sessionId || temporarySession || historyEvents.length === 0) return
    useSessionRuntimeStore
      .getState()
      .reconcileLiveEvents(sessionId, reconcileHistoryEvents)
  }, [
    historyEvents.length,
    temporarySession,
    reconcileHistoryEvents,
    sessionId,
  ])

  useEffect(() => {
    if (!sessionId || !turnActive || !durableActiveTurnOutcome) return
    useSessionRuntimeStore.getState().finishStream(sessionId, {
      outcome: durableActiveTurnOutcome,
    })
  }, [durableActiveTurnOutcome, sessionId, turnActive])

  useEffect(() => {
    if (!ENABLE_DIRECT_SESSION_STREAM) return
    if (
      !sessionId ||
      temporarySession ||
      !historyLoadedForStream ||
      !sessionReadyForStream
    ) {
      return
    }
    if (turnActive && !activeBackendTurnID) return
    ensureSessionStream(sessionId, {
      queryClient,
      replay: replayModeForLoadedSession(activeBackendTurnID),
    })
  }, [
    activeBackendTurnID,
    historyLoadedForStream,
    temporarySession,
    queryClient,
    sessionReadyForStream,
    sessionId,
    turnActive,
  ])

  const send = async (
    text: string,
    options: {
      attachments?: ImageAttachmentMetadata[]
      codeLineComments?: CodeLineCommentPayload[]
    } = {}
  ) => {
    const attachments = options.attachments ?? []
    const codeLineComments = options.codeLineComments ?? []
    if (turnActive) return false
    if (temporarySession) {
      toast.danger("This chat was not created. Start a new chat to try again.")
      return false
    }
    if (!sessionId) {
      toast.danger("Start a new chat before sending a message.")
      return false
    }

    const messagePayload = {
      ...(attachments.length
        ? { attachment_ids: imageAttachmentIDs(attachments) }
        : {}),
      ...(codeLineComments.length
        ? {
            code_line_comments: codeLineComments.map(
              (comment): Record<string, unknown> => ({ ...comment })
            ),
          }
        : {}),
    }
    const optimisticEvent = optimisticUserMessageEvent(
      sessionId,
      text,
      attachments,
      codeLineComments
    )
    const optimisticEventID =
      optimisticEvent.event_id ?? optimisticEvent.id ?? ""
    appendLiveSessionEvent(sessionId, optimisticEvent)
    try {
      const response = await sendSessionMessage.mutateAsync({
        params: { path: { id: sessionId } },
        body: {
          text,
          attachment_ids: messagePayload.attachment_ids,
          code_line_comments: messagePayload.code_line_comments,
        },
      })
      if (response.session) {
        patchSessionInChatCaches(queryClient, response.session)
        hydrateSessionRuntimeFromResponse(response.session, queryClient)
        ensureSessionStream(sessionId, {
          queryClient,
          replay: replayModeForLoadedSession(
            normalizedTurnID(response.session.agent_turn_id)
          ),
        })
      }
      return true
    } catch (error) {
      removeLiveSessionEvent(sessionId, optimisticEventID)
      const message = extractErrorMessage(error, "Could not send message")
      toast.danger(message)
      return false
    }
  }

  const stop = () => {
    if (!sessionId || temporarySession) return
    void interruptSessionTurn(sessionId, queryClient).catch((error) => {
      toast.danger(extractErrorMessage(error, "Could not stop session"))
    })
  }

  const handleOpenCanvasTarget = useCallback(
    (target: CanvasDesignTarget) => {
      openCanvasTarget(sessionId ?? "new-chat", target)
    },
    [openCanvasTarget, sessionId]
  )
  const handleOpenPreviewTarget = useCallback(
    (target: PreviewBrowserTarget) => {
      openBrowserURL(sessionId ?? "new-chat", target.url)
    },
    [openBrowserURL, sessionId]
  )
  const handleInputRequestSubmitted = useCallback(
    (_questionRequestId: string) => {
      if (!sessionId) return
      useSessionRuntimeStore.getState().setStatus(sessionId, "streaming", {
        pendingInput: undefined,
      })
    },
    [sessionId]
  )
  const isBusy = turnActive || sendSessionMessage.isPending
  const showHistorySkeleton =
    !temporarySession && sessionHistoryQuery.isPending && !historyPages?.length
  const followButtonClassName = `!absolute ${
    activeInputRequest || latestPlan ? "!bottom-20" : "!bottom-6"
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
              onOpenCanvasTarget={handleOpenCanvasTarget}
              onOpenPreviewTarget={handleOpenPreviewTarget}
            />
          </>
        )}
      </ScrollToBottom>

      <div className="relative z-20 shrink-0">
        {activeInputRequest ? (
          <RequestUserInputBlock
            block={activeInputRequest}
            sessionId={sessionId}
            agentName={session.agentName}
            onSubmitted={handleInputRequestSubmitted}
          />
        ) : latestPlan ? (
          <SessionPlanCard plan={latestPlan} turnActive={turnActive} />
        ) : null}
        <Composer
          sessionId={sessionId ?? "new-chat"}
          agentId={session.agentId}
          modelId={session.modelId}
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

function normalizedTurnID(value: unknown) {
  return typeof value === "string" && value.trim() ? value.trim() : undefined
}

function appendLiveSessionEvent(
  sessionId: string,
  event: SessionEventResponse
) {
  const store = useSessionRuntimeStore.getState()
  store.setLiveEvents(sessionId, [
    ...(store.liveEventsBySessionId[sessionId] ?? []),
    event,
  ])
}

function removeLiveSessionEvent(sessionId: string, eventID: string) {
  const store = useSessionRuntimeStore.getState()
  store.setLiveEvents(
    sessionId,
    (store.liveEventsBySessionId[sessionId] ?? []).filter(
      (event) => event.id !== eventID && event.event_id !== eventID
    )
  )
}

function optimisticUserMessageEvent(
  sessionId: string,
  text: string,
  attachments: ImageAttachmentMetadata[],
  codeLineComments: CodeLineCommentPayload[]
): SessionEventResponse {
  const eventID = `client:${clientID()}`
  return {
    id: eventID,
    event_id: eventID,
    event_type: "user.message.received",
    event_at: new Date().toISOString(),
    session_id: sessionId,
    source: "web",
    durability: "durable",
    sequence_number: 0,
    payload: {
      text,
      ...(attachments.length ? { attachments } : {}),
      ...(codeLineComments.length
        ? { code_line_comments: codeLineComments }
        : {}),
    },
  }
}

function clientID() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function isTurnActive(status: SessionRuntimeStatus) {
  return (
    status === "queued" ||
    status === "streaming" ||
    status === "waiting_for_user"
  )
}

function latestTurnID(events: SessionEventResponse[]) {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const payload = events[index].payload
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
      continue
    }
    const turnID = (payload as Record<string, unknown>).turn_id
    if (typeof turnID === "string" && turnID.trim()) {
      return turnID.trim()
    }
  }
  return undefined
}
