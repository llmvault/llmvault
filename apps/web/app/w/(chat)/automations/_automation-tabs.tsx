"use client"

import type { ReactNode } from "react"
import { useState } from "react"
import { Input } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"

export type AutomationsTab = "connections" | "schedules" | "webhooks"

const TABS: { key: AutomationsTab; label: string }[] = [
  { key: "connections", label: "Connections" },
  { key: "schedules", label: "Schedules" },
  { key: "webhooks", label: "Webhooks" },
]

export function AutomationsTabs({
  active,
  onChange,
}: {
  active: AutomationsTab
  onChange: (tab: AutomationsTab) => void
}) {
  return (
    <nav aria-label="Automations" className="flex items-center gap-1">
      {TABS.map((tab) => (
        <button
          key={tab.key}
          type="button"
          onClick={() => onChange(tab.key)}
          aria-current={active === tab.key ? "page" : undefined}
          className={cn(
            "rounded-lg px-3 py-1.5 text-sm font-medium transition-colors",
            active === tab.key
              ? "bg-default text-foreground"
              : "text-muted-foreground hover:bg-default/60 hover:text-foreground"
          )}
        >
          {tab.label}
        </button>
      ))}
    </nav>
  )
}

/** Shared page frame so every tab lays out identically to the Connections view. */
export function AutomationsPageShell({ children }: { children: ReactNode }) {
  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">{children}</div>
      </div>
    </div>
  )
}

/**
 * Static tab view following the Connections design (heading + search + empty
 * card). Used as a placeholder until Schedules (DB schedules) and Webhooks
 * (HTTP-based triggers) are wired up.
 */
export function AutomationPlaceholderView({
  nav,
  title,
  description,
  searchLabel,
  icon,
  emptyTitle,
  emptyBody,
}: {
  nav: ReactNode
  title: string
  description: string
  searchLabel: string
  icon: string
  emptyTitle: string
  emptyBody: string
}) {
  const [query, setQuery] = useState("")

  return (
    <AutomationsPageShell>
      {nav}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-foreground">{title}</h1>
          <p className="text-muted-foreground mt-1 text-sm">{description}</p>
        </div>
      </div>

      <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative min-w-0 flex-1">
          <AppIcon
            icon="search"
            className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2"
          />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={`Search ${searchLabel}`}
            className="bg-card h-10 w-full rounded-md pl-9"
          />
        </div>
      </div>

      <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
        <AppIcon icon={icon} className="text-muted-foreground h-7 w-7" />
        <p className="mt-3 text-sm font-medium text-foreground">{emptyTitle}</p>
        <p className="text-muted-foreground mt-1 max-w-sm text-sm">{emptyBody}</p>
      </div>
    </AutomationsPageShell>
  )
}
