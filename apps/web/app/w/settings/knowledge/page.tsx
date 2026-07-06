"use client"

import { useState, type ComponentProps } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { Button, Popover, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import { ProviderIcon } from "./_provider-icon"
import {
  providerMeta,
  STATIC_SOURCES,
  type KnowledgeSource,
  type SourceStatus,
} from "./_data"

const STATUS_META: Record<
  SourceStatus,
  { label: string; icon: string; className: string; barClassName: string }
> = {
  syncing: {
    label: "Syncing",
    icon: "loader-circle",
    className: "text-primary",
    barClassName: "bg-primary",
  },
  active: {
    label: "Up to date",
    icon: "circle-check",
    className: "text-foreground",
    barClassName: "bg-primary",
  },
  paused: {
    label: "Paused",
    icon: "circle-slash",
    className: "text-muted-foreground",
    barClassName: "bg-muted-foreground/50",
  },
  disabled: {
    label: "Disabled",
    icon: "eye-off",
    className: "text-muted-foreground",
    barClassName: "bg-muted-foreground/40",
  },
  error: {
    label: "Sync failed",
    icon: "circle-alert",
    className: "text-danger",
    barClassName: "bg-danger",
  },
}

export default function KnowledgeSettingsPage() {
  const router = useRouter()
  const [sources, setSources] = useState<KnowledgeSource[]>(STATIC_SOURCES)

  function remove(id: string) {
    const source = sources.find((s) => s.id === id)
    setSources((current) => current.filter((s) => s.id !== id))
    if (source) toast.success(`Removed ${source.name}`)
  }

  function setStatus(id: string, status: SourceStatus, message: string) {
    setSources((current) =>
      current.map((s) => (s.id === id ? { ...s, status } : s))
    )
    toast.success(message)
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-foreground">Knowledge</h1>
          <p className="mt-1 max-w-lg text-sm text-muted-foreground">
            Connect sources so your agents can search company knowledge. Each
            source ingests a scope you choose and is available to a channel.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          className="shrink-0"
          onPress={() => router.push("/w/settings/knowledge/new")}
        >
          <AppIcon icon="plus" className="h-4 w-4" />
          Add source
        </Button>
      </div>

      {sources.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="flex flex-col gap-2">
          {sources.map((source) => (
            <SourceCard
              key={source.id}
              source={source}
              onRemove={() => remove(source.id)}
              onSetStatus={(status, message) =>
                setStatus(source.id, status, message)
              }
            />
          ))}
        </div>
      )}
    </div>
  )
}

function SourceCard({
  source,
  onRemove,
  onSetStatus,
}: {
  source: KnowledgeSource
  onRemove: () => void
  onSetStatus: (status: SourceStatus, message: string) => void
}) {
  const provider = providerMeta(source.provider)
  const status = STATUS_META[source.status]

  return (
    <div className="flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3.5">
      <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
        <ProviderIcon icon={provider.icon} className="h-5 w-5" />
      </div>

      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex min-w-0 flex-col gap-0.5">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium text-foreground">
              {source.name}
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {provider.label}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
            <span className="truncate">{source.scopeSummary}</span>
            <span aria-hidden>·</span>
            <span className="inline-flex items-center gap-1">
              <AppIcon icon="hash" className="h-3 w-3" />
              {source.channel}
            </span>
          </div>
        </div>

        <ProgressRow source={source} statusMeta={status} />
      </div>

      <SourceActionsMenu
        source={source}
        onRemove={onRemove}
        onSetStatus={onSetStatus}
      />
    </div>
  )
}

function ProgressRow({
  source,
  statusMeta,
}: {
  source: KnowledgeSource
  statusMeta: (typeof STATUS_META)[SourceStatus]
}) {
  const { percent } = source.progress
  const indeterminate = percent === null
  return (
    <div className="flex flex-col gap-1.5">
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-default">
        {indeterminate ? (
          <div
            className={cn(
              "h-full w-1/3 rounded-full",
              statusMeta.barClassName,
              source.status === "syncing" && "animate-pulse"
            )}
          />
        ) : (
          <div
            className={cn("h-full rounded-full transition-all", statusMeta.barClassName)}
            style={{ width: `${percent}%` }}
          />
        )}
      </div>
      <div className="flex items-center gap-1.5">
        <AppIcon
          icon={statusMeta.icon}
          className={cn(
            "h-3.5 w-3.5",
            statusMeta.className,
            source.status === "syncing" && "animate-spin"
          )}
        />
        <span className={cn("text-xs", statusMeta.className)}>
          {source.progress.label}
        </span>
      </div>
    </div>
  )
}

function SourceActionsMenu({
  source,
  onRemove,
  onSetStatus,
  placement = "bottom end",
}: {
  source: KnowledgeSource
  onRemove: () => void
  onSetStatus: (status: SourceStatus, message: string) => void
  placement?: ComponentProps<typeof Popover.Content>["placement"]
}) {
  const [open, setOpen] = useState(false)
  const disabled = source.status === "disabled"
  const paused = source.status === "paused"

  function act(fn: () => void) {
    fn()
    setOpen(false)
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label="Source options"
        data-open={open ? "true" : undefined}
        className="hover:bg-default data-[open=true]:bg-default -mr-1 flex shrink-0 items-center rounded-md p-1 text-muted-foreground transition-colors"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement={placement}
          offset={6}
          className="w-56 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            <MenuLink href={`/w/settings/knowledge/${source.id}/documents`}>
              View documents
            </MenuLink>

            {paused ? (
              <MenuButton
                onClick={() => act(() => onSetStatus("syncing", `Resumed ${source.name}`))}
              >
                Resume ingestion
              </MenuButton>
            ) : (
              <MenuButton
                onClick={() => act(() => onSetStatus("paused", `Paused ${source.name}`))}
              >
                Pause ingestion
              </MenuButton>
            )}

            {disabled ? (
              <MenuButton
                onClick={() => act(() => onSetStatus("active", `Enabled ${source.name}`))}
              >
                Enable source
              </MenuButton>
            ) : (
              <MenuButton
                onClick={() => act(() => onSetStatus("disabled", `Disabled ${source.name}`))}
              >
                Disable source
              </MenuButton>
            )}

            <div className="my-1 h-px bg-border" />

            <MenuButton danger onClick={() => act(onRemove)}>
              Remove source
            </MenuButton>
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}

function MenuButton({
  children,
  onClick,
  danger,
}: {
  children: React.ReactNode
  onClick: () => void
  danger?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-center rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors",
        danger
          ? "text-danger hover:bg-danger/10"
          : "hover:bg-default text-foreground"
      )}
    >
      {children}
    </button>
  )
}

function MenuLink({
  href,
  children,
}: {
  href: string
  children: React.ReactNode
}) {
  return (
    <Link
      href={href}
      className="hover:bg-default flex items-center rounded-xl px-2.5 py-1.5 text-left text-sm text-foreground transition-colors"
    >
      {children}
    </Link>
  )
}

function EmptyState() {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="folder-open" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        No knowledge sources yet
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        Add a source to let your agents search company knowledge from GitHub,
        Notion, Slack, or your website.
      </p>
      <Link
        href="/w/settings/knowledge/new"
        className="mt-4 inline-flex items-center gap-2 text-sm font-medium text-primary transition-colors hover:text-primary/80"
      >
        <AppIcon icon="plus" className="h-4 w-4" />
        Add source
      </Link>
    </div>
  )
}
