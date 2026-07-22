"use client"

import type { ReactNode } from "react"
import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"

const easeOut = [0.16, 1, 0.3, 1] as const

export const reveal = {
  hidden: { opacity: 0, y: 12 },
  show: (delay = 0) => ({
    opacity: 1,
    y: 0,
    transition: { duration: 0.48, delay, ease: easeOut },
  }),
}

export const leadRows = [
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

export const sheetPages = ["Accounts", "Contacts", "Follow-ups"] as const

export function Status({
  value,
  onPress,
  ariaLabel,
}: {
  value: string
  onPress?: () => void
  ariaLabel?: string
}) {
  const className =
    value === "On track"
      ? "bg-success/15 text-success"
      : value === "Needs review"
        ? "bg-warning/15 text-warning"
        : "bg-accent-soft text-accent"

  const badge = (
    <span
      className={`inline-flex rounded-full px-2 py-0.5 text-[0.68rem] font-medium ${className}`}
    >
      {value}
    </span>
  )

  return onPress ? (
    <Button
      size="sm"
      variant="ghost"
      className="h-auto min-w-0 p-0"
      aria-label={ariaLabel}
      onPress={onPress}
    >
      {badge}
    </Button>
  ) : (
    badge
  )
}

export function SheetShell({
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
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label="Refresh records"
            className="size-8 min-w-8"
            onPress={onReset}
          >
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

export function SheetToolbar({ compact = false }: { compact?: boolean }) {
  return (
    <div className="flex min-w-max items-center gap-1.5 border-b border-border px-3 py-2">
      <div className="flex h-7 w-36 items-center gap-2 rounded-sm border border-border bg-background px-2 text-[0.68rem] text-muted">
        <AppIcon icon="search" size={12} />
        Find a record
      </div>
      <Button
        size="sm"
        variant="ghost"
        className="h-7 min-w-0 px-2 text-[0.68rem]"
      >
        <AppIcon icon="list-filter" size={12} /> Add filter
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="h-7 min-w-0 px-2 text-[0.68rem]"
      >
        <AppIcon icon="arrow-up-down" size={12} /> Sort rows
      </Button>
      {!compact ? (
        <>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 min-w-0 px-2 text-[0.68rem]"
          >
            <AppIcon icon="layout-grid" size={12} /> Saved views
          </Button>
          <span className="flex-1" />
          <Button
            size="sm"
            variant="ghost"
            className="h-7 min-w-0 px-2 text-[0.68rem]"
          >
            <AppIcon icon="undo-2" size={12} /> Undo change
          </Button>
        </>
      ) : null}
    </div>
  )
}
