"use client"

import { memo, useEffect, useMemo, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"
import { ChannelGroup } from "@/app/w/(chat)/_components/sidebar-channel-group"
import { SidebarTeamGroup } from "@/app/w/(chat)/_components/sidebar-team-group"
import {
  CHAT_QUERY_STALE_TIME_MS,
  SIDEBAR_SESSION_PAGE_LIMIT,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  channelRouteSlug,
  channelRouteSlugCounts,
  groupChannelsByTeam,
  sortChannelsByRecentSession,
  type SidebarChannelResponse,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { AccountMenu } from "./sidebar-account-menu"
import { ChannelCreateModal } from "./channel-create-modal"
import { ChannelSkeletonList, SidebarStatusRow } from "./sidebar-channel-state"
import { NavRow, SectionLabel } from "./sidebar-nav"
import { hydrateSessionListRuntime } from "@/app/w/(chat)/_stores/session-stream-manager"

const SIDEBAR_CHANNEL_PAGE_LIMIT = 100
const CHANNELS_INFINITE_KEY = "channels-infinite-v1"

export const Sidebar = memo(function Sidebar({
  onCollapse,
  onRenameSession,
  onShareSession,
  onArchiveSession,
}: {
  onCollapse: () => void
  onRenameSession: (sessionId: string, name: string) => void
  onShareSession: (sessionId: string) => void
  onArchiveSession: (sessionId: string) => void
}) {
  const { startNewChat } = useWorkspace()
  const router = useRouter()
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const [createChannelOpen, setCreateChannelOpen] = useState(false)
  const channelsQuery = $api.useInfiniteQuery(
    "get",
    "/v1/channels",
    {
      _hivyQueryKey: CHANNELS_INFINITE_KEY,
      params: {
        query: {
          limit: SIDEBAR_CHANNEL_PAGE_LIMIT,
          include: "recent_sessions",
          recent_sessions_limit: SIDEBAR_SESSION_PAGE_LIMIT,
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
  // Team names for the group headers. The channel payload only carries
  // team_id, so we enrich from the teams endpoint when it is accessible
  // (currently admin-only). When it is not, grouping falls back to a
  // team-id label and the UI still renders one group per team.
  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
    { params: { query: { limit: 100 } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )

  const channels = useMemo(
    () => channelsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? [],
    [channelsQuery.data?.pages]
  )
  const latestSessionsByChannelID = useMemo(() => {
    const out = new Map<string, SidebarSessionResponse | null>()
    channels.forEach((channel) => {
      if (!channel.id) return
      out.set(channel.id, channel.recent_sessions?.[0] ?? null)
    })
    return out
  }, [channels])
  const sortedChannels = useMemo(
    () => sortChannelsByRecentSession(channels, latestSessionsByChannelID),
    [channels, latestSessionsByChannelID]
  )
  const teamNamesByID = useMemo(() => {
    const out = new Map<string, string>()
    for (const team of teamsQuery.data?.data ?? []) {
      if (team.id && team.name?.trim()) out.set(team.id, team.name.trim())
    }
    return out
  }, [teamsQuery.data?.data])
  const teamGroups = useMemo(
    () => groupChannelsByTeam(sortedChannels, teamNamesByID),
    [sortedChannels, teamNamesByID]
  )
  // Global position of each channel in the sorted list drives the
  // auto-expand-first-few behavior, preserved across team groups.
  const channelOrder = useMemo(() => {
    const out = new Map<SidebarChannelResponse, number>()
    sortedChannels.forEach((channel, index) => out.set(channel, index))
    return out
  }, [sortedChannels])
  const hasTeamGroups = useMemo(
    () => teamGroups.some((group) => group.teamId),
    [teamGroups]
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
  const latestSessions = useMemo(
    () =>
      channels
        .flatMap((channel) => channel.recent_sessions ?? [])
        .filter((session): session is SidebarSessionResponse =>
          Boolean(session)
        ),
    [channels]
  )

  useEffect(() => {
    hydrateSessionListRuntime(latestSessions, queryClient)
  }, [latestSessions, queryClient])

  function openChannelSettings(channel: SidebarChannelResponse) {
    if (channel.id) router.push(`/w/settings/channels/${channel.id}`)
  }

  function openCreatedChannel(channel: SidebarChannelResponse) {
    router.push(`/w/channels/${channelRouteSlug(channel)}`)
  }

  function renderChannel(channel: SidebarChannelResponse, order: number) {
    return (
      <ChannelGroup
        key={channel.id ?? channel.name ?? order}
        channel={channel}
        agentsByID={agentsByID}
        autoExpanded={order < 4}
        onRenameChannel={openChannelSettings}
        onRenameSession={onRenameSession}
        onShareSession={onShareSession}
        onArchiveSession={onArchiveSession}
        onShowChannelDetails={openChannelSettings}
        slugAmbiguous={
          (channelSlugCounts.get(channelRouteSlug(channel)) ?? 0) > 1
        }
      />
    )
  }

  const pluginsActive =
    pathname === "/w/plugins" || pathname.startsWith("/w/plugins/")
  const automationsActive =
    pathname === "/w/automations" || pathname.startsWith("/w/automations/")
  const sheetsActive =
    pathname === "/w/sheets" || pathname.startsWith("/w/sheets/")
  const appsActive = pathname === "/w/apps" || pathname.startsWith("/w/apps/")

  return (
    <div className="flex h-full flex-col bg-surface">
      <div className="flex h-12 shrink-0 items-center gap-1 px-3">
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Collapse sidebar"
          onPress={onCollapse}
        >
          <AppIcon icon="panel-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Back"
          onPress={() => router.back()}
        >
          <AppIcon icon="arrow-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Forward"
          onPress={() => router.forward()}
        >
          <AppIcon icon="arrow-right" className="h-4 w-4 text-muted" />
        </Button>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-3 pb-4">
        <div className="flex flex-col gap-0.5">
          <NavRow
            icon="square-pen"
            label="New chat"
            onClick={startNewChat}
          />
          <NavRow
            icon="toy-brick"
            label="Plugins"
            active={pluginsActive}
            onClick={() => router.push("/w/plugins")}
          />
          <NavRow
            icon="clock"
            label="Automations"
            active={automationsActive}
            onClick={() => router.push("/w/automations")}
          />
          <NavRow
            icon="table"
            label="Sheets"
            active={sheetsActive}
            onClick={() => router.push("/w/sheets")}
          />
          <NavRow
            icon="layout-grid"
            label="Apps"
            active={appsActive}
            onClick={() => router.push("/w/apps")}
          />
        </div>

        <div className="flex flex-col gap-0.5">
          <div className="flex items-center justify-between gap-2">
            <SectionLabel>CHANNELS</SectionLabel>
            <Button
              variant="ghost"
              size="sm"
              isIconOnly
              aria-label="Create channel"
              onPress={() => setCreateChannelOpen(true)}
            >
              <AppIcon icon="plus" className="h-4 w-4 text-muted" />
            </Button>
          </div>
          {channelsQuery.isLoading ? (
            <ChannelSkeletonList />
          ) : channelsQuery.isError ? (
            <SidebarStatusRow
              label="Could not load channels"
              actionLabel="Retry"
              onAction={() => void channelsQuery.refetch()}
            />
          ) : !sortedChannels.length ? (
            <SidebarStatusRow label="No channels" />
          ) : hasTeamGroups ? (
            teamGroups.map((group) => (
              <SidebarTeamGroup key={group.key} name={group.name}>
                {group.channels.map((channel) =>
                  renderChannel(channel, channelOrder.get(channel) ?? 0)
                )}
              </SidebarTeamGroup>
            ))
          ) : (
            sortedChannels.map((channel, index) =>
              renderChannel(channel, index)
            )
          )}
          {channelsQuery.hasNextPage ? (
            <button
              type="button"
              disabled={channelsQuery.isFetchingNextPage}
              onClick={() => void channelsQuery.fetchNextPage()}
              className="rounded-lg px-3 py-1.5 text-left text-sm text-muted transition-colors hover:bg-default"
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
        </div>
      </div>
      <ChannelCreateModal
        open={createChannelOpen}
        onOpenChange={setCreateChannelOpen}
        onCreated={openCreatedChannel}
      />
    </div>
  )
})
