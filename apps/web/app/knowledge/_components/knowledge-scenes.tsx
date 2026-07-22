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

function Window({ title, eyebrow, children, className = "" }: { title: string; eyebrow: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}>
      <div className="flex h-12 items-center justify-between border-b border-border px-4 sm:px-5">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex gap-1.5" aria-hidden="true">
            <span className="size-2 rounded-full bg-muted/25" />
            <span className="size-2 rounded-full bg-muted/25" />
            <span className="size-2 rounded-full bg-muted/25" />
          </span>
          <span className="truncate text-xs font-medium text-muted">{eyebrow}</span>
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
        <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">Choose a source</p>
        <div className="mt-5 space-y-2">
          {sourceExamples.map((item) => (
            <div
              key={item.id}
              className={[
                "flex items-center gap-3 rounded-sm border px-3 py-3",
                item.id === source.id ? "border-foreground/20 bg-surface text-foreground shadow-xs" : "border-transparent text-muted",
              ].join(" ")}
            >
              <IntegrationLogo provider={item.icon} size={32} className="rounded-sm" />
              <span className="text-sm font-medium">{item.label}</span>
              {item.id === source.id ? <AppIcon icon="circle-check" size={15} className="ml-auto" /> : null}
            </div>
          ))}
        </div>
      </div>

      <div className="p-5 sm:p-7 lg:p-9">
        <div className="flex items-start justify-between gap-5">
          <div>
            <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">New knowledge source</p>
            <h3 className="mt-2 text-xl font-medium tracking-[-0.03em]">{source.name}</h3>
          </div>
          <IntegrationLogo provider={source.icon} size={40} className="rounded-sm" />
        </div>

        <div className="mt-8 space-y-6">
          <div>
            <p className="text-xs font-medium">Source name</p>
            <div className="mt-2 rounded-sm border border-border bg-background px-3 py-2.5 text-sm">{source.name}</div>
          </div>
          <div>
            <div className="flex items-center justify-between gap-4">
              <p className="text-xs font-medium">{source.scopeLabel}</p>
              <span className="text-[0.68rem] text-muted">Select what agents can search</span>
            </div>
            <div className="mt-2 flex flex-wrap gap-2 rounded-sm border border-border bg-background p-3">
              {source.scopes.map((scope) => (
                <span key={scope} className="inline-flex items-center gap-1.5 rounded-sm bg-surface-secondary px-2.5 py-1.5 text-xs">
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
                <span key={team} className="inline-flex items-center gap-1.5 rounded-sm border border-border px-2.5 py-1.5 text-xs text-muted">
                  <AppIcon icon="users" size={12} />
                  {team}
                </span>
              ))}
            </div>
          </div>
        </div>

        <div data-testid="knowledge-source-actions" className="mt-9 flex items-center justify-end gap-2 pt-5">
          <span className="rounded-sm px-3 py-2 text-xs text-muted">Back</span>
          <span className="rounded-sm bg-foreground px-3 py-2 text-xs font-medium text-background">Connect source</span>
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
          <Tabs.List aria-label="Choose a knowledge source" className="min-w-[560px]">
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

const sourceStates = [
  {
    icon: "github",
    name: "Engineering repositories",
    detail: "Selected repositories",
    state: "Up to date",
    stateIcon: "circle-check",
  },
  {
    icon: "notion",
    name: "Company handbook",
    detail: "Pages and databases",
    state: "Indexing changed pages",
    stateIcon: "loader-circle",
  },
  {
    icon: "slack",
    name: "Customer conversations",
    detail: "Selected channels",
    state: "Up to date",
    stateIcon: "circle-check",
  },
  {
    icon: "chrome",
    name: "Product documentation",
    detail: "Selected website sections",
    state: "Paused",
    stateIcon: "circle-alert",
  },
] as const

function IndexingScene() {
  return (
    <Window title="Source activity" eyebrow="Knowledge settings">
      <div className="grid min-h-[430px] lg:grid-cols-[1.2fr_0.8fr]">
        <div className="border-b border-border p-4 sm:p-6 lg:border-r lg:border-b-0">
          <div className="flex items-center justify-between gap-4 pb-4">
            <span className="text-sm font-medium">Knowledge sources</span>
            <span className="text-xs text-muted">Status updates live</span>
          </div>
          <div className="overflow-hidden rounded-sm border border-border">
            {sourceStates.map((source, index) => (
              <div
                key={source.name}
                className={["flex items-center gap-3 bg-surface px-3 py-4 sm:px-4", index < sourceStates.length - 1 ? "border-b border-border" : ""].join(" ")}
              >
                <IntegrationLogo provider={source.icon} size={36} className="rounded-sm" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{source.name}</p>
                  <p className="mt-0.5 truncate text-xs text-muted">{source.detail}</p>
                </div>
                <span className="hidden items-center gap-1.5 text-xs text-muted sm:flex">
                  <AppIcon icon={source.stateIcon} size={13} className={source.stateIcon === "loader-circle" ? "animate-spin" : ""} />
                  {source.state}
                </span>
              </div>
            ))}
          </div>
        </div>

        <div className="flex flex-col justify-between bg-surface-secondary p-6 sm:p-8">
          <div>
            <span className="flex size-10 items-center justify-center rounded-sm bg-surface">
              <AppIcon icon="refresh-cw" size={18} />
            </span>
            <p className="mt-6 text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">Current index run</p>
            <h3 className="mt-2 text-xl font-medium tracking-[-0.03em]">Company handbook</h3>
            <p className="mt-3 text-sm leading-6 text-muted">
              Hivy is indexing pages changed since the last successful sync. Existing documents stay searchable while the run continues.
            </p>
          </div>
          <div className="mt-10">
            <div className="h-1.5 overflow-hidden rounded-full bg-border">
              <motion.div
                initial={{ scaleX: 0.08 }}
                whileInView={{ scaleX: 0.68 }}
                viewport={{ once: true }}
                transition={{ duration: 0.8, ease: easeOut }}
                className="h-full origin-left rounded-full bg-foreground"
              />
            </div>
            <div className="mt-3 flex items-center justify-between text-xs text-muted">
              <span>Indexing changed pages</span>
              <span>Running</span>
            </div>
          </div>
        </div>
      </div>
    </Window>
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
            <span className="flex size-9 shrink-0 items-center justify-center rounded-sm bg-foreground text-xs font-semibold text-background">MC</span>
            <div>
              <p className="text-xs font-medium text-muted">Maya’s question</p>
              <p className="mt-1 text-sm leading-6">What did we agree to include in the first audit export?</p>
            </div>
          </div>

          <div className="mt-8 border-t border-border pt-8">
            <div className="flex items-center gap-2 text-xs font-medium text-muted">
              <AppIcon icon="sparkles" size={14} />
              Research agent
            </div>
            <p className="mt-4 max-w-[62ch] text-[1.1rem] leading-8 tracking-[-0.01em]">
              The first audit export should include the approver, approval time, and final status in CSV and PDF. Reviewer notes stay out of the first release.
              <Citation>1</Citation>
            </p>
            <p className="mt-4 max-w-[62ch] text-sm leading-7 text-muted">
              Support threads ask for a record that customers can verify.
              <Citation>2</Citation> The open Linear issue names the same three fields and puts reviewer notes in later work.
              <Citation>3</Citation> The export schema already supports those fields in both formats.
              <Citation>4</Citation>
            </p>
            <div className="mt-7 rounded-sm border border-border bg-surface-secondary p-4">
              <p className="text-xs font-medium">Decision</p>
              <p className="mt-2 text-sm leading-6 text-muted">
                Ship approval metadata in the first release. Review customer use before adding reviewer notes.
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
              <div key={source.number} className="flex items-center gap-3 rounded-sm border border-border bg-surface p-3">
                <IntegrationLogo provider={source.icon} size={32} className="rounded-sm" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium">{source.title}</p>
                  <p className="mt-0.5 truncate text-[0.68rem] text-muted">{source.detail}</p>
                </div>
                <span className="flex size-5 items-center justify-center rounded-sm bg-foreground text-[0.62rem] text-background">{source.number}</span>
              </div>
            ))}
          </div>
          <p className="mt-6 text-xs leading-5 text-muted">Each citation opens the exact page, thread, issue, or file used in the answer.</p>
        </div>
      </div>
    </Window>
  )
}

const documents = [
  {
    icon: "file-text",
    title: "Export requirements and decisions",
    location: "Product decisions / Exports",
    state: "Indexed",
  },
  {
    icon: "file-text",
    title: "Support escalation policy",
    location: "Operating handbook / Support",
    state: "Indexed",
  },
  {
    icon: "file-text",
    title: "Workspace import failure notes",
    location: "Engineering / Incidents",
    state: "Indexed",
  },
  {
    icon: "file-text",
    title: "Security review checklist",
    location: "Operating handbook / Security",
    state: "Indexed",
  },
] as const

function DocumentBrowserScene() {
  return (
    <Window title="Company handbook" eyebrow="Indexed documents">
      <div className="min-h-[440px] p-5 sm:p-7">
        <div className="flex flex-col gap-4 border-b border-border pb-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <IntegrationLogo provider="notion" size={40} className="rounded-sm" />
            <div>
              <p className="text-sm font-medium">Company handbook</p>
              <p className="mt-0.5 text-xs text-muted">Selected pages and databases</p>
            </div>
          </div>
          <div className="flex h-9 w-full items-center gap-2 rounded-sm border border-border bg-background px-3 text-xs text-muted sm:w-64">
            <AppIcon icon="search" size={14} />
            Search documents
          </div>
        </div>

        <div className="mt-5 overflow-hidden rounded-sm border border-border">
          {documents.map((document, index) => (
            <div
              key={document.title}
              className={["group flex items-center gap-3 bg-surface px-3 py-4 sm:px-4", index < documents.length - 1 ? "border-b border-border" : ""].join(" ")}
            >
              <AppIcon icon={document.icon} size={16} className="shrink-0 text-muted" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{document.title}</p>
                <p className="mt-0.5 truncate text-xs text-muted">{document.location}</p>
              </div>
              <span className="hidden items-center gap-1.5 text-xs text-muted sm:flex">
                <AppIcon icon="circle-check" size={13} />
                {document.state}
              </span>
              <AppIcon icon="external-link" size={14} className="text-muted" />
            </div>
          ))}
        </div>
      </div>
    </Window>
  )
}

const grants = [
  {
    team: "Customer operations",
    note: "Support and customer context",
    sources: ["Customer conversations", "Company handbook"],
  },
  {
    team: "Engineering",
    note: "Code and delivery context",
    sources: ["Engineering repositories", "Product delivery"],
  },
  {
    team: "Revenue",
    note: "Approved customer-facing material",
    sources: ["Product documentation"],
  },
  {
    team: "Product",
    note: "Customer feedback and product decisions",
    sources: ["Customer conversations", "Product delivery"],
  },
] as const

function TeamGrantScene() {
  return (
    <Window title="Team knowledge grants" eyebrow="Access control">
      <div className="grid min-h-[490px] lg:grid-cols-[0.7fr_1.3fr]">
        <div className="border-b border-border bg-foreground p-6 text-background sm:p-8 lg:border-r lg:border-b-0">
          <span className="flex size-10 items-center justify-center rounded-sm bg-background/10">
            <AppIcon icon="shield-check" size={19} />
          </span>
          <h3 className="mt-8 text-2xl leading-tight font-medium tracking-[-0.04em]">A team searches only the sources you grant it.</h3>
          <p className="mt-5 text-sm leading-6 text-background/65">
            New teams start with no knowledge access. Admins choose each source explicitly, and every agent session inherits its team’s allowlist.
          </p>
          <div className="mt-10 flex items-center gap-2 text-xs text-background/60">
            <AppIcon icon="circle-check" size={14} />
            No grant means no search result
          </div>
        </div>

        <div className="p-5 sm:p-7 lg:p-9">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-xs font-medium text-muted">Workspace teams</p>
              <p className="mt-1 text-sm font-medium">Knowledge by team</p>
            </div>
            <span className="rounded-sm border border-border px-2.5 py-1.5 text-xs text-muted">Admin controls</span>
          </div>
          <div className="mt-6 space-y-3">
            {grants.map((grant) => (
              <div key={grant.team} className="rounded-sm border border-border p-4">
                <div className="flex items-start gap-3">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-sm bg-surface-secondary">
                    <AppIcon icon="users" size={16} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium">{grant.team}</p>
                    <p className="mt-0.5 text-xs text-muted">{grant.note}</p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      {grant.sources.map((source) => (
                        <span key={source} className="inline-flex items-center gap-1.5 rounded-sm bg-surface-secondary px-2 py-1 text-[0.68rem]">
                          <AppIcon icon="database" size={11} />
                          {source}
                        </span>
                      ))}
                    </div>
                  </div>
                  <AppIcon icon="circle-check" size={17} />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </Window>
  )
}

function RefreshScene() {
  return (
    <Window title="Engineering repositories" eyebrow="Source controls">
      <div className="grid min-h-[430px] lg:grid-cols-[1.08fr_0.92fr]">
        <div className="border-b border-border p-6 sm:p-8 lg:border-r lg:border-b-0">
          <div className="flex items-start justify-between gap-5">
            <div className="flex items-center gap-3">
              <IntegrationLogo provider="github" size={40} className="rounded-sm" />
              <div>
                <p className="text-sm font-medium">Engineering repositories</p>
                <p className="mt-0.5 text-xs text-muted">Selected GitHub repositories</p>
              </div>
            </div>
            <span className="inline-flex items-center gap-1.5 text-xs text-muted">
              <AppIcon icon="circle-check" size={14} />
              Up to date
            </span>
          </div>

          <div className="mt-8 border-t border-border">
            {[
              ["Automatic refresh", "Enabled", "refresh-cw"],
              ["Search access", "Engineering, Product", "users"],
              ["Source state", "Active", "circle-check"],
              ["Document cleanup", "Removes stale records", "trash-2"],
            ].map(([label, value, icon], index) => (
              <div key={label} className={["flex items-center gap-3 py-4", index < 3 ? "border-b border-border" : ""].join(" ")}>
                <AppIcon icon={icon} size={15} className="text-muted" />
                <span className="text-sm text-muted">{label}</span>
                <span className="ml-auto text-right text-sm font-medium">{value}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="bg-surface-secondary p-6 sm:p-8">
          <p className="text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">Recent source activity</p>
          <div className="mt-6 space-y-0">
            {[
              {
                title: "Changed documents indexed",
                detail: "New and edited repository content is searchable.",
              },
              {
                title: "Missing documents removed",
                detail: "Deleted source material no longer appears in search.",
              },
              {
                title: "Search index ready",
                detail: "The last successful version remains available.",
              },
              {
                title: "Next refresh scheduled",
                detail: "Hivy will check the active source again automatically.",
              },
            ].map((event, index) => (
              <div key={event.title} className="flex gap-3">
                <div className="flex flex-col items-center">
                  <span className="mt-1 size-2 rounded-full bg-foreground" />
                  {index < 3 ? <span className="h-16 w-px bg-border" /> : null}
                </div>
                <div className="pb-7">
                  <p className="text-sm font-medium">{event.title}</p>
                  <p className="mt-1 text-xs leading-5 text-muted">{event.detail}</p>
                </div>
              </div>
            ))}
          </div>
          <div className="mt-4 flex gap-2">
            <span className="rounded-sm border border-border bg-surface px-3 py-2 text-xs font-medium">Pause ingestion</span>
            <span className="rounded-sm px-3 py-2 text-xs text-muted">View documents</span>
          </div>
        </div>
      </div>
    </Window>
  )
}
