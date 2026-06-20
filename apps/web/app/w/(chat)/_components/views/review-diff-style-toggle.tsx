"use client"

import { Icon } from "@iconify/react"
import { DIFF_STYLE_OPTIONS } from "./review-diff-config"
import type { ReviewDiffStyle } from "./review-types"

export function DiffStyleToggle({
  value,
  onChange,
}: {
  value: ReviewDiffStyle
  onChange: (value: ReviewDiffStyle) => void
}) {
  return (
    <div className="bg-surface-secondary flex shrink-0 items-center gap-0.5 rounded-lg p-0.5">
      {DIFF_STYLE_OPTIONS.map((option) => (
        <button
          key={option.id}
          type="button"
          aria-pressed={option.id === value}
          onClick={() => onChange(option.id)}
          className={`flex h-7 items-center gap-1.5 rounded-md px-2 text-xs transition-colors ${
            option.id === value
              ? "bg-background text-foreground shadow-sm"
              : "text-muted hover:text-foreground"
          }`}
        >
          <Icon icon={option.icon} className="h-3.5 w-3.5 shrink-0" />
          <span>{option.label}</span>
        </button>
      ))}
    </div>
  )
}
