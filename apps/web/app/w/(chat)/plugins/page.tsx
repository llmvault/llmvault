"use client"

import { useMemo, useState } from "react"
import Image from "next/image"
import NextLink from "next/link"
import { Input, ListBox, Select } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { integrationLogoURL } from "@/components/integration-logo"
import { cn } from "@/lib/utils"
import {
  type ApiPlugin,
  pluginCategories,
  pluginCategory,
  pluginDescription,
  pluginIcon,
  pluginIconColor,
  pluginLogoProvider,
  pluginMatchesCategory,
  pluginMatchesQuery,
  pluginName,
  pluginSlug,
  type PluginCategory,
} from "@/app/w/(chat)/plugins/_lib"

export default function PluginsPage() {
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<PluginCategory>("All")
  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const plugins = useMemo(
    () => (pluginsQuery.data ?? []) as ApiPlugin[],
    [pluginsQuery.data]
  )
  const categories = useMemo(() => pluginCategories(plugins), [plugins])
  const connectedPlugins = useMemo(
    () => plugins.filter((plugin) => plugin.installed),
    [plugins]
  )

  const filteredPlugins = useMemo(() => {
    return plugins.filter(
      (plugin) =>
        pluginMatchesCategory(plugin, category) &&
        pluginMatchesQuery(plugin, query)
    )
  }, [category, plugins, query])

  const groupedPlugins = useMemo(() => {
    if (category !== "All") {
      return { [category]: filteredPlugins }
    }

    const groups: Record<string, ApiPlugin[]> = {}
    for (const plugin of filteredPlugins) {
      const section = pluginCategory(plugin)
      if (!groups[section]) groups[section] = []
      groups[section].push(plugin)
    }

    const ordered: Record<string, ApiPlugin[]> = {}
    for (const section of categories.filter(
      (item) => item !== "All" && item !== "Featured"
    )) {
      if (groups[section]) ordered[section] = groups[section]
    }
    for (const section of Object.keys(groups)) {
      if (!ordered[section]) ordered[section] = groups[section]
    }
    return ordered
  }, [categories, category, filteredPlugins])

  const sectionEntries = Object.entries(groupedPlugins)

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <nav
            aria-label="Plugins and skills"
            className="flex items-center gap-1"
          >
            <NextLink
              href="/w/plugins"
              className="bg-default rounded-lg px-3 py-1.5 text-sm font-medium text-foreground"
            >
              Plugins
            </NextLink>
          </nav>

          <div>
            <h1 className="text-2xl font-semibold text-foreground">Plugins</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Work with Hivy across your favorite tools
            </p>
          </div>

          <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative min-w-0 flex-1">
              <Icon
                icon="lucide:search"
                className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search plugins and skills"
                className="h-10 w-full rounded-md bg-card pl-9"
              />
            </div>

            <CategorySelect
              categories={categories}
              value={category}
              onChange={setCategory}
            />
          </div>

          {connectedPlugins.length > 0 ? (
            <section className="flex flex-col gap-3">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-medium text-foreground">
                  Connected
                </h2>
                <button
                  type="button"
                  className="text-sm text-muted-foreground transition-colors hover:text-foreground"
                >
                  Manage
                </button>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {connectedPlugins.map((plugin) => (
                  <div
                    key={pluginSlug(plugin)}
                    className={cn(
                      "flex h-8 w-8 items-center justify-center rounded-lg transition-colors hover:bg-muted/40",
                      pluginLogoProvider(plugin) ? "bg-white" : "bg-card"
                    )}
                    title={pluginName(plugin)}
                  >
                    <PluginLogo plugin={plugin} size={20} iconSize={14} />
                  </div>
                ))}
              </div>
            </section>
          ) : null}

          {pluginsQuery.isLoading ? (
            <PluginListSkeleton />
          ) : sectionEntries.length === 0 ? (
            <EmptyState query={query} />
          ) : (
            <div className="flex flex-col gap-8">
              {sectionEntries.map(([section, plugins]) => (
                <section key={section} className="flex flex-col gap-3">
                  <h2 className="text-sm font-medium text-foreground">
                    {section}
                  </h2>
                  <div className="flex flex-col bg-card">
                    {plugins.map((plugin) => (
                      <PluginRow key={pluginSlug(plugin)} plugin={plugin} />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function CategorySelect({
  categories,
  value,
  onChange,
}: {
  categories: PluginCategory[]
  value: PluginCategory
  onChange: (category: PluginCategory) => void
}) {
  return (
    <Select
      aria-label="Plugin category"
      value={value}
      onChange={(key) => onChange(String(key))}
      className="w-full sm:w-48"
    >
      <Select.Trigger className="h-10 w-full justify-between rounded-md bg-card px-3 text-sm text-foreground transition-colors hover:bg-muted/20">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-48 rounded-xl p-1.5">
        <ListBox>
          {categories.map((item) => (
            <ListBox.Item key={item} id={item} textValue={item}>
              {item}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function PluginRow({ plugin }: { plugin: ApiPlugin }) {
  return (
    <NextLink
      href={`/w/plugins/${pluginSlug(plugin)}`}
      className="group -mx-3 block py-1.5"
    >
      <div className="group-hover:bg-default group-focus-visible:bg-default rounded-xl px-3 py-1.5 transition-colors group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-foreground/40">
        <div className="flex items-center gap-3">
          <div
            className={pluginLogoFrameClass(
              plugin,
              "flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
            )}
            style={pluginLogoFrameStyle(plugin)}
          >
            <PluginLogo
              plugin={plugin}
              size={27}
              iconSize={18}
              forceIconWhite
            />
          </div>

          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-foreground">
              {pluginName(plugin)}
            </h3>
            <p className="truncate text-sm text-muted-foreground">
              {pluginDescription(plugin)}
            </p>
          </div>

          <Icon
            icon="lucide:chevron-right"
            className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
            aria-hidden="true"
          />
        </div>
      </div>
    </NextLink>
  )
}

function AppIcon({
  icon,
  color,
  size = 20,
}: {
  icon: string
  color: string
  size?: number
}) {
  return (
    <Icon
      icon={icon}
      className="shrink-0"
      style={{ color, width: size, height: size }}
    />
  )
}

function PluginLogo({
  plugin,
  size,
  iconSize = size,
  forceIconWhite = false,
}: {
  plugin: ApiPlugin
  size: number
  iconSize?: number
  forceIconWhite?: boolean
}) {
  const provider = pluginLogoProvider(plugin)
  if (provider) {
    return (
      <Image
        src={integrationLogoURL(provider)}
        alt={provider}
        width={size}
        height={size}
        className="shrink-0 object-contain"
        style={{ width: size, height: size }}
      />
    )
  }
  return (
    <AppIcon
      icon={pluginIcon(plugin)}
      color={forceIconWhite ? "#FFFFFF" : pluginIconColor(plugin)}
      size={iconSize}
    />
  )
}

function pluginLogoFrameClass(plugin: ApiPlugin, className: string): string {
  return cn(className, pluginLogoProvider(plugin) ? "bg-white" : "text-white")
}

function pluginLogoFrameStyle(plugin: ApiPlugin) {
  if (pluginLogoProvider(plugin)) return undefined
  return { backgroundColor: pluginIconColor(plugin) }
}

function EmptyState({ query }: { query: string }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <Icon icon="lucide:plug" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? "No matching plugins" : "No plugins available"}
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {query
          ? "Try a different search or category."
          : "Browse the catalog to connect your favorite tools."}
      </p>
    </div>
  )
}

function PluginListSkeleton() {
  return (
    <div className="flex flex-col rounded-xl bg-card">
      {[0, 1, 2, 3].map((index) => (
        <div key={index} className="flex items-center gap-3 p-3">
          <div className="bg-default h-8 w-8 shrink-0 animate-pulse rounded-lg" />
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <div className="bg-default h-3.5 w-28 animate-pulse rounded" />
            <div className="bg-default h-3 w-64 max-w-full animate-pulse rounded" />
          </div>
          <div className="bg-default h-4 w-4 shrink-0 animate-pulse rounded" />
        </div>
      ))}
    </div>
  )
}
