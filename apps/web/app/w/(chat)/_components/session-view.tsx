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
import {
  beginOptimisticSessionTurn,
  ensureSessionStream,
  interruptSessionTurn,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import {
  useSessionLiveEvents,
  useSessionRuntimeStatus,
  useSessionRuntimeStore,
  type SessionRuntimeStatus,
} from "@/app/w/(chat)/_stores/session-runtime-store"

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
  const liveEvents = useSessionLiveEvents(sessionId)
  const runtimeStatus = useSessionRuntimeStatus(sessionId)
  const turnActive = isTurnActive(runtimeStatus)
  const optimisticSession = sessionId?.startsWith("tmp_") ?? false
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
      enabled: Boolean(sessionId) && !optimisticSession,
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
  const hasPendingClientEvent = useMemo(
    () => historyEvents.some(isPendingClientEvent),
    [historyEvents]
  )
  const combinedEvents = useMemo(
    () =>
      sessionHistoryPagesToEvents([
        { data: historyEvents },
        { data: liveEvents },
      ]),
    [historyEvents, liveEvents]
  )
  const activeTurnID = turnActive
    ? session.agentTurnID?.trim() || latestTurnID(liveEvents)
    : undefined
  const visibleBlocks = useMemo(
    () =>
      sessionEventsToConversationBlocks(combinedEvents, {
        activeTurnID,
        activeTurnStartedAt: session.agentTurnStartedAt,
      }),
    [activeTurnID, combinedEvents, session.agentTurnStartedAt]
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
  const historyReadyForStream =
    optimisticSession ||
    sessionHistoryQuery.isSuccess ||
    sessionHistoryQuery.isError

  useEffect(() => {
    if (!sessionId || optimisticSession || !historyReadyForStream) return
    if (!turnActive) return
    ensureSessionStream(sessionId, {
      queryClient,
      replay: sessionHistoryQuery.isSuccess ? { mode: "none" } : { mode: "all" },
    })
  }, [
    historyReadyForStream,
    optimisticSession,
    queryClient,
    sessionHistoryQuery.isSuccess,
    sessionId,
    turnActive,
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
    if (turnActive) return false
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
    beginOptimisticSessionTurn(sessionId, [optimisticThinking])
    ensureSessionStream(sessionId, { queryClient, replay: { mode: "all" } })
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
      ensureSessionStream(sessionId, { queryClient })
      return true
    } catch (error) {
      const message = extractErrorMessage(error, "Could not send message")
      markOptimisticEventFailed(
        queryClient,
        sessionId,
        optimisticEventID,
        message
      )
      useSessionRuntimeStore.getState().finishStream(sessionId, {
        outcome: "failed",
      })
      toast.danger(message)
      return false
    }
  }

  const stop = () => {
    if (!sessionId || optimisticSession) return
    void interruptSessionTurn(sessionId, queryClient)
      .catch((error) => {
        toast.danger(extractErrorMessage(error, "Could not stop session"))
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
    turnActive || hasPendingClientEvent || sendSessionMessage.isPending
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
          <SessionPlanCard plan={latestPlan} turnActive={turnActive} />
        ) : null}
        <Composer
          sessionId={sessionId ?? "new-chat"}
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
  return ""
}

function safeAgentById(id: string) {
  try {
    return agentById(id)
  } catch {
    return agentById(AGENTS[0].id)
  }
}
