"use client"

import { useState } from "react"
import { usePathname } from "next/navigation"
import { AnimatePresence, motion } from "motion/react"
import { Tooltip } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  useWorkspace,
  type ChatSession,
} from "@/app/w/(chat)/_components/shell"
import { DEFAULT_AGENT_ID, agentById } from "@/app/w/(chat)/_lib/agents"
import {
  CHAT_QUERY_STALE_TIME_MS,
  CHANNEL_SESSIONS_INFINITE_KEY,
  SIDEBAR_SESSION_PAGE_LIMIT,
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

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

export function ChannelGroup({
  channel,
  agentsByID,
  autoExpanded,
  slugAmbiguous,
}: {
  channel: SidebarChannelResponse
  agentsByID: Map<string, SidebarAgentResponse>
  autoExpanded: boolean
  slugAmbiguous: boolean
}) {
  const { openChannel, openChat } = useWorkspace()
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
        },
      },
    },
    {
      enabled: open && Boolean(channelID),
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )

  const sessions = dedupeSessions(
    sessionsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []
  )

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
                      title={sessionDisplayName(session)}
                      agent={sessionAgent}
                      meta={sessionActivityLabel(session)}
                      active={chatActive(id)}
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

type SidebarSessionAgent = {
  name: string
  icon: string
  avatarURL?: string
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
  }
}

function safeAgentById(id: string) {
  try {
    return agentById(id)
  } catch {
    return agentById(DEFAULT_AGENT_ID)
  }
}

function SessionSkeletonList() {
  return (
    <div className="flex flex-col gap-0.5">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9"
        >
          <span className="bg-default h-3 w-3 shrink-0 rounded-full" />
          <span className="bg-default h-3.5 flex-1 rounded" />
          <span className="bg-default h-3 w-8 rounded" />
        </div>
      ))}
    </div>
  )
}

function IndentedStatusRow({
  label,
  actionLabel,
  onAction,
  muted = false,
}: {
  label: string
  actionLabel?: string
  onAction?: () => void
  muted?: boolean
}) {
  return (
    <div
      className={`flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9 text-sm ${
        muted ? "text-muted/60" : "text-muted"
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {actionLabel && onAction ? (
        <button
          type="button"
          onClick={onAction}
          className="shrink-0 text-xs transition-colors hover:text-foreground"
        >
          {actionLabel}
        </button>
      ) : null}
    </div>
  )
}

function ChatRow({
  title,
  agent,
  meta,
  active,
  onIntent,
  onSelect,
}: {
  title: string
  agent: SidebarSessionAgent
  meta?: string
  active?: boolean
  onIntent?: () => void
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onFocus={onIntent}
      onMouseEnter={onIntent}
      onPointerDown={onIntent}
      onClick={onSelect}
      className={`flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9 text-left text-sm transition-colors ${
        active ? "bg-default" : "hover:bg-default"
      }`}
    >
      <span className="flex min-w-0 flex-1 items-center gap-1">
        <SessionAgentAvatar agent={agent} />
        <span className="min-w-0 flex-1 truncate">{title}</span>
      </span>
      {meta ? (
        <span className="shrink-0 text-xs text-muted">{meta}</span>
      ) : null}
    </button>
  )
}

function SessionAgentAvatar({ agent }: { agent: SidebarSessionAgent }) {
  const [failed, setFailed] = useState(false)

  return (
    <Tooltip delay={250} closeDelay={0}>
      <Tooltip.Trigger className="flex h-3 w-3 shrink-0 items-center justify-center">
        <span className="bg-default flex h-3 w-3 items-center justify-center overflow-hidden rounded-full text-muted ring-1 ring-border/70">
          {agent.avatarURL && !failed ? (
            <img
              src={agent.avatarURL}
              alt=""
              className="h-full w-full object-cover"
              onError={() => setFailed(true)}
            />
          ) : (
            <Icon icon={agent.icon} className="h-2 w-2" />
          )}
        </span>
      </Tooltip.Trigger>
      <Tooltip.Content placement="right" offset={8} className="text-xs">
        {agent.name}
      </Tooltip.Content>
    </Tooltip>
  )
}
