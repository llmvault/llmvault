"use client"

import { useMemo, useState } from "react"
import { Avatar, Button, Input, ListBox, Select, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { cn } from "@/lib/utils"

type AgentCategory = "All" | "Featured" | string

interface CatalogAgent {
  id: string
  name: string
  description: string
  category: string
  avatarUrl?: string
  icon: string
  iconColor: string
  featured?: boolean
  installed?: boolean
}

const CATALOG_AGENTS: CatalogAgent[] = [
  {
    id: "hivy",
    name: "Hivy",
    description:
      "Triage work, answer questions, and route requests across the workspace.",
    category: "Workspace",
    avatarUrl: "/assets/hivy.png",
    icon: "lucide:sparkles",
    iconColor: "#6D5EF7",
    featured: true,
    installed: true,
  },
  {
    id: "hakaree",
    name: "Hakaree",
    description: "Coding, devops, infrastructure and debugging specialist.",
    category: "Developer Tools",
    avatarUrl: "/assets/hakaree.png",
    icon: "lucide:code-xml",
    iconColor: "#2563EB",
    featured: true,
  },
  {
    id: "software-engineer",
    name: "Software Engineer",
    description:
      "Build features, fix bugs, run tests, and prepare pull requests from a sandbox.",
    category: "Developer Tools",
    icon: "lucide:code-xml",
    iconColor: "#2563EB",
    featured: true,
  },
  {
    id: "support-engineer",
    name: "Support Engineer",
    description:
      "Investigate customer issues, inspect connected tools, and draft replies.",
    category: "Customer Support",
    icon: "lucide:headset",
    iconColor: "#0F766E",
    featured: true,
  },
  {
    id: "research-analyst",
    name: "Research Analyst",
    description:
      "Search sources, compare findings, and write concise research briefs.",
    category: "Research",
    icon: "lucide:telescope",
    iconColor: "#7C3AED",
  },
  {
    id: "data-analyst",
    name: "Data Analyst",
    description:
      "Query connected data, explain metric changes, and produce charts.",
    category: "Data & Analytics",
    icon: "lucide:chart-spline",
    iconColor: "#0891B2",
  },
  {
    id: "sales-assistant",
    name: "Sales Assistant",
    description:
      "Prepare account summaries, draft follow-ups, and organize deal next steps.",
    category: "Business & Operations",
    icon: "lucide:trending-up",
    iconColor: "#EA580C",
  },
  {
    id: "content-writer",
    name: "Content Writer",
    description:
      "Draft docs, updates, and launch copy from briefs and workspace context.",
    category: "Creativity",
    icon: "lucide:pen-line",
    iconColor: "#DB2777",
  },
]

export default function AgentsSettingsPage() {
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<AgentCategory>("All")
  const [installedAgentIDs, setInstalledAgentIDs] = useState(
    () =>
      new Set(
        CATALOG_AGENTS.filter((agent) => agent.installed).map(
          (agent) => agent.id
        )
      )
  )

  const categories = useMemo(() => agentCategories(CATALOG_AGENTS), [])
  const installedAgents = useMemo(
    () => CATALOG_AGENTS.filter((agent) => installedAgentIDs.has(agent.id)),
    [installedAgentIDs]
  )
  const filteredAgents = useMemo(
    () =>
      CATALOG_AGENTS.filter(
        (agent) =>
          agentMatchesCategory(agent, category) &&
          agentMatchesQuery(agent, query)
      ),
    [category, query]
  )
  const groupedAgents = useMemo(
    () => groupAgents(filteredAgents, categories, category),
    [categories, category, filteredAgents]
  )
  const sectionEntries = Object.entries(groupedAgents)

  function handleInstall(agent: CatalogAgent) {
    if (installedAgentIDs.has(agent.id)) return
    setInstalledAgentIDs((current) => new Set(current).add(agent.id))
    toast.success(`${agent.name} agent installed`)
  }

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

      {installedAgents.length > 0 ? (
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
            {installedAgents.map((agent) => (
              <div
                key={agent.id}
                className="flex h-8 w-8 items-center justify-center rounded-lg bg-card transition-colors hover:bg-muted/40"
                title={agent.name}
              >
                <AgentAvatar agent={agent} size="sm" />
              </div>
            ))}
          </div>
        </section>
      ) : null}

      {sectionEntries.length === 0 ? (
        <EmptyState query={query} />
      ) : (
        <div className="flex flex-col gap-8">
          {sectionEntries.map(([section, agents]) => (
            <section key={section} className="flex flex-col gap-3">
              <h2 className="text-sm font-medium text-foreground">{section}</h2>
              <div className="flex flex-col bg-card">
                {agents.map((agent) => (
                  <AgentRow
                    key={agent.id}
                    agent={agent}
                    installed={installedAgentIDs.has(agent.id)}
                    onInstall={() => handleInstall(agent)}
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

function AgentRow({
  agent,
  installed,
  onInstall,
}: {
  agent: CatalogAgent
  installed: boolean
  onInstall: () => void
}) {
  return (
    <div className="group -mx-3 py-1.5">
      <div className="group-hover:bg-default rounded-xl px-3 py-1.5 transition-colors">
        <div className="flex items-center gap-3">
          <AgentAvatar agent={agent} size="md" />

          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              <h3 className="truncate text-sm font-medium text-foreground">
                {agent.name}
              </h3>
              <span className="hidden shrink-0 text-xs text-muted-foreground sm:inline">
                {agent.category}
              </span>
            </div>
            <p className="truncate text-sm text-muted-foreground">
              {agent.description}
            </p>
          </div>

          <Button
            variant="tertiary"
            size="sm"
            className="shrink-0 rounded-full"
            isDisabled={installed}
            onPress={onInstall}
          >
            {installed ? "Installed" : "Install"}
          </Button>
        </div>
      </div>
    </div>
  )
}

function AgentAvatar({
  agent,
  size,
}: {
  agent: CatalogAgent
  size: "sm" | "md"
}) {
  const dimension = size === "sm" ? "h-6 w-6 rounded-md" : "h-9 w-9 rounded-lg"
  const iconSize = size === "sm" ? "h-3.5 w-3.5" : "h-[18px] w-[18px]"

  if (agent.avatarUrl) {
    return (
      <Avatar size={size === "sm" ? "sm" : "md"} className="shrink-0">
        <Avatar.Image src={agent.avatarUrl} />
        <Avatar.Fallback>{initials(agent.name)}</Avatar.Fallback>
      </Avatar>
    )
  }

  return (
    <div
      className={cn(
        "flex shrink-0 items-center justify-center text-white",
        dimension
      )}
      style={{ backgroundColor: agent.iconColor }}
    >
      <Icon icon={agent.icon} className={iconSize} />
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

function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return (name.trim().slice(0, 2) || "AG").toUpperCase()
}

function agentCategories(agents: CatalogAgent[]): AgentCategory[] {
  const categories = new Set<string>()
  for (const agent of agents) {
    categories.add(agent.category)
  }
  return ["All", "Featured", ...Array.from(categories).sort()]
}

function agentMatchesCategory(agent: CatalogAgent, category: AgentCategory) {
  if (category === "All") return true
  if (category === "Featured") return agent.featured === true
  return agent.category === category
}

function agentMatchesQuery(agent: CatalogAgent, query: string) {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  return [agent.name, agent.description, agent.category].some((value) =>
    value.toLowerCase().includes(normalized)
  )
}

function groupAgents(
  agents: CatalogAgent[],
  categories: AgentCategory[],
  category: AgentCategory
) {
  if (category !== "All") {
    return { [category]: agents }
  }

  const groups: Record<string, CatalogAgent[]> = {}
  for (const agent of agents) {
    if (!groups[agent.category]) groups[agent.category] = []
    groups[agent.category].push(agent)
  }

  const ordered: Record<string, CatalogAgent[]> = {}
  for (const section of categories.filter(
    (item) => item !== "All" && item !== "Featured"
  )) {
    if (groups[section]) ordered[section] = groups[section]
  }
  return ordered
}
