"use client"

import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import type { AdminTab } from "./types"

const adminSections: Array<{ id: AdminTab; label: string; icon: string }> = [
  { id: "integrations", label: "Integrations", icon: "plug" },
  { id: "credentials", label: "System credentials", icon: "key-round" },
]

export function AdminSidebar({
  hasSecret,
  activeTab,
  fetching,
  onRefresh,
  onClearSecret,
  onTabChange,
}: {
  hasSecret: boolean
  activeTab: AdminTab
  fetching: boolean
  onRefresh: () => void
  onClearSecret: () => void
  onTabChange: (tab: AdminTab) => void
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
          {adminSections.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => onTabChange(item.id)}
              className={cn(
                "flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors",
                item.id === activeTab ? "bg-default" : "hover:bg-default"
              )}
            >
              <AppIcon icon={item.icon} className="size-4 shrink-0 text-muted" />
              <span className="min-w-0 flex-1 truncate">{item.label}</span>
            </button>
          ))}
        </nav>
      ) : null}
    </aside>
  )
}
