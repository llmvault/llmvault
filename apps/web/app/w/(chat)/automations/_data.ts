import cronstrue from "cronstrue"
import type { components } from "@/lib/api/schema"
import {
  githubCodeReviewsProvider,
  githubPrOpenedKey,
  isGithubMentionKey,
  isGithubMentionProvider,
} from "@/app/w/(chat)/automations/_trigger-install-form-shared"

export type CatalogAutomation = components["schemas"]["CatalogItem"]
export type InstalledTrigger =
  components["schemas"]["triggerAutomationResponse"]
export type AutomationTab = "Triggers" | "Schedules" | "Webhooks"
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

const AUTOMATION_TABS: AutomationTab[] = ["Triggers", "Schedules"]

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
  "github-app-code-reviews": {
    label: "GitHub Code Reviews",
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
  const isSlackReaction = provider === "slack" && triggerKey === "reaction_added"
  const isGithubCodeReview =
    provider === githubCodeReviewsProvider && isGithubMentionKey(triggerKey)
  const isGithubMention =
    isGithubMentionProvider(provider) && isGithubMentionKey(triggerKey)
  // The code-reviews app's auto-review trigger fires on every PR open, so it
  // renders with review copy rather than the @mention copy.
  const isGithubPrOpened =
    provider === githubCodeReviewsProvider && triggerKey === githubPrOpenedKey
  const mentionBot = isGithubCodeReview ? "@usehivy-reviews" : "Hivy"
  const name =
    trigger.name?.trim() ||
    (isSlackReaction
      ? "React with emoji"
      : isGithubPrOpened
        ? "Reviews new pull requests"
        : isGithubCodeReview
          ? "Code review requested"
          : isGithubMention
            ? "Mentioned on GitHub"
            : humanizeSlug(triggerKey) || "Trigger")
  const channel = trigger.external_resource_name || trigger.channel_name || ""
  const value = trigger.trigger_value ? `:${trigger.trigger_value}:` : "event"
  const agent = trigger.agent_name || "Agent"
  const statusPrefix = trigger.enabled === false ? "Disabled. " : ""
  const repo = trigger.external_resource_key || channel

  return {
    id: trigger.id || "",
    type: "Triggers",
    name,
    description: isSlackReaction
      ? `${statusPrefix}${agent} runs when ${value} is added${channel ? ` in ${channel}` : ""}.`
      : isGithubPrOpened
        ? `${statusPrefix}${agent} reviews new pull requests${repo ? ` in ${repo}` : ""}.`
        : isGithubMention
          ? `${statusPrefix}${agent} runs when ${mentionBot} is @mentioned${repo ? ` in ${repo}` : ""}.`
          : trigger.instructions || "Installed trigger.",
    category:
      isSlackReaction
        ? "Communication"
        : isGithubMention || isGithubPrOpened
          ? "Development"
          : "Other",
    icon: providerMeta?.icon ?? "workflow",
    iconColor: providerMeta?.color ?? "#64748B",
    provider,
    href: trigger.id ? `/w/automations/triggers/${trigger.id}` : undefined,
    trigger,
  }
}

export function automationFromWebhookTrigger(
  trigger: InstalledTrigger
): AutomationItem {
  const agent = trigger.agent_name || "Agent"
  const statusPrefix = trigger.enabled === false ? "Disabled. " : ""
  const channel = trigger.channel_name ? ` in ${trigger.channel_name}` : ""

  return {
    id: trigger.id || "",
    type: "Webhooks",
    name: trigger.name?.trim() || agent,
    description:
      trigger.instructions?.trim() ||
      `${statusPrefix}Runs ${agent}${channel} when the webhook URL is called.`,
    category: "Webhooks",
    icon: "globe",
    iconColor: "#0EA5E9",
    provider: "",
    href: trigger.id ? `/w/automations/webhooks/${trigger.id}` : undefined,
    trigger,
  }
}

export type ScheduleItem = components["schemas"]["scheduleResponse"]

export function describeCron(expression: string): string {
  try {
    const text = cronstrue.toString(expression, {
      use24HourTimeFormat: false,
      verbose: false,
    })
    return `${text} (UTC)`
  } catch {
    return `Cron: ${expression}`
  }
}

function scheduleCadenceLabel(schedule: ScheduleItem): string {
  if (schedule.schedule_kind === "cron" && schedule.cron_expression) {
    return describeCron(schedule.cron_expression)
  }
  const seconds = schedule.interval_seconds ?? 0
  if (seconds > 0) {
    if (seconds % 86400 === 0) {
      const days = seconds / 86400
      return `Every ${days} day${days === 1 ? "" : "s"}`
    }
    if (seconds % 3600 === 0) {
      const hours = seconds / 3600
      return `Every ${hours} hour${hours === 1 ? "" : "s"}`
    }
    if (seconds % 60 === 0) return `Every ${seconds / 60} min`
    return `Every ${seconds}s`
  }
  return "Recurring"
}

export function automationFromSchedule(schedule: ScheduleItem): AutomationItem {
  const agent = schedule.agent_name || "Agent"
  const paused = schedule.status === "paused"
  return {
    id: schedule.id || "",
    type: "Schedules",
    name: schedule.name?.trim() || agent,
    description: `${paused ? "Paused. " : ""}${scheduleCadenceLabel(schedule)} · ${agent}`,
    category: "Schedules",
    icon: "calendar",
    iconColor: "#8B5CF6",
    provider: "",
    href: schedule.id ? `/w/automations/schedules/${schedule.id}` : undefined,
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
