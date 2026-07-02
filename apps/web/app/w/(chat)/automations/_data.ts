import type { components } from "@/lib/api/schema"

export type CatalogAutomation = components["schemas"]["CatalogItem"]
export type InstalledTrigger =
  components["schemas"]["triggerAutomationResponse"]
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
  href?: string
  catalog?: CatalogAutomation
  trigger?: InstalledTrigger
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
    icon: "github",
    color: "#181717",
  },
  linear: {
    label: "Linear",
    icon: "linear",
    color: "#5E6AD2",
  },
  railway: {
    label: "Railway",
    icon: "railway",
    color: "#0B0D0E",
  },
  slack: {
    label: "Slack",
    icon: "slack",
    color: "#4A154B",
  },
}

export function automationFromCatalog(
  item: CatalogAutomation,
  type: AutomationTab,
  href?: string
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
    icon: providerMeta?.icon ?? "workflow",
    iconColor: providerMeta?.color ?? "#64748B",
    provider,
    href,
    catalog: item,
  }
}

export function automationFromInstalledTrigger(
  trigger: InstalledTrigger
): AutomationItem {
  const provider = trigger.provider || "slack"
  const providerMeta = PROVIDER_META[provider]
  const triggerKey = trigger.trigger_key || ""
  const name =
    provider === "slack" && triggerKey === "reaction_added"
      ? "React with emoji"
      : humanizeSlug(triggerKey) || "Trigger"
  const channel = trigger.external_resource_name || trigger.channel_name || ""
  const value = trigger.trigger_value ? `:${trigger.trigger_value}:` : "event"
  const agent = trigger.agent_name || "Agent"
  const statusPrefix = trigger.enabled === false ? "Disabled. " : ""

  return {
    id: trigger.id || "",
    type: "Triggers",
    name,
    description:
      provider === "slack" && triggerKey === "reaction_added"
        ? `${statusPrefix}${agent} runs when ${value} is added${channel ? ` in ${channel}` : ""}.`
        : trigger.instructions || "Installed trigger.",
    category: provider === "slack" ? "Communication" : "Other",
    icon: providerMeta?.icon ?? "workflow",
    iconColor: providerMeta?.color ?? "#64748B",
    provider,
    href: trigger.id ? `/w/automations/triggers/${trigger.id}` : undefined,
    trigger,
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
    automationTriggerKey(automation),
    ...automationRequiredPlugins(automation),
  ]
    .filter(Boolean)
    .some((value) => value.toLowerCase().includes(normalized))
}

export function automationInstructions(automation: AutomationItem): string {
  return (
    automationTriggerDefaultInstructions(automation) ||
    automation.catalog?.instructions ||
    automation.trigger?.instructions ||
    "No instructions available."
  )
}

export function automationSourceLabel(automation: AutomationItem): string {
  const provider = automation.provider
  return PROVIDER_META[provider]?.label ?? humanizeSlug(provider) ?? "Workspace"
}

export function automationTriggerKey(automation: AutomationItem): string {
  return (
    automation.catalog?.trigger?.key ?? automation.trigger?.trigger_key ?? ""
  )
}

export function automationEventLabel(automation: AutomationItem): string {
  const key = automationTriggerKey(automation)
  return key ? humanizeSlug(key) : automation.name
}

export function automationTriggerDefaultValue(
  automation: AutomationItem
): string {
  return automation.catalog?.trigger?.defaults?.value ?? ""
}

export function automationTriggerDefaultInstructions(
  automation: AutomationItem
): string {
  return automation.catalog?.trigger?.defaults?.instructions ?? ""
}

export function automationCadenceLabel(automation: AutomationItem): string {
  return (
    automation.catalog?.schedule?.suggested_time_label ||
    automation.catalog?.schedule?.cron ||
    "Custom cadence"
  )
}

export function automationTimezoneLabel(automation: AutomationItem): string {
  const timezone = automation.catalog?.schedule?.timezone
  if (!timezone || timezone === "workspace") return "Workspace timezone"
  return timezone
}

export function automationDefaultAgent(automation: AutomationItem): string {
  const agent = automation.catalog?.install?.default_agent
  if (!agent) return "Choose during setup"
  if (agent === "hivy") return "Hivy"
  return humanizeSlug(agent) || agent
}

export function automationDefaultChannel(automation: AutomationItem): string {
  const channel = automation.catalog?.install?.default_channel
  if (!channel || channel === "workspace") return "Workspace thread"
  return humanizeSlug(channel) || channel
}

export function automationRequiredPlugins(
  automation: AutomationItem
): string[] {
  return automation.catalog?.plugins?.required ?? []
}

function humanizeSlug(value: string): string {
  return value
    .split(/[-_]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}
