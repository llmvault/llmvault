"use client"

import type { ReactNode } from "react"
import { Chip, Skeleton, Switch } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"

// Shared row/empty-state/skeleton primitives for the team provisioning lists
// (plugins + knowledge sources), kept in one place so both toggle lists in
// team-provisioning.tsx render identically.

export function SectionHeader({
  title,
  description,
}: {
  title: string
  description: string
}) {
  return (
    <div>
      <h2 className="text-sm font-medium">{title}</h2>
      <p className="mt-1 text-sm text-muted">{description}</p>
    </div>
  )
}

export function ProvisioningRow({
  icon,
  title,
  subtitle,
  on,
  disabled,
  label,
  last,
  onChange,
  readOnly,
}: {
  icon: ReactNode
  title: string
  subtitle: string
  on: boolean
  disabled?: boolean
  label: string
  last: boolean
  onChange?: (selected: boolean) => void
  readOnly?: boolean
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      {icon}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">{title}</span>
        <span className="truncate text-sm text-muted">{subtitle}</span>
      </div>
      {readOnly ? (
        <Chip size="sm" className="shrink-0">
          {on ? "Enabled" : "Disabled"}
        </Chip>
      ) : (
        <Switch
          aria-label={label}
          isSelected={on}
          isDisabled={disabled}
          onChange={onChange}
          className="shrink-0"
        >
          <Switch.Control>
            <Switch.Thumb />
          </Switch.Control>
        </Switch>
      )}
    </div>
  )
}

export function EmptyProvisioningRow({ text }: { text: string }) {
  return (
    <div className="flex items-center gap-3 px-4 py-8 text-center text-sm text-muted">
      <AppIcon icon="package-open" className="h-4 w-4 shrink-0" />
      <span>{text}</span>
    </div>
  )
}

export function ProvisioningSkeleton() {
  return (
    <div className="flex flex-col">
      {[0, 1].map((index) => (
        <div
          key={index}
          className={cn(
            "flex items-center gap-3 px-4 py-3.5",
            index === 1 ? "" : "border-b border-border"
          )}
        >
          <Skeleton className="h-8 w-8 shrink-0 rounded-lg" />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-3.5 w-32 rounded" />
            <Skeleton className="h-3 w-48 rounded" />
          </div>
          <Skeleton className="h-6 w-10 shrink-0 rounded-full" />
        </div>
      ))}
    </div>
  )
}
