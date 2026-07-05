"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { Button, Input, ListBox, Select, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { LogoTile } from "@/components/plugin-logo"
import {
  automationCategories,
  automationCategory,
  automationMatchesCategory,
  automationMatchesQuery,
  type AutomationCategory,
  type AutomationItem,
  type AutomationTab,
} from "@/app/w/(chat)/automations/_data"

export function AutomationsListView({
  automations,
  isLoading,
  isError,
  onRetry,
  action,
  nav,
  title = "Automations",
  description = "Start agent work from events or recurring schedules",
  searchLabel = "triggers",
  emptyTab = "Triggers",
}: {
  automations: AutomationItem[]
  isLoading: boolean
  isError: boolean
  onRetry: () => void
  action?: { label: string; href: string; icon?: string }
  nav?: React.ReactNode
  title?: string
  description?: string
  searchLabel?: string
  emptyTab?: AutomationTab
}) {
  const router = useRouter()
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<AutomationCategory>("All")
  const categories = useMemo(
    () => automationCategories(automations),
    [automations]
  )
  const filteredAutomations = useMemo(() => {
    return automations.filter(
      (automation) =>
        automationMatchesCategory(automation, category) &&
        automationMatchesQuery(automation, query)
    )
  }, [automations, category, query])
  const groupedAutomations = useMemo(() => {
    if (category !== "All") return { [category]: filteredAutomations }

    const groups: Record<string, AutomationItem[]> = {}
    for (const automation of filteredAutomations) {
      const section = automationCategory(automation)
      if (!groups[section]) groups[section] = []
      groups[section].push(automation)
    }

    const ordered: Record<string, AutomationItem[]> = {}
    for (const section of categories.filter(
      (item) => item !== "All" && item !== "Featured"
    )) {
      if (groups[section]) ordered[section] = groups[section]
    }
    for (const section of Object.keys(groups)) {
      if (!ordered[section]) ordered[section] = groups[section]
    }
    return ordered
  }, [categories, category, filteredAutomations])
  const sectionEntries = Object.entries(groupedAutomations)

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          {nav}
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h1 className="text-2xl font-semibold text-foreground">
                {title}
              </h1>
              <p className="text-muted-foreground mt-1 text-sm">
                {description}
              </p>
            </div>

            {action ? (
              <Button
                variant="primary"
                size="sm"
                className="shrink-0"
                onPress={() => router.push(action.href)}
              >
                <AppIcon icon={action.icon ?? "plus"} className="h-4 w-4" />
                {action.label}
              </Button>
            ) : null}
          </div>

          <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative min-w-0 flex-1">
              <AppIcon
                icon="search"
                className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2"
              />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={`Search ${searchLabel}`}
                className="bg-card h-10 w-full rounded-md pl-9"
              />
            </div>

            <CategorySelect
              categories={categories}
              value={category}
              onChange={setCategory}
            />
          </div>

          {isLoading ? (
            <CatalogSkeleton />
          ) : isError ? (
            <ErrorState tab={emptyTab} onRetry={onRetry} />
          ) : sectionEntries.length === 0 ? (
            <EmptyState query={query} tab={emptyTab} />
          ) : (
            <div className="flex flex-col gap-8">
              {sectionEntries.map(([section, sectionAutomations]) => (
                <section key={section} className="flex flex-col gap-3">
                  <h2 className="text-sm font-medium text-foreground">
                    {section}
                  </h2>
                  <div className="bg-card flex flex-col">
                    {sectionAutomations.map((automation) => (
                      <AutomationRow
                        key={automation.id}
                        automation={automation}
                      />
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
  categories: AutomationCategory[]
  value: AutomationCategory
  onChange: (category: AutomationCategory) => void
}) {
  return (
    <Select
      aria-label="Automation category"
      value={value}
      onChange={(key) => onChange(String(key) as AutomationCategory)}
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

function AutomationRow({ automation }: { automation: AutomationItem }) {
  return (
    <NextLink
      href={automation.href ?? `/w/automations/${automation.id}`}
      className="group -mx-3 block py-1.5"
    >
      <div className="rounded-xl px-3 py-1.5 transition-colors group-hover:bg-default group-focus-visible:bg-default group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-foreground/40">
        <div className="flex items-center gap-3">
          <LogoTile
            provider={automation.provider}
            icon={automation.icon}
            color={automation.iconColor}
          />

          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-foreground">
              {automation.name}
            </h3>
            <p className="text-muted-foreground truncate text-sm">
              {automation.description}
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

function CatalogSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      {Array.from({ length: 2 }).map((_, section) => (
        <section key={section} className="flex flex-col gap-3">
          <Skeleton className="h-4 w-28 rounded" />
          <div className="bg-card flex flex-col gap-3">
            {Array.from({ length: 3 }).map((_, row) => (
              <div key={row} className="flex items-center gap-3 py-1.5">
                <Skeleton className="h-9 w-9" />
                <div className="min-w-0 flex-1">
                  <Skeleton className="h-4 w-40 rounded" />
                  <Skeleton className="mt-2 h-4 w-full max-w-sm rounded" />
                </div>
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function ErrorState({
  tab,
  onRetry,
}: {
  tab: AutomationTab
  onRetry: () => void
}) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon
        icon="triangle-alert"
        className="text-muted-foreground h-7 w-7"
      />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load {tab.toLowerCase()}
      </p>
      <Button variant="ghost" size="sm" className="mt-3" onPress={onRetry}>
        <AppIcon icon="refresh-cw" className="h-4 w-4" />
        Retry
      </Button>
    </div>
  )
}

function EmptyState({ query, tab }: { query: string; tab: AutomationTab }) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon icon="clock" className="text-muted-foreground h-7 w-7" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? `No matching ${tab.toLowerCase()}` : `No ${tab.toLowerCase()}`}
      </p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        Try a different search or category.
      </p>
    </div>
  )
}
