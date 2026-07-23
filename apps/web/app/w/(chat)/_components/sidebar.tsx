"use client"

import { memo, useEffect, useMemo, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { AnimatePresence, motion } from "motion/react"
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
import { useIsAdmin } from "@/lib/auth/use-role"
import { WorkspaceSwitcher } from "./sidebar-workspace-switcher"
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
  const isAdmin = useIsAdmin()
  const [settingsOpen, setSettingsOpen] = useState(false)
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
  const teamsActive =
    pathname === "/w/teams" || pathname.startsWith("/w/teams/")
  const skillsActive =
    pathname === "/w/skills" || pathname.startsWith("/w/skills/")
  const knowledgeActive =
    pathname === "/w/knowledge" || pathname.startsWith("/w/knowledge/")
  const generalActive =
    pathname === "/w/general" || pathname.startsWith("/w/general/")
  const billingActive =
    pathname === "/w/billing" || pathname.startsWith("/w/billing/")

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
        <div className="ml-auto">
          <ThemeModeToggle />
        </div>
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
          <button
            type="button"
            aria-expanded={settingsOpen}
            aria-controls="sidebar-settings-links"
            onClick={() => setSettingsOpen((open) => !open)}
            className="mt-1 flex w-full items-center gap-2 px-3 pt-2 pb-1 text-left text-xs text-muted uppercase select-none"
          >
            <span className="min-w-0 flex-1 truncate">Settings</span>
            <AppIcon
              icon="chevron-right"
              className={`h-3.5 w-3.5 shrink-0 transition-transform duration-150 ease-out ${
                settingsOpen ? "rotate-90" : ""
              }`}
            />
          </button>
          <AnimatePresence initial={false}>
            {settingsOpen ? (
              <motion.div
                id="sidebar-settings-links"
                initial={{ opacity: 0, y: -4 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -4 }}
                transition={{
                  duration: 0.16,
                  ease: [0.16, 1, 0.3, 1],
                }}
                className="flex flex-col gap-0.5"
              >
                <NavRow
                  icon="users"
                  label="Teams"
                  active={teamsActive}
                  onClick={() => router.push("/w/teams")}
                />
                <NavRow
                  icon="sparkles"
                  label="Skills"
                  active={skillsActive}
                  onClick={() => router.push("/w/skills")}
                />
                {isAdmin ? (
                  <>
                    <NavRow
                      icon="folder-open"
                      label="Knowledge"
                      active={knowledgeActive}
                      onClick={() => router.push("/w/knowledge")}
                    />
                    <NavRow
                      icon="settings"
                      label="General"
                      active={generalActive}
                      onClick={() => router.push("/w/general")}
                    />
                    <NavRow
                      icon="gauge"
                      label="Usage & billing"
                      active={billingActive}
                      onClick={() => router.push("/w/billing")}
                    />
                  </>
                ) : null}
              </motion.div>
            ) : null}
          </AnimatePresence>
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

      <div className="shrink-0 px-3 pb-3">
        <WorkspaceSwitcher />
      </div>
    </div>
  )
})
