"use client"

import { use } from "react"
import NextLink from "next/link"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  STATIC_AUTOMATIONS,
  type AutomationItem,
} from "@/app/w/(chat)/automations/_data"

export default function AutomationDetailPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = use(params)
  const automation = STATIC_AUTOMATIONS.find((item) => item.id === id)

  if (!automation) {
    return (
      <AutomationDetailShell
        content={
          <div className="flex min-h-64 flex-col items-center justify-center rounded-xl border border-border bg-card px-6 text-center">
            <Icon icon="lucide:clock-alert" className="h-7 w-7 text-muted" />
            <p className="mt-3 text-sm font-medium text-foreground">
              Automation not found
            </p>
            <p className="mt-1 text-sm text-muted">
              This static automation may have been removed from the catalog.
            </p>
            <NextLink
              href="/w/automations"
              className="mt-4 text-sm font-medium text-foreground transition-colors hover:text-muted-foreground"
            >
              Back to automations
            </NextLink>
          </div>
        }
      />
    )
  }

  return (
    <AutomationDetailShell
      content={
        <div className="flex flex-col gap-8">
          <NextLink
            href="/w/automations"
            className="inline-flex w-fit items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            <Icon icon="lucide:arrow-left" className="h-4 w-4" />
            Automations
          </NextLink>

          <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 items-center gap-3">
              <AutomationLogo automation={automation} size="lg" />
              <div className="min-w-0">
                <h1 className="text-xl font-semibold text-foreground">
                  {automation.name}
                </h1>
                <p className="mt-1 max-w-xl text-sm leading-5 text-muted-foreground">
                  {automation.description}
                </p>
              </div>
            </div>

            <Button variant="primary" size="sm" className="shrink-0">
              <Icon icon="lucide:plus" className="h-4 w-4" />
              Install automation
            </Button>
          </header>

          <section className="rounded-xl border border-border bg-card p-4">
            <h2 className="text-sm font-medium text-foreground">
              Instructions
            </h2>
            <p className="mt-2 text-sm leading-5 text-muted-foreground">
              {automationInstructions(automation)}
            </p>
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-base font-semibold text-foreground">
              Configuration preview
            </h2>
            <div className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
              {configurationRows(automation).map((row) => (
                <div
                  key={row.label}
                  className="flex items-start justify-between gap-4 px-3 py-2.5"
                >
                  <span className="text-sm text-muted-foreground">
                    {row.label}
                  </span>
                  <span className="max-w-[65%] text-right text-sm font-medium text-foreground">
                    {row.value}
                  </span>
                </div>
              ))}
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-base font-semibold text-foreground">
              How it works
            </h2>
            <div className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
              {workflowSteps(automation).map((step, index) => (
                <div key={step} className="flex items-start gap-3 px-3 py-3">
                  <span className="bg-default flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-medium text-muted-foreground">
                    {index + 1}
                  </span>
                  <p className="text-sm leading-5 text-foreground">{step}</p>
                </div>
              ))}
            </div>
          </section>
        </div>
      }
    />
  )
}

function AutomationDetailShell({ content }: { content: React.ReactNode }) {
  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">{content}</div>
    </div>
  )
}

function AutomationLogo({
  automation,
  size = "sm",
}: {
  automation: AutomationItem
  size?: "sm" | "lg"
}) {
  return (
    <div
      className={
        size === "lg"
          ? "flex h-12 w-12 shrink-0 items-center justify-center rounded-xl"
          : "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
      }
      style={{ backgroundColor: automation.iconColor }}
    >
      <Icon
        icon={automation.icon}
        className={size === "lg" ? "h-6 w-6 text-white" : "h-4 w-4 text-white"}
      />
    </div>
  )
}

function configurationRows(automation: AutomationItem) {
  if (automation.type === "Triggers") {
    return [
      { label: "Event", value: automation.name },
      { label: "Source", value: sourceLabel(automation) },
      { label: "Agent", value: "Choose during setup" },
      { label: "Delivery", value: "Workspace thread" },
    ]
  }

  return [
    { label: "Cadence", value: cadenceLabel(automation) },
    { label: "Timezone", value: "Workspace timezone" },
    { label: "Agent", value: "Choose during setup" },
    { label: "Delivery", value: "Workspace thread" },
  ]
}

function automationInstructions(automation: AutomationItem): string {
  if (automation.type === "Triggers") {
    return `When ${automation.name.toLowerCase()} happens, review the event context, identify the important update, and draft the next action for the workspace.`
  }

  return `On each ${cadenceLabel(automation).toLowerCase()} run, gather the latest workspace context, summarize the important changes, and draft the next action for the workspace.`
}

function workflowSteps(automation: AutomationItem): string[] {
  if (automation.type === "Triggers") {
    return [
      `Watch for ${automation.name.toLowerCase()} events from ${sourceLabel(automation).toLowerCase()}.`,
      "Start an agent run with the event context and the instructions you configure.",
      "Post the result back to a workspace thread for review.",
    ]
  }

  return [
    `Run on the ${cadenceLabel(automation).toLowerCase()} cadence you configure.`,
    "Start an agent run with saved instructions and the latest workspace context.",
    "Post the recurring result back to a workspace thread for review.",
  ]
}

function sourceLabel(automation: AutomationItem): string {
  const labels: Partial<Record<AutomationItem["category"], string>> = {
    "Business & Operations": "CRM or support workspace",
    Communication: "Team communication app",
    Creativity: "Creative workspace",
    "Data & Analytics": "Analytics workspace",
    "Developer Tools": "Developer platform",
    "Education & Research": "Learning workspace",
    Finance: "Finance workspace",
    Productivity: "Productivity workspace",
    Research: "Research source",
    Security: "Security event stream",
    Travel: "Travel workspace",
    Other: "Custom source",
  }
  return labels[automation.category] ?? "Workspace event"
}

function cadenceLabel(automation: AutomationItem): string {
  if (automation.name.toLowerCase().includes("daily")) return "Daily"
  if (automation.name.toLowerCase().includes("weekly")) return "Weekly"
  if (automation.name.toLowerCase().includes("monthly")) return "Monthly"
  if (automation.name.toLowerCase().includes("quarterly")) return "Quarterly"
  return "Custom"
}
