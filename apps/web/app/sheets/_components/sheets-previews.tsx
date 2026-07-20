"use client"

import type { ReactNode } from "react"
import { Button, Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"

const easeOut = [0.16, 1, 0.3, 1] as const

const reveal = {
  hidden: { opacity: 0, y: 12 },
  show: (delay = 0) => ({
    opacity: 1,
    y: 0,
    transition: { duration: 0.48, delay, ease: easeOut },
  }),
}

const leadRows = [
  {
    company: "Northline Foods",
    contact: "Dara Cole",
    status: "At risk",
    owner: "Maya",
    next: "Review support history",
  },
  {
    company: "Marrow Health",
    contact: "Jonah Bell",
    status: "Needs review",
    owner: "Leah",
    next: "Confirm renewal date",
  },
  {
    company: "Fieldwork Studio",
    contact: "Sofia Reyes",
    status: "On track",
    owner: "Omar",
    next: "Send usage summary",
  },
  {
    company: "Cinder Works",
    contact: "Eli Martin",
    status: "At risk",
    owner: "Maya",
    next: "Prepare save plan",
  },
] as const

function Status({ value }: { value: string }) {
  const className = value === "On track" ? "bg-success/15 text-success" : value === "Needs review" ? "bg-warning/15 text-warning" : "bg-accent-soft text-accent"

  return <span className={`inline-flex rounded-full px-2 py-0.5 text-[0.68rem] font-medium ${className}`}>{value}</span>
}

function SheetShell({ children, title = "Renewal review", activePage = "Accounts" }: { children: ReactNode; title?: string; activePage?: string }) {
  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface shadow-surface">
      <div className="flex h-11 items-center justify-between border-b border-border px-4">
        <div className="flex items-center gap-2.5">
          <span className="flex size-6 items-center justify-center rounded-sm bg-accent-soft text-accent">
            <AppIcon icon="table" size={14} />
          </span>
          <span className="text-sm font-medium">{title}</span>
        </div>
        <div className="flex items-center gap-3 text-xs text-muted">
          <span className="hidden items-center gap-1.5 sm:inline-flex">
            <span className="size-1.5 rounded-full bg-success" /> Live
          </span>
          <AppIcon icon="ellipsis" size={16} />
        </div>
      </div>
      <div className="flex gap-1 overflow-x-auto border-b border-border px-3 pt-2">
        {["Accounts", "Contacts", "Follow-ups"].map((page) => (
          <span
            key={page}
            className={
              page === activePage
                ? "shrink-0 rounded-t-sm border border-b-0 border-border bg-surface px-3 py-2 text-xs font-medium"
                : "shrink-0 px-3 py-2 text-xs text-muted"
            }
          >
            {page}
          </span>
        ))}
      </div>
      {children}
    </div>
  )
}

function SheetToolbar({ compact = false }: { compact?: boolean }) {
  return (
    <div className="flex min-w-max items-center gap-1.5 border-b border-border px-3 py-2">
      <div className="flex h-7 w-36 items-center gap-2 rounded-sm border border-border bg-background px-2 text-[0.68rem] text-muted">
        <AppIcon icon="search" size={12} />
        Find a record
      </div>
      <Button size="sm" variant="ghost" className="h-7 min-w-0 px-2 text-[0.68rem]">
        <AppIcon icon="list-filter" size={12} /> Add filter
      </Button>
      <Button size="sm" variant="ghost" className="h-7 min-w-0 px-2 text-[0.68rem]">
        <AppIcon icon="arrow-up-down" size={12} /> Sort rows
      </Button>
      {!compact ? (
        <>
          <Button size="sm" variant="ghost" className="h-7 min-w-0 px-2 text-[0.68rem]">
            <AppIcon icon="layout-grid" size={12} /> Saved views
          </Button>
          <span className="flex-1" />
          <Button size="sm" variant="ghost" className="h-7 min-w-0 px-2 text-[0.68rem]">
            <AppIcon icon="undo-2" size={12} /> Undo change
          </Button>
        </>
      ) : null}
    </div>
  )
}

function LeadTable({ animated = false }: { animated?: boolean }) {
  return (
    <div className="overflow-x-auto">
      <div className="min-w-[700px]">
        <div className="grid grid-cols-[38px_1.05fr_1.05fr_0.8fr_0.7fr_1.25fr] border-b border-border bg-surface-secondary text-[0.66rem] font-medium text-muted">
          <span className="px-3 py-2.5">#</span>
          <span className="border-l border-border px-3 py-2.5">Company</span>
          <span className="border-l border-border px-3 py-2.5">Contact</span>
          <span className="border-l border-border px-3 py-2.5">Status</span>
          <span className="border-l border-border px-3 py-2.5">Owner</span>
          <span className="border-l border-border px-3 py-2.5">Next step</span>
        </div>
        {leadRows.map((row, index) => {
          const content = (
            <div className="grid grid-cols-[38px_1.05fr_1.05fr_0.8fr_0.7fr_1.25fr] border-b border-border text-xs last:border-b-0">
              <span className="px-3 py-3 text-muted">{index + 1}</span>
              <span className="border-l border-border px-3 py-3 font-medium">{row.company}</span>
              <span className="border-l border-border px-3 py-3">{row.contact}</span>
              <span className="border-l border-border px-3 py-2.5">
                <Status value={row.status} />
              </span>
              <span className="border-l border-border px-3 py-3">{row.owner}</span>
              <span className="border-l border-border px-3 py-3 text-muted">{row.next}</span>
            </div>
          )

          return animated ? (
            <motion.div key={row.company} variants={reveal} custom={0.18 + index * 0.12}>
              {content}
            </motion.div>
          ) : (
            <div key={row.company}>{content}</div>
          )
        })}
      </div>
    </div>
  )
}

export function DatabaseBrowserPreview() {
  return (
    <MotionConfig reducedMotion="user">
      <motion.div initial="hidden" whileInView="show" viewport={{ once: true, amount: 0.25 }}>
        <SheetShell>
          <div className="overflow-x-auto">
            <SheetToolbar />
          </div>
          <LeadTable animated />
          <div className="flex items-center justify-between border-t border-border px-4 py-3 text-[0.68rem] text-muted">
            <span>4 account records</span>
            <span className="inline-flex items-center gap-1.5">
              <AppIcon icon="users" size={12} /> Revenue team
            </span>
          </div>
        </SheetShell>
      </motion.div>
    </MotionConfig>
  )
}

function SessionMessage({ actor, children, icon }: { actor: string; children: ReactNode; icon: string }) {
  return (
    <div className="flex gap-3 py-3">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-sm border border-border bg-surface">
        <AppIcon icon={icon} size={15} />
      </span>
      <div className="min-w-0">
        <p className="text-xs font-medium">{actor}</p>
        <div className="mt-1 text-xs leading-5 text-muted">{children}</div>
      </div>
    </div>
  )
}

export function LiveAgentUpdatePreview() {
  const newRows = leadRows.slice(0, 3)

  return (
    <MotionConfig reducedMotion="user">
      <div className="grid overflow-hidden rounded-sm border border-border bg-surface shadow-surface lg:grid-cols-[0.38fr_0.62fr]">
        <div className="border-b border-border p-5 lg:border-r lg:border-b-0 lg:p-7">
          <div className="flex items-center justify-between border-b border-border pb-3">
            <span className="text-xs font-medium">Renewal agent</span>
            <span className="text-[0.68rem] text-muted">Account review</span>
          </div>
          <SessionMessage actor="You" icon="circle-user">
            Review today’s account notes. Add each renewal with an owner and a clear next step.
          </SessionMessage>
          <motion.div initial="hidden" whileInView="show" viewport={{ once: true, amount: 0.4 }}>
            <motion.div variants={reveal} custom={0.45}>
              <SessionMessage actor="Renewal agent" icon="bot">
                I found three accounts with a named owner and next step. I added them to <span className="font-medium text-foreground">Renewal review</span>{" "}
                without changing the other records.
              </SessionMessage>
            </motion.div>
            <motion.div variants={reveal} custom={0.85} className="mt-2 rounded-sm bg-success/15 px-3 py-2 text-[0.68rem] text-success">
              <span className="inline-flex items-center gap-1.5">
                <AppIcon icon="check" size={12} /> Added 3 account records
              </span>
            </motion.div>
          </motion.div>
        </div>

        <div className="min-w-0 bg-surface-secondary p-4 md:p-6">
          <SheetShell>
            <div className="overflow-x-auto">
              <SheetToolbar compact />
            </div>
            <div className="overflow-x-auto">
              <div className="min-w-[560px]">
                <div className="grid grid-cols-[36px_1fr_0.8fr_0.8fr_1.2fr] border-b border-border bg-surface-secondary text-[0.64rem] font-medium text-muted">
                  <span className="px-2 py-2">#</span>
                  <span className="border-l border-border px-3 py-2">Company</span>
                  <span className="border-l border-border px-3 py-2">Status</span>
                  <span className="border-l border-border px-3 py-2">Owner</span>
                  <span className="border-l border-border px-3 py-2">Next step</span>
                </div>
                <motion.div initial="hidden" whileInView="show" viewport={{ once: true, amount: 0.35 }}>
                  {newRows.map((row, index) => (
                    <motion.div
                      key={row.company}
                      variants={reveal}
                      custom={0.35 + index * 0.28}
                      className="grid grid-cols-[36px_1fr_0.8fr_0.8fr_1.2fr] border-b border-border bg-surface text-[0.7rem] last:border-b-0"
                    >
                      <span className="px-2 py-3 text-muted">{index + 1}</span>
                      <span className="border-l border-border px-3 py-3 font-medium">{row.company}</span>
                      <span className="border-l border-border px-3 py-2.5">
                        <Status value={row.status} />
                      </span>
                      <span className="border-l border-border px-3 py-3">{row.owner}</span>
                      <span className="border-l border-border px-3 py-3 text-muted">{row.next}</span>
                    </motion.div>
                  ))}
                </motion.div>
              </div>
            </div>
          </SheetShell>
        </div>
      </div>
    </MotionConfig>
  )
}

const fieldGroups = [
  {
    id: "essentials",
    label: "Everyday fields",
    fields: [
      ["text", "Text", "Northline"],
      ["text", "Long text", "Security review requires…"],
      ["text-cursor-input", "Number", "24"],
      ["check", "Checkbox", "Checked"],
      ["list-ordered", "Select", "Qualified"],
      ["list-checks", "Multi-select", "Security, Legal"],
    ],
  },
  {
    id: "contact",
    label: "Contact and time",
    fields: [
      ["calendar", "Date", "2026-07-28"],
      ["link", "URL", "northline.example"],
      ["mail", "Email", "amara@example.com"],
      ["phone", "Phone", "+1 555 0142"],
    ],
  },
  {
    id: "connected",
    label: "Connected records",
    fields: [
      ["paperclip", "Attachment", "security-notes.pdf"],
      ["link", "Relation", "Amara Okafor → Northline"],
    ],
  },
] as const

function FieldTypesPreview() {
  return (
    <Tabs variant="primary" defaultSelectedKey="essentials" className="w-full">
      <Tabs.ListContainer className="max-w-full overflow-x-auto">
        <Tabs.List aria-label="Sheet field type groups" className="min-w-[520px]">
          {fieldGroups.map((group) => (
            <Tabs.Tab id={group.id} key={group.id}>
              {group.label}
            </Tabs.Tab>
          ))}
        </Tabs.List>
      </Tabs.ListContainer>
      {fieldGroups.map((group) => (
        <Tabs.Panel id={group.id} key={group.id} className="mt-6 p-0">
          <motion.div
            initial={{ opacity: 0, y: 7 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.32, ease: easeOut }}
            className="overflow-hidden rounded-sm border border-border bg-surface"
          >
            <div className="grid grid-cols-[0.8fr_1.2fr] border-b border-border bg-surface-secondary px-5 py-3 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
              <span>Field type</span>
              <span>Example value</span>
            </div>
            {group.fields.map(([icon, label, example], index) => (
              <div
                key={label}
                className={
                  index < group.fields.length - 1
                    ? "grid grid-cols-[0.8fr_1.2fr] items-center border-b border-border px-5 py-4"
                    : "grid grid-cols-[0.8fr_1.2fr] items-center px-5 py-4"
                }
              >
                <span className="flex items-center gap-2.5 text-sm font-medium">
                  <AppIcon icon={icon} size={15} className="text-muted" />
                  {label}
                </span>
                <span className="truncate text-sm text-muted">{example}</span>
              </div>
            ))}
          </motion.div>
        </Tabs.Panel>
      ))}
    </Tabs>
  )
}

function QueryPreview() {
  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface shadow-surface">
      <div className="flex flex-wrap items-center gap-2 border-b border-border p-3">
        <div className="flex h-8 min-w-[190px] flex-1 items-center gap-2 rounded-sm border border-border bg-background px-3 text-xs">
          <AppIcon icon="search" size={13} className="text-muted" />
          <span>security review</span>
        </div>
        <Button size="sm" variant="secondary">
          <AppIcon icon="list-filter" size={13} /> Status is Qualified
        </Button>
        <Button size="sm" variant="secondary">
          <AppIcon icon="arrow-up-a-z" size={13} /> Next step
        </Button>
        <Button size="sm" variant="ghost">
          <AppIcon icon="save" size={13} /> Save view
        </Button>
      </div>
      <div className="grid min-h-[330px] lg:grid-cols-[0.32fr_0.68fr]">
        <div className="border-b border-border bg-surface-secondary p-5 lg:border-r lg:border-b-0">
          <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Saved views</p>
          <div className="mt-4 space-y-1 text-sm">
            <div className="flex items-center justify-between rounded-sm bg-accent-soft px-3 py-2.5 font-medium text-accent">
              <span>Security follow-up</span>
              <span>2</span>
            </div>
            <div className="flex items-center justify-between px-3 py-2.5 text-muted">
              <span>Qualified leads</span>
              <span>8</span>
            </div>
            <div className="flex items-center justify-between px-3 py-2.5 text-muted">
              <span>No next step</span>
              <span>4</span>
            </div>
          </div>
          <p className="mt-8 text-xs leading-5 text-muted">A saved view keeps its filters, sort order, hidden fields, and column widths.</p>
        </div>
        <div className="overflow-x-auto">
          <div className="min-w-[560px]">
            <div className="grid grid-cols-[1fr_0.75fr_1.35fr] border-b border-border bg-surface-secondary px-4 py-3 text-[0.68rem] font-medium text-muted">
              <span>Company</span>
              <span>Status</span>
              <span>Next step</span>
            </div>
            {leadRows
              .filter((row) => row.status === "At risk")
              .map((row) => (
                <div key={row.company} className="grid grid-cols-[1fr_0.75fr_1.35fr] items-center border-b border-border px-4 py-4 text-sm last:border-b-0">
                  <span className="font-medium">{row.company}</span>
                  <span>
                    <Status value={row.status} />
                  </span>
                  <span className="text-muted">{row.next}</span>
                </div>
              ))}
            <div className="m-4 flex items-start gap-2.5 rounded-sm border border-border bg-background p-4 text-xs leading-5 text-muted">
              <AppIcon icon="bot" size={15} className="mt-0.5 shrink-0 text-foreground" />
              <p>
                <span className="font-medium text-foreground">Agent query:</span> Find qualified companies whose notes mention security, then return the company
                and next step.
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function SessionContinuityPreview() {
  return (
    <div className="relative grid gap-5 lg:grid-cols-[1fr_auto_1fr] lg:items-stretch">
      <div className="rounded-sm border border-border bg-surface p-6">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium">Monday, research session</span>
          <span className="text-[0.68rem] text-muted">Session 18</span>
        </div>
        <div className="mt-6 border-t border-border pt-5">
          <SessionMessage actor="Research agent" icon="bot">
            I added four interview notes to the Customer research Sheet. Each row has company, theme, quote, and source call.
          </SessionMessage>
        </div>
        <div className="mt-5 inline-flex items-center gap-2 rounded-sm bg-success/15 px-3 py-2 text-xs text-success">
          <AppIcon icon="database" size={13} /> Customer research updated
        </div>
      </div>
      <div className="hidden items-center text-muted lg:flex">
        <span className="h-px w-7 bg-border" />
        <AppIcon icon="arrow-right" size={15} />
        <span className="h-px w-7 bg-border" />
      </div>
      <div className="rounded-sm border border-border bg-surface p-6">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium">Thursday, planning session</span>
          <span className="text-[0.68rem] text-muted">Session 27</span>
        </div>
        <div className="mt-6 border-t border-border pt-5">
          <SessionMessage actor="You" icon="circle-user">
            Which interview notes mention approval history? Group the answer by customer type.
          </SessionMessage>
        </div>
        <div className="mt-5 inline-flex items-center gap-2 rounded-sm bg-accent-soft px-3 py-2 text-xs text-accent">
          <AppIcon icon="search" size={13} /> Reading the same Sheet
        </div>
      </div>
    </div>
  )
}

function AppOverSheetPreview() {
  const approvals = [
    ["Northline", "Security review", "Ready"],
    ["Marrow Labs", "Data region", "Needs input"],
    ["Cinder", "Pilot scope", "Ready"],
  ] as const

  return (
    <div className="grid overflow-hidden rounded-sm border border-border bg-surface lg:grid-cols-[0.58fr_0.42fr]">
      <div className="border-b border-border p-5 lg:border-r lg:border-b-0 lg:p-8">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Agent-built app</p>
            <h3 className="mt-2 text-lg font-medium">Deal approval queue</h3>
          </div>
          <Button size="sm">Review next</Button>
        </div>
        <div className="mt-7 overflow-hidden rounded-sm border border-border">
          {approvals.map(([company, request, state], index) => (
            <div
              key={company}
              className={
                index < approvals.length - 1
                  ? "flex items-center justify-between gap-4 border-b border-border p-4"
                  : "flex items-center justify-between gap-4 p-4"
              }
            >
              <div>
                <p className="text-sm font-medium">{company}</p>
                <p className="mt-1 text-xs text-muted">{request}</p>
              </div>
              <span
                className={
                  state === "Ready"
                    ? "rounded-full bg-success/15 px-2.5 py-1 text-[0.68rem] font-medium text-success"
                    : "rounded-full bg-warning/15 px-2.5 py-1 text-[0.68rem] font-medium text-warning"
                }
              >
                {state}
              </span>
            </div>
          ))}
        </div>
      </div>
      <div className="flex flex-col justify-between bg-surface-secondary p-6 lg:p-8">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium">
            <AppIcon icon="table" size={14} /> Sales pipeline
          </div>
          <p className="mt-5 text-sm leading-6 text-muted">
            The app reads and edits rows in one bound Sheet. Change a record in either interface and the other sees the same data.
          </p>
        </div>
        <div className="mt-10 space-y-2 text-xs">
          <div className="flex items-center justify-between border-t border-border pt-3">
            <span className="text-muted">Bound page</span>
            <span>Companies</span>
          </div>
          <div className="flex items-center justify-between border-t border-border pt-3">
            <span className="text-muted">Allowed work</span>
            <span>Row read and write</span>
          </div>
          <div className="flex items-center justify-between border-t border-border pt-3">
            <span className="text-muted">Schema changes</span>
            <span>Not allowed</span>
          </div>
        </div>
      </div>
    </div>
  )
}

function TeamBoundaryPreview() {
  const rows = [
    {
      icon: "users",
      label: "Revenue team members",
      value: "Can open and edit",
    },
    {
      icon: "bot",
      label: "Agents in Revenue sessions",
      value: "Can read and write with Sheets access",
    },
    {
      icon: "users-round",
      label: "People outside Revenue",
      value: "Cannot see this Sheet",
    },
    {
      icon: "app-window",
      label: "Bound agent-built app",
      value: "Can work with rows only",
    },
  ] as const

  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface">
      <div className="grid gap-8 border-b border-border p-6 md:grid-cols-[0.38fr_0.62fr] md:p-9">
        <div>
          <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Scope</p>
          <div className="mt-4 flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-sm bg-accent-soft text-accent">
              <AppIcon icon="shield-check" size={19} />
            </span>
            <div>
              <p className="font-medium">Revenue team</p>
              <p className="mt-1 text-xs text-muted">Sales pipeline Sheet</p>
            </div>
          </div>
        </div>
        <p className="max-w-[56ch] text-sm leading-6 text-muted">
          A Sheet belongs to one team. People need that team’s access; agents must work inside a session for the same team and have the Sheets connection before
          they can touch a record.
        </p>
      </div>
      <div>
        {rows.map((row, index) => (
          <div
            key={row.label}
            className={
              index < rows.length - 1
                ? "grid gap-2 border-b border-border px-6 py-5 sm:grid-cols-[0.9fr_1.1fr] sm:items-center md:px-9"
                : "grid gap-2 px-6 py-5 sm:grid-cols-[0.9fr_1.1fr] sm:items-center md:px-9"
            }
          >
            <span className="flex items-center gap-2.5 text-sm font-medium">
              <AppIcon icon={row.icon} size={15} className="text-muted" />
              {row.label}
            </span>
            <span className="text-sm text-muted sm:text-right">{row.value}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
