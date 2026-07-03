"use client"

import { use, useCallback, useMemo } from "react"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  PLUGINS_QUERY_KEY,
  pluginName,
  pluginSlug,
  type ApiPlugin,
} from "@/app/w/(chat)/plugins/_lib"
import { AgentAvatar } from "../_agent-avatar"
import { availableModelIds } from "@/app/w/(chat)/_lib/model-options"
import {
  AGENT_CATALOG_QUERY_KEY,
  INSTALLED_AGENTS_QUERY_KEY,
  agentCanInstall,
  agentDescription,
  agentIsInstalled,
  agentMissingPlugins,
  agentName,
  agentRequiredPlugins,
  agentRequiredPluginSlugs,
  normalizeAgentSandboxImage,
  normalizeAgentSandboxSize,
  pluginsBySlug,
  type AgentSandboxImage,
  type AgentSandboxSize,
  type CatalogAgent,
  type InstalledAgent,
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

export default function AgentDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = use(params)
  const queryClient = useQueryClient()
  const agentQuery = $api.useQuery("get", "/v1/agents/catalog/{slug}", {
    params: { path: { slug } },
  })
  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const agentModelsQuery = $api.useQuery("get", "/v1/agents/models")
  const installAgent = $api.useMutation(
    "post",
    "/v1/agents/catalog/{slug}/install"
  )
  const updateAgentModel = $api.useMutation("patch", "/v1/agents/{id}/model")
  const updateAgent = $api.useMutation("patch", "/v1/agents/{id}")
  const enableAgentPlugin = $api.useMutation(
    "post",
    "/v1/agents/{id}/plugins/{slug}"
  )
  const disableAgentPlugin = $api.useMutation(
    "delete",
    "/v1/agents/{id}/plugins/{slug}"
  )
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
  const agentPluginsQuery = $api.useQuery(
    "get",
    "/v1/agents/{id}/plugins",
    {
      params: { path: { id: installedAgentID } },
    },
    { enabled: installedAgentID.length > 0 }
  )
  const installedAgent = installedAgentQuery.data as InstalledAgent | undefined
  const plugins = useMemo(
    () => (pluginsQuery.data ?? []) as ApiPlugin[],
    [pluginsQuery.data]
  )
  const agentPlugins = useMemo(
    () => (agentPluginsQuery.data ?? []) as ApiPlugin[],
    [agentPluginsQuery.data]
  )
  const pluginLookup = useMemo(() => pluginsBySlug(plugins), [plugins])
  const requiredPlugins = agentRequiredPlugins(agent)
  const requiredPluginSlugs = useMemo(
    () => agentRequiredPluginSlugs(agent),
    [agent]
  )
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
  const installed = agent ? agentIsInstalled(agent) : false
  const canInstall = agentCanInstall(agent)
  const busy = installAgent.isPending
  const modelBusy = installedAgentQuery.isLoading || updateAgentModel.isPending
  const sandboxConfigBusy =
    installedAgentQuery.isLoading || updateAgent.isPending
  const agentPluginBusy =
    enableAgentPlugin.isPending || disableAgentPlugin.isPending

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: AGENT_CATALOG_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY })
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/agents/catalog/{slug}"],
    })
    if (installedAgentID) {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents/{id}"] })
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/agents/{id}/plugins"],
      })
    }
  }, [installedAgentID, queryClient])

  function handleInstall() {
    if (!agent || !canInstall) return
    installAgent.mutate(
      { params: { path: { slug } } },
      {
        onSuccess: () => {
          toast.success(`${agentName(agent)} agent installed`)
          refresh()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not install agent")),
      }
    )
  }

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

  function handleAgentPluginToggle(plugin: ApiPlugin, selected: boolean) {
    const pluginPathSlug = pluginSlug(plugin)
    if (
      !installedAgentID ||
      !pluginPathSlug ||
      requiredPluginSlugs.has(pluginPathSlug)
    ) {
      return
    }
    const mutation = selected ? enableAgentPlugin : disableAgentPlugin
    mutation.mutate(
      { params: { path: { id: installedAgentID, slug: pluginPathSlug } } },
      {
        onSuccess: () => {
          toast.success(
            `${pluginName(plugin)} ${
              selected
                ? "installed for this agent"
                : "uninstalled from this agent"
            }`
          )
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update plugin access")
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
        href="/w/settings/agents"
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

        <Button
          size="sm"
          variant="primary"
          className="shrink-0"
          isDisabled={busy || installed || !canInstall}
          onPress={handleInstall}
        >
          {busy ? <Spinner color="current" size="sm" /> : null}
          {installed ? "Installed" : "Install"}
        </Button>
      </header>

      {missingPlugins.length > 0 ? (
        <MissingPluginsWarning count={missingPlugins.length} />
      ) : null}

      {installed ? (
        <div className="flex flex-col gap-6">
          <AgentSettingsSection
            availableModels={availableModels}
            selectedModel={selectedModel}
            isBusy={modelBusy}
            onModelChange={handleModelChange}
          />
          <AgentPluginsSection
            agentID={installedAgentID}
            agentName={agentName(agent)}
            plugins={agentPlugins}
            requiredSlugs={requiredPluginSlugs}
            isLoading={agentPluginsQuery.isLoading}
            isBusy={agentPluginBusy}
            onToggle={handleAgentPluginToggle}
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
      <div className="h-4 w-20 animate-pulse rounded bg-default" />
      <header className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="h-12 w-12 animate-pulse rounded-xl bg-default" />
          <div className="flex flex-col gap-3">
            <div className="h-5 w-36 animate-pulse rounded bg-default" />
            <div className="h-4 w-80 max-w-full animate-pulse rounded bg-default" />
          </div>
        </div>
        <div className="h-8 w-20 animate-pulse rounded-full bg-default" />
      </header>
      <div className="h-24 animate-pulse rounded-xl bg-default" />
      <div className="h-44 animate-pulse rounded-xl bg-default" />
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
        href="/w/settings/agents"
        className="text-muted-foreground mt-4 text-sm transition-colors hover:text-foreground"
      >
        Back to agents
      </NextLink>
    </div>
  )
}
