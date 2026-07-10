"use client"

import { use, useCallback, useMemo } from "react"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { Skeleton, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { PLUGINS_QUERY_KEY } from "@/app/w/(chat)/plugins/_lib"
import { useIsAdmin } from "@/lib/auth/use-role"
import { AgentAvatar } from "../_agent-avatar"
import { availableModelIds } from "@/app/w/(chat)/_lib/model-options"
import {
  AGENT_CATALOG_QUERY_KEY,
  INSTALLED_AGENTS_QUERY_KEY,
  agentDescription,
  agentMissingPlugins,
  agentName,
  agentRequiredPlugins,
  normalizeAgentSandboxImage,
  normalizeAgentSandboxSize,
  pluginsBySlug,
  type AgentSandboxImage,
  type AgentSandboxSize,
  type CatalogAgent,
  type InstalledAgent,
  type Team,
} from "../_lib"
import {
  AgentPluginsSection,
  NoRequirementsSection,
  RequiredPluginsSection,
} from "./_agent-plugin-sections"
import {
  AgentSettingsSection,
  SandboxImageSection,
  SandboxSizeSection,
} from "./_agent-settings-section"
import { AgentTeamsSection } from "./_agent-teams-section"

const EMPTY_TEAMS: Team[] = []

export default function AgentDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = use(params)
  const queryClient = useQueryClient()
  const isAdmin = useIsAdmin()
  // Installing/uninstalling a catalog agent is now team-scoped and member
  // reachable: each of the caller's teams gets its own clone, so the per-team
  // controls live in AgentTeamsSection. This page still lets anyone view and
  // configure an installed clone (model, sandbox, plugins).
  const agentQuery = $api.useQuery("get", "/v1/agents/catalog/{slug}", {
    params: { path: { slug } },
  })
  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const agentModelsQuery = $api.useQuery("get", "/v1/agents/models")
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const updateAgentModel = $api.useMutation("patch", "/v1/agents/{id}/model")
  const updateAgent = $api.useMutation("patch", "/v1/agents/{id}")
  const agent = agentQuery.data as CatalogAgent | undefined
  const installedAgentID = agent?.installed_agent_id ?? ""
  const installedAgentQuery = $api.useQuery(
    "get",
    "/v1/agents/{id}",
    {
      params: { path: { id: installedAgentID } },
    },
    { enabled: installedAgentID.length > 0 }
  )
  const installedAgent = installedAgentQuery.data as InstalledAgent | undefined
  const installedAgentTeamID = installedAgent?.team_id ?? ""
  const teamPluginsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/plugins",
    { params: { path: { teamID: installedAgentTeamID } } },
    { enabled: Boolean(installedAgentTeamID), retry: false }
  )
  const plugins = useMemo(() => pluginsQuery.data ?? [], [pluginsQuery.data])
  const pluginLookup = useMemo(() => pluginsBySlug(plugins), [plugins])
  const requiredPlugins = agentRequiredPlugins(agent)
  const missingPlugins = agentMissingPlugins(agent)
  const availableModels = useMemo(
    () => availableModelIds(agentModelsQuery.data ?? []),
    [agentModelsQuery.data]
  )
  const selectedModel =
    installedAgent?.model || agent?.model || availableModels[0] || ""
  const selectedSandboxImage = normalizeAgentSandboxImage(
    installedAgent?.sandbox_image ?? agent?.sandbox_image
  )
  const selectedSandboxSize = normalizeAgentSandboxSize(
    installedAgent?.sandbox_size
  )
  const teams = teamsQuery.data?.data ?? EMPTY_TEAMS
  const teamPluginIDs = useMemo(
    () =>
      (teamPluginsQuery.data?.data ?? [])
        .map((plugin) => plugin.id)
        .filter((id): id is string => Boolean(id)),
    [teamPluginsQuery.data?.data]
  )
  const disabledPluginIDs = installedAgent?.disabled_plugin_ids ?? []
  const requiredPluginSlugs = useMemo(
    () =>
      requiredPlugins
        .map((plugin) => plugin.slug)
        .filter((slug): slug is string => Boolean(slug)),
    [requiredPlugins]
  )
  const isDefaultAgent = Boolean(agent?.is_default)
  const hasInstalledClone = installedAgentID.length > 0
  const modelBusy = installedAgentQuery.isLoading || updateAgentModel.isPending
  const sandboxConfigBusy =
    installedAgentQuery.isLoading || updateAgent.isPending
  const canManageAgentPlugins =
    isAdmin || teams.some((team) => team.id === installedAgentTeamID)

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: AGENT_CATALOG_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY })
    queryClient.invalidateQueries({
      queryKey: queryKeys.agentCatalog(),
    })
    if (installedAgentID) {
      queryClient.invalidateQueries({ queryKey: queryKeys.agent() })
    }
  }, [installedAgentID, queryClient])

  function handleModelChange(model: string) {
    if (!installedAgentID || !model || model === selectedModel) return
    updateAgentModel.mutate(
      { params: { path: { id: installedAgentID } }, body: { model } },
      {
        onSuccess: () => {
          toast.success("Default model updated")
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update default model")
          ),
      }
    )
  }

  function handleSandboxSizeChange(size: AgentSandboxSize) {
    if (!installedAgentID || size === selectedSandboxSize) return
    updateAgent.mutate(
      {
        params: { path: { id: installedAgentID } },
        body: { sandbox_size: size },
      },
      {
        onSuccess: () => {
          toast.success("Sandbox size updated")
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update sandbox size")
          ),
      }
    )
  }

  function handleSandboxImageChange(image: AgentSandboxImage) {
    if (!installedAgentID || image === selectedSandboxImage) return
    updateAgent.mutate(
      {
        params: { path: { id: installedAgentID } },
        body: { sandbox_image: image },
      },
      {
        onSuccess: () => {
          toast.success("Image template updated")
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update image template")
          ),
      }
    )
  }

  function handleDisabledPluginIDsChange(pluginIDs: string[]) {
    if (!installedAgentID) return
    updateAgent.mutate(
      {
        params: { path: { id: installedAgentID } },
        body: { disabled_plugin_ids: pluginIDs },
      },
      {
        onSuccess: () => {
          toast.success("Agent plugins updated")
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update agent plugins")
          ),
      }
    )
  }

  if (agentQuery.isLoading) {
    return <DetailSkeleton />
  }

  if (!agent) {
    return <NotFoundState />
  }

  return (
    <div className="flex flex-col gap-8">
      <NextLink
        href="/w/agents"
        className="text-muted-foreground flex w-fit items-center gap-2 text-sm transition-colors hover:text-foreground"
      >
        <AppIcon icon="arrow-left" className="h-4 w-4" />
        Agents
      </NextLink>

      <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <AgentAvatar agent={agent} size="lg" />
          <div className="min-w-0">
            <h1 className="text-lg font-semibold text-foreground">
              {agentName(agent)}
            </h1>
            <p className="text-muted-foreground mt-1 max-w-xl text-sm leading-5">
              {agentDescription(agent)}
            </p>
          </div>
        </div>
      </header>

      {missingPlugins.length > 0 ? (
        <MissingPluginsWarning count={missingPlugins.length} />
      ) : null}

      {isDefaultAgent ? null : (
        <AgentTeamsSection
          slug={slug}
          agent={agent}
          teams={teams}
          isLoading={teamsQuery.isLoading}
          onChanged={refresh}
        />
      )}

      {hasInstalledClone ? (
        <div className="flex flex-col gap-6">
          <AgentSettingsSection
            availableModels={availableModels}
            selectedModel={selectedModel}
            isBusy={modelBusy}
            onModelChange={handleModelChange}
          />
          <AgentPluginsSection
            agentID={installedAgentID}
            plugins={plugins}
            teamId={installedAgentTeamID}
            teamPluginIDs={teamPluginIDs}
            disabledPluginIDs={disabledPluginIDs}
            requiredPluginSlugs={requiredPluginSlugs}
            canManage={canManageAgentPlugins}
            isSaving={updateAgent.isPending}
            isLoading={
              installedAgentQuery.isLoading ||
              pluginsQuery.isLoading ||
              teamPluginsQuery.isLoading
            }
            onDisabledPluginIDsChange={handleDisabledPluginIDsChange}
          />
          <SandboxImageSection
            selectedSandboxImage={selectedSandboxImage}
            isBusy={sandboxConfigBusy}
            onSandboxImageChange={handleSandboxImageChange}
          />
          <SandboxSizeSection
            selectedSandboxSize={selectedSandboxSize}
            isBusy={sandboxConfigBusy}
            onSandboxSizeChange={handleSandboxSizeChange}
          />
        </div>
      ) : null}

      {requiredPlugins.length > 0 ? (
        <RequiredPluginsSection
          plugins={requiredPlugins}
          pluginLookup={pluginLookup}
        />
      ) : (
        <NoRequirementsSection />
      )}
    </div>
  )
}

function MissingPluginsWarning({ count }: { count: number }) {
  return (
    <div className="flex gap-3 rounded-xl border border-warning/40 bg-warning/10 p-4">
      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-warning/15 text-warning">
        <AppIcon icon="triangle-alert" className="h-5 w-5" />
      </div>
      <div className="min-w-0">
        <h2 className="text-sm font-medium text-foreground">
          Required plugins missing
        </h2>
        <p className="text-muted-foreground mt-1 text-sm leading-5">
          Install {count === 1 ? "this plugin" : "these plugins"} before
          installing this agent.
        </p>
      </div>
    </div>
  )
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <Skeleton className="h-4 w-20 rounded" />
      <header className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <Skeleton className="h-12 w-12" />
          <div className="flex flex-col gap-3">
            <Skeleton className="h-5 w-36 rounded" />
            <Skeleton className="h-4 w-80 max-w-full rounded" />
          </div>
        </div>
        <Skeleton className="h-8 w-20 rounded-full" />
      </header>
      <Skeleton className="h-24" />
      <Skeleton className="h-44" />
    </div>
  )
}

function NotFoundState() {
  return (
    <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
      <AppIcon icon="bot" className="h-7 w-7 text-muted" />
      <p className="mt-3 text-sm font-medium text-foreground">
        Agent not found
      </p>
      <p className="mt-1 text-sm text-muted">
        This agent may have been removed from the catalog.
      </p>
      <NextLink
        href="/w/agents"
        className="text-muted-foreground mt-4 text-sm transition-colors hover:text-foreground"
      >
        Back to agents
      </NextLink>
    </div>
  )
}
