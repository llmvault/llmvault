"use client"

import type { ReactNode } from "react"
import { useMemo, useState } from "react"
import { Button, Popover, SearchField, Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"

const easeOut = [0.16, 1, 0.3, 1] as const

const recordListReveal = {
  hidden: {},
  show: {
    transition: {
      delayChildren: 0.18,
      staggerChildren: 0.16,
    },
  },
}

const recordReveal = {
  hidden: { opacity: 0, y: 14 },
  show: {
    opacity: 1,
    y: 0,
    transition: { duration: 0.62, ease: easeOut },
  },
}

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

const sheetPages = ["Accounts", "Contacts", "Follow-ups"] as const
type SheetPageName = (typeof sheetPages)[number]

type DemoRecord = {
  id: string
  cells: string[]
}

const pageFields: Record<SheetPageName, string[]> = {
  Accounts: ["Company", "Contact", "Status", "Owner", "Next step"],
  Contacts: ["Name", "Company", "Role", "Owner", "Last contact"],
  "Follow-ups": ["Account", "Task", "Due", "Priority", "Owner"],
}

const pageRecords: Record<SheetPageName, DemoRecord[]> = {
  Accounts: leadRows.map((row) => ({
    id: row.company,
    cells: [row.company, row.contact, row.status, row.owner, row.next],
  })),
  Contacts: [
    { id: "dara-cole", cells: ["Dara Cole", "Northline Foods", "VP Operations", "Maya", "Today"] },
    { id: "jonah-bell", cells: ["Jonah Bell", "Marrow Health", "Finance lead", "Leah", "Yesterday"] },
    { id: "sofia-reyes", cells: ["Sofia Reyes", "Fieldwork Studio", "COO", "Omar", "Jul 18"] },
    { id: "eli-martin", cells: ["Eli Martin", "Cinder Works", "Head of Support", "Maya", "Jul 16"] },
  ],
  "Follow-ups": [
    { id: "support-history", cells: ["Northline Foods", "Review support history", "Today", "High", "Maya"] },
    { id: "renewal-date", cells: ["Marrow Health", "Confirm renewal date", "Tomorrow", "Medium", "Leah"] },
    { id: "usage-summary", cells: ["Fieldwork Studio", "Send usage summary", "Jul 24", "Low", "Omar"] },
    { id: "save-plan", cells: ["Cinder Works", "Prepare save plan", "Jul 25", "High", "Maya"] },
  ],
}

type DemoSheetState = {
  page: SheetPageName
  filter: "all" | "attention"
  sort: "manual" | "asc" | "desc"
  view: "grid" | "renewal-risks"
  statusOverrides: Record<string, string>
}

const initialDemoState: DemoSheetState = {
  page: "Accounts",
  filter: "all",
  sort: "manual",
  view: "grid",
  statusOverrides: {},
}

function Status({ value, onPress, ariaLabel }: { value: string; onPress?: () => void; ariaLabel?: string }) {
  const className = value === "On track" ? "bg-success/15 text-success" : value === "Needs review" ? "bg-warning/15 text-warning" : "bg-accent-soft text-accent"

  const badge = <span className={`inline-flex rounded-full px-2 py-0.5 text-[0.68rem] font-medium ${className}`}>{value}</span>

  return onPress ? (
    <Button size="sm" variant="ghost" className="h-auto min-w-0 p-0" aria-label={ariaLabel} onPress={onPress}>
      {badge}
    </Button>
  ) : (
    badge
  )
}

function SheetShell({
  children,
  title = "Renewal review",
  activePage = "Accounts",
  showPageTabs = true,
  onReset,
}: {
  children: ReactNode
  title?: string
  activePage?: string
  showPageTabs?: boolean
  onReset?: () => void
}) {
  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface shadow-surface">
      <div className="flex h-11 items-center justify-between border-b border-border px-3">
        <div className="flex items-center gap-2.5">
          <span className="flex h-8 min-w-0 items-center gap-2 px-2 text-sm font-medium">
            <AppIcon icon="table" size={14} className="text-muted" />
            {title}
          </span>
        </div>
        {onReset ? (
          <Button variant="ghost" size="sm" isIconOnly aria-label="Refresh records" className="size-8 min-w-8" onPress={onReset}>
            <AppIcon icon="refresh-cw" size={14} className="text-muted" />
          </Button>
        ) : (
          <div className="flex items-center gap-3 text-xs text-muted">
            <span className="hidden items-center gap-1.5 sm:inline-flex">
              <span className="size-1.5 rounded-full bg-success" /> Live
            </span>
            <AppIcon icon="ellipsis" size={16} />
          </div>
        )}
      </div>
      {showPageTabs ? (
        <div className="flex gap-1 overflow-x-auto border-b border-border px-3 pt-2">
          {sheetPages.map((page) => (
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
      ) : null}
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

function ToolbarPopover({
  ariaLabel,
  icon,
  label,
  active = false,
  options,
  selected,
  onSelect,
}: {
  ariaLabel: string
  icon: string
  label: string
  active?: boolean
  options: Array<{ id: string; label: string; description?: string }>
  selected: string
  onSelect: (id: string) => void
}) {
  const [open, setOpen] = useState(false)

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={ariaLabel}
        className={
          active
            ? "flex h-8 items-center gap-1.5 rounded-sm bg-accent-soft px-2 text-xs font-medium text-accent"
            : "flex h-8 items-center gap-1.5 rounded-sm px-2 text-xs text-foreground transition-colors hover:bg-default"
        }
      >
        <AppIcon icon={icon} size={14} />
        {label}
        <AppIcon icon="chevron-down" size={11} className="text-muted" />
      </Popover.Trigger>
      <Popover.Content className="w-64 border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          {options.map((option) => (
            <Button
              key={option.id}
              size="sm"
              variant="ghost"
              className="h-auto w-full justify-start px-2.5 py-2 text-left"
              onPress={() => {
                onSelect(option.id)
                setOpen(false)
              }}
            >
              <span className="min-w-0 flex-1">
                <span className="block text-xs font-medium">{option.label}</span>
                {option.description ? <span className="mt-0.5 block text-[0.68rem] font-normal text-muted">{option.description}</span> : null}
              </span>
              {option.id === selected ? <AppIcon icon="check" size={13} className="shrink-0" /> : null}
            </Button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function InteractiveSheetToolbar({
  query,
  onQueryChange,
  state,
  onChange,
  onUndo,
  canUndo,
}: {
  query: string
  onQueryChange: (value: string) => void
  state: DemoSheetState
  onChange: (patch: Partial<DemoSheetState>) => void
  onUndo: () => void
  canUndo: boolean
}) {
  return (
    <div className="flex min-w-max items-center gap-1.5 border-b border-border px-2 py-1.5">
      <SearchField aria-label="Search rows" value={query} onChange={onQueryChange} className="w-44">
        <SearchField.Group className="h-8 border border-border bg-surface">
          <SearchField.SearchIcon className="h-3.5 w-3.5 text-muted" />
          <SearchField.Input placeholder="Search…" className="text-xs" />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      <ToolbarPopover
        ariaLabel="Filter rows"
        icon="list-filter"
        label={state.filter === "attention" ? "Needs attention" : "Filter"}
        active={state.filter !== "all"}
        selected={state.filter}
        options={[
          { id: "all", label: "All records", description: "Show every row on this page" },
          { id: "attention", label: "Needs attention", description: "Show risk and high-priority work" },
        ]}
        onSelect={(filter) => onChange({ filter: filter as DemoSheetState["filter"], view: "grid" })}
      />
      <ToolbarPopover
        ariaLabel="Sort rows"
        icon="arrow-up-down"
        label={state.sort === "manual" ? "Sort" : state.sort === "asc" ? "A to Z" : "Z to A"}
        active={state.sort !== "manual"}
        selected={state.sort}
        options={[
          { id: "manual", label: "Manual order" },
          { id: "asc", label: "First column, A to Z" },
          { id: "desc", label: "First column, Z to A" },
        ]}
        onSelect={(sort) => onChange({ sort: sort as DemoSheetState["sort"], view: "grid" })}
      />
      <ToolbarPopover
        ariaLabel="Saved views"
        icon="layout-grid"
        label={state.view === "renewal-risks" ? "Renewal risks" : "Grid"}
        active={state.view !== "grid"}
        selected={state.view}
        options={[
          { id: "grid", label: "Grid", description: "The default page view" },
          { id: "renewal-risks", label: "Renewal risks", description: "At-risk accounts, sorted by company" },
        ]}
        onSelect={(view) =>
          onChange(
            view === "renewal-risks"
              ? { page: "Accounts", view: "renewal-risks", filter: "attention", sort: "asc" }
              : { view: "grid", filter: "all", sort: "manual" }
          )
        }
      />
      <span className="min-w-2 flex-1" />
      <Button size="sm" variant="ghost" className="h-8 min-w-0 px-2 text-xs" isDisabled={!canUndo} onPress={onUndo}>
        <AppIcon icon="undo-2" size={14} /> Undo
      </Button>
    </div>
  )
}

function InteractiveRecordTable({
  page,
  records,
  onChangeStatus,
}: {
  page: SheetPageName
  records: DemoRecord[]
  onChangeStatus: (record: DemoRecord) => void
}) {
  const fields = pageFields[page]

  return (
    <div className="overflow-x-auto">
      <div className="min-w-[760px]">
        <div className="grid grid-cols-[38px_1.05fr_1.05fr_0.8fr_0.7fr_1.25fr] border-b border-border bg-surface-secondary text-[0.66rem] font-medium text-muted">
          <span className="px-3 py-2.5">#</span>
          {fields.map((field) => (
            <span key={field} className="border-l border-border px-3 py-2.5">{field}</span>
          ))}
        </div>
        {records.length > 0 ? (
          <motion.div
            key={page}
            variants={recordListReveal}
            initial="hidden"
            whileInView="show"
            viewport={{ once: true, amount: 0.25 }}
          >
            {records.map((record, index) => (
              <motion.div
                key={record.id}
                variants={recordReveal}
                className="grid grid-cols-[38px_1.05fr_1.05fr_0.8fr_0.7fr_1.25fr] border-b border-border text-xs last:border-b-0"
              >
                <span className="px-3 py-3 text-muted">{index + 1}</span>
                {record.cells.map((cell, cellIndex) => (
                  <span
                    key={`${record.id}-${fields[cellIndex]}`}
                    className={`border-l border-border px-3 ${cellIndex === 2 ? "py-2.5" : "py-3"} ${cellIndex === 0 ? "font-medium" : ""} ${cellIndex === 4 ? "text-muted" : ""}`}
                  >
                    {page === "Accounts" && cellIndex === 2 ? (
                      <Status value={cell} ariaLabel={`Change status for ${record.cells[0]}`} onPress={() => onChangeStatus(record)} />
                    ) : page === "Follow-ups" && cellIndex === 3 ? (
                      <Status value={cell} />
                    ) : (
                      cell
                    )}
                  </span>
                ))}
              </motion.div>
            ))}
          </motion.div>
        ) : (
          <div className="flex min-h-44 flex-col items-center justify-center gap-2 px-6 text-center">
            <AppIcon icon="list-filter" size={20} className="text-muted" />
            <p className="text-sm font-medium">No records match</p>
            <p className="text-xs text-muted">Clear the search or filter to see the full page.</p>
          </div>
        )}
      </div>
    </div>
  )
}

export function DatabaseBrowserPreview() {
  const [state, setState] = useState<DemoSheetState>(initialDemoState)
  const [history, setHistory] = useState<DemoSheetState[]>([])
  const [query, setQuery] = useState("")

  const commit = (patch: Partial<DemoSheetState>) => {
    setState((current) => {
      setHistory((items) => [...items, current].slice(-12))
      return { ...current, ...patch }
    })
  }

  const undo = () => {
    setHistory((items) => {
      const previous = items.at(-1)
      if (previous) setState(previous)
      return items.slice(0, -1)
    })
  }

  const records = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    const withOverrides = pageRecords[state.page].map((record) =>
      state.page === "Accounts" && state.statusOverrides[record.id]
        ? { ...record, cells: record.cells.map((cell, index) => (index === 2 ? state.statusOverrides[record.id]! : cell)) }
        : record
    )
    const filtered = withOverrides.filter((record) => {
      const matchesQuery = !normalizedQuery || record.cells.some((cell) => cell.toLowerCase().includes(normalizedQuery))
      const matchesAttention =
        state.filter === "all" ||
        (state.page === "Accounts" && ["At risk", "Needs review"].includes(record.cells[2] ?? "")) ||
        (state.page === "Follow-ups" && record.cells[3] === "High")
      return matchesQuery && matchesAttention
    })
    if (state.sort === "manual") return filtered
    return [...filtered].sort((a, b) => {
      const comparison = (a.cells[0] ?? "").localeCompare(b.cells[0] ?? "")
      return state.sort === "asc" ? comparison : -comparison
    })
  }, [query, state])

  const changeStatus = (record: DemoRecord) => {
    const current = record.cells[2] ?? "At risk"
    const statuses = ["At risk", "Needs review", "On track"]
    const next = statuses[(statuses.indexOf(current) + 1) % statuses.length]!
    commit({ statusOverrides: { ...state.statusOverrides, [record.id]: next } })
  }

  const selectPage = (page: SheetPageName) => {
    setQuery("")
    commit({ page, filter: "all", sort: "manual", view: "grid" })
  }

  return (
    <MotionConfig reducedMotion="user">
      <div data-testid="interactive-sheet-preview">
        <SheetShell
          showPageTabs={false}
          onReset={() => {
            setQuery("")
            commit(initialDemoState)
          }}
        >
          <div className="overflow-x-auto">
            <InteractiveSheetToolbar
              query={query}
              onQueryChange={setQuery}
              state={state}
              onChange={commit}
              onUndo={undo}
              canUndo={history.length > 0}
            />
          </div>
          <Tabs
            variant="primary"
            selectedKey={state.page}
            onSelectionChange={(key) => selectPage(String(key) as SheetPageName)}
            className="w-full"
          >
            {sheetPages.map((page) => (
              <Tabs.Panel id={page} key={page} className="p-0">
                {page === state.page ? <InteractiveRecordTable page={page} records={records} onChangeStatus={changeStatus} /> : null}
              </Tabs.Panel>
            ))}
            <Tabs.ListContainer className="max-w-full overflow-x-auto border-t border-border px-2 py-1">
              <Tabs.List aria-label="Sheet pages" className="min-w-max">
                {sheetPages.map((page) => (
                  <Tabs.Tab id={page} key={page}>
                    {page}
                    <Tabs.Indicator />
                  </Tabs.Tab>
                ))}
              </Tabs.List>
            </Tabs.ListContainer>
          </Tabs>
          <p className="sr-only" aria-live="polite">{records.length} records shown</p>
        </SheetShell>
      </div>
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
      <div
        data-testid="sheet-agent-update-grid"
        className="grid overflow-hidden rounded-sm border border-border bg-surface shadow-surface lg:grid-cols-[1fr_2fr]"
      >
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
              <Tabs.Indicator />
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
