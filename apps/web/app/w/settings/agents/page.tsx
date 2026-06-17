"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import { Input, ListBox, Select } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { AgentAvatar } from "./_agent-avatar"
import {
  agentCategories,
  agentCategory,
  agentDescription,
  agentIsInstalled,
  agentMatchesCategory,
  agentMatchesQuery,
  agentName,
  agentSlug,
  groupAgents,
  type AgentCategory,
  type InstalledAgent,
  type CatalogAgent,
} from "./_lib"

const EMPTY_CATALOG_AGENTS: CatalogAgent[] = []
const EMPTY_INSTALLED_AGENTS: InstalledAgent[] = []

export default function AgentsSettingsPage() {
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<AgentCategory>("All")

  const catalogQuery = $api.useQuery("get", "/v1/agents/catalog")
  const installedQuery = $api.useQuery("get", "/v1/agents", {
    params: { query: { status: "active", limit: 100 } },
  })

  const catalogAgents =
    (catalogQuery.data as CatalogAgent[] | undefined) ?? EMPTY_CATALOG_AGENTS
  const installedAgents =
    (installedQuery.data?.data as InstalledAgent[] | undefined) ??
    EMPTY_INSTALLED_AGENTS
  const categories = useMemo(
    () => agentCategories(catalogAgents),
    [catalogAgents]
  )
  const filteredAgents = useMemo(
    () =>
      catalogAgents.filter(
        (agent) =>
          agentMatchesCategory(agent, category) &&
          agentMatchesQuery(agent, query)
      ),
    [catalogAgents, category, query]
  )
  const groupedAgents = useMemo(
    () => groupAgents(filteredAgents, categories, category),
    [categories, category, filteredAgents]
  )
  const sectionEntries = Object.entries(groupedAgents)

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-semibold text-foreground">Agents</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Install workspace agents from the catalog.
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
            placeholder="Search agents"
            className="h-10 w-full rounded-md bg-card pl-9"
          />
        </div>

        <CategorySelect
          categories={categories}
          value={category}
          onChange={setCategory}
        />
      </div>

      <InstalledAgentsSection
        agents={installedAgents}
        isLoading={installedQuery.isLoading}
      />

      {catalogQuery.isLoading ? (
        <CatalogSkeleton />
      ) : catalogQuery.isError ? (
        <ErrorState />
      ) : sectionEntries.length === 0 ? (
        <EmptyState query={query} />
      ) : (
        <div className="flex flex-col gap-8">
          {sectionEntries.map(([section, agents]) => (
            <section key={section} className="flex flex-col gap-3">
              <h2 className="text-sm font-medium text-foreground">{section}</h2>
              <div className="flex flex-col bg-card">
                {agents.map((agent, index) => (
                  <AgentRow
                    key={agentSlug(agent) || `${section}-${index}`}
                    agent={agent}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}

function InstalledAgentsSection({
  agents,
  isLoading,
}: {
  agents: InstalledAgent[]
  isLoading: boolean
}) {
  if (isLoading) {
    return (
      <section className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-foreground">
            Installed agents
          </h2>
          <span className="text-sm text-muted-foreground">Manage</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <div
              key={index}
              className="bg-default h-8 w-8 animate-pulse rounded-lg"
            />
          ))}
        </div>
      </section>
    )
  }

  if (agents.length === 0) return null

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-foreground">
          Installed agents
        </h2>
        <button
          type="button"
          className="text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          Manage
        </button>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {agents.map((agent) => (
          <div
            key={agent.id ?? agentName(agent)}
            className="flex h-8 w-8 items-center justify-center rounded-lg bg-card transition-colors hover:bg-muted/40"
            title={agentName(agent)}
          >
            <AgentAvatar agent={agent} size="sm" />
          </div>
        ))}
      </div>
    </section>
  )
}

function CategorySelect({
  categories,
  value,
  onChange,
}: {
  categories: AgentCategory[]
  value: AgentCategory
  onChange: (category: AgentCategory) => void
}) {
  return (
    <Select
      aria-label="Agent category"
      value={value}
      onChange={(key) => onChange(String(key))}
      className="w-full sm:w-52"
    >
      <Select.Trigger className="h-10 w-full justify-between rounded-md px-3 text-sm transition-colors">
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

function AgentRow({ agent }: { agent: CatalogAgent }) {
  const slug = agentSlug(agent)

  return (
    <NextLink
      href={`/w/settings/agents/${slug}`}
      className="group -mx-3 block py-1.5"
    >
      <div className="group-hover:bg-default rounded-xl px-3 py-1.5 transition-colors">
        <div className="flex items-center gap-3">
          <AgentAvatar agent={agent} size="md" />

          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              <h3 className="truncate text-sm font-medium text-foreground">
                {agentName(agent)}
              </h3>
              <span className="hidden shrink-0 text-xs text-muted-foreground sm:inline">
                {agentCategory(agent)}
              </span>
              {agentIsInstalled(agent) ? (
                <span className="bg-default hidden shrink-0 rounded-full px-2 py-0.5 text-xs text-muted-foreground sm:inline">
                  Installed
                </span>
              ) : null}
            </div>
            <p className="truncate text-sm text-muted-foreground">
              {agentDescription(agent)}
            </p>
          </div>

          <Icon
            icon="lucide:chevron-right"
            className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
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
          <div className="bg-default h-4 w-28 animate-pulse rounded" />
          <div className="flex flex-col gap-3 bg-card">
            {Array.from({ length: 3 }).map((_, row) => (
              <div key={row} className="flex items-center gap-3 py-1.5">
                <div className="bg-default h-9 w-9 animate-pulse rounded-lg" />
                <div className="min-w-0 flex-1">
                  <div className="bg-default h-4 w-40 animate-pulse rounded" />
                  <div className="bg-default mt-2 h-4 w-full max-w-sm animate-pulse rounded" />
                </div>
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function ErrorState() {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <Icon
        icon="lucide:triangle-alert"
        className="h-7 w-7 text-muted-foreground"
      />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load agents
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        Refresh the page to try again.
      </p>
    </div>
  )
}

function EmptyState({ query }: { query: string }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <Icon icon="lucide:bot" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? "No matching agents" : "No agents available"}
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {query
          ? "Try a different search or category."
          : "Browse the catalog to add agents to this workspace."}
      </p>
    </div>
  )
}
