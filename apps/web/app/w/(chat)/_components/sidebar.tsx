"use client"

import { memo, useEffect, useMemo } from "react"
import { usePathname, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"
import { SidebarTeamSessionGroup } from "@/app/w/(chat)/_components/sidebar-team-session-group"
import { CHAT_QUERY_STALE_TIME_MS } from "@/app/w/(chat)/_lib/chat-cache"
import {
  buildSidebarTeamGroups,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { ThemeModeToggle } from "@/components/theme-mode-toggle"
import { AccountMenu } from "./sidebar-account-menu"
import { TeamSkeletonList, SidebarStatusRow } from "./sidebar-team-state"
import { NavRow } from "./sidebar-nav"
import { hydrateSessionListRuntime } from "@/app/w/(chat)/_stores/session-stream-manager"

const SIDEBAR_TEAM_PAGE_LIMIT = 100

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
  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    { params: { query: { status: "active", limit: 100 } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
    { params: { query: { limit: SIDEBAR_TEAM_PAGE_LIMIT } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )

  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
  )
  const sessionsQuery = $api.useQuery(
    "get",
    "/v1/sessions",
    { params: { query: { limit: 100 } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const sessions = useMemo(
    () => sessionsQuery.data?.data ?? [],
    [sessionsQuery.data?.data]
  )
  const teamGroups = useMemo(
    () => buildSidebarTeamGroups(teams, sessions),
    [teams, sessions]
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
  useEffect(() => {
    hydrateSessionListRuntime(sessions as SidebarSessionResponse[], queryClient)
  }, [sessions, queryClient])

  const agentsActive =
    pathname === "/w/agents" || pathname.startsWith("/w/agents/")
  const connectionsActive =
    pathname === "/w/connections" || pathname.startsWith("/w/connections/")
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
          <NavRow icon="square-pen" label="New chat" onClick={startNewChat} />
          <NavRow
            icon="bot"
            label="Agents"
            active={agentsActive}
            onClick={() => router.push("/w/agents")}
          />
          <NavRow
            icon="toy-brick"
            label="Connections"
            active={connectionsActive}
            onClick={() => router.push("/w/connections")}
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
          {teamsQuery.isLoading || sessionsQuery.isLoading ? (
            <TeamSkeletonList />
          ) : teamsQuery.isError || sessionsQuery.isError ? (
            <SidebarStatusRow
              label="Could not load chats"
              actionLabel="Retry"
              onAction={() => {
                void teamsQuery.refetch()
                void sessionsQuery.refetch()
              }}
            />
          ) : !teamGroups.length ? (
            <SidebarStatusRow label="No teams" />
          ) : (
            teamGroups.map((group) => (
              <SidebarTeamSessionGroup
                key={group.key}
                name={group.name}
                sessions={group.sessions}
                agentsByID={agentsByID}
                onRenameSession={onRenameSession}
                onShareSession={onShareSession}
                onArchiveSession={onArchiveSession}
              />
            ))
          )}
        </div>
      </div>

      <div className="shrink-0 border-t border-border px-3 py-2">
        <div className="flex items-center gap-1">
          <AccountMenu />
          <ThemeModeToggle />
        </div>
      </div>
    </div>
  )
})
