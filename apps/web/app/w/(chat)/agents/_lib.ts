import type { components } from "@/lib/api/schema"
import type { ApiPlugin } from "@/app/w/(chat)/plugins/_lib"
import { extractErrorMessage } from "@/lib/api/error"

export type PluginMCPToolDeny = components["schemas"]["PluginMCPToolDeny"]

export function withPluginMCPToolDenied(
  current: PluginMCPToolDeny,
  pluginID: string,
  tool: string,
  denied: boolean
): PluginMCPToolDeny {
  const next: PluginMCPToolDeny = { ...current }
  const tools = new Set(current[pluginID] ?? [])
  if (denied) tools.add(tool)
  else tools.delete(tool)
  const normalized = Array.from(tools).sort()
  if (normalized.length > 0) next[pluginID] = normalized
  else delete next[pluginID]
  return next
}

export type CatalogAgent = components["schemas"]["agentCatalogResponse"]
export type InstalledAgent = components["schemas"]["agentListItem"]
export type AgentPluginRequirement =
  components["schemas"]["agentCatalogPluginSummary"]
export type Team = components["schemas"]["teamResponse"]
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

function agentCategory(agent: CatalogAgent): string {
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

function agentIsFeatured(agent: CatalogAgent): boolean {
  return Boolean(agent.official || agent.is_default)
}

export function agentIsInstalled(agent: CatalogAgent): boolean {
  return (
    Boolean(agent.installed_agent_id) ||
    agentInstalledTeamIDs(agent).length > 0
  )
}

// installed_team_ids lists the caller's visible teams that already have a
// team-scoped clone of this catalog agent (one clone per team).
export function agentInstalledTeamIDs(agent: CatalogAgent | undefined): string[] {
  return (agent?.installed_team_ids ?? []).filter(
    (id): id is string => typeof id === "string" && id.trim().length > 0
  )
}

export function teamHasAgent(
  agent: CatalogAgent | undefined,
  teamID: string
): boolean {
  return agentInstalledTeamIDs(agent).includes(teamID)
}

export function teamNameByID(teams: Team[], id: string): string {
  return firstText(
    teams.find((team) => team.id === id)?.name,
    id,
    "your team"
  )
}

// Teams (of the caller's visible teams) that already have a clone.
export function installedTeamsFor(
  teams: Team[],
  agent: CatalogAgent | undefined
): Team[] {
  return teams.filter((team) => team.id && teamHasAgent(agent, team.id))
}

// Teams the caller could still install this agent into.
export function availableTeamsFor(
  teams: Team[],
  agent: CatalogAgent | undefined
): Team[] {
  return teams.filter((team) => team.id && !teamHasAgent(agent, team.id))
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

// Maps plugin slug -> friendly display name using the catalog agent's own
// required/recommended plugin summaries. The 422 install response only carries
// slugs, so this recovers a human name where the catalog knows one.
function agentPluginNameBySlug(
  agent: CatalogAgent | undefined
): Map<string, string> {
  const map = new Map<string, string>()
  const summaries = [
    ...agentRequiredPlugins(agent),
    ...(agent?.recommended_plugins ?? []),
  ]
  for (const plugin of summaries) {
    const slug = pluginRequirementSlug(plugin)
    if (slug && !map.has(slug)) map.set(slug, pluginRequirementName(plugin))
  }
  return map
}

// Reads the `missing_plugins` slug list off a 422 install error body.
export function missingPluginSlugsFromError(error: unknown): string[] {
  if (!isErrorRecord(error)) return []
  const value = error.missing_plugins
  if (!Array.isArray(value)) return []
  return value.filter(
    (slug): slug is string => typeof slug === "string" && slug.trim().length > 0
  )
}

// Non-generic copy for the missing-plugins block, naming the target team and
// each plugin (friendly name when known, else the slug).
export function formatMissingPluginsMessage(
  teamLabel: string,
  slugs: string[],
  agent: CatalogAgent | undefined
): string {
  const lookup = agentPluginNameBySlug(agent)
  const names = slugs.map((slug) => lookup.get(slug) ?? slug)
  const noun = names.length === 1 ? "plugin" : "plugins"
  return `${teamLabel} can't use this agent yet — it needs the ${formatNameList(
    names
  )} ${noun}. Ask an org admin to enable it for ${teamLabel}.`
}

// Turns an install mutation error into user-facing copy: missing-plugins block,
// not-a-member (403), or the backend message / a fallback.
export function installErrorMessage(
  error: unknown,
  teamLabel: string,
  agent: CatalogAgent | undefined
): string {
  const missing = missingPluginSlugsFromError(error)
  if (missing.length > 0) {
    return formatMissingPluginsMessage(teamLabel, missing, agent)
  }
  const message = extractErrorMessage(error, "")
  if (/\bmember\b/i.test(message)) {
    return "You're not a member of that team."
  }
  return message || "Could not install agent"
}

function formatNameList(names: string[]): string {
  if (names.length <= 1) return names[0] ?? ""
  if (names.length === 2) return `${names[0]} and ${names[1]}`
  return `${names.slice(0, -1).join(", ")}, and ${names[names.length - 1]}`
}

function isErrorRecord(
  value: unknown
): value is { missing_plugins?: unknown; error?: unknown } {
  return value !== null && typeof value === "object" && !Array.isArray(value)
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
