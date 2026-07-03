"use client"

import { ListBox, Select } from "@heroui/react"

/** Compact HeroUI Select + ListBox for dense toolbar rows. */
export function ToolbarSelect({
  value,
  onChange,
  options,
  ariaLabel,
  className = "",
  placeholder = "Select…",
}: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  ariaLabel: string
  className?: string
  placeholder?: string
}) {
  const selectedLabel = options.find((option) => option.value === value)?.label
  return (
    <Select
      aria-label={ariaLabel}
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className={`min-w-0 ${className}`}
    >
      <Select.Trigger className="flex h-8 w-full min-w-0 items-center justify-between gap-1 rounded-lg border border-border bg-surface px-2 text-xs text-foreground">
        <span className={`truncate ${selectedLabel ? "" : "text-muted"}`}>
          {selectedLabel ?? placeholder}
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="min-w-44 rounded-xl p-1">
        <ListBox>
          {options.map((option) => (
            <ListBox.Item
              key={option.value}
              id={option.value}
              textValue={option.label}
            >
              <span className="text-xs">{option.label}</span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
