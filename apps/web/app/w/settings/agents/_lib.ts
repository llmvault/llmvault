import type { components } from "@/lib/api/schema"
import type { ApiPlugin } from "@/app/w/(chat)/plugins/_lib"

export type CatalogAgent = components["schemas"]["agentCatalogResponse"]
export type InstalledAgent = components["schemas"]["agentListItem"]
export type AgentPluginRequirement =
  components["schemas"]["agentCatalogPluginSummary"]
export type AgentCategory = "All" | "Featured" | string
export type AgentSandboxImage = "default" | "developer"
export type AgentSandboxSize = "nano" | "small" | "medium" | "large" | "xlarge"

export const AGENT_SANDBOX_IMAGE_OPTIONS: Array<{
  key: AgentSandboxImage
  label: string
  description: string
}> = [
  {
    key: "default",
    label: "Default",
    description: "Standard runtime for general agent work",
  },
  {
    key: "developer",
    label: "Developer",
    description: "Runtime with developer tooling preinstalled",
  },
]

export const AGENT_SANDBOX_SIZE_OPTIONS: Array<{
  key: AgentSandboxSize
  label: string
  specs: string
}> = [
  { key: "nano", label: "Nano", specs: "1 CPU / 1 GB RAM / 5 GB disk" },
  { key: "small", label: "Small", specs: "1 CPU / 2 GB RAM / 10 GB disk" },
  { key: "medium", label: "Medium", specs: "2 CPU / 4 GB RAM / 20 GB disk" },
  { key: "large", label: "Large", specs: "4 CPU / 8 GB RAM / 40 GB disk" },
  {
    key: "xlarge",
    label: "Extra large",
    specs: "8 CPU / 16 GB RAM / 60 GB disk",
  },
]

export const AGENT_CATALOG_QUERY_KEY = ["get", "/v1/agents/catalog"] as const
export const INSTALLED_AGENTS_QUERY_KEY = ["get", "/v1/agents"] as const

export function agentSlug(agent: CatalogAgent): string {
  return firstText(agent.slug, agent.id, agent.name)
}

export function agentName(agent: CatalogAgent | InstalledAgent): string {
  return firstText(agent.name, "Untitled agent")
}

export function agentDescription(agent: CatalogAgent | InstalledAgent): string {
  return firstText(agent.description, "No description available.")
}

export function agentCategory(agent: CatalogAgent): string {
  return firstText(agent.category, "General")
}

export function agentAvatarURL(
  agent: CatalogAgent | InstalledAgent
): string | undefined {
  const avatarURL = agent.avatar_url?.trim()
  if (avatarURL) return avatarURL

  if ("catalog" in agent) {
    const catalogAvatarURL = agent.catalog?.avatar_url?.trim()
    if (catalogAvatarURL) return catalogAvatarURL
  }

  return undefined
}

export function agentIsFeatured(agent: CatalogAgent): boolean {
  return Boolean(agent.official || agent.is_default)
}

export function agentIsInstalled(agent: CatalogAgent): boolean {
  return Boolean(agent.installed_agent_id)
}

// An installed agent is org-created when it has no catalog source (built via the
// create-agent form rather than installed from the catalog).
export function agentIsOrgCreated(agent: InstalledAgent): boolean {
  return !agent.catalog
}

export function agentRequiredPlugins(
  agent: CatalogAgent | undefined
): AgentPluginRequirement[] {
  return agent?.required_plugins ?? []
}

export function agentRequiredPluginSlugs(
  agent: CatalogAgent | undefined
): Set<string> {
  const slugs = new Set<string>()
  for (const plugin of agentRequiredPlugins(agent)) {
    const slug = pluginRequirementSlug(plugin)
    if (slug) slugs.add(slug)
  }
  return slugs
}

export function agentAvailableModels(
  agent: CatalogAgent | undefined
): string[] {
  const values = agent?.available_models ?? []
  const seen = new Set<string>()
  const out: string[] = []
  for (const value of values) {
    const model = value.trim()
    if (!model || seen.has(model)) continue
    seen.add(model)
    out.push(model)
  }
  const defaultModel = agent?.model?.trim()
  if (defaultModel && !seen.has(defaultModel)) {
    out.unshift(defaultModel)
  }
  return out
}

export function normalizeAgentSandboxSize(
  value: string | undefined
): AgentSandboxSize {
  return AGENT_SANDBOX_SIZE_OPTIONS.some((option) => option.key === value)
    ? (value as AgentSandboxSize)
    : "small"
}

export function normalizeAgentSandboxImage(
  value: string | undefined
): AgentSandboxImage {
  return AGENT_SANDBOX_IMAGE_OPTIONS.some((option) => option.key === value)
    ? (value as AgentSandboxImage)
    : "default"
}

export function agentMissingPlugins(
  agent: CatalogAgent | undefined
): AgentPluginRequirement[] {
  return agentRequiredPlugins(agent).filter((plugin) => !plugin.installed)
}

export function agentCanInstall(agent: CatalogAgent | undefined): boolean {
  return Boolean(
    agent && !agentIsInstalled(agent) && agentMissingPlugins(agent).length === 0
  )
}

export function pluginRequirementName(plugin: AgentPluginRequirement): string {
  return firstText(plugin.name, plugin.slug, "Plugin")
}

export function pluginRequirementSlug(
  plugin: AgentPluginRequirement
): string | undefined {
  return plugin.slug?.trim() || undefined
}

export function pluginsBySlug(plugins: ApiPlugin[]): Map<string, ApiPlugin> {
  const out = new Map<string, ApiPlugin>()
  for (const plugin of plugins) {
    const slug = plugin.slug?.trim()
    if (slug) out.set(slug, plugin)
  }
  return out
}

export function pluginForRequirement(
  requirement: AgentPluginRequirement,
  lookup: Map<string, ApiPlugin>
): ApiPlugin | undefined {
  const slug = pluginRequirementSlug(requirement)
  return slug ? lookup.get(slug) : undefined
}

export function pluginEnabledForAgent(
  plugin: ApiPlugin,
  agentID: string | undefined
): boolean {
  const id = agentID?.trim()
  if (!id) return false
  return (plugin.enabled_agent_ids ?? []).includes(id)
}

export function agentInitials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return (name.trim().slice(0, 2) || "AG").toUpperCase()
}

export function agentCategories(agents: CatalogAgent[]): AgentCategory[] {
  const categories = new Set<string>()
  for (const agent of agents) {
    categories.add(agentCategory(agent))
  }
  return ["All", "Featured", ...Array.from(categories).sort()]
}

export function agentMatchesCategory(
  agent: CatalogAgent,
  category: AgentCategory
): boolean {
  if (category === "All") return true
  if (category === "Featured") return agentIsFeatured(agent)
  return agentCategory(agent) === category
}

export function agentMatchesQuery(agent: CatalogAgent, query: string): boolean {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  return [
    agentName(agent),
    agentDescription(agent),
    agentCategory(agent),
    agent.developer ?? "",
    agent.slug ?? "",
  ].some((value) => value.toLowerCase().includes(normalized))
}

export function groupAgents(
  agents: CatalogAgent[],
  categories: AgentCategory[],
  category: AgentCategory
): Record<string, CatalogAgent[]> {
  if (category !== "All") {
    return { [category]: agents }
  }

  const groups: Record<string, CatalogAgent[]> = {}
  for (const agent of agents) {
    const section = agentCategory(agent)
    if (!groups[section]) groups[section] = []
    groups[section].push(agent)
  }

  const ordered: Record<string, CatalogAgent[]> = {}
  for (const section of categories.filter(
    (item) => item !== "All" && item !== "Featured"
  )) {
    if (groups[section]) ordered[section] = groups[section]
  }
  return ordered
}

function firstText(...values: Array<string | undefined>): string {
  for (const value of values) {
    const trimmed = value?.trim()
    if (trimmed) return trimmed
  }
  return ""
}
