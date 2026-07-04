"use client"

import { use, useMemo } from "react"
import NextLink from "next/link"
import { Button, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import {
  automationCadenceLabel,
  automationDefaultAgent,
  automationDefaultChannel,
  automationEventLabel,
  automationFromCatalog,
  automationInstructions,
  automationRequiredPlugins,
  automationSourceLabel,
  automationTimezoneLabel,
  type AutomationItem,
} from "@/app/w/(chat)/automations/_data"

export default function AutomationDetailPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = use(params)
  const triggersQuery = $api.useQuery("get", "/v1/catalog/triggers")
  const schedulesQuery = $api.useQuery("get", "/v1/catalog/automations")
  const automations = useMemo(
    () => [
      ...(triggersQuery.data?.data ?? []).map((item) =>
        automationFromCatalog(item, "Triggers")
      ),
      ...(schedulesQuery.data?.data ?? []).map((item) =>
        automationFromCatalog(item, "Schedules")
      ),
    ],
    [schedulesQuery.data?.data, triggersQuery.data?.data]
  )
  const automation = automations.find((item) => item.id === id)
  const isLoading = triggersQuery.isLoading || schedulesQuery.isLoading
  const isError = triggersQuery.isError || schedulesQuery.isError

  if (isLoading) {
    return <AutomationDetailShell content={<AutomationDetailSkeleton />} />
  }

  if (isError) {
    return (
      <AutomationDetailShell
        content={
          <AutomationCatalogError
            onRetry={() => {
              void triggersQuery.refetch()
              void schedulesQuery.refetch()
            }}
          />
        }
      />
    )
  }

  if (!automation) {
    return (
      <AutomationDetailShell
        content={
          <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
            <AppIcon icon="clock-alert" className="h-7 w-7 text-muted" />
            <p className="mt-3 text-sm font-medium text-foreground">
              Automation not found
            </p>
            <p className="mt-1 text-sm text-muted">
              This automation may have been removed from the catalog.
            </p>
            <NextLink
              href="/w/automations"
              className="hover:text-muted-foreground mt-4 text-sm font-medium text-foreground transition-colors"
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
            className="text-muted-foreground inline-flex w-fit items-center gap-1.5 text-sm font-medium transition-colors hover:text-foreground"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Automations
          </NextLink>

          <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 items-center gap-3">
              <AutomationLogo automation={automation} size="lg" />
              <div className="min-w-0">
                <h1 className="text-xl font-semibold text-foreground">
                  {automation.name}
                </h1>
                <p className="text-muted-foreground mt-1 max-w-xl text-sm leading-5">
                  {automation.description}
                </p>
              </div>
            </div>

            {automation.type === "Triggers" ? (
              <NextLink
                href={`/w/automations/${automation.id}/install`}
                className="inline-flex h-8 shrink-0 items-center justify-center gap-2 rounded-full border border-border bg-surface px-3 text-sm font-medium text-foreground transition-colors hover:bg-surface-secondary"
              >
                <AppIcon icon="plus" className="h-4 w-4" />
                Install trigger
              </NextLink>
            ) : null}
          </header>

          <section className="bg-card rounded-xl border border-border p-4">
            <h2 className="text-sm font-medium text-foreground">
              Instructions
            </h2>
            <div className="mt-3">
              <MarkdownProse text={automationInstructions(automation)} muted />
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-base font-semibold text-foreground">
              Configuration preview
            </h2>
            <div className="bg-card flex flex-col divide-y divide-border rounded-xl border border-border">
              {configurationRows(automation).map((row) => (
                <div
                  key={row.label}
                  className="flex items-start justify-between gap-4 px-3 py-2.5"
                >
                  <span className="text-muted-foreground text-sm">
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
            <div className="bg-card flex flex-col divide-y divide-border rounded-xl border border-border">
              {workflowSteps(automation).map((step, index) => (
                <div key={step} className="flex items-start gap-3 px-3 py-3">
                  <span className="text-muted-foreground flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-default text-xs font-medium">
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
      <AppIcon
        icon={automation.icon}
        className={size === "lg" ? "h-6 w-6 text-white" : "h-4 w-4 text-white"}
      />
    </div>
  )
}

function configurationRows(automation: AutomationItem) {
  if (automation.type === "Triggers") {
    return [
      { label: "Event", value: automationEventLabel(automation) },
      { label: "Source", value: automationSourceLabel(automation) },
      { label: "Agent", value: automationDefaultAgent(automation) },
      { label: "Delivery", value: automationDefaultChannel(automation) },
    ]
  }

  return [
    { label: "Cadence", value: cadenceLabel(automation) },
    { label: "Timezone", value: automationTimezoneLabel(automation) },
    { label: "Source", value: automationSourceLabel(automation) },
    { label: "Agent", value: automationDefaultAgent(automation) },
    { label: "Delivery", value: automationDefaultChannel(automation) },
  ]
}

function workflowSteps(automation: AutomationItem): string[] {
  const plugins = automationRequiredPlugins(automation)
  if (automation.type === "Triggers") {
    return [
      `Watch for ${automationEventLabel(automation)} events from ${automationSourceLabel(automation)}.`,
      `Install ${plugins.length > 0 ? plugins.join(", ") : automationSourceLabel(automation)} access for the selected agent.`,
      "Start an agent run with the event context and saved instructions.",
      "Post the result back to a workspace thread for review.",
    ]
  }

  return [
    `Run on the ${cadenceLabel(automation).toLowerCase()} cadence you configure.`,
    `Use ${automationSourceLabel(automation)} context available to the selected agent.`,
    "Start an agent run with saved instructions and the latest workspace context.",
    "Post the recurring result back to a workspace thread for review.",
  ]
}

function cadenceLabel(automation: AutomationItem): string {
  return automationCadenceLabel(automation)
}

function AutomationDetailSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <Skeleton className="h-5 w-28 rounded" />
      <div className="flex items-start gap-3">
        <Skeleton className="h-12 w-12 rounded-xl" />
        <div className="min-w-0 flex-1">
          <Skeleton className="h-6 w-56 rounded" />
          <Skeleton className="mt-3 h-4 w-full max-w-lg rounded" />
        </div>
      </div>
      {Array.from({ length: 3 }).map((_, index) => (
        <section
          key={index}
          className="bg-card rounded-xl border border-border p-4"
        >
          <Skeleton className="h-4 w-32 rounded" />
          <Skeleton className="mt-3 h-4 w-full rounded" />
          <Skeleton className="mt-2 h-4 w-4/5 rounded" />
        </section>
      ))}
    </div>
  )
}

function AutomationCatalogError({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
      <AppIcon
        icon="triangle-alert"
        className="text-muted-foreground h-7 w-7"
      />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load automation
      </p>
      <Button variant="ghost" size="sm" className="mt-3" onPress={onRetry}>
        <AppIcon icon="refresh-cw" className="h-4 w-4" />
        Retry
      </Button>
    </div>
  )
}
