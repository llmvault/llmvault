"use client"

import { use } from "react"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { AgentAvatar } from "../_agent-avatar"
import {
  AGENT_CATALOG_QUERY_KEY,
  INSTALLED_AGENTS_QUERY_KEY,
  agentCanInstall,
  agentDescription,
  agentIsInstalled,
  agentMissingPlugins,
  agentName,
  agentRequiredPlugins,
  pluginRequirementName,
  pluginRequirementSlug,
  type AgentPluginRequirement,
  type CatalogAgent,
} from "../_lib"

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
  const installAgent = $api.useMutation(
    "post",
    "/v1/agents/catalog/{slug}/install"
  )
  const agent = agentQuery.data as CatalogAgent | undefined
  const requiredPlugins = agentRequiredPlugins(agent)
  const missingPlugins = agentMissingPlugins(agent)
  const installed = agent ? agentIsInstalled(agent) : false
  const canInstall = agentCanInstall(agent)
  const busy = installAgent.isPending

  function refresh() {
    queryClient.invalidateQueries({ queryKey: AGENT_CATALOG_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/agents/catalog/{slug}"],
    })
    agentQuery.refetch()
  }

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
        className="flex w-fit items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <Icon icon="lucide:arrow-left" className="h-4 w-4" />
        Agents
      </NextLink>

      <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <AgentAvatar agent={agent} size="lg" />
          <div className="min-w-0">
            <h1 className="text-xl font-semibold text-foreground">
              {agentName(agent)}
            </h1>
            <p className="mt-1 max-w-xl text-sm leading-5 text-muted-foreground">
              {agentDescription(agent)}
            </p>
          </div>
        </div>

        <Button
          size="sm"
          className="shrink-0 rounded-full bg-foreground text-background hover:bg-foreground/90"
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

      {requiredPlugins.length > 0 ? (
        <RequiredPluginsSection plugins={requiredPlugins} />
      ) : (
        <NoRequirementsSection />
      )}
    </div>
  )
}

function MissingPluginsWarning({ count }: { count: number }) {
  return (
    <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-xl border p-4">
      <div className="bg-warning/15 text-warning flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
        <Icon icon="lucide:triangle-alert" className="h-5 w-5" />
      </div>
      <div className="min-w-0">
        <h2 className="text-sm font-medium text-foreground">
          Required plugins missing
        </h2>
        <p className="mt-1 text-sm leading-5 text-muted-foreground">
          Install {count === 1 ? "this plugin" : "these plugins"} before
          installing this agent.
        </p>
      </div>
    </div>
  )
}

function RequiredPluginsSection({
  plugins,
}: {
  plugins: AgentPluginRequirement[]
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold text-foreground">
        Required plugins
      </h2>
      <div className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border bg-card">
        {plugins.map((plugin, index) => (
          <RequiredPluginRow
            key={plugin.slug ?? plugin.name ?? index}
            plugin={plugin}
          />
        ))}
      </div>
    </section>
  )
}

function RequiredPluginRow({ plugin }: { plugin: AgentPluginRequirement }) {
  const slug = pluginRequirementSlug(plugin)
  const installed = plugin.installed === true
  const row = (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5">
      <div className="flex min-w-0 items-center gap-3">
        <div className="bg-default flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground">
          <Icon icon="lucide:plug" className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {pluginRequirementName(plugin)}
          </p>
          <p className="text-xs text-muted-foreground">
            {installed ? "Installed" : "Required before install"}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <span
          className={
            installed
              ? "bg-default rounded-full px-2 py-0.5 text-xs text-muted-foreground"
              : "bg-warning/10 text-warning rounded-full px-2 py-0.5 text-xs"
          }
        >
          {installed ? "Installed" : "Missing"}
        </span>
        {slug ? (
          <Icon
            icon="lucide:chevron-right"
            className="h-4 w-4 text-muted-foreground"
          />
        ) : null}
      </div>
    </div>
  )

  if (!slug) return row

  return (
    <NextLink
      href={`/w/plugins/${slug}`}
      className="block transition-colors hover:bg-muted/20"
    >
      {row}
    </NextLink>
  )
}

function NoRequirementsSection() {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold text-foreground">
        Required plugins
      </h2>
      <div className="flex items-center gap-3 rounded-xl border border-border bg-card px-3 py-2.5">
        <div className="bg-default flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground">
          <Icon icon="lucide:check" className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <p className="text-sm font-medium text-foreground">
            No required plugins
          </p>
          <p className="text-xs text-muted-foreground">
            This agent can be installed without extra workspace plugins.
          </p>
        </div>
      </div>
    </section>
  )
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <div className="bg-default h-4 w-20 animate-pulse rounded" />
      <header className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="bg-default h-12 w-12 animate-pulse rounded-xl" />
          <div className="flex flex-col gap-3">
            <div className="bg-default h-5 w-36 animate-pulse rounded" />
            <div className="bg-default h-4 w-80 max-w-full animate-pulse rounded" />
          </div>
        </div>
        <div className="bg-default h-8 w-20 animate-pulse rounded-full" />
      </header>
      <div className="bg-default h-24 animate-pulse rounded-xl" />
      <div className="bg-default h-44 animate-pulse rounded-xl" />
    </div>
  )
}

function NotFoundState() {
  return (
    <div className="flex min-h-64 flex-col items-center justify-center rounded-xl border border-border bg-card px-6 text-center">
      <Icon icon="lucide:bot" className="h-7 w-7 text-muted" />
      <p className="mt-3 text-sm font-medium text-foreground">
        Agent not found
      </p>
      <p className="mt-1 text-sm text-muted">
        This agent may have been removed from the catalog.
      </p>
      <NextLink
        href="/w/settings/agents"
        className="mt-4 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        Back to agents
      </NextLink>
    </div>
  )
}
