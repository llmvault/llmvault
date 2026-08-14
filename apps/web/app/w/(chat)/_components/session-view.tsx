"use client"

import { useCallback, useEffect, useMemo } from "react"
import { toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import ScrollToBottom from "react-scroll-to-bottom"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import { Composer } from "@/app/w/(chat)/_components/composer"
import { Conversation } from "@/app/w/(chat)/_components/conversation"
import { ExternalSessionNotice } from "@/app/w/(chat)/_components/external-session-notice"
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
} from "@/app/w/(chat)/_lib/session-history"
import {
  appendLiveSessionEvent,
  latestTurnID,
  optimisticUserMessageEvent,
  removeLiveSessionEvent,
} from "@/app/w/(chat)/_lib/session-live-events"
import {
  isSubagentSessionEvent,
  mergeSubagentRuns,
  sessionSubagentRunsFromEvents,
} from "@/app/w/(chat)/_lib/session-subagent-runs"
import { subagentOpenTarget } from "@/app/w/(chat)/_lib/session-subagent-open"
import {
  eventTurnIDs,
  normalizedTurnID,
  replayModeForLoadedSession,
  suppressBackendEventsForLiveTurn,
  suppressBackendEventsForLiveTurns,
  terminalOutcomeForTurnEvents,
} from "@/app/w/(chat)/_lib/session-stream-handoff"
import {
  imageAttachmentIDs,
  type ImageAttachmentMetadata,
} from "@/app/w/(chat)/_lib/image-attachments"
import type { PreviewBrowserTarget } from "@/app/w/(chat)/_lib/preview-browser-links"
import { launchAppPreview } from "@/app/w/(chat)/_lib/app-launch"
import { type CodeLineCommentPayload } from "@/app/w/(chat)/_lib/code-line-comments"
import type { SubagentConversationBlock } from "@/app/w/(chat)/_lib/static-data"
import { latestSessionPlan } from "@/app/w/(chat)/_lib/session-plan"
import { externalSessionContinuation } from "@/app/w/(chat)/_lib/external-session"
import {
  eventMatchesInputRequest,
  latestInputRequestBlock,
} from "@/app/w/(chat)/_lib/session-input-requests"
import {
  ensureSessionNotices,
  ensureSessionStream,
  hydrateSessionRuntimeFromResponse,
  interruptSessionTurn,
  stopSessionStream,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import {
  useSessionLiveEvents,
  useSessionRuntimeSummary,
  useSessionSubagentRuns,
  useSessionRuntimeStore,
} from "@/app/w/(chat)/_stores/session-runtime-store"
import { isRuntimeActive } from "@/app/w/(chat)/_lib/session-runtime-helpers"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import {
  configureDesktopRuntime,
  deliverDesktopMessage,
  isDesktopApp,
} from "@/lib/desktop/bridge"
import { ensureDesktopSessionStream } from "@/app/w/(chat)/_stores/desktop-session-stream-manager"

const ENABLE_DIRECT_SESSION_STREAM = true

export function SessionThreadView({
  session,
  sessionId,
}: {
  session: ChatSession
  sessionId?: string
}) {
  const queryClient = useQueryClient()
  const { activeOrg } = useAuth()
  const activeOrgId = activeOrg?.id
  const liveEvents = useSessionLiveEvents(sessionId)
  const liveSubagentRuns = useSessionSubagentRuns(sessionId)
  const openBrowserURL = useSessionWorkspaceStore(
    (state) => state.openBrowserURL
  )
  const navigateBrowser = useSessionWorkspaceStore(
    (state) => state.navigateBrowser
  )
  const openSubagentRun = useSessionWorkspaceStore(
    (state) => state.openSubagentRun
  )
  const workspaceSessionId = sessionId ?? "new-chat"
  const activeSubagentJobId = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionId).subagents.activeJobId
  )
  const renderedLiveEvents = useMemo(
    () => (ENABLE_DIRECT_SESSION_STREAM ? liveEvents : []),
    [liveEvents]
  )
  const runtimeSummary = useSessionRuntimeSummary(sessionId)
  const runtimeStatus = runtimeSummary.status
  const turnActive = isRuntimeActive(runtimeStatus)
  const temporarySession = sessionId?.startsWith("tmp_") ?? false
  const sendSessionMessage = $api.useMutation(
    "post",
    "/v1/sessions/{id}/messages"
  )
  const bootstrapDesktopAgent = $api.useMutation(
    "post",
    "/v1/desktop/agents/{agentID}/runtime-config"
  )
  const sendDesktopSessionMessage = $api.useMutation(
    "post",
    "/v1/desktop/sessions/{id}/messages"
  )
  const recordDesktopDelivery = $api.useMutation(
    "post",
    "/v1/desktop/sessions/{id}/delivery"
  )
  const interruptSession = $api.useMutation(
    "post",
    "/v1/sessions/{id}/interrupt"
  )
  const sessionUsageQuery = $api.useQuery(
    "get",
    "/v1/sessions/{id}/usage",
    { params: { path: { id: sessionId ?? "" } } },
    {
      enabled: Boolean(sessionId) && !temporarySession,
      retry: false,
    }
  )
  const sessionHistoryQuery = $api.useInfiniteQuery(
    "get",
    "/v1/sessions/{id}/events",
    {
      _hivyQueryKey: SESSION_EVENTS_INFINITE_KEY,
      params: {
        path: { id: sessionId ?? "" },
        query: {
          limit: SESSION_HISTORY_PAGE_LIMIT,
          view: "transcript",
        },
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
  const parentHistoryEvents = useMemo(
    () => historyEvents.filter((event) => !isSubagentSessionEvent(event)),
    [historyEvents]
  )
  const historySubagentRuns = useMemo(
    () => sessionSubagentRunsFromEvents(historyEvents),
    [historyEvents]
  )
  const subagentRuns = useMemo(
    () => mergeSubagentRuns(historySubagentRuns, liveSubagentRuns),
    [historySubagentRuns, liveSubagentRuns]
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
      suppressBackendEventsForLiveTurns(parentHistoryEvents, [
        activeBackendTurnID,
        ...liveTurnIDs,
      ]),
    [activeBackendTurnID, parentHistoryEvents, liveTurnIDs]
  )
  const reconcileHistoryEvents = useMemo(
    () =>
      suppressBackendEventsForLiveTurn(
        parentHistoryEvents,
        activeBackendTurnID
      ),
    [activeBackendTurnID, parentHistoryEvents]
  )
  const durableActiveTurnOutcome = useMemo(
    () =>
      terminalOutcomeForTurnEvents(parentHistoryEvents, activeBackendTurnID),
    [activeBackendTurnID, parentHistoryEvents]
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
  const externalContinuation = externalSessionContinuation(session)
  const visibleBlocks = useMemo(
    () =>
      sessionEventsToConversationBlocks(visibleConversationEvents, {
        activeTurnID,
        activeTurnStartedAt: session.agentTurnStartedAt,
        subagentRuns,
        activeSubagentJobId,
      }),
    [
      activeSubagentJobId,
      activeTurnID,
      session.agentTurnStartedAt,
      subagentRuns,
      visibleConversationEvents,
    ]
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
    if (!sessionId || temporarySession || !sessionUsageQuery.data) return
    useSessionRuntimeStore
      .getState()
      .hydrateSessionUsage(sessionId, sessionUsageQuery.data)
  }, [sessionId, sessionUsageQuery.data, temporarySession])

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
    if (!sessionId || temporarySession || historySubagentRuns.length === 0) {
      return
    }
    useSessionRuntimeStore
      .getState()
      .mergeSubagentRuns(sessionId, historySubagentRuns)
  }, [historySubagentRuns, sessionId, temporarySession])

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
    ensureSessionNotices(sessionId, { queryClient, orgId: activeOrgId })
    if (!turnActive) {
      stopSessionStream(sessionId)
      return
    }
    if (!activeBackendTurnID) return
    if (isDesktopApp()) {
      stopSessionStream(sessionId)
      ensureDesktopSessionStream(sessionId, activeBackendTurnID, queryClient)
      return
    }
    ensureSessionStream(sessionId, {
      queryClient,
      orgId: activeOrgId,
      replay: replayModeForLoadedSession(activeBackendTurnID),
    })
  }, [
    activeBackendTurnID,
    activeOrgId,
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
    if (externalContinuation) {
      toast.danger(
        `Continue this conversation in ${externalContinuation.providerLabel}.`
      )
      return false
    }
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
      const body = {
        text,
        attachment_ids: messagePayload.attachment_ids,
        code_line_comments: messagePayload.code_line_comments,
      }
      let response
      if (isDesktopApp()) {
        const bootstrap = await bootstrapDesktopAgent.mutateAsync({
          params: { path: { agentID: session.agentId } },
        })
        if (!bootstrap.config) {
          throw new Error("Cloud returned an empty desktop runtime configuration")
        }
        await configureDesktopRuntime(session.agentId, bootstrap.config)
        const prepared = await sendDesktopSessionMessage.mutateAsync({
          params: { path: { id: sessionId } },
          body,
        })
        if (!prepared.runtime_request) {
          throw new Error("Cloud returned an empty desktop runtime request")
        }
        const delivery = await deliverDesktopMessage<{
          stream_id?: string
          turn_id?: string
        }>(session.agentId, sessionId, prepared.runtime_request)
        if (!delivery.turn_id) {
          throw new Error("Local runtime accepted the message without a turn id")
        }
        response = await recordDesktopDelivery.mutateAsync({
          params: { path: { id: sessionId } },
          body: {
            stream_id: delivery.stream_id ?? "",
            turn_id: delivery.turn_id,
          },
        })
      } else {
        response = await sendSessionMessage.mutateAsync({
          params: { path: { id: sessionId } },
          body,
        })
      }
      if (response.session) {
        patchSessionInChatCaches(queryClient, response.session)
        hydrateSessionRuntimeFromResponse(response.session, queryClient)
        const turnId = normalizedTurnID(response.session.agent_turn_id)
        if (isDesktopApp() && turnId) {
          ensureDesktopSessionStream(sessionId, turnId, queryClient)
        } else {
          ensureSessionStream(sessionId, {
            queryClient,
            orgId: activeOrgId,
            replay: replayModeForLoadedSession(turnId),
          })
        }
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
    void interruptSessionTurn(sessionId, queryClient, () =>
      interruptSession.mutateAsync({
        params: { path: { id: sessionId } },
      })
    ).catch((error) => {
      toast.danger(extractErrorMessage(error, "Could not stop session"))
    })
  }

  const handleOpenPreviewTarget = useCallback(
    (target: PreviewBrowserTarget) => {
      const sid = sessionId ?? "new-chat"
      if (!target.appId) {
        openBrowserURL(sid, target.url)
        return
      }
      // App card: open the panel immediately (blank), exchange a one-time
      // launch token, then load the app's /auth/callback so the iframe boots
      // with the user already authenticated. The address bar keeps the
      // shareable app url; only the iframe src swaps to the callback.
      openBrowserURL(sid, target.url, "about:blank")
      void launchAppPreview(target.appId)
        .then(({ iframeUrl }) => navigateBrowser(sid, iframeUrl))
        .catch((error) =>
          toast.danger(extractErrorMessage(error, "Could not open app"))
        )
    },
    [openBrowserURL, navigateBrowser, sessionId]
  )
  const handleOpenSubagentRun = useCallback(
    (block: SubagentConversationBlock) => {
      const { activeJobId } = subagentOpenTarget(block, subagentRuns)
      if (!activeJobId) return

      openSubagentRun(workspaceSessionId, activeJobId)
    },
    [openSubagentRun, subagentRuns, workspaceSessionId]
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
  const isBusy =
    turnActive ||
    sendSessionMessage.isPending ||
    bootstrapDesktopAgent.isPending ||
    sendDesktopSessionMessage.isPending ||
    recordDesktopDelivery.isPending
  const showHistorySkeleton =
    !temporarySession && sessionHistoryQuery.isPending && !historyPages?.length
  const showBottomDock = Boolean(
    externalContinuation || activeInputRequest || latestPlan
  )
  const followButtonClassName = `!absolute ${
    showBottomDock ? "!bottom-20" : "!bottom-6"
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
              onOpenPreviewTarget={handleOpenPreviewTarget}
              onOpenSubagentRun={handleOpenSubagentRun}
            />
          </>
        )}
      </ScrollToBottom>

      <div className="relative z-20 shrink-0">
        {showBottomDock ? (
          <div className="pointer-events-none absolute inset-x-0 bottom-full z-30 pb-2">
            <div className="mx-auto flex w-full max-w-3xl flex-col gap-2 px-4">
              {externalContinuation ? (
                <ExternalSessionNotice continuation={externalContinuation} />
              ) : activeInputRequest ? (
                <RequestUserInputBlock
                  block={activeInputRequest}
                  sessionId={sessionId}
                  agentName={session.agentName}
                  onSubmitted={handleInputRequestSubmitted}
                  inline
                />
              ) : latestPlan ? (
                <SessionPlanCard
                  plan={latestPlan}
                  turnActive={turnActive}
                  inline
                />
              ) : null}
            </div>
          </div>
        ) : null}
        <Composer
          sessionId={sessionId ?? "new-chat"}
          agentId={session.agentId}
          modelId={session.modelId}
          onSend={(text, attachments, codeLineComments) =>
            send(text, { attachments, codeLineComments })
          }
          isStreaming={isBusy}
          isDisabled={Boolean(externalContinuation)}
          placeholder={
            externalContinuation
              ? `Continue in ${externalContinuation.providerLabel}`
              : undefined
          }
          onStop={stop}
        />
      </div>
    </div>
  )
}
