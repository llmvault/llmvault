export type AutomationTab = "Triggers" | "Schedules"

export type AutomationCategory =
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

export interface AutomationItem {
  id: string
  type: AutomationTab
  name: string
  description: string
  category: Exclude<AutomationCategory, "All" | "Featured">
  icon: string
  iconColor: string
  featured?: boolean
}

export const AUTOMATION_TABS: AutomationTab[] = ["Triggers", "Schedules"]

export const BASE_AUTOMATION_CATEGORIES: AutomationCategory[] = [
  "All",
  "Featured",
]

export const STATIC_AUTOMATIONS: AutomationItem[] = [
  {
    id: "slack-mention",
    type: "Triggers",
    name: "Slack mention",
    description: "Start when Hivy is mentioned in a selected Slack channel",
    category: "Communication",
    icon: "simple-icons:slack",
    iconColor: "#4A154B",
    featured: true,
  },
  {
    id: "pull-request-review",
    type: "Triggers",
    name: "Pull request review",
    description: "Start when a GitHub pull request needs agent review",
    category: "Developer Tools",
    icon: "simple-icons:github",
    iconColor: "#181717",
    featured: true,
  },
  {
    id: "deal-stage-changed",
    type: "Triggers",
    name: "Deal stage changed",
    description: "Start when an opportunity moves to a tracked CRM stage",
    category: "Business & Operations",
    icon: "lucide:briefcase-business",
    iconColor: "#F97316",
    featured: true,
  },
  {
    id: "new-calendar-event",
    type: "Triggers",
    name: "Calendar event created",
    description: "Prepare context when a new meeting appears on the calendar",
    category: "Productivity",
    icon: "lucide:calendar-plus",
    iconColor: "#2563EB",
  },
  {
    id: "invoice-paid",
    type: "Triggers",
    name: "Invoice paid",
    description: "Follow up when a customer payment is confirmed",
    category: "Finance",
    icon: "lucide:receipt-text",
    iconColor: "#10B981",
  },
  {
    id: "metric-threshold",
    type: "Triggers",
    name: "Metric threshold crossed",
    description: "Investigate when a dashboard metric moves outside range",
    category: "Data & Analytics",
    icon: "lucide:chart-no-axes-combined",
    iconColor: "#0EA5E9",
  },
  {
    id: "design-file-updated",
    type: "Triggers",
    name: "Design file updated",
    description: "Summarize changes when a tracked design file changes",
    category: "Creativity",
    icon: "lucide:palette",
    iconColor: "#A855F7",
  },
  {
    id: "research-source-saved",
    type: "Triggers",
    name: "Research source saved",
    description: "Extract notes from a newly saved article or paper",
    category: "Research",
    icon: "lucide:book-open-text",
    iconColor: "#8B5CF6",
  },
  {
    id: "course-roster-updated",
    type: "Triggers",
    name: "Roster updated",
    description: "Check assignments when a learning workspace roster changes",
    category: "Education & Research",
    icon: "lucide:graduation-cap",
    iconColor: "#6366F1",
  },
  {
    id: "security-alert",
    type: "Triggers",
    name: "Security alert",
    description: "Open an investigation when a monitored alert is raised",
    category: "Security",
    icon: "lucide:shield-alert",
    iconColor: "#DC2626",
  },
  {
    id: "trip-booking-confirmed",
    type: "Triggers",
    name: "Trip booking confirmed",
    description: "Collect itinerary details after travel confirmation arrives",
    category: "Travel",
    icon: "lucide:plane",
    iconColor: "#0891B2",
  },
  {
    id: "webhook-received",
    type: "Triggers",
    name: "Webhook received",
    description: "Start from a custom event sent by another workspace tool",
    category: "Other",
    icon: "lucide:webhook",
    iconColor: "#64748B",
  },
  {
    id: "support-ticket-created",
    type: "Triggers",
    name: "Support ticket created",
    description: "Route a new customer issue to the right follow-up workflow",
    category: "Business & Operations",
    icon: "lucide:ticket-check",
    iconColor: "#F97316",
  },
  {
    id: "new-channel-message",
    type: "Triggers",
    name: "Channel message posted",
    description: "Summarize or triage a new message in a watched channel",
    category: "Communication",
    icon: "lucide:message-circle-more",
    iconColor: "#4A154B",
  },
  {
    id: "campaign-asset-approved",
    type: "Triggers",
    name: "Campaign asset approved",
    description: "Prepare launch tasks when a creative asset is approved",
    category: "Creativity",
    icon: "lucide:badge-check",
    iconColor: "#A855F7",
  },
  {
    id: "report-refresh-completed",
    type: "Triggers",
    name: "Report refresh completed",
    description: "Review key movements after a dashboard data refresh finishes",
    category: "Data & Analytics",
    icon: "lucide:database-zap",
    iconColor: "#0EA5E9",
  },
  {
    id: "build-failed",
    type: "Triggers",
    name: "Build failed",
    description:
      "Investigate a failed deployment or CI run as soon as it lands",
    category: "Developer Tools",
    icon: "lucide:circle-x",
    iconColor: "#181717",
  },
  {
    id: "assignment-submitted",
    type: "Triggers",
    name: "Assignment submitted",
    description: "Review a new learner submission and draft feedback notes",
    category: "Education & Research",
    icon: "lucide:clipboard-check",
    iconColor: "#6366F1",
  },
  {
    id: "expense-submitted",
    type: "Triggers",
    name: "Expense submitted",
    description:
      "Check a new expense report for missing details or policy flags",
    category: "Finance",
    icon: "lucide:wallet-cards",
    iconColor: "#10B981",
  },
  {
    id: "workspace-form-submitted",
    type: "Triggers",
    name: "Form submitted",
    description: "Start a custom workflow from a submitted intake form",
    category: "Other",
    icon: "lucide:clipboard-list",
    iconColor: "#64748B",
  },
  {
    id: "task-status-changed",
    type: "Triggers",
    name: "Task status changed",
    description: "Update related work when a tracked task changes state",
    category: "Productivity",
    icon: "lucide:list-checks",
    iconColor: "#2563EB",
  },
  {
    id: "saved-search-updated",
    type: "Triggers",
    name: "Saved search updated",
    description: "Scan new results when a tracked research query changes",
    category: "Research",
    icon: "lucide:file-search",
    iconColor: "#8B5CF6",
  },
  {
    id: "new-login-detected",
    type: "Triggers",
    name: "New login detected",
    description: "Review account context when a sensitive login event appears",
    category: "Security",
    icon: "lucide:scan-face",
    iconColor: "#DC2626",
  },
  {
    id: "flight-status-changed",
    type: "Triggers",
    name: "Flight status changed",
    description: "Adjust itinerary notes when a tracked flight is delayed",
    category: "Travel",
    icon: "lucide:plane-takeoff",
    iconColor: "#0891B2",
  },
  {
    id: "daily-brief",
    type: "Schedules",
    name: "Daily brief",
    description: "Summarize priority work every weekday morning",
    category: "Productivity",
    icon: "lucide:sunrise",
    iconColor: "#F59E0B",
    featured: true,
  },
  {
    id: "weekly-pipeline-review",
    type: "Schedules",
    name: "Weekly pipeline review",
    description: "Prepare sales pipeline movement before the team meeting",
    category: "Business & Operations",
    icon: "lucide:trending-up",
    iconColor: "#F97316",
    featured: true,
  },
  {
    id: "monthly-board-pack",
    type: "Schedules",
    name: "Monthly board pack",
    description: "Draft a monthly operating summary from connected sources",
    category: "Data & Analytics",
    icon: "lucide:presentation",
    iconColor: "#0EA5E9",
    featured: true,
  },
  {
    id: "slack-digest",
    type: "Schedules",
    name: "Slack digest",
    description: "Recap important channel activity on a recurring cadence",
    category: "Communication",
    icon: "lucide:messages-square",
    iconColor: "#4A154B",
  },
  {
    id: "content-calendar",
    type: "Schedules",
    name: "Content calendar",
    description: "Draft upcoming campaign tasks and creative reminders",
    category: "Creativity",
    icon: "lucide:sparkles",
    iconColor: "#A855F7",
  },
  {
    id: "dependency-scan",
    type: "Schedules",
    name: "Dependency scan",
    description: "Review repository dependency changes on a fixed schedule",
    category: "Developer Tools",
    icon: "lucide:git-pull-request-arrow",
    iconColor: "#181717",
  },
  {
    id: "monthly-invoice-sweep",
    type: "Schedules",
    name: "Monthly invoice sweep",
    description: "Find overdue invoices and draft follow-up tasks",
    category: "Finance",
    icon: "lucide:badge-dollar-sign",
    iconColor: "#10B981",
  },
  {
    id: "learning-progress-check",
    type: "Schedules",
    name: "Learning progress check",
    description: "Summarize course progress and missing submissions",
    category: "Education & Research",
    icon: "lucide:school",
    iconColor: "#6366F1",
  },
  {
    id: "research-digest",
    type: "Schedules",
    name: "Research digest",
    description: "Compile new findings from saved feeds and documents",
    category: "Research",
    icon: "lucide:search-check",
    iconColor: "#8B5CF6",
  },
  {
    id: "access-review",
    type: "Schedules",
    name: "Access review",
    description: "Check workspace access and stale permissions quarterly",
    category: "Security",
    icon: "lucide:key-round",
    iconColor: "#DC2626",
  },
  {
    id: "travel-readiness",
    type: "Schedules",
    name: "Travel readiness",
    description: "Prepare itinerary, weather, and document reminders",
    category: "Travel",
    icon: "lucide:luggage",
    iconColor: "#0891B2",
  },
  {
    id: "custom-cron",
    type: "Schedules",
    name: "Custom cadence",
    description: "Run a saved instruction on a simple recurring cadence",
    category: "Other",
    icon: "lucide:timer-reset",
    iconColor: "#64748B",
  },
  {
    id: "quarterly-business-review",
    type: "Schedules",
    name: "Quarterly business review",
    description: "Assemble account health, risks, and recent wins each quarter",
    category: "Business & Operations",
    icon: "lucide:clipboard-chart",
    iconColor: "#F97316",
  },
  {
    id: "weekly-team-recap",
    type: "Schedules",
    name: "Weekly team recap",
    description: "Send a recurring summary of decisions and open questions",
    category: "Communication",
    icon: "lucide:newspaper",
    iconColor: "#4A154B",
  },
  {
    id: "monthly-brand-audit",
    type: "Schedules",
    name: "Monthly brand audit",
    description: "Review live assets for stale copy, images, and broken links",
    category: "Creativity",
    icon: "lucide:swatch-book",
    iconColor: "#A855F7",
  },
  {
    id: "weekly-metrics-readout",
    type: "Schedules",
    name: "Weekly metrics readout",
    description: "Prepare KPI movement, anomalies, and likely drivers",
    category: "Data & Analytics",
    icon: "lucide:chart-line",
    iconColor: "#0EA5E9",
  },
  {
    id: "release-readiness-check",
    type: "Schedules",
    name: "Release readiness check",
    description: "Review open issues, deploy status, and rollout notes",
    category: "Developer Tools",
    icon: "lucide:rocket",
    iconColor: "#181717",
  },
  {
    id: "weekly-class-summary",
    type: "Schedules",
    name: "Weekly class summary",
    description: "Prepare learner progress, blockers, and discussion themes",
    category: "Education & Research",
    icon: "lucide:book-marked",
    iconColor: "#6366F1",
  },
  {
    id: "cash-flow-review",
    type: "Schedules",
    name: "Cash flow review",
    description: "Summarize receivables, payables, and notable balance changes",
    category: "Finance",
    icon: "lucide:chart-candlestick",
    iconColor: "#10B981",
  },
  {
    id: "maintenance-window",
    type: "Schedules",
    name: "Maintenance window",
    description: "Run a recurring checklist for custom operational tasks",
    category: "Other",
    icon: "lucide:wrench",
    iconColor: "#64748B",
  },
  {
    id: "end-of-day-planning",
    type: "Schedules",
    name: "End of day planning",
    description: "Collect unfinished work and prepare tomorrow's priorities",
    category: "Productivity",
    icon: "lucide:calendar-check",
    iconColor: "#2563EB",
  },
  {
    id: "literature-watch",
    type: "Schedules",
    name: "Literature watch",
    description: "Scan tracked sources for new papers, filings, and datasets",
    category: "Research",
    icon: "lucide:library-big",
    iconColor: "#8B5CF6",
  },
  {
    id: "vendor-access-audit",
    type: "Schedules",
    name: "Vendor access audit",
    description: "Review external app access and stale vendor permissions",
    category: "Security",
    icon: "lucide:user-lock",
    iconColor: "#DC2626",
  },
  {
    id: "weekly-travel-check",
    type: "Schedules",
    name: "Weekly travel check",
    description: "Review upcoming trips, bookings, changes, and reminders",
    category: "Travel",
    icon: "lucide:map",
    iconColor: "#0891B2",
  },
]

export function automationCategory(item: AutomationItem): string {
  return item.category || "Other"
}

export function automationCategories(
  automations: AutomationItem[]
): AutomationCategory[] {
  const categories = new Set<AutomationCategory>()
  for (const automation of automations) {
    categories.add(automation.category)
  }
  return [...BASE_AUTOMATION_CATEGORIES, ...Array.from(categories).sort()]
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
  ].some((value) => value.toLowerCase().includes(normalized))
}
