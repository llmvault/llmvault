"use client"

import type { KeyboardEvent } from "react"
import { cn } from "@/lib/utils"

export type TeamDetailTab =
  | "overview"
  | "connections"
  | "skills"
  | "knowledge"
  | "environment-variables"

type TeamDetailTabOption = {
  id: TeamDetailTab
  label: string
  adminOnly?: boolean
}

const TEAM_DETAIL_TAB_OPTIONS: readonly TeamDetailTabOption[] = [
  { id: "overview", label: "Overview" },
  { id: "connections", label: "Connections" },
  { id: "skills", label: "Skills" },
  { id: "knowledge", label: "Knowledge", adminOnly: true },
  { id: "environment-variables", label: "Env" },
]

export function teamDetailTabs(
  isAdmin: boolean
): readonly TeamDetailTabOption[] {
  return TEAM_DETAIL_TAB_OPTIONS.filter((tab) => isAdmin || !tab.adminOnly)
}

type TeamDetailTabsProps = {
  tabs: readonly TeamDetailTabOption[]
  activeTab: TeamDetailTab
  onSelect: (tab: TeamDetailTab) => void
}

export function TeamDetailTabs({
  tabs,
  activeTab,
  onSelect,
}: TeamDetailTabsProps) {
  function handleKeyDown(
    event: KeyboardEvent<HTMLButtonElement>,
    index: number
  ) {
    let nextIndex: number | undefined

    if (event.key === "Home") {
      nextIndex = 0
    } else if (event.key === "End") {
      nextIndex = tabs.length - 1
    } else if (event.key === "ArrowLeft") {
      nextIndex = (index - 1 + tabs.length) % tabs.length
    } else if (event.key === "ArrowRight") {
      nextIndex = (index + 1) % tabs.length
    }

    if (nextIndex === undefined) return

    event.preventDefault()
    const nextTab = tabs[nextIndex]
    onSelect(nextTab.id)
    document.getElementById(`team-${nextTab.id}-tab`)?.focus()
  }

  return (
    <div className="overflow-x-auto">
      <div
        aria-label="Team settings"
        className="flex min-w-max gap-6 border-b border-border"
        role="tablist"
      >
        {tabs.map((tab, index) => {
          const isSelected = tab.id === activeTab

          return (
            <button
              aria-controls={`team-${tab.id}-panel`}
              aria-selected={isSelected}
              className={cn(
                "-mb-px border-b-2 px-1 pb-2 text-sm font-medium transition-colors",
                isSelected
                  ? "border-foreground text-foreground"
                  : "text-muted-foreground border-transparent hover:text-foreground"
              )}
              id={`team-${tab.id}-tab`}
              key={tab.id}
              onClick={() => onSelect(tab.id)}
              onKeyDown={(event) => handleKeyDown(event, index)}
              role="tab"
              tabIndex={isSelected ? 0 : -1}
              type="button"
            >
              {tab.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}
