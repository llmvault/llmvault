"use client"

import { Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import type { AdminIntegrationDefinition } from "./types"

export function IntegrationList({
  loading,
  integrations,
  onSelect,
}: {
  loading: boolean
  integrations: AdminIntegrationDefinition[]
  onSelect: (definition: AdminIntegrationDefinition) => void
}) {
  if (loading) return <IntegrationSkeleton />

  if (integrations.length === 0) {
    return (
      <div className="bg-surface rounded-2xl border border-border px-6 py-10 text-center text-sm text-muted">
        No integrations found.
      </div>
    )
  }

  return (
    <div className="bg-surface flex flex-col divide-y divide-border rounded-2xl border border-border">
      {integrations.map((definition, index) => (
        <IntegrationRow
          key={definition.id ?? definition.unique_key ?? index}
          definition={definition}
          onSelect={() => onSelect(definition)}
        />
      ))}
    </div>
  )
}

function IntegrationRow({
  definition,
  onSelect,
}: {
  definition: AdminIntegrationDefinition
  onSelect: () => void
}) {
  const displayName = definition.display_name ?? "Untitled integration"
  const provider = definition.provider ?? ""
  const existing = definition.existing
  const activeConnections = existing?.active_connections ?? 0

  return (
    <button
      type="button"
      onClick={onSelect}
      className="group hover:bg-default flex min-h-20 items-center justify-between gap-4 px-4 py-3 text-left transition-colors"
    >
      <div className="flex min-w-0 items-center gap-3">
        {provider ? (
          <IntegrationLogo
            provider={provider}
            className="size-10 border border-border bg-background p-2"
          />
        ) : (
          <div className="flex size-10 items-center justify-center rounded-lg border border-border bg-background">
            <AppIcon icon="plug" className="size-4 text-muted" />
          </div>
        )}
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <p className="truncate text-sm font-medium">{displayName}</p>
            <StatusPill
              label={existing ? "Configured" : "Not configured"}
              tone={existing ? "default" : "muted"}
            />
          </div>
          <p className="mt-1 truncate text-xs text-muted">
            {definition.unique_key ?? "unknown"} ·{" "}
            {definition.auth_mode ?? "No auth"}
          </p>
        </div>
      </div>
      <div className="hidden items-center gap-2 sm:flex">
        {definition.supports_rag_source ? (
          <StatusPill label="RAG" tone="muted" />
        ) : null}
        {activeConnections > 0 ? (
          <StatusPill label={`${activeConnections} connected`} tone="muted" />
        ) : null}
        <span className="rounded-full border border-border px-3 py-1 text-xs font-medium text-foreground">
          {existing ? "Update" : "Configure"}
        </span>
      </div>
    </button>
  )
}

function IntegrationSkeleton() {
  return (
    <div className="bg-surface flex flex-col divide-y divide-border rounded-2xl border border-border">
      {Array.from({ length: 6 }).map((_, index) => (
        <div key={index} className="flex items-center gap-3 px-4 py-3">
          <div className="bg-default size-10 animate-pulse rounded-lg" />
          <div className="min-w-0 flex-1 space-y-2">
            <div className="bg-default h-4 w-40 animate-pulse rounded" />
            <div className="bg-default h-3 w-64 max-w-full animate-pulse rounded" />
          </div>
          <Spinner size="sm" className="opacity-0" />
        </div>
      ))}
    </div>
  )
}

function StatusPill({
  label,
  tone = "default",
}: {
  label: string
  tone?: "default" | "muted"
}) {
  return (
    <span
      className={
        tone === "default"
          ? "bg-default shrink-0 rounded-full border border-border px-2 py-0.5 text-[11px] font-medium text-foreground"
          : "shrink-0 rounded-full border border-border px-2 py-0.5 text-[11px] font-medium text-muted"
      }
    >
      {label}
    </span>
  )
}
