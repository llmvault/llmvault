import type { components } from "@/lib/api/schema"

export type CatalogAutomation = components["schemas"]["CatalogItem"]
export type AutomationTab = "Triggers" | "Schedules"
export type AutomationCategory = "All" | "Featured" | string

export interface AutomationItem {
  id: string
  type: AutomationTab
  name: string
  description: string
  category: string
  icon: string
  iconColor: string
  provider: string
  featured?: boolean
  catalog: CatalogAutomation
}

export const AUTOMATION_TABS: AutomationTab[] = ["Triggers", "Schedules"]

type ProviderMeta = {
  label: string
  icon: string
  color: string
}

const PROVIDER_META: Record<string, ProviderMeta> = {
  "github-app": {
    label: "GitHub",
    icon: "simple-icons:github",
    color: "#181717",
  },
  linear: {
    label: "Linear",
    icon: "simple-icons:linear",
    color: "#5E6AD2",
  },
  railway: {
    label: "Railway",
    icon: "simple-icons:railway",
    color: "#0B0D0E",
  },
}

export function automationFromCatalog(
  item: CatalogAutomation,
  type: AutomationTab
): AutomationItem {
  const provider = item.integration?.provider || ""
  const providerMeta = PROVIDER_META[provider]
  const slug = item.slug || ""

  return {
    id: slug,
    type,
    name: item.name || humanizeSlug(slug) || "Automation",
    description: item.description || "No description available.",
    category: item.category || "Other",
    icon: providerMeta?.icon ?? "lucide:workflow",
    iconColor: providerMeta?.color ?? "#64748B",
    provider,
    catalog: item,
  }
}

export function automationCategories(
  automations: AutomationItem[]
): AutomationCategory[] {
  const categories = new Set<string>()
  let hasFeatured = false

  for (const automation of automations) {
    categories.add(automation.category)
    hasFeatured = hasFeatured || automation.featured === true
  }

  return [
    "All",
    ...(hasFeatured ? ["Featured"] : []),
    ...Array.from(categories).sort(),
  ]
}

export function automationCategory(item: AutomationItem): string {
  return item.category || "Other"
}

export function automationMatchesCategory(
  automation: AutomationItem,
  category: AutomationCategory
): boolean {
  if (category === "All") return true
  if (category === "Featured") return automation.featured === true
  return automation.category === category
}

export function automationMatchesQuery(
  automation: AutomationItem,
  query: string
): boolean {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  return [
    automation.name,
    automation.description,
    automation.category,
    automation.type,
    automationSourceLabel(automation),
    ...automationTriggerKeys(automation),
    ...automationRequiredPlugins(automation),
  ]
    .filter(Boolean)
    .some((value) => value.toLowerCase().includes(normalized))
}

export function automationInstructions(automation: AutomationItem): string {
  return automation.catalog.instructions || "No instructions available."
}

export function automationSourceLabel(automation: AutomationItem): string {
  const provider = automation.provider
  return PROVIDER_META[provider]?.label ?? humanizeSlug(provider) ?? "Workspace"
}

export function automationTriggerKeys(automation: AutomationItem): string[] {
  return automation.catalog.trigger?.keys ?? []
}

export function automationEventLabel(automation: AutomationItem): string {
  const keys = automationTriggerKeys(automation)
  return keys.length > 0 ? keys.join(", ") : automation.name
}

export function automationCadenceLabel(automation: AutomationItem): string {
  return (
    automation.catalog.schedule?.suggested_time_label ||
    automation.catalog.schedule?.cron ||
    "Custom cadence"
  )
}

export function automationTimezoneLabel(automation: AutomationItem): string {
  const timezone = automation.catalog.schedule?.timezone
  if (!timezone || timezone === "workspace") return "Workspace timezone"
  return timezone
}

export function automationDefaultAgent(automation: AutomationItem): string {
  const agent = automation.catalog.install?.default_agent
  if (!agent) return "Choose during setup"
  if (agent === "hivy") return "Hivy"
  return humanizeSlug(agent) || agent
}

export function automationDefaultChannel(automation: AutomationItem): string {
  const channel = automation.catalog.install?.default_channel
  if (!channel || channel === "workspace") return "Workspace thread"
  return humanizeSlug(channel) || channel
}

export function automationRequiredPlugins(
  automation: AutomationItem
): string[] {
  return automation.catalog.plugins?.required ?? []
}

function humanizeSlug(value: string): string {
  return value
    .split(/[-_]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}
