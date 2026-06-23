"use client"

import { useEffect, useMemo, useState } from "react"
import { usePathname } from "next/navigation"
import { useQueryClient, type InfiniteData } from "@tanstack/react-query"
import { AnimatePresence, motion } from "motion/react"
import { Icon } from "@iconify/react"
import {
  useWorkspace,
  type ChatSession,
} from "@/app/w/(chat)/_components/shell"
import { DEFAULT_AGENT_ID, agentById } from "@/app/w/(chat)/_lib/agents"
import {
  CHAT_QUERY_STALE_TIME_MS,
  CHANNEL_SESSIONS_INFINITE_KEY,
  SIDEBAR_SESSION_SORT,
  SIDEBAR_SESSION_PAGE_LIMIT,
  type PaginatedSessions,
} from "@/app/w/(chat)/_lib/chat-cache"
import { $api } from "@/lib/api/hooks"
import {
  agentAvatarURL,
  agentDisplayName,
  agentIcon,
  agentModel,
  channelDisplayName,
  channelRouteSlug,
  dedupeSessions,
  sessionActivityLabel,
  sessionDisplayName,
  type SidebarAgentResponse,
  type SidebarChannelResponse,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { ChatRow, type SidebarSessionAgent } from "./sidebar-chat-row"
import {
  IndentedStatusRow,
  SessionSkeletonList,
} from "./sidebar-channel-status"
import { hydrateSessionListRuntime } from "@/app/w/(chat)/_stores/session-stream-manager"

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

export function ChannelGroup({
  channel,
  agentsByID,
  autoExpanded,
  onRenameSession,
  onShareSession,
  slugAmbiguous,
}: {
  channel: SidebarChannelResponse
  agentsByID: Map<string, SidebarAgentResponse>
  autoExpanded: boolean
  onRenameSession: (sessionId: string, name: string) => void
  onShareSession: (sessionId: string) => void
  slugAmbiguous: boolean
}) {
  const { openChannel, openChat } = useWorkspace()
  const queryClient = useQueryClient()
  const pathname = usePathname()
  const slug = channelRouteSlug(channel)
  const channelPath = `/w/channels/${slug}`
  const channelActive =
    !slugAmbiguous &&
    (pathname === channelPath || pathname.startsWith(`${channelPath}/`))
  const [manualOpen, setManualOpen] = useState<boolean | null>(null)
  const open = channelActive || (manualOpen ?? autoExpanded)
  const chatActive = (id: string) => pathname === `${channelPath}/${id}`
  const channelID = channel.id ?? ""
  const initialSessions = channel.recent_sessions
  const initialSessionsData = useMemo<
    InfiniteData<PaginatedSessions> | undefined
  >(() => {
    if (!initialSessions) return undefined
    return {
      pageParams: ["0"],
      pages: [
        {
          data: initialSessions,
          has_more: Boolean(channel.recent_sessions_has_more),
          next_cursor: channel.recent_sessions_next_cursor,
        },
      ],
    }
  }, [
    initialSessions,
    channel.recent_sessions_has_more,
    channel.recent_sessions_next_cursor,
  ])

  const sessionsQuery = $api.useInfiniteQuery(
    "get",
    "/v1/channels/{id}/sessions",
    {
      _hivyQueryKey: CHANNEL_SESSIONS_INFINITE_KEY,
      params: {
        path: {
          id: channelID,
        },
        query: {
          limit: SIDEBAR_SESSION_PAGE_LIMIT,
          sort: SIDEBAR_SESSION_SORT,
        },
      },
    },
    {
      enabled: open && Boolean(channelID) && !initialSessionsData,
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      initialData: initialSessionsData,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )

  const sessions = dedupeSessions(
    sessionsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []
  )

  useEffect(() => {
    hydrateSessionListRuntime(sessions, queryClient)
  }, [queryClient, sessions])

  return (
    <div className="flex flex-col">
      <div
        className={`group flex items-center rounded-lg text-sm transition-colors ${
          channelActive ? "bg-default" : "hover:bg-default"
        }`}
      >
        <button
          type="button"
          onClick={() => openChannel(slug)}
          className="flex min-w-0 flex-1 items-center gap-2.5 px-3 py-1.5 text-left"
        >
          <Icon icon="lucide:hash" className="h-4 w-4 shrink-0 text-muted" />
          <span className="min-w-0 flex-1 truncate">
            {channelDisplayName(channel)}
          </span>
        </button>
        <button
          type="button"
          aria-label={open ? "Collapse channel" : "Expand channel"}
          onClick={() => setManualOpen(!open)}
          className="hover:bg-surface mr-1 rounded-md p-1 text-muted transition-colors"
        >
          <Icon
            icon={open ? "lucide:chevron-down" : "lucide:chevron-right"}
            className="h-3.5 w-3.5"
          />
        </button>
      </div>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            key="chats"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={COLLAPSE_TRANSITION}
            className="overflow-hidden"
          >
            <div className="flex flex-col gap-0.5 pt-0.5">
              {sessionsQuery.isLoading ? (
                <SessionSkeletonList />
              ) : sessionsQuery.isError ? (
                <IndentedStatusRow
                  label="Could not load chats"
                  actionLabel="Retry"
                  onAction={() => void sessionsQuery.refetch()}
                />
              ) : sessions.length ? (
                sessions.map((session) => {
                  const id = session.id ?? ""
                  const sessionAgent = sidebarSessionAgent(session, agentsByID)
                  const apiAgent = apiAgentForSession(session, agentsByID)
                  return (
                    <ChatRow
                      key={id}
                      sessionId={id}
                      title={sessionDisplayName(session)}
                      agent={sessionAgent}
                      meta={sessionActivityLabel(session)}
                      active={chatActive(id)}
                      onRename={
                        id
                          ? () => onRenameSession(id, sessionDisplayName(session))
                          : undefined
                      }
                      onShare={id ? () => onShareSession(id) : undefined}
                      onSelect={() => {
                        openChat(
                          slug,
                          id,
                          chatSessionFromResponse(session, apiAgent)
                        )
                      }}
                    />
                  )
                })
              ) : (
                <IndentedStatusRow label="No chats" muted />
              )}
              {sessionsQuery.hasNextPage ? (
                <button
                  type="button"
                  disabled={sessionsQuery.isFetchingNextPage}
                  onClick={() => void sessionsQuery.fetchNextPage()}
                  className="hover:bg-default rounded-lg px-3 py-1 pl-9 text-left text-sm text-muted transition-colors"
                >
                  {sessionsQuery.isFetchingNextPage
                    ? "Loading..."
                    : "Show more"}
                </button>
              ) : null}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

function apiAgentForSession(
  session: SidebarSessionResponse,
  agentsByID: Map<string, SidebarAgentResponse>
): SidebarAgentResponse | undefined {
  const agentID = session.agent_id?.trim()
  return agentID ? agentsByID.get(agentID) : undefined
}

function sidebarSessionAgent(
  session: SidebarSessionResponse,
  agentsByID: Map<string, SidebarAgentResponse>
): SidebarSessionAgent {
  const agentID = session.agent_id?.trim() || DEFAULT_AGENT_ID
  const apiAgent = agentsByID.get(agentID)
  const fallback = safeAgentById(agentID)
  return {
    name: apiAgent ? agentDisplayName(apiAgent) : fallback.name,
    icon: apiAgent ? agentIcon(apiAgent) : fallback.icon,
    avatarURL: agentAvatarURL(apiAgent),
  }
}

function chatSessionFromResponse(
  session: SidebarSessionResponse,
  apiAgent?: SidebarAgentResponse
): ChatSession {
  const agentID = session.agent_id?.trim() || DEFAULT_AGENT_ID
  const fallback = safeAgentById(agentID)
  const modelId =
    session.model?.trim() || agentModel(apiAgent) || fallback.defaultModelId
  return {
    title: sessionDisplayName(session),
    agentId: agentID,
    agentName: apiAgent ? agentDisplayName(apiAgent) : fallback.name,
    agentIcon: apiAgent ? agentIcon(apiAgent) : fallback.icon,
    agentAvatarURL: agentAvatarURL(apiAgent),
    modelId,
    agentTurnStatus: session.agent_turn_status,
    agentTurnID: session.agent_turn_id,
    agentTurnStartedAt: session.agent_turn_started_at,
    lastTurnOutcome: session.last_turn_outcome,
  }
}

function safeAgentById(id: string) {
  try {
    return agentById(id)
  } catch {
    return agentById(DEFAULT_AGENT_ID)
  }
}
