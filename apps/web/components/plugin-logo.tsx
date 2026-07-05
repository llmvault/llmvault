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
 * Self-framed 32px logo tile keyed by an integration provider (brand logo on a
 * white tile), a brand `icon` (brand mark on white), or an arbitrary `icon` +
 * brand `color` (white glyph on the color). Shared by plugin and automation
 * lists so their icons render identically.
 */
const TILE_SIZES: Record<number, { box: string; glyph: string; pad: string }> = {
  32: { box: "h-8 w-8", glyph: "h-4 w-4", pad: "p-1" },
  40: { box: "h-10 w-10", glyph: "h-5 w-5", pad: "p-1.5" },
  48: { box: "h-12 w-12", glyph: "h-6 w-6", pad: "p-2" },
}

export function LogoTile({
  provider,
  icon,
  color,
  size = 32,
  className,
}: {
  provider?: string | null
  icon: string
  color: string
  size?: 32 | 40 | 48
  className?: string
}) {
  const { box, glyph, pad } = TILE_SIZES[size] ?? TILE_SIZES[32]

  if (provider) {
    return (
      <IntegrationLogo
        provider={provider}
        size={size}
        className={cn("rounded-lg", className)}
      />
    )
  }

  const brand = brandForIcon(icon)
  if (brand) {
    return (
      <div
        className={cn(
          "flex shrink-0 items-center justify-center rounded-lg bg-white",
          box,
          pad,
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
        "flex shrink-0 items-center justify-center rounded-lg text-white",
        box,
        className
      )}
      style={{ backgroundColor: color }}
    >
      <AppIcon icon={icon} className={glyph} />
    </div>
  )
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

  return (
    <LogoTile
      provider={pluginLogoProvider(plugin)}
      icon={pluginIcon(plugin)}
      color={pluginIconColor(plugin)}
      className={className}
    />
  )
}
