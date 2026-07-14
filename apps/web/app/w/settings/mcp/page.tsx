"use client"

import { useMemo, useState } from "react"
import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { Button, Skeleton, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { useIsAdmin } from "@/lib/auth/use-role"
import { cn } from "@/lib/utils"
import {
  CreateServerForm,
  type CreateServerSubmission,
} from "./_create-server-form"
import { ServerRow, type AccessOption } from "./_server-row"
import {
  attachMcpServerToAgent,
  configureMcpAuthorization,
  createMcpServer,
  deleteMcpServer,
  detachMcpServerFromAgent,
  grantMcpServerToTeam,
  listAgentMcpServerIDs,
  listMcpServers,
  listTeamMcpServerIDs,
  revokeMcpServerFromTeam,
  startMcpOAuth,
  testMcpServer,
  type McpAuthorizationInput,
  type McpServer,
  type McpServerScope,
} from "./_lib/mcp-api"
import { serversForScope } from "./_lib/mcp-ui"

type AccessChange =
  | {
      kind: "team"
      server: McpServer
      resourceID: string
      enabled: boolean
    }
  | {
      kind: "agent"
      server: McpServer
      resourceID: string
      enabled: boolean
    }

export default function McpSettingsPage() {
  const queryClient = useQueryClient()
  const isAdmin = useIsAdmin()
  const [scope, setScope] = useState<McpServerScope>("personal")
  const [showCreate, setShowCreate] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<McpServer | null>(null)
  const [testedServerIDs, setTestedServerIDs] = useState<Set<string>>(
    () => new Set()
  )

  const serversQuery = useQuery({
    queryKey: queryKeys.mcpServers(),
    queryFn: ({ signal }) => listMcpServers(signal),
    retry: false,
  })
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const agentsQuery = $api.useQuery("get", "/v1/agents", {
    params: { query: { status: "active", limit: 200 } },
  })

  const teams = useMemo<AccessOption[]>(
    () =>
      (teamsQuery.data?.data ?? [])
        .filter((team) => Boolean(team.id))
        .map((team) => ({
          id: team.id!,
          name: team.name || "Untitled team",
          description: team.description || undefined,
        })),
    [teamsQuery.data?.data]
  )
  const teamNames = useMemo(
    () => new Map(teams.map((team) => [team.id, team.name])),
    [teams]
  )
  const agents = useMemo<AccessOption[]>(
    () =>
      (agentsQuery.data?.data ?? [])
        .filter((agent) => Boolean(agent.id))
        .map((agent) => ({
          id: agent.id!,
          name: agent.name || "Untitled agent",
          description: agent.team_id
            ? teamNames.get(agent.team_id) || "Team agent"
            : undefined,
        })),
    [agentsQuery.data?.data, teamNames]
  )

  const teamAccessQueries = useQueries({
    queries: teams.map((team) => ({
      queryKey: queryKeys.teamMcpServers(team.id),
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        listTeamMcpServerIDs(team.id, signal),
      enabled: isAdmin,
      retry: false,
    })),
  })
  const orgAgentAccessQueries = useQueries({
    queries: agents.map((agent) => ({
      queryKey: queryKeys.agentMcpServers(agent.id),
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        listAgentMcpServerIDs(agent.id, "org", signal),
      enabled: isAdmin,
      retry: false,
    })),
  })
  const personalAgentAccessQueries = useQueries({
    queries: agents.map((agent) => ({
      queryKey: queryKeys.agentPersonalMcpServers(agent.id),
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        listAgentMcpServerIDs(agent.id, "personal", signal),
      retry: false,
    })),
  })

  const effectiveServers = useMemo(() => {
    const rows = serversQuery.data ?? []
    return rows.map((server) => ({
      ...server,
      healthStatus: testedServerIDs.has(server.id)
        ? ("healthy" as const)
        : server.healthStatus,
      teamIds:
        server.scope === "org" && isAdmin
          ? teams
              .filter((_, index) =>
                teamAccessQueries[index]?.data?.includes(server.id)
              )
              .map((team) => team.id)
          : server.teamIds,
      agentIds: agents
        .filter((_, index) =>
          (server.scope === "personal"
            ? personalAgentAccessQueries[index]
            : orgAgentAccessQueries[index]
          )?.data?.includes(server.id)
        )
        .map((agent) => agent.id),
    }))
  }, [
    agents,
    isAdmin,
    orgAgentAccessQueries,
    personalAgentAccessQueries,
    serversQuery.data,
    teamAccessQueries,
    teams,
    testedServerIDs,
  ])

  const shownServers = serversForScope(effectiveServers, scope)
  const personalCount = serversForScope(effectiveServers, "personal").length
  const orgCount = serversForScope(effectiveServers, "org").length
  const accessLoading =
    teamsQuery.isLoading ||
    agentsQuery.isLoading ||
    (scope === "org" && isAdmin
      ? teamAccessQueries.some((query) => query.isLoading) ||
        orgAgentAccessQueries.some((query) => query.isLoading)
      : personalAgentAccessQueries.some((query) => query.isLoading))

  const createMutation = useMutation({
    mutationFn: async (submission: CreateServerSubmission) => {
      const server = await createMcpServer(submission.server)
      if (submission.startOAuth) {
        const authorizationURL = await startMcpOAuth(
          server.id,
          submission.oauthPrincipalType ?? "user",
          submission.oauthRegistration
        )
        window.location.assign(authorizationURL)
      }
      return server
    },
    onSuccess: (server, submission) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.mcpServers() })
      setScope(server.scope)
      setShowCreate(false)
      if (!submission.startOAuth) toast.success(`${server.name} added`)
    },
    onError: (error) =>
      toast.danger(error.message || "Could not add MCP server"),
    onSettled: () => {
      queueMicrotask(() => createMutation.reset())
    },
  })
  const testMutation = useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) => ({
      id,
      name,
      result: await testMcpServer(id),
    }),
    onSuccess: ({ id, name, result }) => {
      if (result.connected) {
        setTestedServerIDs((current) => new Set(current).add(id))
      }
      toast.success(`${name} is reachable`)
    },
    onError: (error) =>
      toast.danger(error.message || "Could not test MCP server"),
  })
  const oauthMutation = useMutation({
    mutationFn: ({
      id,
      principalType,
    }: {
      id: string
      principalType: "user" | "org_service"
    }) => startMcpOAuth(id, principalType),
    onSuccess: (authorizationURL) => window.location.assign(authorizationURL),
    onError: (error) =>
      toast.danger(error.message || "Could not start sign in"),
  })
  const authorizationMutation = useMutation({
    mutationFn: ({ id, input }: { id: string; input: McpAuthorizationInput }) =>
      configureMcpAuthorization(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.mcpServers() })
      toast.success("Credential saved")
    },
    onError: (error) =>
      toast.danger(error.message || "Could not save credential"),
    onSettled: () => {
      queueMicrotask(() => authorizationMutation.reset())
    },
  })
  const deleteMutation = useMutation({
    mutationFn: deleteMcpServer,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.mcpServers() })
      toast.success("MCP server removed")
      setRemoveTarget(null)
    },
    onError: (error) =>
      toast.danger(error.message || "Could not remove MCP server"),
  })
  const accessMutation = useMutation({
    mutationFn: async (change: AccessChange) => {
      if (change.kind === "team") {
        if (change.enabled) {
          await grantMcpServerToTeam(change.server.id, change.resourceID)
        } else {
          await revokeMcpServerFromTeam(change.server.id, change.resourceID)
        }
        return change
      }
      if (change.enabled) {
        await attachMcpServerToAgent(
          change.server.id,
          change.resourceID,
          change.server.scope
        )
      } else {
        await detachMcpServerFromAgent(
          change.server.id,
          change.resourceID,
          change.server.scope
        )
      }
      return change
    },
    onSuccess: (change) => {
      const key =
        change.kind === "team"
          ? queryKeys.teamMcpServers(change.resourceID)
          : change.server.scope === "personal"
            ? queryKeys.agentPersonalMcpServers(change.resourceID)
            : queryKeys.agentMcpServers(change.resourceID)
      void queryClient.invalidateQueries({ queryKey: key })
      toast.success(change.enabled ? "Access granted" : "Access removed")
    },
    onError: (error) =>
      toast.danger(error.message || "Could not update access"),
  })

  const anyMutationPending =
    testMutation.isPending ||
    oauthMutation.isPending ||
    authorizationMutation.isPending ||
    deleteMutation.isPending ||
    accessMutation.isPending
  const canAddCurrentScope = scope === "personal" || isAdmin

  async function submitCreate(submission: CreateServerSubmission) {
    try {
      await createMutation.mutateAsync(submission)
    } catch {
      // The mutation owns user-facing error reporting.
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-foreground">MCP servers</h1>
          <p className="text-muted-foreground mt-1 max-w-xl text-sm">
            Connect remote tools, then choose whether they belong to you or your
            organization. Agents discover their tools only when needed.
          </p>
        </div>
        {canAddCurrentScope && !showCreate ? (
          <Button
            variant="primary"
            size="sm"
            className="shrink-0"
            onPress={() => setShowCreate(true)}
          >
            <AppIcon icon="plus" className="h-4 w-4" />
            Add server
          </Button>
        ) : null}
      </header>

      {isAdmin ? (
        <div
          role="tablist"
          aria-label="MCP server ownership"
          className="flex items-center gap-1 border-b border-border"
        >
          <ScopeTab
            selected={scope === "personal"}
            label="Personal"
            count={personalCount}
            onSelect={() => {
              setScope("personal")
              setShowCreate(false)
            }}
          />
          <ScopeTab
            selected={scope === "org"}
            label="Organization"
            count={orgCount}
            onSelect={() => {
              setScope("org")
              setShowCreate(false)
            }}
          />
        </div>
      ) : null}

      {scope === "personal" ? (
        <p className="text-muted-foreground -mt-4 flex items-start gap-2 text-xs">
          <AppIcon icon="info" className="mt-px h-3.5 w-3.5 shrink-0" />
          Personal servers are available in Hivy chats and schedules you own.
          Slack, automations, and webhooks do not load them.
        </p>
      ) : null}

      {showCreate ? (
        <CreateServerForm
          key={scope}
          initialScope={scope}
          isAdmin={isAdmin}
          isPending={createMutation.isPending}
          onCancel={() => setShowCreate(false)}
          onCreate={submitCreate}
        />
      ) : null}

      {!showCreate ? (
        serversQuery.isLoading ? (
          <ServerListSkeleton />
        ) : serversQuery.isError ? (
          <ErrorState onRetry={() => serversQuery.refetch()} />
        ) : shownServers.length === 0 ? (
          <EmptyState
            scope={scope}
            isAdmin={isAdmin}
            onAdd={() => setShowCreate(true)}
          />
        ) : (
          <div className="flex flex-col gap-2">
            {shownServers.map((server) => (
              <ServerRow
                key={server.id}
                server={server}
                teams={teams}
                agents={agents}
                isAdmin={isAdmin}
                isBusy={anyMutationPending}
                accessLoading={accessLoading}
                onTest={() =>
                  testMutation.mutate({ id: server.id, name: server.name })
                }
                onOAuth={() =>
                  oauthMutation.mutate({
                    id: server.id,
                    principalType:
                      server.scope === "personal" ||
                      server.authorizationPolicy === "user_required" ||
                      server.authorizationPolicy === "prefer_user"
                        ? "user"
                        : "org_service",
                  })
                }
                onConfigureAuth={async (input) => {
                  await authorizationMutation.mutateAsync({
                    id: server.id,
                    input,
                  })
                }}
                onToggleTeam={(resourceID, enabled) =>
                  accessMutation.mutate({
                    kind: "team",
                    server,
                    resourceID,
                    enabled,
                  })
                }
                onToggleAgent={(resourceID, enabled) =>
                  accessMutation.mutate({
                    kind: "agent",
                    server,
                    resourceID,
                    enabled,
                  })
                }
                onRemove={() => setRemoveTarget(server)}
              />
            ))}
          </div>
        )
      ) : null}

      <ConfirmDialog
        open={removeTarget !== null}
        pending={deleteMutation.isPending}
        heading="Remove MCP server?"
        description={
          removeTarget
            ? `“${removeTarget.name}” will be removed along with its authorization and access grants. Agents will no longer be able to use its tools.`
            : ""
        }
        confirmLabel="Remove server"
        onOpenChange={(open) => {
          if (!open) setRemoveTarget(null)
        }}
        onConfirm={() => {
          if (removeTarget) deleteMutation.mutate(removeTarget.id)
        }}
      />
    </div>
  )
}

function ScopeTab({
  selected,
  label,
  count,
  onSelect,
}: {
  selected: boolean
  label: string
  count: number
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={selected}
      onClick={onSelect}
      className={cn(
        "focus-visible:outline-primary relative flex items-center gap-2 px-3 pt-1 pb-2.5 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2",
        selected
          ? "after:bg-primary font-medium text-foreground after:absolute after:inset-x-2 after:-bottom-px after:h-0.5 after:rounded-full"
          : "text-muted-foreground hover:text-foreground"
      )}
    >
      {label}
      <span className="text-muted-foreground rounded-md bg-default px-1.5 py-0.5 text-[11px]">
        {count}
      </span>
    </button>
  )
}

function ServerListSkeleton() {
  return (
    <div className="flex flex-col gap-2" aria-label="Loading MCP servers">
      {[0, 1, 2].map((index) => (
        <div
          key={index}
          className="flex items-start gap-3.5 rounded-2xl border border-border bg-surface px-4 py-4"
        >
          <Skeleton className="h-9 w-9 shrink-0 rounded-lg" />
          <div className="flex flex-1 flex-col gap-2">
            <Skeleton className="h-4 w-40 rounded" />
            <Skeleton className="h-3 w-56 rounded" />
            <Skeleton className="h-3 w-72 rounded" />
          </div>
        </div>
      ))}
    </div>
  )
}

function ErrorState({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-2xl px-6 text-center">
      <AppIcon icon="circle-alert" className="h-7 w-7 text-danger" />
      <p className="mt-3 text-sm font-medium text-foreground">
        Couldn’t load MCP servers
      </p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        Check your connection and try again.
      </p>
      <Button variant="tertiary" size="sm" className="mt-4" onPress={onRetry}>
        Try again
      </Button>
    </div>
  )
}

function EmptyState({
  scope,
  isAdmin,
  onAdd,
}: {
  scope: McpServerScope
  isAdmin: boolean
  onAdd: () => void
}) {
  const canAdd = scope === "personal" || isAdmin
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-2xl px-6 text-center">
      <AppIcon icon="plug-zap" className="text-muted-foreground h-7 w-7" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {scope === "personal"
          ? "No personal MCP servers yet"
          : "No organization MCP servers yet"}
      </p>
      <p className="text-muted-foreground mt-1 max-w-md text-sm">
        {scope === "personal"
          ? "Add a server that only you can attach to agents and use in your chats or schedules."
          : isAdmin
            ? "Add a shared server, then grant it to teams or individual agents."
            : "A workspace admin can add servers and grant them to teams or agents."}
      </p>
      {canAdd ? (
        <Button variant="tertiary" size="sm" className="mt-4" onPress={onAdd}>
          <AppIcon icon="plus" className="h-4 w-4" />
          Add server
        </Button>
      ) : null}
    </div>
  )
}
