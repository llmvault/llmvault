"use client"

import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export function AdminSidebar({
  hasSecret,
  fetching,
  onRefresh,
  onClearSecret,
}: {
  hasSecret: boolean
  fetching: boolean
  onRefresh: () => void
  onClearSecret: () => void
}) {
  return (
    <aside className="bg-surface flex w-72 shrink-0 flex-col border-r border-border">
      <div className="flex flex-col gap-4 px-4 py-5">
        <div className="flex items-center gap-3">
          <div className="flex size-9 items-center justify-center rounded-xl border border-border bg-background">
            <AppIcon icon="shield-check" className="size-4" />
          </div>
          <div className="min-w-0">
            <p className="text-xs tracking-[0.14em] text-muted uppercase">
              Admin
            </p>
            <h1 className="truncate text-sm font-semibold">System setup</h1>
          </div>
        </div>

        {hasSecret ? (
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="secondary"
              isIconOnly
              aria-label="Refresh admin data"
              isPending={fetching}
              onPress={onRefresh}
            >
              <AppIcon icon="refresh-cw" className="size-4" />
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="justify-start"
              onPress={onClearSecret}
            >
              Change secret
            </Button>
          </div>
        ) : null}
      </div>

      {hasSecret ? (
        <nav className="flex flex-col gap-0.5 px-3">
          <span className="px-3 pb-1 text-xs text-muted">Setup</span>
          <div className="flex items-center gap-2.5 rounded-lg bg-default px-3 py-1.5 text-sm">
            <AppIcon icon="key-round" className="size-4 shrink-0 text-muted" />
            <span className="min-w-0 flex-1 truncate">System credentials</span>
          </div>
        </nav>
      ) : null}
    </aside>
  )
}
