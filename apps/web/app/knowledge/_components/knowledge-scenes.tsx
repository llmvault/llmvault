"use client"

import { Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"

const easeOut = [0.16, 1, 0.3, 1] as const
const sourceExamples = [
  {
    id: "github",
    label: "GitHub",
    icon: "github",
    name: "Agent platform repositories",
    scopeLabel: "Repositories",
    scopes: ["usehivy/web", "usehivy/runners"],
    teams: ["Engineering", "Product"],
  },
  {
    id: "slack",
    label: "Slack",
    icon: "slack",
    name: "Customer feedback",
    scopeLabel: "Channels",
    scopes: ["#customer-voice", "#support-escalations"],
    teams: ["Customer success", "Product"],
  },
  {
    id: "notion",
    label: "Notion",
    icon: "notion",
    name: "Company playbook",
    scopeLabel: "Pages",
    scopes: ["Support playbook", "Product decisions"],
    teams: ["Operations", "Product"],
  },
  {
    id: "linear",
    label: "Linear",
    icon: "linear",
    name: "Roadmap and delivery",
    scopeLabel: "Teams",
    scopes: ["Platform", "Customer experience"],
    teams: ["Engineering", "Product"],
  },
  {
    id: "website",
    label: "Website",
    icon: "chrome",
    name: "Help center",
    scopeLabel: "Site sections",
    scopes: ["/docs/agents", "/docs/integrations"],
    teams: ["Customer success", "Revenue"],
  },
] as const

type SourceExample = (typeof sourceExamples)[number]

function Window({
  title,
  eyebrow,
  children,
  className = "",
}: {
  title: string
  eyebrow: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}
    >
      <div className="flex h-12 items-center justify-between border-b border-border px-4 sm:px-5">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex gap-1.5" aria-hidden="true">
            <span className="size-2 rounded-full bg-muted/25" />
            <span className="size-2 rounded-full bg-muted/25" />
            <span className="size-2 rounded-full bg-muted/25" />
          </span>
          <span className="truncate text-xs font-medium text-muted">
            {eyebrow}
          </span>
        </div>
        <span className="truncate pl-4 text-xs font-medium">{title}</span>
      </div>
      {children}
    </div>
  )
}

function SourceForm({ source }: { source: SourceExample }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 7 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.34, ease: easeOut }}
      className="grid min-h-[470px] lg:grid-cols-[calc(33.333333%_-_16px)_1fr]"
    >
      <div className="border-b border-border bg-surface-secondary p-5 lg:border-r lg:border-b-0 lg:p-7">
        <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">
          Choose a source
        </p>
        <div className="mt-5 space-y-2">
          {sourceExamples.map((item) => (
            <div
              key={item.id}
              className={[
                "flex items-center gap-3 rounded-sm border px-3 py-3",
                item.id === source.id
                  ? "border-foreground/20 bg-surface text-foreground shadow-xs"
                  : "border-transparent text-muted",
              ].join(" ")}
            >
              <IntegrationLogo
                provider={item.icon}
                size={32}
                className="rounded-sm"
              />
              <span className="text-sm font-medium">{item.label}</span>
              {item.id === source.id ? (
                <AppIcon icon="circle-check" size={15} className="ml-auto" />
              ) : null}
            </div>
          ))}
        </div>
      </div>

      <div className="p-5 sm:p-7 lg:p-9">
        <div className="flex items-start justify-between gap-5">
          <div>
            <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">
              New knowledge source
            </p>
            <h3 className="mt-2 text-xl font-medium tracking-[-0.03em]">
              {source.name}
            </h3>
          </div>
          <IntegrationLogo
            provider={source.icon}
            size={40}
            className="rounded-sm"
          />
        </div>

        <div className="mt-8 space-y-6">
          <div>
            <p className="text-xs font-medium">Source name</p>
            <div className="mt-2 rounded-sm border border-border bg-background px-3 py-2.5 text-sm">
              {source.name}
            </div>
          </div>
          <div>
            <div className="flex items-center justify-between gap-4">
              <p className="text-xs font-medium">{source.scopeLabel}</p>
              <span className="text-[0.68rem] text-muted">
                Select what agents can search
              </span>
            </div>
            <div className="mt-2 flex flex-wrap gap-2 rounded-sm border border-border bg-background p-3">
              {source.scopes.map((scope) => (
                <span
                  key={scope}
                  className="inline-flex items-center gap-1.5 rounded-sm bg-surface-secondary px-2.5 py-1.5 text-xs"
                >
                  <AppIcon icon="check" size={12} />
                  {scope}
                </span>
              ))}
            </div>
          </div>
          <div>
            <p className="text-xs font-medium">Available to teams</p>
            <div className="mt-2 flex flex-wrap gap-2">
              {source.teams.map((team) => (
                <span
                  key={team}
                  className="inline-flex items-center gap-1.5 rounded-sm border border-border px-2.5 py-1.5 text-xs text-muted"
                >
                  <AppIcon icon="users" size={12} />
                  {team}
                </span>
              ))}
            </div>
          </div>
        </div>

        <div
          data-testid="knowledge-source-actions"
          className="mt-9 flex items-center justify-end gap-2 pt-5"
        >
          <span className="rounded-sm px-3 py-2 text-xs text-muted">Back</span>
          <span className="rounded-sm bg-foreground px-3 py-2 text-xs font-medium text-background">
            Connect source
          </span>
        </div>
      </div>
    </motion.div>
  )
}

export function SourceSetupScene() {
  return (
    <MotionConfig reducedMotion="user">
      <Tabs variant="primary" defaultSelectedKey="github" className="w-full">
        <Tabs.ListContainer className="max-w-full overflow-x-auto">
          <Tabs.List
            aria-label="Choose a knowledge source"
            className="min-w-[560px]"
          >
            {sourceExamples.map((source) => (
              <Tabs.Tab id={source.id} key={source.id}>
                <span className="flex items-center justify-center gap-2 whitespace-nowrap">
                  <IntegrationLogo provider={source.icon} size={16} />
                  {source.label}
                </span>
                <Tabs.Indicator />
              </Tabs.Tab>
            ))}
          </Tabs.List>
        </Tabs.ListContainer>
        {sourceExamples.map((source) => (
          <Tabs.Panel id={source.id} key={source.id} className="mt-5 p-0">
            <Window title={source.name} eyebrow="Knowledge">
              <SourceForm source={source} />
            </Window>
          </Tabs.Panel>
        ))}
      </Tabs>
    </MotionConfig>
  )
}

const citedSources = [
  {
    number: "1",
    icon: "notion",
    title: "Audit export decision",
    detail: "Notion · Product decisions",
  },
  {
    number: "2",
    icon: "slack",
    title: "Customer requests for audit history",
    detail: "Slack · #support-escalations",
  },
  {
    number: "3",
    icon: "linear",
    title: "Export audit fields",
    detail: "Linear · Customer experience",
  },
  {
    number: "4",
    icon: "github",
    title: "CSV and PDF export schema",
    detail: "GitHub · usehivy/web",
  },
] as const

function Citation({ children }: { children: React.ReactNode }) {
  return (
    <span className="mx-0.5 inline-flex min-w-5 items-center justify-center rounded-sm bg-surface-secondary px-1 py-0.5 align-text-top text-[0.7rem] font-medium">
      {children}
    </span>
  )
}

export function CitedAnswerScene() {
  return (
    <Window title="Answer with sources" eyebrow="Product research session">
      <div className="grid min-h-[530px] lg:grid-cols-[1.35fr_0.65fr]">
        <div className="border-b border-border p-5 sm:p-8 lg:border-r lg:border-b-0 lg:p-10">
          <div className="flex gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-sm bg-foreground text-xs font-semibold text-background">
              MC
            </span>
            <div>
              <p className="text-xs font-medium text-muted">Maya’s question</p>
              <p className="mt-1 text-sm leading-6">
                What did we agree to include in the first audit export?
              </p>
            </div>
          </div>

          <div className="mt-8 border-t border-border pt-8">
            <div className="flex items-center gap-2 text-xs font-medium text-muted">
              <AppIcon icon="sparkles" size={14} />
              Research agent
            </div>
            <p className="mt-4 max-w-[62ch] text-[1.1rem] leading-8 tracking-[-0.01em]">
              The first audit export should include the approver, approval time,
              and final status in CSV and PDF. Reviewer notes stay out of the
              first release.
              <Citation>1</Citation>
            </p>
            <p className="mt-4 max-w-[62ch] text-sm leading-7 text-muted">
              Support threads ask for a record that customers can verify.
              <Citation>2</Citation> The open Linear issue names the same three
              fields and puts reviewer notes in later work.
              <Citation>3</Citation> The export schema already supports those
              fields in both formats.
              <Citation>4</Citation>
            </p>
            <div className="mt-7 rounded-sm border border-border bg-surface-secondary p-4">
              <p className="text-xs font-medium">Decision</p>
              <p className="mt-2 text-sm leading-6 text-muted">
                Ship approval metadata in the first release. Review customer use
                before adding reviewer notes.
              </p>
            </div>
          </div>
        </div>

        <div className="bg-surface-secondary p-5 sm:p-7">
          <div className="flex items-center justify-between gap-3">
            <span className="text-xs font-medium">Evidence</span>
            <span className="text-[0.68rem] text-muted">Open a source</span>
          </div>
          <div className="mt-5 space-y-2">
            {citedSources.map((source) => (
              <div
                key={source.number}
                className="flex items-center gap-3 rounded-sm border border-border bg-surface p-3"
              >
                <IntegrationLogo
                  provider={source.icon}
                  size={32}
                  className="rounded-sm"
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium">{source.title}</p>
                  <p className="mt-0.5 truncate text-[0.68rem] text-muted">
                    {source.detail}
                  </p>
                </div>
                <span className="flex size-5 items-center justify-center rounded-sm bg-foreground text-[0.62rem] text-background">
                  {source.number}
                </span>
              </div>
            ))}
          </div>
          <p className="mt-6 text-xs leading-5 text-muted">
            Each citation opens the exact page, thread, issue, or file used in
            the answer.
          </p>
        </div>
      </div>
    </Window>
  )
}
