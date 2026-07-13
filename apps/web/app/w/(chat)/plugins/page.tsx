"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import { Input, ListBox, Select, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import {
  PluginLogo,
  PluginLogoTile,
  pluginLogoFrameClass,
  pluginLogoFrameStyle,
} from "@/components/plugin-logo"
import {
  type ApiPlugin,
  pluginCategories,
  pluginCategory,
  pluginDescription,
  pluginHasMissingResourceRequirements,
  pluginMatchesCategory,
  pluginMatchesQuery,
  pluginName,
  pluginSlug,
  type PluginCategory,
} from "@/app/w/(chat)/plugins/_lib"
import { TutorialBanner } from "@/components/tutorial-banner"

export default function PluginsPage() {
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<PluginCategory>("All")
  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const plugins = useMemo(() => pluginsQuery.data ?? [], [pluginsQuery.data])
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
              className="rounded-lg bg-default px-3 py-1.5 text-sm font-medium text-foreground"
            >
              Plugins
            </NextLink>
          </nav>

          <div>
            <h1 className="text-lg font-semibold text-foreground">Plugins</h1>
            <p className="text-muted-foreground mt-1 text-sm">
              Work with Hivy across your favorite tools
            </p>
          </div>

          <TutorialBanner
            tutorial="plugins"
            title="Put plugins to work"
            description="Learn the difference between a connection, a plugin install, and team access."
            docsPath="plugins"
          />

          <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative min-w-0 flex-1">
              <AppIcon
                icon="search"
                className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2"
              />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search plugins and skills"
                className="bg-card h-10 w-full rounded-md pl-9"
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
              <h2 className="text-sm font-medium text-foreground">Connected</h2>
              <div className="flex flex-wrap items-center gap-2">
                {connectedPlugins.map((plugin) => (
                  <NextLink
                    key={pluginSlug(plugin)}
                    href={`/w/plugins/${pluginSlug(plugin)}`}
                    className="relative inline-flex rounded-lg transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground/40"
                    title={pluginName(plugin)}
                  >
                    <PluginLogoTile plugin={plugin} />
                    {pluginHasMissingResourceRequirements(plugin) ? (
                      <span
                        aria-label="Resource selection required"
                        className="absolute -top-0.5 -right-0.5 flex h-3.5 w-3.5 items-center justify-center rounded-full bg-warning text-warning-foreground"
                      >
                        <AppIcon
                          icon="triangle-alert"
                          className="h-2.5 w-2.5"
                        />
                      </span>
                    ) : null}
                  </NextLink>
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
                  <div className="bg-card flex flex-col">
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
      <Select.Trigger className="bg-card h-10 w-full justify-between px-3 text-sm text-foreground transition-colors hover:bg-muted/20">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-48 p-1.5">
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
      <div className="rounded-xl px-3 py-1.5 transition-colors group-hover:bg-default group-focus-visible:bg-default group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-foreground/40">
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
            <p className="text-muted-foreground truncate text-sm">
              {pluginDescription(plugin)}
            </p>
          </div>

          <AppIcon
            icon="chevron-right"
            className="text-muted-foreground h-4 w-4 shrink-0 transition-colors group-hover:text-foreground"
            aria-hidden="true"
          />
        </div>
      </div>
    </NextLink>
  )
}

function EmptyState({ query }: { query: string }) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon icon="plug" className="text-muted-foreground h-7 w-7" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? "No matching plugins" : "No plugins available"}
      </p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        {query
          ? "Try a different search or category."
          : "Browse the catalog to connect your favorite tools."}
      </p>
    </div>
  )
}

function PluginListSkeleton() {
  return (
    <div className="bg-card flex flex-col rounded-xl">
      {[0, 1, 2, 3].map((index) => (
        <div key={index} className="flex items-center gap-3 p-3">
          <Skeleton className="h-8 w-8 shrink-0" />
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <Skeleton className="h-3.5 w-28 rounded" />
            <Skeleton className="h-3 w-64 max-w-full rounded" />
          </div>
          <Skeleton className="h-4 w-4 shrink-0 rounded" />
        </div>
      ))}
    </div>
  )
}
