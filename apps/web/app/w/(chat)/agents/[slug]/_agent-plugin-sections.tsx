"use client"

import { useState } from "react"
import NextLink from "next/link"
import { Button, Skeleton, Switch } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { PluginLogoTile } from "@/components/plugin-logo"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  pluginConnectionKind,
  pluginDescription,
  pluginName,
  pluginRequiredConnections,
  pluginSlug,
  type ApiPlugin,
  type PluginRequirement,
} from "@/app/w/(chat)/plugins/_lib"
import {
  pluginEnabledForAgent,
  pluginForRequirement,
  pluginRequirementName,
  pluginRequirementSlug,
  type AgentPluginRequirement,
} from "../_lib"

export function AgentPluginsSection({
  agentID,
  plugins,
  teamId,
  teamPluginIDs,
  disabledPluginIDs,
  requiredPluginSlugs,
  canManage,
  isSaving,
  isLoading,
  onDisabledPluginIDsChange,
  pluginMCPToolDeny,
  onPluginMCPToolDenyChange,
}: {
  agentID: string
  plugins: ApiPlugin[]
  teamId: string
  teamPluginIDs: string[]
  disabledPluginIDs: string[]
  requiredPluginSlugs: string[]
  canManage: boolean
  isSaving: boolean
  isLoading: boolean
  onDisabledPluginIDsChange: (pluginIDs: string[]) => void
  pluginMCPToolDeny: Record<string, string[]>
  onPluginMCPToolDenyChange: (
    pluginID: string,
    tool: string,
    denied: boolean
  ) => void
}) {
  const teamPluginIDSet = new Set(teamPluginIDs)
  const disabledPluginIDSet = new Set(disabledPluginIDs)
  const requiredPluginSlugSet = new Set(requiredPluginSlugs)
  const effectivePlugins = plugins.filter((plugin) => {
    const id = plugin.id
    return Boolean(
      plugin.auto_install === true ||
      (id && teamPluginIDSet.has(id)) ||
      pluginEnabledForAgent(plugin, agentID)
    )
  })
  const manageHref = teamId && canManage ? `/w/settings/teams/${teamId}` : null
  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold text-foreground">Agent plugins</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Team plugins are inherited by default. Disable an optional plugin here
          for only this agent.{" "}
          {manageHref ? (
            <NextLink
              href={manageHref}
              className="text-primary transition-colors hover:underline"
            >
              Manage in team settings
            </NextLink>
          ) : null}
        </p>
      </div>

      {isLoading ? (
        <AgentPluginSkeleton />
      ) : effectivePlugins.length > 0 ? (
        <div className="grid gap-3">
          {effectivePlugins.map((plugin) => (
            <AgentPluginRow
              key={pluginSlug(plugin) || plugin.id || plugin.name}
              plugin={plugin}
              inheritedFromTeam={Boolean(
                plugin.id && teamPluginIDSet.has(plugin.id)
              )}
              disabled={Boolean(
                plugin.id && disabledPluginIDSet.has(plugin.id)
              )}
              required={requiredPluginSlugSet.has(pluginSlug(plugin))}
              canManage={canManage}
              isSaving={isSaving}
              onDisabledChange={(disabled) => {
                const id = plugin.id
                if (!id) return
                const next = new Set(disabledPluginIDSet)
                if (disabled) next.add(id)
                else next.delete(id)
                onDisabledPluginIDsChange(Array.from(next))
              }}
              deniedTools={
                plugin.id ? (pluginMCPToolDeny[plugin.id] ?? []) : []
              }
              onToolDeniedChange={(tool, denied) => {
                if (plugin.id)
                  onPluginMCPToolDenyChange(plugin.id, tool, denied)
              }}
            />
          ))}
        </div>
      ) : (
        <AgentPluginsEmptyState />
      )}
    </section>
  )
}

export function RequiredPluginsSection({
  plugins,
  pluginLookup,
}: {
  plugins: AgentPluginRequirement[]
  pluginLookup: Map<string, ApiPlugin>
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold text-foreground">
        Required plugins
      </h2>
      <div className="bg-card flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
        {plugins.map((plugin, index) => (
          <RequiredPluginRow
            key={plugin.slug ?? plugin.name ?? index}
            plugin={plugin}
            catalogPlugin={pluginForRequirement(plugin, pluginLookup)}
          />
        ))}
      </div>
    </section>
  )
}

export function NoRequirementsSection() {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold text-foreground">
        Required plugins
      </h2>
      <div className="bg-card flex items-center gap-3 rounded-xl border border-border px-3 py-2.5">
        <div className="text-muted-foreground flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default">
          <AppIcon icon="check" className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <p className="text-sm font-medium text-foreground">
            No required plugins
          </p>
          <p className="text-muted-foreground text-xs">
            This agent can be installed without extra workspace plugins.
          </p>
        </div>
      </div>
    </section>
  )
}

function AgentPluginRow({
  plugin,
  inheritedFromTeam,
  disabled,
  required,
  canManage,
  isSaving,
  onDisabledChange,
  deniedTools,
  onToolDeniedChange,
}: {
  plugin: ApiPlugin
  inheritedFromTeam: boolean
  disabled: boolean
  required: boolean
  canManage: boolean
  isSaving: boolean
  onDisabledChange: (disabled: boolean) => void
  deniedTools: string[]
  onToolDeniedChange: (tool: string, denied: boolean) => void
}) {
  const [toolsOpen, setToolsOpen] = useState(false)
  const slug = pluginSlug(plugin)
  const alwaysOn = plugin.auto_install === true || !inheritedFromTeam
  const locked = alwaysOn || required
  const enabled = locked || !disabled
  const toolRequirements = pluginRequiredConnections(plugin).filter(
    (requirement) => {
      const kind = pluginConnectionKind(requirement)
      return kind === "integration" || kind === "database"
    }
  )
  const details = (
    <div className="flex min-w-0 flex-1 items-center gap-3 overflow-hidden">
      <PluginLogoTile plugin={plugin} />
      <div className="min-w-0 flex-1 overflow-hidden">
        <p className="block truncate text-sm font-medium text-foreground">
          {pluginName(plugin)}
        </p>
        <p className="text-muted-foreground block truncate text-xs">
          {pluginDescription(plugin)}
        </p>
      </div>
    </div>
  )

  return (
    <div className="bg-card overflow-hidden rounded-lg border border-border">
      <div className="flex min-w-0 items-center justify-between gap-4 p-3">
        {slug ? (
          <NextLink
            href={`/w/plugins/${slug}`}
            className="min-w-0 flex-1 overflow-hidden rounded-md transition-colors hover:text-foreground"
          >
            {details}
          </NextLink>
        ) : (
          details
        )}
        <div className="flex shrink-0 items-center gap-3">
          {toolRequirements.length > 0 ? (
            <Button
              size="sm"
              variant="tertiary"
              isDisabled={!enabled}
              onPress={() => setToolsOpen((open) => !open)}
              aria-expanded={toolsOpen}
            >
              Tools
              <AppIcon
                icon={toolsOpen ? "chevron-down" : "chevron-right"}
                className="h-3.5 w-3.5"
              />
            </Button>
          ) : null}
          <span className="text-muted-foreground rounded-full bg-default px-2 py-0.5 text-xs">
            {required ? "Required" : alwaysOn ? "Always on" : "From team"}
          </span>
          <Switch
            aria-label={`${enabled ? "Disable" : "Enable"} ${pluginName(plugin)}`}
            isSelected={enabled}
            isDisabled={locked || !canManage || isSaving}
            onChange={() => onDisabledChange(enabled)}
            className="shrink-0"
          >
            <Switch.Control>
              <Switch.Thumb />
            </Switch.Control>
          </Switch>
        </div>
      </div>
      {toolsOpen ? (
        <PluginToolAccess
          requirements={toolRequirements}
          deniedTools={deniedTools}
          canManage={canManage}
          isSaving={isSaving}
          onToolDeniedChange={onToolDeniedChange}
        />
      ) : null}
    </div>
  )
}

type ActionSummary = components["schemas"]["actionSummary"]

function PluginToolAccess({
  requirements,
  deniedTools,
  canManage,
  isSaving,
  onToolDeniedChange,
}: {
  requirements: PluginRequirement[]
  deniedTools: string[]
  canManage: boolean
  isSaving: boolean
  onToolDeniedChange: (tool: string, denied: boolean) => void
}) {
  return (
    <div className="border-t border-border bg-default/30 px-3 py-3">
      <div className="mb-3">
        <p className="text-sm font-medium text-foreground">Tool access</p>
        <p className="text-muted-foreground text-xs">
          New plugin tools are enabled automatically. Turn off tools this agent
          should not use.
        </p>
      </div>
      <div className="grid gap-4">
        {requirements.map((requirement) => (
          <ProviderToolAccess
            key={`${pluginConnectionKind(requirement)}:${requirement.provider}`}
            requirement={requirement}
            deniedTools={deniedTools}
            canManage={canManage}
            isSaving={isSaving}
            onToolDeniedChange={onToolDeniedChange}
          />
        ))}
      </div>
    </div>
  )
}

function ProviderToolAccess({
  requirement,
  deniedTools,
  canManage,
  isSaving,
  onToolDeniedChange,
}: {
  requirement: PluginRequirement
  deniedTools: string[]
  canManage: boolean
  isSaving: boolean
  onToolDeniedChange: (tool: string, denied: boolean) => void
}) {
  const kind = pluginConnectionKind(requirement)
  const provider = requirement.provider ?? ""
  const actionsQuery = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/actions",
    { params: { path: { id: provider } } },
    { enabled: kind === "integration" && provider.length > 0, retry: false }
  )
  const actions: ActionSummary[] =
    kind === "database"
      ? [
          {
            key: "run_query",
            display_name: "Run query",
            description:
              "Query this database connection without modifying data.",
            access: "read",
            mcp_enabled: true,
          },
        ]
      : (actionsQuery.data ?? []).filter(
          (action) => action.mcp_enabled === true
        )

  if (actionsQuery.isLoading && kind === "integration") {
    return <Skeleton className="h-16 w-full rounded-lg" />
  }
  if (actionsQuery.isError && kind === "integration") {
    return (
      <div className="bg-card flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2.5">
        <p className="text-muted-foreground text-xs">
          Could not load tools for {provider}.
        </p>
        <Button
          size="sm"
          variant="tertiary"
          onPress={() => actionsQuery.refetch()}
        >
          Retry
        </Button>
      </div>
    )
  }
  if (actions.length === 0) return null

  const denied = new Set(deniedTools)
  return (
    <div>
      <p className="text-muted-foreground mb-1.5 text-xs font-medium tracking-wide uppercase">
        {provider}
      </p>
      <div className="bg-card divide-y divide-border rounded-lg border border-border">
        {actions.map((action) => {
          const key = action.key ?? ""
          const enabled = key !== "" && !denied.has(key)
          return (
            <div
              key={key}
              className="flex items-center justify-between gap-4 px-3 py-2.5"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <p className="truncate text-sm font-medium text-foreground">
                    {action.display_name || key}
                  </p>
                  <span className="text-muted-foreground rounded-full bg-default px-1.5 py-0.5 text-[11px] capitalize">
                    {action.access || "read"}
                  </span>
                </div>
                {action.description ? (
                  <p className="text-muted-foreground mt-0.5 line-clamp-2 text-xs">
                    {action.description}
                  </p>
                ) : null}
              </div>
              <Switch
                aria-label={`${enabled ? "Disable" : "Enable"} ${action.display_name || key}`}
                isSelected={enabled}
                isDisabled={!canManage || isSaving || key === ""}
                onChange={() => onToolDeniedChange(key, enabled)}
                className="shrink-0"
              >
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
              </Switch>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function RequiredPluginRow({
  plugin,
  catalogPlugin,
}: {
  plugin: AgentPluginRequirement
  catalogPlugin?: ApiPlugin
}) {
  const slug = pluginRequirementSlug(plugin)
  const installed = plugin.installed === true
  const row = (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5">
      <div className="flex min-w-0 flex-1 items-center gap-3 overflow-hidden">
        <PluginLogoTile plugin={catalogPlugin} />
        <div className="min-w-0 flex-1 overflow-hidden">
          <p className="block truncate text-sm font-medium text-foreground">
            {pluginRequirementName(plugin)}
          </p>
          <p className="text-muted-foreground text-xs">
            {installed ? "Installed" : "Required before install"}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <span
          className={
            installed
              ? "text-muted-foreground rounded-full bg-default px-2 py-0.5 text-xs"
              : "rounded-full bg-warning/10 px-2 py-0.5 text-xs text-warning"
          }
        >
          {installed ? "Installed" : "Missing"}
        </span>
        {slug ? (
          <AppIcon
            icon="chevron-right"
            className="text-muted-foreground h-4 w-4"
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

function AgentPluginSkeleton() {
  return (
    <div className="grid gap-3">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="bg-card flex items-center justify-between gap-4 rounded-lg border border-border p-3"
        >
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <Skeleton className="h-8 w-8" />
            <div className="flex min-w-0 flex-1 flex-col gap-2">
              <Skeleton className="h-4 w-32 rounded" />
              <Skeleton className="h-3 w-56 max-w-full rounded" />
            </div>
          </div>
          <Skeleton className="h-6 w-10 rounded-full" />
        </div>
      ))}
    </div>
  )
}

function AgentPluginsEmptyState() {
  return (
    <div className="bg-card flex items-center gap-3 rounded-lg border border-border px-3 py-2.5">
      <div className="text-muted-foreground flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default">
        <AppIcon icon="package-open" className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <p className="text-sm font-medium text-foreground">
          No workspace plugins installed
        </p>
        <p className="text-muted-foreground text-xs">
          Installed workspace plugins will appear here.
        </p>
      </div>
    </div>
  )
}
