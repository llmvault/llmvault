"use client"

import { useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueries } from "@tanstack/react-query"
import { Button, Popover, Typography } from "@heroui/react"
import { Icon } from "@iconify/react"
import { api } from "@/lib/api/client"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"
import { ChannelGroup } from "@/app/w/(chat)/_components/sidebar-channel-group"
import {
  CHAT_QUERY_STALE_TIME_MS,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  channelRouteSlug,
  channelRouteSlugCounts,
  sortChannelsByRecentSession,
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
  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    { params: { query: { status: "active", limit: 100 } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )

  const channels = useMemo(
    () => channelsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? [],
    [channelsQuery.data?.pages]
  )
  const latestSessionQueries = useQueries({
    queries: channels.map((channel) => {
      const channelID = channel.id ?? ""
      return {
        queryKey: ["sidebar-channel-latest-session", channelID, 1] as const,
        enabled: Boolean(channelID),
        retry: false,
        staleTime: CHAT_QUERY_STALE_TIME_MS,
        queryFn: async () => {
          const { data, error } = await api.GET("/v1/channels/{id}/sessions", {
            params: {
              path: { id: channelID },
              query: { limit: 1 },
            },
          })
          if (error) throw new Error("Could not load latest channel session")
          return data?.data?.[0] ?? null
        },
      }
    }),
  })
  const latestSessionsByChannelID = useMemo(() => {
    const out = new Map<string, SidebarSessionResponse | null>()
    channels.forEach((channel, index) => {
      if (!channel.id) return
      const result = latestSessionQueries[index]
      if (result?.data !== undefined) {
        out.set(channel.id, result.data ?? null)
      }
    })
    return out
  }, [channels, latestSessionQueries])
  const sortedChannels = useMemo(
    () => sortChannelsByRecentSession(channels, latestSessionsByChannelID),
    [channels, latestSessionsByChannelID]
  )
  const agentsByID = useMemo(
    () =>
      new Map(
        (agentsQuery.data?.data ?? []).flatMap((agent) =>
          agent.id ? ([[agent.id, agent]] as const) : []
        )
      ),
    [agentsQuery.data?.data]
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
          ) : sortedChannels.length ? (
            sortedChannels.map((channel, index) => (
              <ChannelGroup
                key={channel.id ?? channel.name ?? index}
                channel={channel}
                agentsByID={agentsByID}
                autoExpanded={index < 4}
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
