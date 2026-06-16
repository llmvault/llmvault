"use client"

import { useMemo, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { AnimatePresence, motion } from "motion/react"
import { Button, Popover, Typography } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import {
  useWorkspace,
  type ChatSession,
} from "@/app/w/(chat)/_components/shell"
import { DEFAULT_AGENT_ID, agentById } from "@/app/w/(chat)/_lib/agents"
import {
  CHAT_QUERY_STALE_TIME_MS,
  CHANNEL_SESSIONS_INFINITE_KEY,
  SIDEBAR_SESSION_PAGE_LIMIT,
  prefetchSessionRoute,
  seedSessionDetail,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  channelDisplayName,
  channelRouteSlug,
  channelRouteSlugCounts,
  dedupeSessions,
  sessionActivityLabel,
  sessionDisplayName,
  type SidebarChannelResponse,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"

const SIDEBAR_CHANNEL_PAGE_LIMIT = 100
const CHANNELS_INFINITE_KEY = "channels-infinite-v1"

export function Sidebar({ onCollapse }: { onCollapse: () => void }) {
  const { startNewChat } = useWorkspace()
  const router = useRouter()
  const channelsQuery = $api.useInfiniteQuery(
    "get",
    "/v1/channels",
    {
      _hivyQueryKey: CHANNELS_INFINITE_KEY,
      params: {
        query: {
          limit: SIDEBAR_CHANNEL_PAGE_LIMIT,
        },
      },
    },
    {
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )

  const channels = useMemo(
    () => channelsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? [],
    [channelsQuery.data?.pages]
  )
  const channelSlugCounts = useMemo(
    () => channelRouteSlugCounts(channels),
    [channels]
  )

  return (
    <div className="bg-surface flex h-full flex-col">
      <div className="flex h-12 shrink-0 items-center gap-1 px-3">
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Collapse sidebar"
          onPress={onCollapse}
        >
          <Icon icon="lucide:panel-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Back">
          <Icon icon="lucide:arrow-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Forward">
          <Icon icon="lucide:arrow-right" className="h-4 w-4 text-muted/50" />
        </Button>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-3 pb-4">
        <div className="flex flex-col gap-0.5">
          <NavRow
            icon="lucide:square-pen"
            label="New chat"
            onClick={startNewChat}
          />
          <NavRow icon="lucide:search" label="Search" />
          <NavRow
            icon="lucide:toy-brick"
            label="Plugins"
            onClick={() => router.push("/w/plugins")}
          />
          <NavRow icon="lucide:clock" label="Automations" />
        </div>

        <div className="flex flex-col gap-0.5">
          <SectionLabel>CHANNELS</SectionLabel>
          {channelsQuery.isLoading ? (
            <ChannelSkeletonList />
          ) : channelsQuery.isError ? (
            <SidebarStatusRow
              label="Could not load channels"
              actionLabel="Retry"
              onAction={() => void channelsQuery.refetch()}
            />
          ) : channels.length ? (
            channels.map((channel, index) => (
              <ChannelGroup
                key={channel.id ?? channel.name ?? index}
                channel={channel}
                slugAmbiguous={
                  (channelSlugCounts.get(channelRouteSlug(channel)) ?? 0) > 1
                }
              />
            ))
          ) : (
            <SidebarStatusRow label="No channels" />
          )}
          {channelsQuery.hasNextPage ? (
            <button
              type="button"
              disabled={channelsQuery.isFetchingNextPage}
              onClick={() => void channelsQuery.fetchNextPage()}
              className="hover:bg-default rounded-lg px-3 py-1.5 text-left text-sm text-muted transition-colors"
            >
              {channelsQuery.isFetchingNextPage
                ? "Loading channels..."
                : "Show more channels"}
            </button>
          ) : null}
        </div>
      </div>

      <div className="shrink-0 border-t border-border px-3 py-2">
        <div className="flex items-center">
          <AccountMenu />
          <Button variant="ghost" size="sm" isIconOnly aria-label="Mobile app">
            <Icon icon="lucide:smartphone" className="h-4 w-4 text-muted" />
          </Button>
        </div>
      </div>
    </div>
  )
}

function AccountMenu() {
  const [open, setOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const router = useRouter()
  const { user, activeOrg, logout } = useAuth()

  const go = (path?: string) => {
    setOpen(false)
    if (path) router.push(path)
  }

  const handleLogout = async () => {
    setOpen(false)
    setLoggingOut(true)
    await logout()
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label="Account and settings"
        className="hover:bg-default flex flex-1 items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors"
      >
        <Icon icon="lucide:settings" className="h-4 w-4 shrink-0 text-muted" />
        <span className="min-w-0 flex-1 truncate">Settings</span>
      </Popover.Trigger>
      <Popover.Content className="w-64 rounded-2xl border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          <div className="flex flex-col gap-1 px-2.5 pt-1.5 pb-2">
            <div className="flex items-center gap-2 text-sm text-muted">
              <Icon icon="lucide:circle-user" className="h-4 w-4 shrink-0" />
              <span className="truncate">{user?.email ?? "Signed in"}</span>
            </div>
            <div className="flex items-center gap-2 text-sm text-muted">
              <Icon icon="lucide:settings" className="h-4 w-4 shrink-0" />
              <span className="truncate">{activeOrg?.name ?? "Workspace"}</span>
            </div>
          </div>
          <div className="mx-1 border-t border-border" />
          <AccountItem
            icon="lucide:circle-user"
            label="Profile"
            onPress={() => go("/w/settings")}
          />
          <AccountItem
            icon="lucide:settings"
            label="Settings"
            shortcut="⌘,"
            onPress={() => go("/w/settings")}
          />
          <div className="mx-1 border-t border-border" />
          <AccountItem
            icon="lucide:gauge"
            label="Usage remaining"
            chevron
            onPress={() => go()}
          />
          <AccountItem
            icon="lucide:mail"
            label="Invite a friend"
            onPress={() => go()}
          />
          <AccountItem
            icon="lucide:log-out"
            label={loggingOut ? "Logging out..." : "Log out"}
            disabled={loggingOut}
            onPress={handleLogout}
          />
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function AccountItem({
  icon,
  label,
  shortcut,
  chevron,
  disabled,
  onPress,
}: {
  icon: string
  label: string
  shortcut?: string
  chevron?: boolean
  disabled?: boolean
  onPress: () => void | Promise<void>
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onPress}
      className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors disabled:cursor-progress disabled:opacity-60"
    >
      <Icon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {shortcut ? <span className="text-xs text-muted">{shortcut}</span> : null}
      {chevron ? (
        <Icon icon="lucide:chevron-right" className="h-3.5 w-3.5 text-muted" />
      ) : null}
    </button>
  )
}

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

function ChannelGroup({
  channel,
  slugAmbiguous,
}: {
  channel: SidebarChannelResponse
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
  const [expanded, setExpanded] = useState(false)
  const open = channelActive || expanded
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

  const warmSession = (session: SidebarSessionResponse) => {
    if (!session.id) return
    seedSessionDetail(queryClient, session)
    prefetchSessionRoute(queryClient, session.id)
  }

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
          onClick={() => setExpanded((open) => !open)}
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
                  return (
                    <ChatRow
                      key={id}
                      title={sessionDisplayName(session)}
                      meta={sessionActivityLabel(session)}
                      active={chatActive(id)}
                      onIntent={() => warmSession(session)}
                      onSelect={() => {
                        warmSession(session)
                        openChat(slug, id, chatSessionFromResponse(session))
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

function chatSessionFromResponse(session: SidebarSessionResponse): ChatSession {
  const agentID = session.agent_id?.trim() || DEFAULT_AGENT_ID
  const agent = safeAgentById(agentID)
  const modelId = session.model?.trim() || agent.defaultModelId
  return {
    title: sessionDisplayName(session),
    agentId: agentID,
    agentName: agent.name,
    agentIcon: agent.icon,
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

function ChannelSkeletonList() {
  return (
    <div className="flex flex-col gap-0.5">
      {Array.from({ length: 6 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-2.5 rounded-lg px-3 py-1.5"
        >
          <span className="bg-default h-4 w-4 shrink-0 rounded" />
          <span className="bg-default h-3.5 flex-1 rounded" />
          <span className="bg-default h-3.5 w-3.5 rounded" />
        </div>
      ))}
    </div>
  )
}

function SessionSkeletonList() {
  return (
    <div className="flex flex-col gap-0.5">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9"
        >
          <span className="bg-default h-3.5 flex-1 rounded" />
          <span className="bg-default h-3 w-8 rounded" />
        </div>
      ))}
    </div>
  )
}

function SidebarStatusRow({
  label,
  actionLabel,
  onAction,
}: {
  label: string
  actionLabel?: string
  onAction?: () => void
}) {
  return (
    <div className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-muted">
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

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <Typography.Paragraph
      size="xs"
      color="muted"
      className="px-3 pt-2 pb-1 select-none"
    >
      {children}
    </Typography.Paragraph>
  )
}

function NavRow({
  icon,
  label,
  className = "",
  onClick,
}: {
  icon: string
  label: string
  className?: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`hover:bg-default flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors ${className}`}
    >
      <Icon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </button>
  )
}

function ChatRow({
  title,
  meta,
  active,
  onIntent,
  onSelect,
}: {
  title: string
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
      <span className="min-w-0 flex-1 truncate">{title}</span>
      {meta ? (
        <span className="shrink-0 text-xs text-muted">{meta}</span>
      ) : null}
    </button>
  )
}
