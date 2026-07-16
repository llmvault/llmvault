"use client"

import { Input } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export function AdminContentHeader({
  search,
  onSearchChange,
}: {
  search: string
  onSearchChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div>
        <h2 className="text-2xl font-semibold">System credentials</h2>
        <p className="mt-1 text-sm text-muted">
          Manage global LLM provider credentials stored by the backend.
        </p>
      </div>
      <div className="relative w-full md:max-w-xs">
        <AppIcon
          icon="search"
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted"
        />
        <Input
          value={search}
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder="Search credentials"
          className="pl-9"
        />
      </div>
    </div>
  )
}
