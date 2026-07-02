"use client"

import NextLink from "next/link"
import { Description, Switch, Tooltip } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import {
  pluginCanInstall,
  pluginDescription,
  pluginIcon,
  pluginIconColor,
  pluginLogoProvider,
  pluginName,
  pluginSlug,
  type ApiPlugin,
} from "@/app/w/(chat)/plugins/_lib"

export function PluginsField({
  plugins,
  selectedSlugs,
  onPluginToggle,
  isLoading = false,
  disabled = false,
}: {
  plugins: ApiPlugin[]
  selectedSlugs: string[]
  onPluginToggle: (slug: string, selected: boolean) => void
  isLoading?: boolean
  disabled?: boolean
}) {
  if (isLoading) {
    return <PluginsSkeleton />
  }
  if (plugins.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No workspace plugins available.
      </p>
    )
  }

  const selected = new Set(selectedSlugs)

  return (
    <div className="flex flex-col gap-2">
      {plugins.map((plugin) => {
        const slug = pluginSlug(plugin)
        const auto = plugin.auto_install === true
        const connectable = pluginCanInstall(plugin)
        const on = auto || selected.has(slug)
        const locked = auto || !connectable || disabled

        return (
          <div
            key={slug || plugin.id || plugin.name}
            data-testid={`plugin-row-${slug}`}
            data-plugin-on={on ? "true" : "false"}
            data-plugin-connectable={connectable ? "true" : "false"}
            className="flex min-w-0 items-center justify-between gap-4 rounded-lg border border-border bg-card p-3"
          >
            <div className="flex min-w-0 flex-1 items-center gap-3">
              <PluginLogo plugin={plugin} />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-foreground">
                  {pluginName(plugin)}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {connectable
                    ? pluginDescription(plugin)
                    : "Requires a connection before it can be enabled"}
                </p>
              </div>
            </div>

            {!connectable && !auto ? (
              <NextLink
                href={`/w/plugins/${slug}`}
                className="flex shrink-0 items-center gap-1 text-xs text-primary transition-colors hover:underline"
              >
                Connect
                <AppIcon icon="arrow-up-right" className="h-3.5 w-3.5" />
              </NextLink>
            ) : (
              <PluginSwitch
                name={pluginName(plugin)}
                on={on}
                locked={locked}
                auto={auto}
                onToggle={(value) => onPluginToggle(slug, value)}
              />
            )}
          </div>
        )
      })}
      <Description className="text-xs text-muted-foreground">
        Plugin tools and skills take effect on the agent&apos;s next session —
        running sessions keep their current set.
      </Description>
    </div>
  )
}

function PluginSwitch({
  name,
  on,
  locked,
  auto,
  onToggle,
}: {
  name: string
  on: boolean
  locked: boolean
  auto: boolean
  onToggle: (value: boolean) => void
}) {
  const control = (
    <Switch
      aria-label={`${on ? "Disable" : "Enable"} ${name}`}
      isSelected={on}
      isDisabled={locked}
      onChange={(value) => onToggle(value)}
      className="shrink-0"
    >
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
    </Switch>
  )
  if (!auto) return control
  return (
    <Tooltip delay={200} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">{control}</Tooltip.Trigger>
      <Tooltip.Content placement="left" offset={8} className="text-xs">
        Installed for all agents.
      </Tooltip.Content>
    </Tooltip>
  )
}

function PluginLogo({ plugin }: { plugin: ApiPlugin }) {
  const provider = pluginLogoProvider(plugin)
  if (provider) {
    return (
      <IntegrationLogo provider={provider} size={32} className="rounded-lg" />
    )
  }
  return (
    <div
      className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-white"
      style={{ backgroundColor: pluginIconColor(plugin) }}
    >
      <AppIcon icon={pluginIcon(plugin)} className="h-4 w-4" />
    </div>
  )
}

function PluginsSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center justify-between gap-4 rounded-lg border border-border bg-card p-3"
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
