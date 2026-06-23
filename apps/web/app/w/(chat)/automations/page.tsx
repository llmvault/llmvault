"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import { Button, Input, ListBox, Select } from "@heroui/react"
import { Icon } from "@iconify/react"
import { cn } from "@/lib/utils"
import {
  AUTOMATION_TABS,
  STATIC_AUTOMATIONS,
  automationCategories,
  automationCategory,
  automationMatchesCategory,
  automationMatchesQuery,
  type AutomationCategory,
  type AutomationItem,
  type AutomationTab,
} from "@/app/w/(chat)/automations/_data"

export default function AutomationsPage() {
  const [tab, setTab] = useState<AutomationTab>("Triggers")
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<AutomationCategory>("All")

  const tabAutomations = useMemo(
    () => STATIC_AUTOMATIONS.filter((automation) => automation.type === tab),
    [tab]
  )
  const categories = useMemo(
    () => automationCategories(tabAutomations),
    [tabAutomations]
  )
  const filteredAutomations = useMemo(() => {
    return tabAutomations.filter(
      (automation) =>
        automationMatchesCategory(automation, category) &&
        automationMatchesQuery(automation, query)
    )
  }, [category, query, tabAutomations])

  const groupedAutomations = useMemo(() => {
    if (category !== "All") {
      return { [category]: filteredAutomations }
    }

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
          <nav aria-label="Automation type" className="flex items-center gap-1">
            {AUTOMATION_TABS.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => {
                  setTab(item)
                  setQuery("")
                  setCategory("All")
                }}
                className={cn(
                  "rounded-lg px-3 py-1.5 text-sm font-medium transition-colors",
                  item === tab
                    ? "bg-default text-foreground"
                    : "text-muted-foreground hover:bg-muted/30 hover:text-foreground"
                )}
              >
                {item}
              </button>
            ))}
          </nav>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h1 className="text-2xl font-semibold text-foreground">
                Automations
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">
                Start agent work from events or recurring schedules
              </p>
            </div>

            <Button variant="primary" size="sm" className="shrink-0">
              <Icon icon="lucide:plus" className="h-4 w-4" />
              Add trigger
            </Button>
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
                placeholder={`Search ${tab.toLowerCase()}`}
                className="h-10 w-full rounded-md bg-card pl-9"
              />
            </div>

            <CategorySelect
              categories={categories}
              value={category}
              onChange={setCategory}
            />
          </div>

          {sectionEntries.length === 0 ? (
            <EmptyState query={query} tab={tab} />
          ) : (
            <div className="flex flex-col gap-8">
              {sectionEntries.map(([section, automations]) => (
                <section key={section} className="flex flex-col gap-3">
                  <h2 className="text-sm font-medium text-foreground">
                    {section}
                  </h2>
                  <div className="flex flex-col bg-card">
                    {automations.map((automation) => (
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

function AutomationRow({ automation }: { automation: AutomationItem }) {
  return (
    <NextLink
      href={`/w/automations/${automation.id}`}
      className="group -mx-3 block py-1.5"
    >
      <div className="group-hover:bg-default group-focus-visible:bg-default rounded-xl px-3 py-1.5 transition-colors group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-foreground/40">
        <div className="flex items-center gap-3">
          <div
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
            style={{ backgroundColor: automation.iconColor }}
          >
            <Icon
              icon={automation.icon}
              className="h-[18px] w-[18px] shrink-0 text-white"
            />
          </div>

          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-foreground">
              {automation.name}
            </h3>
            <p className="truncate text-sm text-muted-foreground">
              {automation.description}
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

function EmptyState({ query, tab }: { query: string; tab: AutomationTab }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <Icon icon="lucide:clock" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? `No matching ${tab.toLowerCase()}` : `No ${tab.toLowerCase()}`}
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        Try a different search or category.
      </p>
    </div>
  )
}
