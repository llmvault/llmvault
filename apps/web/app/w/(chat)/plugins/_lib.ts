import type { components } from "@/lib/api/schema"

export type ApiPlugin = components["schemas"]["pluginResponse"]
export type PluginRequirement =
  components["schemas"]["pluginConnectionRequirement"]
export type PluginResourceRequirement =
  components["schemas"]["pluginResourceRequirement"]

export type PluginCategory = "All" | "Featured" | string

const BASE_CATEGORIES: PluginCategory[] = ["All", "Featured"]

export const PLUGINS_QUERY_KEY = ["get", "/v1/plugins"]

export function pluginSlug(plugin: ApiPlugin): string {
  return plugin.slug || plugin.id || ""
}

export function pluginCategory(plugin: ApiPlugin): string {
  return plugin.category || "Other"
}

function pluginDetailCategory(plugin: ApiPlugin): string {
  return plugin.detail_category || plugin.category || "Other"
}

export function pluginIcon(plugin: ApiPlugin): string {
  return plugin.icon || "plug"
}

export function pluginIconColor(plugin: ApiPlugin): string {
  return plugin.icon_color || "#18181B"
}

function pluginDeveloper(plugin: ApiPlugin): string {
  return plugin.developer || "Hivy"
}

export function pluginDescription(plugin: ApiPlugin): string {
  return plugin.description || "No description available."
}

export function pluginName(plugin: ApiPlugin): string {
  return plugin.name || pluginSlug(plugin) || "Plugin"
}

function pluginLongDescription(plugin: ApiPlugin): string {
  return plugin.long_description || pluginDescription(plugin)
}

function pluginCapabilities(plugin: ApiPlugin): string[] {
  return plugin.capabilities?.length ? plugin.capabilities : ["Read"]
}

export function pluginLogoProvider(plugin: ApiPlugin): string | null {
  const required = pluginRequiredConnections(plugin)
  if (required.length !== 1) return null
  return required[0].provider || null
}

export function pluginRequiredConnections(
  plugin: ApiPlugin
): PluginRequirement[] {
  return (plugin.required_connections ?? []).filter(
    (connection) => connection.required !== false
  )
}

export function pluginShownRequiredConnections(
  plugin: ApiPlugin
): PluginRequirement[] {
  const required = pluginRequiredConnections(plugin)
  return required.length > 0 ? required : pluginMissingRequirements(plugin)
}

function pluginRequiredIntegrationProvider(
  plugin: ApiPlugin
): string | null {
  const required = pluginRequiredConnections(plugin)
  if (
    required.length !== 1 ||
    pluginConnectionKind(required[0]) !== "integration"
  ) {
    return null
  }
  return required[0].provider || null
}

function pluginRequiredDatabaseProvider(
  plugin: ApiPlugin
): string | null {
  const required = pluginRequiredConnections(plugin)
  if (
    required.length !== 1 ||
    pluginConnectionKind(required[0]) !== "database"
  ) {
    return null
  }
  return required[0].provider || null
}

export function pluginConnectionKind(requirement: PluginRequirement): string {
  return requirement.kind || "integration"
}

export function pluginMissingRequirements(
  plugin: ApiPlugin
): PluginRequirement[] {
  return plugin.missing_requirements ?? []
}

function pluginResourceRequirements(
  plugin: ApiPlugin
): PluginResourceRequirement[] {
  return plugin.resource_requirements ?? []
}

export function pluginMissingResourceRequirements(
  plugin: ApiPlugin
): PluginResourceRequirement[] {
  return pluginResourceRequirements(plugin).filter(
    (requirement) => requirement.missing === true && !!requirement.connection_id
  )
}

export function pluginHasMissingResourceRequirements(
  plugin: ApiPlugin
): boolean {
  return pluginMissingResourceRequirements(plugin).length > 0
}

export function pluginResourceDisplayName(
  requirement: PluginResourceRequirement
): string {
  return requirement.display_name || requirement.resource_key || "resources"
}

export function pluginResourceSelectLabel(
  requirement: PluginResourceRequirement
): string {
  return `Select ${pluginResourceDisplayName(requirement).toLowerCase()}`
}

function pluginNextMissingRequirement(
  plugin: ApiPlugin
): PluginRequirement | null {
  return (
    pluginMissingRequirements(plugin).find(
      (requirement) => requirement.required !== false
    ) ?? null
  )
}

export function pluginRequirementKind(
  requirement: PluginRequirement | null | undefined
): string {
  return requirement?.kind || "integration"
}

export function pluginCanInstall(plugin: ApiPlugin): boolean {
  return pluginMissingRequirements(plugin).length === 0
}

export function pluginCategories(plugins: ApiPlugin[]): PluginCategory[] {
  const categories = new Set<string>()
  for (const plugin of plugins) {
    categories.add(pluginCategory(plugin))
  }
  return [...BASE_CATEGORIES, ...Array.from(categories).sort()]
}

export function pluginMatchesQuery(plugin: ApiPlugin, query: string): boolean {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  return [
    plugin.name,
    plugin.description,
    plugin.category,
    plugin.detail_category,
    plugin.developer,
    ...(plugin.skills ?? []).map((skill) => skill.name),
  ]
    .filter(Boolean)
    .some((value) => value?.toLowerCase().includes(normalized))
}

export function pluginMatchesCategory(
  plugin: ApiPlugin,
  category: PluginCategory
): boolean {
  if (category === "All") return true
  if (category === "Featured") return plugin.featured === true
  return plugin.category === category || plugin.detail_category === category
}
