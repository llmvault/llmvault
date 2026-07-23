"use client"

import type { ReactNode } from "react"
import { Tabs } from "@heroui/react"
import { motion } from "motion/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"

const easeOut = [0.16, 1, 0.3, 1] as const

function AutomationSourceIcon({
  icon,
  size = 16,
}: {
  icon: string
  size?: 16 | 20
}) {
  if (icon === "github" || icon === "slack") {
    return <IntegrationLogo provider={icon} size={size} />
  }
  return <AppIcon icon={icon} size={size} />
}

function StatusDot({ tone = "success" }: { tone?: "success" | "muted" }) {
  return (
    <span
      className={
        tone === "success"
          ? "size-2 rounded-full bg-success"
          : "size-2 rounded-full bg-muted/45"
      }
    />
  )
}

function ProductWindow({
  title,
  action,
  children,
  className = "",
}: {
  title: string
  action?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}
    >
      <div className="flex min-h-12 items-center justify-between gap-4 border-b border-border px-4 md:px-5">
        <div className="flex items-center gap-2">
          <span className="size-2 rounded-full bg-muted/35" />
          <span className="text-sm font-medium">{title}</span>
        </div>
        {action}
      </div>
      {children}
    </div>
  )
}

const startMethods = [
  {
    id: "github",
    label: "Pull requests",
    icon: "github",
    eyebrow: "GitHub trigger",
    title: "A pull request opens.",
    description:
      "The review agent reads the diff and CI results, then posts focused feedback on the pull request.",
  },
  {
    id: "slack",
    label: "Slack reactions",
    icon: "slack",
    eyebrow: "Slack trigger",
    title: "A teammate reacts with :eyes:.",
    description:
      "The support agent reads the message and its thread, investigates the report, then answers in Slack.",
  },
  {
    id: "schedule",
    label: "Schedules",
    icon: "calendar",
    eyebrow: "Scheduled trigger",
    title: "Monday hits 9:00 AM.",
    description:
      "The product agent checks slipping roadmap work and opens a session with the items that need an owner.",
  },
  {
    id: "webhook",
    label: "Webhooks",
    icon: "globe",
    eyebrow: "Webhook trigger",
    title: "Your app sends a POST.",
    description:
      "The revenue agent reads the request body and follows the task saved with that endpoint.",
  },
] as const

type StartMethod = (typeof startMethods)[number]

function TriggerSetup({ method }: { method: StartMethod }) {
  const source = {
    github: {
      source: "usehivy/api",
      signal: "Pull request opened",
      agent: "Review agent",
      instruction:
        "Check the diff and CI results. Post only actionable review comments.",
      result: "Review posted with inline comments",
    },
    slack: {
      source: "#support-escalations",
      signal: "Reaction :eyes:",
      agent: "Support agent",
      instruction:
        "Read the message and thread. Investigate the report, then reply in Slack.",
      result: "Cause and suggested fix posted to the thread",
    },
    schedule: {
      source: "Product team",
      signal: "Mondays at 9:00 AM",
      agent: "Product agent",
      instruction:
        "Check slipping roadmap work. Name an owner and next step for each item.",
      result: "Roadmap review opened with owners and next steps",
    },
    webhook: {
      source: "Trial lifecycle endpoint",
      signal: "Authenticated POST",
      agent: "Revenue agent",
      instruction:
        "Read the event payload and draft the right account follow-up.",
      result: "Account follow-up ready for review",
    },
  }[method.id]

  return (
    <motion.div
      data-testid="automation-trigger-grid"
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.38, ease: easeOut }}
      className="grid min-h-[500px] overflow-hidden rounded-sm bg-surface-secondary lg:grid-cols-[1fr_2fr]"
    >
      <div className="flex flex-col justify-between border-b border-border p-7 lg:border-r lg:border-b-0 lg:p-10">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium text-muted">
            <AutomationSourceIcon icon={method.icon} /> {method.eyebrow}
          </div>
          <h3 className="mt-7 max-w-[14ch] text-[clamp(1.8rem,3vw,3rem)] leading-[0.98] font-medium tracking-[-0.045em]">
            {method.title}
          </h3>
          <p className="mt-5 max-w-[35ch] text-sm leading-6 text-muted">
            {method.description}
          </p>
        </div>
        <div className="mt-10 inline-flex items-center gap-2 text-xs font-medium">
          <StatusDot /> Waiting for a matching event
        </div>
      </div>

      <div className="flex items-center p-4 md:p-8 lg:p-10">
        <ProductWindow title="Automation setup" className="w-full">
          <dl className="divide-y divide-border">
            <SetupRow label="Connection" value={source.source} />
            <SetupRow label="Trigger" value={source.signal} />
            <SetupRow label="Assigned agent" value={source.agent} />
            <SetupRow label="Task" value={source.instruction} multiline />
          </dl>
          <div className="flex items-start gap-3 border-t border-border bg-accent-soft px-5 py-4">
            <span className="mt-0.5 flex size-6 items-center justify-center rounded-full bg-success/15 text-success">
              <AppIcon icon="check" size={13} />
            </span>
            <div>
              <p className="text-xs font-medium">Latest run</p>
              <p className="mt-1 text-sm">{source.result}</p>
            </div>
          </div>
        </ProductWindow>
      </div>
    </motion.div>
  )
}

function SetupRow({
  label,
  value,
  multiline = false,
}: {
  label: string
  value: string
  multiline?: boolean
}) {
  return (
    <div
      className={`grid gap-2 px-5 py-4 ${
        multiline
          ? "sm:grid-cols-[110px_1fr]"
          : "grid-cols-[90px_1fr] sm:grid-cols-[110px_1fr]"
      }`}
    >
      <dt className="text-xs text-muted">{label}</dt>
      <dd className="text-sm leading-6">{value}</dd>
    </div>
  )
}

export function StartMethodsTabs() {
  return (
    <Tabs variant="primary" defaultSelectedKey="github" className="w-full">
      <Tabs.ListContainer className="max-w-full overflow-x-auto">
        <Tabs.List
          aria-label="Choose what starts the automation"
          className="w-fit min-w-[650px]"
        >
          {startMethods.map((method) => (
            <Tabs.Tab id={method.id} key={method.id}>
              <span className="flex items-center justify-center gap-2 whitespace-nowrap">
                <AutomationSourceIcon icon={method.icon} />
                {method.label}
              </span>
              <Tabs.Indicator />
            </Tabs.Tab>
          ))}
        </Tabs.List>
      </Tabs.ListContainer>

      {startMethods.map((method) => (
        <Tabs.Panel id={method.id} key={method.id} className="mt-8 p-0">
          <TriggerSetup method={method} />
        </Tabs.Panel>
      ))}
    </Tabs>
  )
}

const runs = [
  {
    name: "Weekday CI review",
    time: "Today, 9:30 AM",
    status: "Done",
    session: "CI failures · Jul 20",
  },
  {
    name: "Pull request review",
    time: "Today, 9:12 AM",
    status: "Done",
    session: "Review PR #428",
  },
  {
    name: "Support escalation",
    time: "Today, 8:46 AM",
    status: "Waiting on reply",
    session: "Import failure · Checkout service",
  },
] as const

export function RunHistoryMockup() {
  return (
    <div
      data-testid="automation-history-grid"
      className="grid overflow-hidden rounded-sm border border-border bg-surface lg:grid-cols-[1fr_2fr]"
    >
      <div className="border-b border-border lg:border-r lg:border-b-0">
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <p className="text-sm font-medium">Automation runs</p>
          <span className="text-xs text-muted">Today</span>
        </div>
        <div className="divide-y divide-border">
          {runs.map((run, index) => (
            <div
              key={run.session}
              className={
                index === 0 ? "bg-surface-secondary px-5 py-5" : "px-5 py-5"
              }
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <p className="text-sm font-medium">{run.session}</p>
                  <p className="mt-1 text-xs text-muted">{run.name}</p>
                </div>
                <span
                  className={
                    run.status === "Done"
                      ? "text-xs font-medium text-success"
                      : "text-xs font-medium text-warning"
                  }
                >
                  {run.status}
                </span>
              </div>
              <p className="mt-4 text-[0.68rem] text-muted">{run.time}</p>
            </div>
          ))}
        </div>
      </div>

      <div className="p-6 md:p-8 lg:p-10">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <span className="flex size-9 items-center justify-center rounded-sm bg-surface-secondary">
              <AppIcon icon="bot" size={17} />
            </span>
            <div>
              <p className="text-sm font-medium">Reliability agent</p>
              <p className="text-xs text-muted">Started by schedule</p>
            </div>
          </div>
          <span className="inline-flex items-center gap-2 text-xs font-medium">
            <StatusDot /> Done
          </span>
        </div>
        <div className="mt-8 border-t border-border pt-6">
          <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">
            Agent report
          </p>
          <h3 className="mt-4 text-xl font-medium tracking-[-0.03em]">
            The OAuth callback test needs a fix.
          </h3>
          <p className="mt-4 text-sm leading-6 text-muted">
            The API integration suite failed because the expected redirect host
            doesn’t match the preview environment. The remaining failures share
            a runner-timeout signature and passed on retry.
          </p>
          <div className="mt-6 grid gap-3 sm:grid-cols-2">
            <div className="rounded-sm bg-surface-secondary p-4">
              <p className="text-xs text-muted">Owner</p>
              <p className="mt-1 text-sm font-medium">Identity team</p>
            </div>
            <div className="rounded-sm bg-surface-secondary p-4">
              <p className="text-xs text-muted">Next step</p>
              <p className="mt-1 text-sm font-medium">
                Update the callback test host
              </p>
            </div>
          </div>
          <p className="mt-6 inline-flex items-center gap-2 text-xs font-medium">
            Open full session <AppIcon icon="arrow-right" size={13} />
          </p>
        </div>
      </div>
    </div>
  )
}
