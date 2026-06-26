"use client"

import NextLink from "next/link"
import { Switch, Tooltip } from "@heroui/react"
import { Icon } from "@iconify/react"
import { IntegrationLogo } from "@/components/integration-logo"
import {
  pluginDescription,
  pluginIcon,
  pluginIconColor,
  pluginLogoProvider,
  pluginName,
  pluginSlug,
  type ApiPlugin,
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
  agentName: name,
  plugins,
  requiredSlugs,
  isLoading,
  isBusy,
  onToggle,
}: {
  agentID: string
  agentName: string
  plugins: ApiPlugin[]
  requiredSlugs: ReadonlySet<string>
  isLoading: boolean
  isBusy: boolean
  onToggle: (plugin: ApiPlugin, selected: boolean) => void
}) {
  const installedPlugins = plugins.filter((plugin) => plugin.installed)
  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold text-foreground">Agent plugins</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Manage workspace plugins available to {name}.
        </p>
      </div>

      {isLoading ? (
        <AgentPluginSkeleton />
      ) : installedPlugins.length > 0 ? (
        <div className="grid gap-3">
          {installedPlugins.map((plugin) => {
            const slug = pluginSlug(plugin)
            return (
              <AgentPluginCard
                key={slug || plugin.id || plugin.name}
                agentID={agentID}
                plugin={plugin}
                required={slug ? requiredSlugs.has(slug) : false}
                isBusy={isBusy}
                onToggle={onToggle}
              />
            )
          })}
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
          <Icon icon="lucide:check" className="h-4 w-4" />
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

function AgentPluginCard({
  agentID,
  plugin,
  required,
  isBusy,
  onToggle,
}: {
  agentID: string
  plugin: ApiPlugin
  required: boolean
  isBusy: boolean
  onToggle: (plugin: ApiPlugin, selected: boolean) => void
}) {
  const slug = pluginSlug(plugin)
  const globalInstall = plugin.auto_install === true
  const enabled =
    required || globalInstall || pluginEnabledForAgent(plugin, agentID)
  const disabled = required || globalInstall || isBusy || !slug
  const details = (
    <div className="flex min-w-0 flex-1 items-center gap-3 overflow-hidden">
      <PluginLogoMark plugin={plugin} />
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
    <div className="bg-card flex min-w-0 items-center justify-between gap-4 rounded-lg border border-border p-3">
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
      <AgentPluginSwitch
        plugin={plugin}
        enabled={enabled}
        disabled={disabled}
        required={required}
        globalInstall={globalInstall}
        onToggle={onToggle}
      />
    </div>
  )
}

function AgentPluginSwitch({
  plugin,
  enabled,
  disabled,
  required,
  globalInstall,
  onToggle,
}: {
  plugin: ApiPlugin
  enabled: boolean
  disabled: boolean
  required: boolean
  globalInstall: boolean
  onToggle: (plugin: ApiPlugin, selected: boolean) => void
}) {
  const control = (
    <Switch
      aria-label={`${enabled ? "Uninstall" : "Install"} ${pluginName(plugin)} for this agent`}
      isSelected={enabled}
      isDisabled={disabled}
      onChange={(selected) => onToggle(plugin, selected)}
      className="shrink-0"
    >
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
    </Switch>
  )

  if (!required && !globalInstall) {
    return control
  }

  return (
    <Tooltip delay={250} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">{control}</Tooltip.Trigger>
      <Tooltip.Content placement="left" offset={8} className="text-xs">
        {globalInstall
          ? "This plugin is installed for all agents and cannot be uninstalled here."
          : "This plugin is required by this agent and cannot be uninstalled."}
      </Tooltip.Content>
    </Tooltip>
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
        <PluginLogoMark plugin={catalogPlugin} />
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
          <Icon
            icon="lucide:chevron-right"
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

function PluginLogoMark({ plugin }: { plugin?: ApiPlugin }) {
  const provider = plugin ? pluginLogoProvider(plugin) : null
  if (provider) {
    return (
      <IntegrationLogo provider={provider} size={32} className="rounded-lg" />
    )
  }

  if (!plugin) {
    return (
      <div className="text-muted-foreground flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default">
        <Icon icon="lucide:plug" className="h-4 w-4" />
      </div>
    )
  }

  return (
    <div
      className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-white"
      style={{ backgroundColor: pluginIconColor(plugin) }}
    >
      <Icon icon={pluginIcon(plugin)} className="h-4 w-4" />
    </div>
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
            <div className="h-8 w-8 animate-pulse rounded-lg bg-default" />
            <div className="flex min-w-0 flex-1 flex-col gap-2">
              <div className="h-4 w-32 animate-pulse rounded bg-default" />
              <div className="h-3 w-56 max-w-full animate-pulse rounded bg-default" />
            </div>
          </div>
          <div className="h-6 w-10 animate-pulse rounded-full bg-default" />
        </div>
      ))}
    </div>
  )
}

function AgentPluginsEmptyState() {
  return (
    <div className="bg-card flex items-center gap-3 rounded-lg border border-border px-3 py-2.5">
      <div className="text-muted-foreground flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default">
        <Icon icon="lucide:package-open" className="h-4 w-4" />
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
