"use client"

import { Icon } from "@iconify/react"

export function LauncherRow({
  icon,
  label,
  shortcut,
  compact,
  onPress,
}: {
  icon: string
  label: string
  shortcut: string
  compact?: boolean
  onPress: () => void
}) {
  return (
    <button
      type="button"
      onClick={onPress}
      className={`hover:bg-default flex w-full items-center gap-2.5 rounded-xl text-left text-sm transition-colors ${
        compact ? "px-2.5 py-1.5" : "bg-background px-3 py-2.5"
      }`}
    >
      <Icon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <span className="shrink-0 font-sans text-xs text-muted">{shortcut}</span>
    </button>
  )
}
