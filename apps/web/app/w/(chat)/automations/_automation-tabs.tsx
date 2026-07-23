"use client"

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
