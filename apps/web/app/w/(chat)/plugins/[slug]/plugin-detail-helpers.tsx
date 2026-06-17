"use client"

import Image from "next/image"
import { Icon } from "@iconify/react"
import { IntegrationLogo, integrationLogoURL } from "@/components/integration-logo"
import { cn } from "@/lib/utils"
import { isDatabaseProvider } from "@/app/w/(chat)/plugins/database-connection-modal-content"
import {
  type ApiPlugin,
  pluginIcon,
  pluginIconColor,
  pluginLogoProvider,
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
        <Icon icon="lucide:plug" className="h-4 w-4 text-muted-foreground" />
      </div>
    )
  }

  return (
    <IntegrationLogo provider={provider} size={28} className="rounded-lg" />
  )
}

function AppIcon({
  icon,
  color,
  size = 20,
}: {
  icon: string
  color: string
  size?: number
}) {
  return (
    <Icon
      icon={icon}
      className="shrink-0"
      style={{ color, width: size, height: size }}
    />
  )
}

export function PluginLogo({
  plugin,
  size,
  iconSize = size,
  forceIconWhite = false,
}: {
  plugin: ApiPlugin
  size: number
  iconSize?: number
  forceIconWhite?: boolean
}) {
  const provider = pluginLogoProvider(plugin)
  if (provider) {
    return (
      <Image
        src={integrationLogoURL(provider)}
        alt={provider}
        width={size}
        height={size}
        className="shrink-0 object-contain"
        style={{ width: size, height: size }}
      />
    )
  }
  return (
    <AppIcon
      icon={pluginIcon(plugin)}
      color={forceIconWhite ? "#FFFFFF" : pluginIconColor(plugin)}
      size={iconSize}
    />
  )
}

export function pluginLogoFrameClass(
  plugin: ApiPlugin,
  className: string
): string {
  return cn(className, pluginLogoProvider(plugin) ? "bg-white" : "text-white")
}

export function pluginLogoFrameStyle(plugin: ApiPlugin) {
  if (pluginLogoProvider(plugin)) return undefined
  return { backgroundColor: pluginIconColor(plugin) }
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
