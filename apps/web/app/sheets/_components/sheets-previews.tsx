"use client"

import { useMemo, useState } from "react"
import { Button, Popover, SearchField, Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"
import {
  leadRows,
  sheetPages,
  SheetShell,
  Status,
} from "./sheets-preview-primitives"

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
    {
      id: "dara-cole",
      cells: ["Dara Cole", "Northline Foods", "VP Operations", "Maya", "Today"],
    },
    {
      id: "jonah-bell",
      cells: [
        "Jonah Bell",
        "Marrow Health",
        "Finance lead",
        "Leah",
        "Yesterday",
      ],
    },
    {
      id: "sofia-reyes",
      cells: ["Sofia Reyes", "Fieldwork Studio", "COO", "Omar", "Jul 18"],
    },
    {
      id: "eli-martin",
      cells: [
        "Eli Martin",
        "Cinder Works",
        "Head of Support",
        "Maya",
        "Jul 16",
      ],
    },
  ],
  "Follow-ups": [
    {
      id: "support-history",
      cells: [
        "Northline Foods",
        "Review support history",
        "Today",
        "High",
        "Maya",
      ],
    },
    {
      id: "renewal-date",
      cells: [
        "Marrow Health",
        "Confirm renewal date",
        "Tomorrow",
        "Medium",
        "Leah",
      ],
    },
    {
      id: "usage-summary",
      cells: [
        "Fieldwork Studio",
        "Send usage summary",
        "Jul 24",
        "Low",
        "Omar",
      ],
    },
    {
      id: "save-plan",
      cells: ["Cinder Works", "Prepare save plan", "Jul 25", "High", "Maya"],
    },
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
                <span className="block text-xs font-medium">
                  {option.label}
                </span>
                {option.description ? (
                  <span className="mt-0.5 block text-[0.68rem] font-normal text-muted">
                    {option.description}
                  </span>
                ) : null}
              </span>
              {option.id === selected ? (
                <AppIcon icon="check" size={13} className="shrink-0" />
              ) : null}
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
      <SearchField
        aria-label="Search rows"
        value={query}
        onChange={onQueryChange}
        className="w-44"
      >
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
          {
            id: "all",
            label: "All records",
            description: "Show every row on this page",
          },
          {
            id: "attention",
            label: "Needs attention",
            description: "Show risk and high-priority work",
          },
        ]}
        onSelect={(filter) =>
          onChange({ filter: filter as DemoSheetState["filter"], view: "grid" })
        }
      />
      <ToolbarPopover
        ariaLabel="Sort rows"
        icon="arrow-up-down"
        label={
          state.sort === "manual"
            ? "Sort"
            : state.sort === "asc"
              ? "A to Z"
              : "Z to A"
        }
        active={state.sort !== "manual"}
        selected={state.sort}
        options={[
          { id: "manual", label: "Manual order" },
          { id: "asc", label: "First column, A to Z" },
          { id: "desc", label: "First column, Z to A" },
        ]}
        onSelect={(sort) =>
          onChange({ sort: sort as DemoSheetState["sort"], view: "grid" })
        }
      />
      <ToolbarPopover
        ariaLabel="Saved views"
        icon="layout-grid"
        label={state.view === "renewal-risks" ? "Renewal risks" : "Grid"}
        active={state.view !== "grid"}
        selected={state.view}
        options={[
          { id: "grid", label: "Grid", description: "The default page view" },
          {
            id: "renewal-risks",
            label: "Renewal risks",
            description: "At-risk accounts, sorted by company",
          },
        ]}
        onSelect={(view) =>
          onChange(
            view === "renewal-risks"
              ? {
                  page: "Accounts",
                  view: "renewal-risks",
                  filter: "attention",
                  sort: "asc",
                }
              : { view: "grid", filter: "all", sort: "manual" }
          )
        }
      />
      <span className="min-w-2 flex-1" />
      <Button
        size="sm"
        variant="ghost"
        className="h-8 min-w-0 px-2 text-xs"
        isDisabled={!canUndo}
        onPress={onUndo}
      >
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
            <span key={field} className="border-l border-border px-3 py-2.5">
              {field}
            </span>
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
                      <Status
                        value={cell}
                        ariaLabel={`Change status for ${record.cells[0]}`}
                        onPress={() => onChangeStatus(record)}
                      />
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
            <p className="text-xs text-muted">
              Clear the search or filter to see the full page.
            </p>
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
        ? {
            ...record,
            cells: record.cells.map((cell, index) =>
              index === 2 ? state.statusOverrides[record.id]! : cell
            ),
          }
        : record
    )
    const filtered = withOverrides.filter((record) => {
      const matchesQuery =
        !normalizedQuery ||
        record.cells.some((cell) =>
          cell.toLowerCase().includes(normalizedQuery)
        )
      const matchesAttention =
        state.filter === "all" ||
        (state.page === "Accounts" &&
          ["At risk", "Needs review"].includes(record.cells[2] ?? "")) ||
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
            onSelectionChange={(key) =>
              selectPage(String(key) as SheetPageName)
            }
            className="w-full"
          >
            {sheetPages.map((page) => (
              <Tabs.Panel id={page} key={page} className="p-0">
                {page === state.page ? (
                  <InteractiveRecordTable
                    page={page}
                    records={records}
                    onChangeStatus={changeStatus}
                  />
                ) : null}
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
          <p className="sr-only" aria-live="polite">
            {records.length} records shown
          </p>
        </SheetShell>
      </div>
    </MotionConfig>
  )
}
