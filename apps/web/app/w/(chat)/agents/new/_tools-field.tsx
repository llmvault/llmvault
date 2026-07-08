"use client"

import { AppIcon } from "@/components/icon"
import { Tooltip } from "@heroui/react"
import type { ToolGroup, ToolSelection } from "./_lib"

export function ToolsField({
  groups,
  selection,
  onToolsChange,
  lockedOn = [],
  disabled = false,
}: {
  groups: ToolGroup[]
  selection: ToolSelection
  onToolsChange: (next: ToolSelection) => void
  lockedOn?: string[]
  disabled?: boolean
}) {
  const lockedSet = new Set(lockedOn)
  const toolIds = groups.flatMap((group) => group.tools.map((tool) => tool.id))
  const count = toolIds.filter((id) => selection[id]).length

  function toggle(id: string) {
    if (disabled || lockedSet.has(id)) return
    onToolsChange({ ...selection, [id]: !selection[id] })
  }

  function setAll(value: boolean) {
    if (disabled) return
    const next: ToolSelection = { ...selection }
    for (const id of toolIds) next[id] = value || lockedSet.has(id)
    onToolsChange(next)
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">
          {count} of {toolIds.length} enabled
        </span>
        <div className="flex items-center gap-2 text-xs">
          <button
            type="button"
            className="text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
            onClick={() => setAll(true)}
            disabled={disabled}
          >
            All
          </button>
          <span className="text-muted-foreground">·</span>
          <button
            type="button"
            className="text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
            onClick={() => setAll(false)}
            disabled={disabled}
          >
            None
          </button>
        </div>
      </div>

      {groups.map((group) => (
        <div key={group.title} className="flex flex-col gap-2">
          <span className="text-xs font-medium text-muted-foreground">
            {group.title}
          </span>
          <div className="flex flex-wrap gap-2">
            {group.tools.map((tool) => {
              const on = Boolean(selection[tool.id])
              const locked = lockedSet.has(tool.id)
              const chip = (
                <button
                  type="button"
                  key={tool.id}
                  onClick={() => toggle(tool.id)}
                  disabled={disabled || locked}
                  aria-pressed={on}
                  aria-label={`${tool.label} tool`}
                  data-testid={`tool-${tool.id}`}
                  data-tool-on={on ? "true" : "false"}
                  className={[
                    "flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs transition-colors",
                    on
                      ? "border-primary/30 bg-primary/10 text-foreground"
                      : "border-border bg-card text-muted-foreground hover:text-foreground",
                    locked || disabled ? "cursor-not-allowed opacity-80" : "",
                  ].join(" ")}
                >
                  <AppIcon
                    icon={on ? "check" : "plus"}
                    className="h-3 w-3"
                  />
                  {tool.label}
                </button>
              )
              if (!locked) return chip
              return (
                <Tooltip key={tool.id} delay={200} closeDelay={0}>
                  <Tooltip.Trigger className="flex">{chip}</Tooltip.Trigger>
                  <Tooltip.Content placement="top" offset={6} className="text-xs">
                    Required so this agent can dispatch to its sub-agents.
                  </Tooltip.Content>
                </Tooltip>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}
