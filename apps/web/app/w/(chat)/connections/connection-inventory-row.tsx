"use client"

import { useState } from "react"
import { Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"

export function ConnectionInventoryRow({
  provider,
  name,
  description,
  needsName,
  canManage,
  onConfigure,
  onReconnect,
  onRename,
  onDisconnect,
}: {
  provider: string
  name: string
  description: string
  needsName: boolean
  canManage: boolean
  onConfigure?: () => void
  onReconnect?: () => void
  onRename: () => void
  onDisconnect: () => void
}) {
  return (
    <div className="group -mx-3 py-1.5">
      <div className="rounded-xl px-3 py-1.5 transition-colors group-focus-within:bg-default group-hover:bg-default">
        <div className="flex items-center gap-3">
          <IntegrationLogo provider={provider} size={36} />
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              <h3 className="truncate text-sm font-medium text-foreground">
                {name}
              </h3>
              {needsName ? (
                <span className="rounded-full bg-warning/15 px-2 py-0.5 text-xs text-warning">
                  Rename
                </span>
              ) : null}
            </div>
            <p className="text-muted-foreground truncate text-sm">
              {description}
            </p>
          </div>
          {canManage ? (
            <ConnectionOptionsMenu
              provider={provider}
              onConfigure={onConfigure}
              onReconnect={onReconnect}
              onRename={onRename}
              onDisconnect={onDisconnect}
            />
          ) : null}
        </div>
      </div>
    </div>
  )
}

function ConnectionOptionsMenu({
  provider,
  onConfigure,
  onReconnect,
  onRename,
  onDisconnect,
}: {
  provider: string
  onConfigure?: () => void
  onReconnect?: () => void
  onRename: () => void
  onDisconnect: () => void
}) {
  const [open, setOpen] = useState(false)

  function select(action: () => void) {
    setOpen(false)
    action()
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`${provider} connection options`}
        data-open={open ? "true" : undefined}
        className="text-muted-foreground flex h-7 w-7 items-center justify-center rounded-lg transition-colors group-hover:text-foreground data-[open=true]:text-foreground"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement="bottom end"
          offset={6}
          className="w-44 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {onConfigure ? (
              <Action icon="shield-check" onClick={() => select(onConfigure)}>
                Configure access
              </Action>
            ) : null}
            <Action icon="pencil" onClick={() => select(onRename)}>
              Rename
            </Action>
            {onReconnect ? (
              <Action icon="refresh-cw" onClick={() => select(onReconnect)}>
                Reconnect
              </Action>
            ) : null}
            <Action icon="unlink" danger onClick={() => select(onDisconnect)}>
              Disconnect
            </Action>
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}

function Action({
  icon,
  danger = false,
  children,
  onClick,
}: {
  icon: string
  danger?: boolean
  children: React.ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default ${danger ? "text-danger" : ""}`}
    >
      <AppIcon icon={icon} className="h-4 w-4 shrink-0" />
      {children}
    </button>
  )
}
