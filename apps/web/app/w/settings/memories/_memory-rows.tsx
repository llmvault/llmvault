"use client"

import { useState } from "react"
import { ListBox, Popover, Select, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  observationKind,
  type Directive,
  type Observation,
} from "@/lib/api/memory"

export function DirectiveRow({
  directive,
  onDeactivate,
  onDelete,
}: {
  directive: Directive
  onDeactivate: () => void
  onDelete: () => void
}) {
  return (
    <div className="group flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className="text-sm leading-5 text-foreground">{directive.content}</p>
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-muted-foreground rounded-md bg-default px-1.5 py-0.5 text-xs">
            {directive.source === "user-pinned"
              ? "Pinned by you"
              : "From memory"}
          </span>
          {directive.createdAt ? (
            <span className="text-muted-foreground text-xs">
              · {relativeTime(directive.createdAt)}
            </span>
          ) : null}
        </div>
      </div>
      <ActionsMenu
        label="Rule options"
        items={[
          {
            key: "deactivate",
            label: "Deactivate rule",
            icon: "circle-slash",
            onSelect: onDeactivate,
          },
          {
            key: "delete",
            label: "Delete rule",
            icon: "trash-2",
            danger: true,
            onSelect: onDelete,
          },
        ]}
      />
    </div>
  )
}

export function ObservationRow({
  observation,
  onConfirm,
  onEdit,
  onPin,
  onDelete,
}: {
  observation: Observation
  onConfirm: () => void
  onEdit: () => void
  onPin: () => void
  onDelete: () => void
}) {
  const kind = observationKind(observation.kind)
  return (
    <div className="group flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className="text-sm leading-5 text-foreground">
          {observation.content}
        </p>
        <div className="flex flex-wrap items-center gap-1.5">
          <span
            className={`rounded-md px-1.5 py-0.5 text-xs font-medium ${kind.chipClass}`}
          >
            {kind.label}
          </span>
          {observation.humanVerified ? (
            <span className="flex items-center gap-1 rounded-md bg-success/15 px-1.5 py-0.5 text-xs text-success">
              <AppIcon icon="shield-check" className="h-3 w-3" />
              Verified
            </span>
          ) : null}
          {observation.proofCount > 1 ? (
            <span className="text-muted-foreground text-xs">
              confirmed {observation.proofCount}×
            </span>
          ) : null}
          {observation.entities.map((entity) => (
            <span
              key={entity}
              className="text-muted-foreground rounded-md bg-default px-1.5 py-0.5 text-xs"
            >
              {entity}
            </span>
          ))}
          {observation.expiresAt ? (
            <span className="flex items-center gap-1 text-xs text-warning">
              <AppIcon icon="clock-alert" className="h-3 w-3" />
              Expires {shortDate(observation.expiresAt)}
            </span>
          ) : null}
          <span className="text-muted-foreground text-xs">
            ·{" "}
            {relativeTime(observation.lastMentionedAt || observation.createdAt)}
          </span>
        </div>
      </div>
      <ActionsMenu
        label="Memory options"
        items={[
          {
            key: "confirm",
            label: "Confirm",
            icon: "check-circle",
            onSelect: onConfirm,
          },
          { key: "edit", label: "Edit", icon: "pencil", onSelect: onEdit },
          {
            key: "pin",
            label: "Pin as rule",
            icon: "list-checks",
            onSelect: onPin,
          },
          {
            key: "delete",
            label: "Delete",
            icon: "trash-2",
            danger: true,
            onSelect: onDelete,
          },
        ]}
      />
    </div>
  )
}

type MenuItem = {
  key: string
  label: string
  icon: string
  danger?: boolean
  onSelect: () => void
}

function ActionsMenu({
  label,
  items,
}: {
  label: string
  items: MenuItem[]
}) {
  const [open, setOpen] = useState(false)
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={label}
        data-open={open ? "true" : undefined}
        className="text-muted-foreground -mr-1 flex shrink-0 items-center rounded-md p-1 transition-colors hover:bg-default data-[open=true]:bg-default"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement="bottom end"
          offset={6}
          className="w-52 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {items.map((item) => (
              <button
                key={item.key}
                type="button"
                onClick={() => {
                  item.onSelect()
                  setOpen(false)
                }}
                className={
                  item.danger
                    ? "flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm text-danger transition-colors hover:bg-danger/10"
                    : "flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
                }
              >
                <AppIcon icon={item.icon} className="h-4 w-4 shrink-0" />
                {item.label}
              </button>
            ))}
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}
export function ChannelSelect({
  options,
  value,
  onChange,
}: {
  options: { id: string; label: string }[]
  value: string
  onChange: (value: string) => void
}) {
  const selected = options.find((option) => option.id === value)
  return (
    <Select
      aria-label="Channel"
      selectedKey={value || null}
      onSelectionChange={(key) => onChange(key === null ? "" : String(key))}
      className="w-full sm:w-64"
    >
      <Select.Trigger className="h-10 w-full justify-between px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon
            icon="hash"
            className="text-muted-foreground h-4 w-4 shrink-0"
          />
          <span className="truncate">
            {selected?.label ?? "Select channel"}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-64 p-1.5">
        <ListBox>
          {options.map((option) => (
            <ListBox.Item
              key={option.id}
              id={option.id}
              textValue={option.label}
            >
              {option.label}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function RowSkeletons({ count }: { count: number }) {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: count }).map((_, row) => (
        <div
          key={row}
          className="flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3"
        >
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <Skeleton className="h-3.5 w-full max-w-md rounded" />
            <div className="flex items-center gap-1.5">
              <Skeleton className="h-4 w-14 rounded-md" />
              <Skeleton className="h-4 w-16 rounded-md" />
            </div>
          </div>
          <Skeleton className="h-6 w-6 rounded-md" />
        </div>
      ))}
    </div>
  )
}

export function LoadingState() {
  return (
    <div className="flex flex-col gap-8">
      <Skeleton className="h-10 w-64 rounded-lg" />
      <RowSkeletons count={2} />
      <RowSkeletons count={3} />
    </div>
  )
}

export function ErrorState({ label }: { label: string }) {
  return (
    <div className="bg-card flex min-h-32 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon
        icon="triangle-alert"
        className="text-muted-foreground h-7 w-7"
      />
      <p className="mt-3 text-sm font-medium text-foreground">{label}</p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        Refresh the page to try again.
      </p>
    </div>
  )
}

export function EmptyCard({
  icon,
  title,
  body,
}: {
  icon: string
  title: string
  body: string
}) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon icon={icon} className="text-muted-foreground h-7 w-7" />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">{body}</p>
    </div>
  )
}

function relativeTime(iso: string): string {
  if (!iso) return ""
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ""
  const seconds = Math.max(0, Math.floor((Date.now() - then) / 1000))
  if (seconds < 60) return "just now"
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  const months = Math.floor(days / 30)
  if (months < 12) return `${months}mo ago`
  return `${Math.floor(months / 12)}y ago`
}

function shortDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  return date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  })
}
