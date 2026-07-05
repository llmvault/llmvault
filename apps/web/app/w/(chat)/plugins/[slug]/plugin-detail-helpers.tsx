"use client"

import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { isDatabaseProvider } from "@/app/w/(chat)/plugins/database-connection-modal-content"
import {
  pluginRequirementKind,
  type PluginRequirement,
} from "@/app/w/(chat)/plugins/_lib"

export function RequirementLogo({
  requirement,
}: {
  requirement: PluginRequirement
}) {
  const provider = requirement.provider
  if (!provider) {
    return (
      <div className="bg-default flex h-7 w-7 shrink-0 items-center justify-center rounded-lg">
        <AppIcon icon="plug" className="h-4 w-4 text-muted-foreground" />
      </div>
    )
  }

  return (
    <IntegrationLogo provider={provider} size={28} className="rounded-lg" />
  )
}

export function providerLabel(provider: string): string {
  return provider
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}

export function connectionKindLabel(requirement: PluginRequirement): string {
  return pluginRequirementKind(requirement) === "database"
    ? "Database"
    : "Integration"
}

function sameRequirement(
  left: PluginRequirement,
  right: PluginRequirement
): boolean {
  return (
    (left.provider ?? "") === (right.provider ?? "") &&
    pluginRequirementKind(left) === pluginRequirementKind(right)
  )
}

export function isRequirementMissing(
  requirement: PluginRequirement,
  missing: PluginRequirement[]
): boolean {
  return missing.some((item) => sameRequirement(requirement, item))
}

export function isIntegrationRequirement(
  requirement: PluginRequirement | null | undefined
): requirement is PluginRequirement & { provider: string } {
  return (
    pluginRequirementKind(requirement) === "integration" &&
    Boolean(requirement?.provider)
  )
}

export function isDatabaseRequirement(
  requirement: PluginRequirement | null | undefined
): requirement is PluginRequirement & { provider: string } {
  return (
    pluginRequirementKind(requirement) === "database" &&
    isDatabaseProvider(requirement?.provider)
  )
}
