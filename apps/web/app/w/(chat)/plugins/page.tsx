"use client"

import { useMemo, useState } from "react"
import Link from "next/link"
import { toast } from "sonner"
import {
  Button,
  Input,
  Popover,
  Modal,
  Badge,
  Switch,
} from "@heroui/react"
import { Icon } from "@iconify/react"
import { cn } from "@/lib/utils"

type PluginCategory =
  | "All"
  | "Featured"
  | "Business & Operations"
  | "Communication"
  | "Creativity"
  | "Data & Analytics"
  | "Developer Tools"
  | "Education & Research"
  | "Finance"
  | "Other"
  | "Productivity"
  | "Research"
  | "Security"
  | "Travel"

interface Plugin {
  id: string
  name: string
  description: string
  category: PluginCategory
  icon: string
  iconColor: string
  official?: boolean
}

const CATEGORIES: PluginCategory[] = [
  "All",
  "Featured",
  "Business & Operations",
  "Communication",
  "Creativity",
  "Data & Analytics",
  "Developer Tools",
  "Education & Research",
  "Finance",
  "Other",
  "Productivity",
  "Research",
  "Security",
  "Travel",
]

const SOURCES = [
  { id: "curated", label: "Curated by Hivy" },
  { id: "official", label: "hivy-official" },
]

const CONNECTED_APPS = [
  { id: "notion", name: "Notion", color: "#000000", icon: "simple-icons:notion" },
  { id: "linear", name: "Linear", color: "#5E6AD2", icon: "simple-icons:linear" },
  { id: "slack", name: "Slack", color: "#4A154B", icon: "simple-icons:slack" },
  { id: "github", name: "GitHub", color: "#181717", icon: "simple-icons:github" },
  { id: "hubspot", name: "HubSpot", color: "#FF7A59", icon: "simple-icons:hubspot" },
  { id: "salesforce", name: "Salesforce", color: "#00A1E0", icon: "simple-icons:salesforce" },
  { id: "attio", name: "Attio", color: "#111111", icon: "lucide:database" },
  { id: "pipedrive", name: "Pipedrive", color: "#008542", icon: "lucide:briefcase" },
]

const FEATURED_PLUGINS: Plugin[] = [
  {
    id: "product-design",
    name: "Product Design",
    description: "Explore and prototype ideas",
    category: "Featured",
    icon: "lucide:component",
    iconColor: "#A855F7",
  },
  {
    id: "creative-production",
    name: "Creative Production",
    description: "Create marketing visuals from a brief or product image.",
    category: "Featured",
    icon: "lucide:sparkles",
    iconColor: "#8B5CF6",
  },
  {
    id: "sales",
    name: "Sales",
    description: "Prepare sales work faster",
    category: "Featured",
    icon: "lucide:trending-up",
    iconColor: "#F97316",
  },
  {
    id: "investment-banking",
    name: "Investment Banking",
    description: "M&A, capital markets, LevFin, valuation, diligence, and pitch workflows",
    category: "Featured",
    icon: "lucide:landmark",
    iconColor: "#10B981",
  },
  {
    id: "public-equity",
    name: "Public Equity Investing",
    description: "Public equity PM research, long/short, earnings, ETF/index diligence, and memos",
    category: "Featured",
    icon: "lucide:bar-chart-3",
    iconColor: "#22C55E",
  },
]

const CATALOG_PLUGINS: Plugin[] = [
  {
    id: "attio",
    name: "Attio",
    description: "Attio connects Hivy directly to your CRM workspace, letting you manage customer relationships...",
    category: "Business & Operations",
    icon: "lucide:database",
    iconColor: "#111111",
  },
  {
    id: "carta-crm",
    name: "Carta CRM",
    description: "Carta CRM helps investment teams stay on top of deal flow by keeping deals, companies, and...",
    category: "Business & Operations",
    icon: "lucide:briefcase",
    iconColor: "#4B5563",
  },
  {
    id: "demandbase",
    name: "Demandbase",
    description: "Demandbase integration with Hivy gives sales, marketing, and GTM teams seamless access to...",
    category: "Business & Operations",
    icon: "lucide:target",
    iconColor: "#1D4ED8",
  },
  {
    id: "hubspot",
    name: "HubSpot",
    description: "Work with your HubSpot data to analyze patterns, create and update records, and manage your...",
    category: "Business & Operations",
    icon: "simple-icons:hubspot",
    iconColor: "#FF7A59",
  },
  {
    id: "pipedrive",
    name: "Pipedrive",
    description: "Connect to sync Pipedrive deals and contacts for use in Hivy.",
    category: "Business & Operations",
    icon: "lucide:chart-pie",
    iconColor: "#008542",
  },
  {
    id: "atlassian-jira",
    name: "Atlassian Jira",
    description: "Manage Jira issues, sprints, and projects without leaving Hivy.",
    category: "Productivity",
    icon: "simple-icons:jira",
    iconColor: "#0052CC",
  },
  {
    id: "box",
    name: "Box",
    description: "Search and summarize files stored in your Box account.",
    category: "Productivity",
    icon: "simple-icons:box",
    iconColor: "#0061D5",
  },
  {
    id: "brand24",
    name: "Brand24",
    description: "The Brand24 integration lets you monitor brand mentions and sentiment in real time.",
    category: "Productivity",
    icon: "lucide:radio",
    iconColor: "#10B981",
  },
  {
    id: "clickup",
    name: "ClickUp",
    description: "Turn Hivy chats into ClickUp tasks and keep projects moving.",
    category: "Productivity",
    icon: "simple-icons:clickup",
    iconColor: "#7B68EE",
  },
  {
    id: "latex",
    name: "LaTeX",
    description: "Compile LaTeX documents and render equations from your chats.",
    category: "Research",
    icon: "lucide:sigma",
    iconColor: "#374151",
  },
  {
    id: "codex-security",
    name: "Codex Security",
    description: "Security scanning for your codebase",
    category: "Security",
    icon: "lucide:shield-check",
    iconColor: "#2563EB",
  },
  {
    id: "finn",
    name: "FINN",
    description: "A FINN car subscription is a flexible way to stay mobile anytime - without long-term commitments...",
    category: "Travel",
    icon: "lucide:car",
    iconColor: "#F59E0B",
  },
  {
    id: "weather-promise",
    name: "WeatherPromise",
    description: "Protect your trip with WeatherPromise and get back the full cost if it rains more than promised...",
    category: "Travel",
    icon: "lucide:cloud-rain",
    iconColor: "#F97316",
  },
]

const ALL_PLUGINS = [...FEATURED_PLUGINS, ...CATALOG_PLUGINS]

const SECTION_ORDER: PluginCategory[] = [
  "Featured",
  "Business & Operations",
  "Productivity",
  "Research",
  "Security",
  "Travel",
]

export default function PluginsPage() {
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState<PluginCategory>("All")
  const [source, setSource] = useState("curated")
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null)

  const filteredPlugins = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()

    return ALL_PLUGINS.filter((plugin) => {
      const matchesCategory =
        category === "All" ||
        plugin.category === category ||
        (category === "Featured" && FEATURED_PLUGINS.includes(plugin))
      const matchesQuery =
        normalizedQuery.length === 0 ||
        plugin.name.toLowerCase().includes(normalizedQuery) ||
        plugin.description.toLowerCase().includes(normalizedQuery) ||
        plugin.category.toLowerCase().includes(normalizedQuery)
      const matchesSource =
        source === "curated" || plugin.official === true

      return matchesCategory && matchesQuery && matchesSource
    })
  }, [category, query, source])

  const groupedPlugins = useMemo(() => {
    if (category !== "All") {
      return { [category]: filteredPlugins }
    }

    const groups: Record<string, Plugin[]> = {}
    for (const plugin of filteredPlugins) {
      const section = plugin.category
      if (!groups[section]) groups[section] = []
      groups[section].push(plugin)
    }

    const ordered: Record<string, Plugin[]> = {}
    for (const section of SECTION_ORDER) {
      if (groups[section]) ordered[section] = groups[section]
    }
    for (const section of Object.keys(groups)) {
      if (!ordered[section]) ordered[section] = groups[section]
    }
    return ordered
  }, [category, filteredPlugins])

  const sectionEntries = Object.entries(groupedPlugins)

  function handleAdd(plugin: Plugin) {
    setSelectedPlugin(plugin)
  }

  function handleConnect() {
    if (!selectedPlugin) return
    toast.success(`${selectedPlugin.name} plugin added`)
    setSelectedPlugin(null)
  }

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <nav
            aria-label="Plugins and skills"
            className="flex items-center gap-1"
          >
            <Link
              href="/w/plugins"
              className="rounded-lg bg-default px-3 py-1.5 text-sm font-medium text-foreground"
            >
              Plugins
            </Link>
            <Link
              href="/w/skills"
              className="rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              Skills
            </Link>
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
                className="w-full rounded-md bg-card pl-9"
              />
            </div>

            <CategorySelect value={category} onChange={setCategory} />
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {SOURCES.map((item) => {
              const active = source === item.id
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => setSource(item.id)}
                  className={cn(
                    "rounded-full border px-3 py-1 text-sm transition-colors",
                    active
                      ? "border-border bg-card font-medium text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  )}
                >
                  {item.label}
                </button>
              )
            })}
          </div>

          <section className="flex flex-col gap-3">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-medium text-foreground">Connected</h2>
              <button
                type="button"
                className="text-sm text-muted-foreground transition-colors hover:text-foreground"
              >
                Manage
              </button>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {CONNECTED_APPS.map((app) => (
                <div
                  key={app.id}
                  className="flex h-9 w-9 items-center justify-center rounded-lg bg-card ring-1 ring-border transition-colors hover:bg-muted/40"
                  title={app.name}
                >
                  <AppIcon icon={app.icon} color={app.color} size={16} />
                </div>
              ))}
            </div>
          </section>

          {sectionEntries.length === 0 ? (
            <EmptyState query={query} />
          ) : (
            <div className="flex flex-col gap-8">
              {sectionEntries.map(([section, plugins]) => (
                <section key={section} className="flex flex-col gap-3">
                  <h2 className="text-sm font-medium text-foreground">{section}</h2>
                  <div className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
                    {plugins.map((plugin) => (
                      <PluginRow
                        key={plugin.id}
                        plugin={plugin}
                        onAdd={() => handleAdd(plugin)}
                      />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          )}
        </div>
      </div>

      <ConnectModal
        plugin={selectedPlugin}
        onClose={() => setSelectedPlugin(null)}
        onConnect={handleConnect}
      />
    </div>
  )
}

function CategorySelect({
  value,
  onChange,
}: {
  value: PluginCategory
  onChange: (category: PluginCategory) => void
}) {
  const [open, setOpen] = useState(false)

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`Category: ${value}`}
        className="flex h-10 w-full items-center justify-between rounded-md border border-border bg-card px-3 text-sm text-foreground transition-colors hover:bg-muted/20 sm:w-48"
      >
        <span>{value}</span>
        <Icon icon="lucide:chevron-down" className="h-4 w-4 text-muted-foreground" />
      </Popover.Trigger>
      <Popover.Content className="w-48 rounded-xl border border-border p-1.5">
        <Popover.Dialog className="flex max-h-72 w-full flex-col gap-0.5 overflow-y-auto p-0">
          {CATEGORIES.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => {
                onChange(item)
                setOpen(false)
              }}
              className={cn(
                "flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-sm transition-colors",
                item === value
                  ? "bg-default font-medium text-foreground"
                  : "text-muted-foreground hover:bg-muted/20 hover:text-foreground"
              )}
            >
              <span className="min-w-0 flex-1">{item}</span>
              {item === value ? (
                <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
              ) : null}
            </button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function PluginRow({
  plugin,
  onAdd,
}: {
  plugin: Plugin
  onAdd: () => void
}) {
  return (
    <div className="flex items-center gap-3 p-3 transition-colors hover:bg-muted/20">
      <div
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-white"
        style={{ backgroundColor: plugin.iconColor }}
      >
        <AppIcon icon={plugin.icon} color="#FFFFFF" size={16} />
      </div>

      <div className="min-w-0 flex-1">
        <h3 className="text-sm font-medium text-foreground">{plugin.name}</h3>
        <p className="text-sm text-muted-foreground">{plugin.description}</p>
      </div>

      <Button
        variant="outline"
        size="sm"
        className="shrink-0 rounded-full"
        onPress={onAdd}
      >
        Add
      </Button>
    </div>
  )
}

function AppIcon({ icon, color, size = 20 }: { icon: string; color: string; size?: number }) {
  return <Icon icon={icon} className="shrink-0" style={{ color, width: size, height: size }} />
}

function EmptyState({ query }: { query: string }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl border border-border bg-card px-6 text-center">
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

function ConnectModal({
  plugin,
  onClose,
  onConnect,
}: {
  plugin: Plugin | null
  onClose: () => void
  onConnect: () => void
}) {
  const [referenceMemory, setReferenceMemory] = useState(true)
  const open = plugin !== null

  return (
    <Modal isOpen={open} onOpenChange={(next) => !next && onClose()}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="w-full max-w-md rounded-2xl border border-border bg-background p-0 shadow-xl outline-none">
            {plugin ? (
              <div className="flex flex-col gap-5 p-6">
                <div className="flex items-center justify-between gap-4">
                  <div className="flex items-center gap-3">
                    <div
                      className="flex h-10 w-10 items-center justify-center rounded-lg text-white"
                      style={{ backgroundColor: plugin.iconColor }}
                    >
                      <AppIcon icon={plugin.icon} color="#FFFFFF" size={18} />
                    </div>
                    <Icon icon="lucide:arrow-right" className="h-4 w-4 text-muted-foreground" />
                    <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-foreground text-background">
                      <Icon icon="lucide:bot" className="h-5 w-5" />
                    </div>
                  </div>
                  <Button variant="ghost" isIconOnly onPress={onClose} aria-label="Close">
                    <Icon icon="lucide:x" className="h-4 w-4" />
                  </Button>
                </div>

                <div className="text-center">
                  <h2 className="text-xl font-semibold text-foreground">
                    Connect {plugin.name}
                  </h2>
                  <Badge color="success" variant="soft" className="mt-2">
                    <Icon icon="lucide:check" className="mr-1 h-3 w-3" />
                    Approved by your admin
                  </Badge>
                </div>

                <div className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4">
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <h3 className="text-sm font-medium text-foreground">
                        Reference memories and chats
                      </h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        Allow Hivy to reference relevant chats and memories when
                        sharing data with {plugin.name} for more helpful responses.
                      </p>
                    </div>
                    <Switch
                      isSelected={referenceMemory}
                      onChange={setReferenceMemory}
                      className="shrink-0"
                    >
                      <Switch.Control>
                        <Switch.Thumb />
                      </Switch.Control>
                    </Switch>
                  </div>
                </div>

                <div className="flex flex-col gap-4 text-sm text-muted-foreground">
                  <div>
                    <h3 className="font-medium text-foreground">You&apos;re in control</h3>
                    <p className="mt-1">
                      Hivy always respects your training data preferences, and is
                      limited to permissions you&apos;ve explicitly set.
                    </p>
                  </div>
                  <div>
                    <h3 className="font-medium text-foreground">
                      Apps may introduce elevated risk
                    </h3>
                    <p className="mt-1">
                      Hivy is built to protect your data, but attackers may attempt to
                      use Hivy to access your data in the app, or use the app to
                      attempt to access your data in Hivy.
                    </p>
                  </div>
                  <div>
                    <h3 className="font-medium text-foreground">Data shared with this app</h3>
                    <p className="mt-1">
                      By adding this app, you allow it to access basic information
                      typically shared when you visit a website, such as your IP address
                      and approximate location, and a summary of your recent context
                      and intent within Hivy.
                    </p>
                  </div>
                </div>

                <Button fullWidth className="rounded-full" onPress={onConnect}>
                  Continue to {plugin.name}
                  <Icon icon="lucide:arrow-up-right" className="ml-1 h-4 w-4" />
                </Button>

                <button
                  type="button"
                  className="text-center text-sm text-muted-foreground transition-colors hover:text-foreground"
                >
                  Advanced settings (opens hivy.com)
                </button>
              </div>
            ) : null}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
