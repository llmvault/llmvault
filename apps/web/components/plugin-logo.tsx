import type { CSSProperties } from "react"
import { AppIcon } from "@/components/icon"
import {
  BrandMark,
  IntegrationLogo,
  ProviderLogoMark,
  brandForIcon,
} from "@/components/integration-logo"
import { cn } from "@/lib/utils"
import {
  type ApiPlugin,
  pluginIcon,
  pluginIconColor,
  pluginLogoProvider,
} from "@/app/w/(chat)/plugins/_lib"

export function PluginColoredIcon({
  icon,
  color,
  size = 20,
}: {
  icon: string
  color: string
  size?: number
}) {
  return (
    <AppIcon
      icon={icon}
      className="shrink-0"
      style={{ color, width: size, height: size }}
    />
  )
}

/**
 * A plugin has a real brand mark when it maps to an integration provider logo
 * or its `icon` is itself a brand (e.g. "github"). Brand marks render on a
 * white tile; everything else renders as a white glyph on the plugin's brand
 * color.
 */
export function pluginHasBrandLogo(plugin: ApiPlugin): boolean {
  return (
    pluginLogoProvider(plugin) !== null ||
    brandForIcon(pluginIcon(plugin)) !== undefined
  )
}

/**
 * Unframed plugin logo. Callers wrap it with `pluginLogoFrameClass` /
 * `pluginLogoFrameStyle` to get the correct tile background.
 */
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
    return <ProviderLogoMark provider={provider} size={size} />
  }

  const icon = pluginIcon(plugin)
  const brand = brandForIcon(icon)
  if (brand) {
    return <BrandMark brand={brand} label={icon} size={size} />
  }

  return (
    <PluginColoredIcon
      icon={icon}
      color={forceIconWhite ? "#FFFFFF" : pluginIconColor(plugin)}
      size={iconSize}
    />
  )
}

export function pluginLogoFrameClass(
  plugin: ApiPlugin,
  className: string
): string {
  return cn(className, pluginHasBrandLogo(plugin) ? "bg-white" : "text-white")
}

export function pluginLogoFrameStyle(
  plugin: ApiPlugin
): CSSProperties | undefined {
  if (pluginHasBrandLogo(plugin)) return undefined
  return { backgroundColor: pluginIconColor(plugin) }
}

/**
 * Self-framed 32px plugin tile used in agent plugin lists. Tolerates an
 * undefined plugin (renders a neutral placeholder).
 */
export function PluginLogoTile({
  plugin,
  className,
}: {
  plugin?: ApiPlugin
  className?: string
}) {
  if (!plugin) {
    return (
      <div
        className={cn(
          "text-muted-foreground flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default",
          className
        )}
      >
        <AppIcon icon="plug" className="h-4 w-4" />
      </div>
    )
  }

  const provider = pluginLogoProvider(plugin)
  if (provider) {
    return (
      <IntegrationLogo
        provider={provider}
        size={32}
        className={cn("rounded-lg", className)}
      />
    )
  }

  const icon = pluginIcon(plugin)
  const brand = brandForIcon(icon)
  if (brand) {
    return (
      <div
        className={cn(
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white p-0.5",
          className
        )}
      >
        <BrandMark brand={brand} label={icon} className="size-full" />
      </div>
    )
  }

  return (
    <div
      className={cn(
        "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-white",
        className
      )}
      style={{ backgroundColor: pluginIconColor(plugin) }}
    >
      <AppIcon icon={icon} className="h-4 w-4" />
    </div>
  )
}
