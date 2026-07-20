"use client"

import type { ReactNode } from "react"
import { Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"

const easeOut = [0.16, 1, 0.3, 1] as const

function AutomationSourceIcon({ icon, size = 16 }: { icon: string; size?: 16 | 20 }) {
  if (icon === "github" || icon === "slack") {
    return <IntegrationLogo provider={icon} size={size} />
  }

  return <AppIcon icon={icon} size={size} />
}

const reveal = {
  hidden: { opacity: 0, y: 10 },
  show: (delay = 0) => ({
    opacity: 1,
    y: 0,
    transition: { duration: 0.45, delay, ease: easeOut },
  }),
}

function StatusDot({ tone = "success" }: { tone?: "success" | "muted" }) {
  return <span className={tone === "success" ? "size-2 rounded-full bg-success" : "size-2 rounded-full bg-muted/45"} />
}

function ProductWindow({ title, action, children, className = "" }: { title: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <div className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}>
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

const overviewRows = [
  {
    icon: "slack",
    name: "Support escalation",
    detail: ":eyes: in #support-escalations",
    owner: "Support agent",
  },
  {
    icon: "github",
    name: "Review every new pull request",
    detail: "New pull request in usehivy/api",
    owner: "Code review agent",
  },
  {
    icon: "calendar",
    name: "Daily CI failure digest",
    detail: "Weekdays at 9:30 AM",
    owner: "Reliability agent",
  },
  {
    icon: "globe",
    name: "Trial ended follow-up",
    detail: "Inbound HTTP request",
    owner: "Revenue agent",
  },
] as const

function AutomationOverviewMockup() {
  return (
    <MotionConfig reducedMotion="user">
      <motion.div initial="hidden" whileInView="show" viewport={{ once: true, amount: 0.25 }}>
        <ProductWindow
          title="Automations"
          action={
            <span className="inline-flex items-center gap-1.5 rounded-sm bg-accent px-3 py-1.5 text-xs font-medium text-accent-foreground">
              <AppIcon icon="plus" size={13} /> Add automation
            </span>
          }
        >
          <div className="grid border-b border-border sm:grid-cols-[1fr_auto] sm:items-center">
            <div className="flex gap-1 overflow-x-auto px-4 py-3 md:px-5">
              {["Connections", "Schedules", "Webhooks"].map((label, index) => (
                <span key={label} className={index === 0 ? "rounded-sm bg-default px-3 py-1.5 text-xs font-medium text-foreground" : "rounded-sm px-3 py-1.5 text-xs text-muted"}>
                  {label}
                </span>
              ))}
            </div>
            <div className="hidden items-center gap-2 px-5 sm:flex">
              <AppIcon icon="search" size={14} className="text-muted" />
              <span className="text-xs text-muted">Search automations</span>
            </div>
          </div>

          <div className="divide-y divide-border">
            {overviewRows.map((row, index) => (
              <motion.div
                variants={reveal}
                custom={0.12 + index * 0.08}
                key={row.name}
                className="grid gap-4 px-4 py-4 sm:grid-cols-[minmax(0,1.25fr)_minmax(150px,0.75fr)_auto] sm:items-center md:px-5"
              >
                <div className="flex min-w-0 items-center gap-3">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-sm border border-border bg-background">
                    <AutomationSourceIcon icon={row.icon} size={20} />
                  </span>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{row.name}</p>
                    <p className="mt-0.5 truncate text-xs text-muted">{row.detail}</p>
                  </div>
                </div>
                <span className="text-xs text-muted">{row.owner}</span>
                <span className="inline-flex w-fit items-center gap-2 text-xs font-medium">
                  <StatusDot /> Active
                </span>
              </motion.div>
            ))}
          </div>
        </ProductWindow>
      </motion.div>
    </MotionConfig>
  )
}

const startMethods = [
  {
    id: "github",
    label: "Pull requests",
    icon: "github",
    eyebrow: "GitHub trigger",
    title: "A pull request opens.",
    description: "The review agent reads the diff and CI results, then posts focused feedback on the pull request.",
  },
  {
    id: "slack",
    label: "Slack reactions",
    icon: "slack",
    eyebrow: "Slack trigger",
    title: "A teammate reacts with :eyes:.",
    description: "The support agent reads the message and its thread, investigates the report, then answers in Slack.",
  },
  {
    id: "schedule",
    label: "Schedules",
    icon: "calendar",
    eyebrow: "Scheduled trigger",
    title: "Monday hits 9:00 AM.",
    description: "The product agent checks slipping roadmap work and opens a session with the items that need an owner.",
  },
  {
    id: "webhook",
    label: "Webhooks",
    icon: "globe",
    eyebrow: "Webhook trigger",
    title: "Your app sends a POST.",
    description: "The revenue agent reads the request body and follows the task saved with that endpoint.",
  },
] as const

type StartMethod = (typeof startMethods)[number]

function TriggerSetup({ method }: { method: StartMethod }) {
  const source = {
    github: {
      source: "usehivy/api",
      signal: "Pull request opened",
      agent: "Review agent",
      instruction: "Check the diff and CI results. Post only actionable review comments.",
      result: "Review posted with inline comments",
    },
    slack: {
      source: "#support-escalations",
      signal: "Reaction :eyes:",
      agent: "Support agent",
      instruction: "Read the message and thread. Investigate the report, then reply in Slack.",
      result: "Cause and suggested fix posted to the thread",
    },
    schedule: {
      source: "Product team",
      signal: "Mondays at 9:00 AM",
      agent: "Product agent",
      instruction: "Check slipping roadmap work. Name an owner and next step for each item.",
      result: "Roadmap review opened with owners and next steps",
    },
    webhook: {
      source: "Trial lifecycle endpoint",
      signal: "Authenticated POST",
      agent: "Revenue agent",
      instruction: "Read the event payload and draft the right account follow-up.",
      result: "Account follow-up ready for review",
    },
  }[method.id]

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.38, ease: easeOut }}
      className="grid min-h-[500px] overflow-hidden rounded-sm bg-surface-secondary lg:grid-cols-[0.72fr_1.28fr]"
    >
      <div className="flex flex-col justify-between border-b border-border p-7 lg:border-r lg:border-b-0 lg:p-10">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium text-muted">
            <AutomationSourceIcon icon={method.icon} /> {method.eyebrow}
          </div>
          <h3 className="mt-7 max-w-[14ch] text-[clamp(1.8rem,3vw,3rem)] leading-[0.98] font-medium tracking-[-0.045em]">{method.title}</h3>
          <p className="mt-5 max-w-[35ch] text-sm leading-6 text-muted">{method.description}</p>
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

function SetupRow({ label, value, multiline = false }: { label: string; value: string; multiline?: boolean }) {
  return (
    <div className={`grid gap-2 px-5 py-4 ${multiline ? "sm:grid-cols-[110px_1fr]" : "grid-cols-[90px_1fr] sm:grid-cols-[110px_1fr]"}`}>
      <dt className="text-xs text-muted">{label}</dt>
      <dd className="text-sm leading-6">{value}</dd>
    </div>
  )
}

export function StartMethodsTabs() {
  return (
    <Tabs variant="primary" defaultSelectedKey="github" className="w-full">
      <Tabs.ListContainer className="max-w-full overflow-x-auto">
        <Tabs.List aria-label="Choose what starts the automation" className="w-fit min-w-[650px]">
          {startMethods.map((method) => (
            <Tabs.Tab id={method.id} key={method.id}>
              <span className="flex items-center justify-center gap-2 whitespace-nowrap">
                <AutomationSourceIcon icon={method.icon} />
                {method.label}
              </span>
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

function ScheduleBuilderMockup() {
  const weekdays = ["M", "T", "W", "T", "F", "S", "S"]

  return (
    <ProductWindow title="Add schedule" action={<span className="rounded-sm bg-accent px-3 py-1.5 text-xs font-medium text-accent-foreground">Create schedule</span>}>
      <div className="grid lg:grid-cols-[1.22fr_0.78fr]">
        <div className="space-y-6 p-5 md:p-7 lg:border-r lg:border-border">
          <MockField label="Name" value="Daily CI failure digest" />
          <div className="grid gap-5 sm:grid-cols-2">
            <MockField label="Team" value="Engineering" icon="users" />
            <MockField label="Agent" value="Reliability agent" icon="bot" />
          </div>
          <div>
            <p className="text-xs text-muted">Repeat</p>
            <div className="mt-2 grid gap-3 sm:grid-cols-[1fr_1fr]">
              <div className="rounded-sm border border-border bg-background px-3 py-2.5 text-sm">Every weekday</div>
              <div className="rounded-sm border border-border bg-background px-3 py-2.5 text-sm">9:30 AM</div>
            </div>
            <div className="mt-3 flex items-center gap-1.5">
              {weekdays.map((day, index) => (
                <span
                  key={`${day}-${index}`}
                  className={
                    index < 5
                      ? "flex size-8 items-center justify-center rounded-sm bg-accent text-xs font-medium text-accent-foreground"
                      : "flex size-8 items-center justify-center rounded-sm bg-default text-xs text-muted"
                  }
                >
                  {day}
                </span>
              ))}
            </div>
            <p className="mt-3 text-xs text-muted">You picked 9:30 AM (Africa/Lagos). Hivy stores schedules in UTC.</p>
          </div>
          <div>
            <p className="text-xs text-muted">Task</p>
            <div className="mt-2 min-h-28 rounded-sm border border-border bg-background p-3 text-sm leading-6">
              Review failed GitHub Actions runs from the last 24 hours. Group repeated failures, identify the likely cause, and name the next owner action. Don’t rerun jobs or change workflow files.
            </div>
          </div>
        </div>

        <div className="flex flex-col justify-between bg-surface-secondary p-5 md:p-7">
          <div>
            <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">Schedule preview</p>
            <div className="mt-7 flex items-start gap-3">
              <span className="flex size-9 items-center justify-center rounded-sm bg-surface">
                <AppIcon icon="calendar" size={17} />
              </span>
              <div>
                <p className="text-sm font-medium">Weekdays at 9:30 AM</p>
                <p className="mt-1 text-xs text-muted">Next run: tomorrow</p>
              </div>
            </div>
            <div className="mt-8 border-t border-border pt-6">
              <p className="text-xs text-muted">Each run opens</p>
              <p className="mt-2 text-sm font-medium">A fresh Engineering session</p>
              <p className="mt-2 text-xs leading-5 text-muted">The team can read the result, inspect cost, and ask the agent a follow-up in that session.</p>
            </div>
          </div>
          <div className="mt-10 inline-flex items-center gap-2 text-xs font-medium">
            <StatusDot /> Active
          </div>
        </div>
      </div>
    </ProductWindow>
  )
}

function MockField({ label, value, icon }: { label: string; value: string; icon?: string }) {
  return (
    <div>
      <p className="text-xs text-muted">{label}</p>
      <div className="mt-2 flex items-center gap-2 rounded-sm border border-border bg-background px-3 py-2.5 text-sm">
        {icon ? <AppIcon icon={icon} size={14} className="text-muted" /> : null}
        {value}
      </div>
    </div>
  )
}

function WebhookRequestMockup() {
  return (
    <MotionConfig reducedMotion="user">
      <motion.div initial="hidden" whileInView="show" viewport={{ once: true, amount: 0.2 }} className="grid overflow-hidden rounded-sm border border-border bg-surface lg:grid-cols-[0.92fr_1.08fr]">
        <div className="border-b border-border bg-foreground p-6 text-background md:p-8 lg:border-r lg:border-b-0">
          <div className="flex items-center justify-between gap-4">
            <span className="text-xs font-medium text-background/65">Request</span>
            <span className="rounded-sm bg-background/10 px-2 py-1 text-[0.68rem]">POST</span>
          </div>
          <pre className="mt-7 overflow-x-auto text-[0.78rem] leading-6 text-background/78">
            <code>{`curl -X POST "YOUR_WEBHOOK_URL" \\
  -H "Authorization: Bearer ••••••••" \\
  -H "Content-Type: application/json" \\
  -d '{
    "customer_id": "customer-123",
    "event": "trial_ended",
    "owner": "sam@company.test"
  }'`}</code>
          </pre>
          <div className="mt-8 flex items-center gap-2 border-t border-background/15 pt-5 text-xs text-background/65">
            <AppIcon icon="shield-check" size={15} /> Shared secret required
          </div>
        </div>

        <div className="p-6 md:p-8">
          <motion.div variants={reveal} custom={0.15}>
            <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">Accepted for processing</p>
            <div className="mt-5 flex items-start gap-3">
              <span className="flex size-9 items-center justify-center rounded-full bg-success/15 text-success">
                <AppIcon icon="check" size={16} />
              </span>
              <div>
                <p className="text-sm font-medium">200 OK</p>
                <p className="mt-1 text-xs leading-5 text-muted">The request started an asynchronous agent run. Its result lives in the session, not the HTTP response.</p>
              </div>
            </div>
          </motion.div>

          <motion.div variants={reveal} custom={0.3} className="mt-8 border-t border-border pt-6">
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-3">
                <span className="flex size-9 items-center justify-center rounded-sm border border-border bg-background">
                  <AppIcon icon="bot" size={16} />
                </span>
                <div>
                  <p className="text-sm font-medium">Revenue agent</p>
                  <p className="text-xs text-muted">Revenue team</p>
                </div>
              </div>
              <span className="text-xs font-medium text-success">Completed</span>
            </div>
            <div className="mt-5 rounded-sm bg-surface-secondary p-4">
              <p className="text-xs font-medium">Result</p>
              <p className="mt-2 text-sm leading-6 text-muted">
                The trial ended with two invited teammates who never activated. I drafted a follow-up that offers a 15-minute setup call and links directly to the unfinished workspace setup.
              </p>
            </div>
          </motion.div>
        </div>
      </motion.div>
    </MotionConfig>
  )
}

function RoutingMockup() {
  const routeRows = [
    { icon: "users", label: "Team", value: "Engineering" },
    { icon: "bot", label: "Agent", value: "Reliability agent" },
    { icon: "github", label: "Connection", value: "usehivy GitHub" },
    { icon: "file-text", label: "Instructions", value: "CI failure triage" },
  ] as const

  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface">
      <div className="grid lg:grid-cols-[0.78fr_1.22fr]">
        <div className="flex flex-col justify-between border-b border-border p-7 lg:border-r lg:border-b-0 lg:p-10">
          <div>
            <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">One saved route</p>
            <div className="mt-8 flex items-center gap-3">
              <IntegrationLogo provider="github" size={40} className="rounded-sm" />
              <AppIcon icon="arrow-right" size={16} className="text-muted" />
              <span className="flex size-11 items-center justify-center rounded-sm bg-surface-secondary">
                <AppIcon icon="bot" size={21} />
              </span>
              <AppIcon icon="arrow-right" size={16} className="text-muted" />
              <span className="flex size-11 items-center justify-center rounded-sm bg-surface-secondary">
                <AppIcon icon="messages-square" size={21} />
              </span>
            </div>
          </div>
          <p className="mt-12 max-w-[32ch] text-sm leading-6 text-muted">Each matched event carries its repository or channel context into a session owned by the chosen team.</p>
        </div>
        <dl className="divide-y divide-border">
          {routeRows.map((row) => (
            <div key={row.label} className="grid grid-cols-[1fr_auto] items-center gap-4 px-5 py-5 md:px-8">
              <dt className="flex items-center gap-3 text-sm text-muted">
                {row.icon === "github" ? <IntegrationLogo provider="github" size={16} /> : <AppIcon icon={row.icon} size={16} />}
                {row.label}
              </dt>
              <dd className="text-right text-sm font-medium">{row.value}</dd>
            </div>
          ))}
        </dl>
      </div>
    </div>
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
    <div className="grid overflow-hidden rounded-sm border border-border bg-surface lg:grid-cols-[0.9fr_1.1fr]">
      <div className="border-b border-border lg:border-r lg:border-b-0">
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <p className="text-sm font-medium">Automation runs</p>
          <span className="text-xs text-muted">Today</span>
        </div>
        <div className="divide-y divide-border">
          {runs.map((run, index) => (
            <div key={run.session} className={index === 0 ? "bg-surface-secondary px-5 py-5" : "px-5 py-5"}>
              <div className="flex items-start justify-between gap-4">
                <div>
                  <p className="text-sm font-medium">{run.session}</p>
                  <p className="mt-1 text-xs text-muted">{run.name}</p>
                </div>
                <span className={run.status === "Done" ? "text-xs font-medium text-success" : "text-xs font-medium text-warning"}>{run.status}</span>
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
          <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">Agent report</p>
          <h3 className="mt-4 text-xl font-medium tracking-[-0.03em]">The OAuth callback test needs a fix.</h3>
          <p className="mt-4 text-sm leading-6 text-muted">
            The API integration suite failed because the expected redirect host doesn’t match the preview environment. The remaining failures share a runner-timeout signature and passed on retry.
          </p>
          <div className="mt-6 grid gap-3 sm:grid-cols-2">
            <div className="rounded-sm bg-surface-secondary p-4">
              <p className="text-xs text-muted">Owner</p>
              <p className="mt-1 text-sm font-medium">Identity team</p>
            </div>
            <div className="rounded-sm bg-surface-secondary p-4">
              <p className="text-xs text-muted">Next step</p>
              <p className="mt-1 text-sm font-medium">Update the callback test host</p>
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

function AutomationControlsMockup() {
  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface">
      <div className="grid md:grid-cols-[1.08fr_0.92fr]">
        <div className="border-b border-border p-6 md:border-r md:border-b-0 md:p-8">
          <div className="flex items-start justify-between gap-5">
            <div>
              <p className="text-sm font-medium">Daily CI failure digest</p>
              <p className="mt-1 text-xs text-muted">Reliability agent · Engineering</p>
            </div>
            <span className="inline-flex items-center gap-2 rounded-sm bg-success/15 px-2.5 py-1.5 text-xs font-medium text-success">
              <StatusDot /> Active
            </span>
          </div>
          <dl className="mt-8 divide-y divide-border border-y border-border">
            <ControlRow label="Last run" value="Completed today at 9:30 AM" />
            <ControlRow label="Next run" value="Tomorrow at 9:30 AM" />
            <ControlRow label="Cadence" value="Weekdays" />
          </dl>
        </div>
        <div className="flex flex-col justify-between bg-surface-secondary p-6 md:p-8">
          <div>
            <p className="text-xs font-medium">Pause without rebuilding</p>
            <p className="mt-3 text-sm leading-6 text-muted">Pausing keeps the name, team, agent, cadence, and task. Resume it when the recurring work should start again.</p>
          </div>
          <div className="mt-10 flex items-center justify-between gap-4 border-t border-border pt-5">
            <span className="text-xs text-muted">Automation status</span>
            <span className="inline-flex items-center gap-2 rounded-sm border border-border bg-surface px-3 py-2 text-xs font-medium">
              <AppIcon icon="circle-slash" size={13} /> Pause schedule
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}

function ControlRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[0.7fr_1.3fr] gap-4 py-4 text-sm">
      <dt className="text-muted">{label}</dt>
      <dd className="text-right font-medium">{value}</dd>
    </div>
  )
}
