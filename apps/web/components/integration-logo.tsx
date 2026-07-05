import type { ComponentType, SVGProps } from "react"
import Image from "next/image"
import ThesvgChrome from "@thesvg/react/chrome"
import ThesvgGithub from "@thesvg/react/github"
import ThesvgGoogle from "@thesvg/react/google"
import ThesvgLinear from "@thesvg/react/linear"
import ThesvgMongodb from "@thesvg/react/mongodb"
import ThesvgMysql from "@thesvg/react/mysql"
import ThesvgNotion from "@thesvg/react/notion"
import ThesvgPlaywright from "@thesvg/react/playwright"
import ThesvgPostgresql from "@thesvg/react/postgresql"
import ThesvgRailway from "@thesvg/react/railway"
import ThesvgRedis from "@thesvg/react/redis"
import ThesvgSlack from "@thesvg/react/slack"
import ThesvgVercel from "@thesvg/react/vercel"
import { cn } from "@/lib/utils"

const LOGO_BASE = "https://connections.usehivy.com/images/template-logos"

const LOGO_PROVIDER_ALIASES: Record<string, string> = {
  "github-app-code-reviews": "github-app",
}

const LOCAL_PROVIDER_LOGOS: Record<string, string> = {
  glitchtip: "/logomarks/glitchtip.svg",
  mongodb: "/logomarks/mongodb.svg",
  mysql: "/logomarks/mysql.svg",
  postgres: "/logomarks/postgres.svg",
}

type BrandComponent = ComponentType<SVGProps<SVGSVGElement>>

/**
 * A @thesvg/react brand mark plus the variant that renders it visibly on our
 * white tile. thesvg's `default` variant is inconsistent: some brands ship the
 * dark-mode (white) mark as their default, which is invisible on white. For
 * those we pick `light` (dark ink) or `mono` (a monochrome path we tint with
 * the brand `color`).
 */
type BrandConfig = {
  Logo: BrandComponent
  variant?: "default" | "light" | "dark" | "mono" | "wordmark"
  color?: string
}

const BRANDS: Record<string, BrandConfig> = {
  // `default` is already colored/dark — renders on white as-is.
  github: { Logo: ThesvgGithub },
  slack: { Logo: ThesvgSlack },
  linear: { Logo: ThesvgLinear },
  playwright: { Logo: ThesvgPlaywright },
  mongodb: { Logo: ThesvgMongodb },
  redis: { Logo: ThesvgRedis },
  google: { Logo: ThesvgGoogle },
  chrome: { Logo: ThesvgChrome },
  // `default` is a white (dark-mode) mark — use the dark-ink `light` variant.
  vercel: { Logo: ThesvgVercel, variant: "light" },
  railway: { Logo: ThesvgRailway, variant: "light" },
  mysql: { Logo: ThesvgMysql, variant: "light" },
  // No colored light-bg variant — render the monochrome path tinted to brand.
  postgresql: { Logo: ThesvgPostgresql },
  notion: { Logo: ThesvgNotion, variant: "mono", color: "#000000" },
}

/** Integration provider slugs -> canonical brand key. */
const PROVIDER_BRAND_ALIASES: Record<string, string> = {
  "github-app": "github",
  "github-app-code-reviews": "github",
  postgres: "postgresql",
}

/** Icon-registry names (e.g. a plugin's `icon`) -> canonical brand key. */
const ICON_BRAND_ALIASES: Record<string, string> = {
  postgres: "postgresql",
}

/** Brand config for an integration provider, if @thesvg/react ships one. */
export function providerBrand(provider: string): BrandConfig | undefined {
  return BRANDS[PROVIDER_BRAND_ALIASES[provider] ?? provider]
}

/** Brand config for an icon-registry name, if one exists. */
export function brandForIcon(icon: string): BrandConfig | undefined {
  return BRANDS[ICON_BRAND_ALIASES[icon] ?? icon]
}

/** Renders a brand mark with its configured variant and tint. */
export function BrandMark({
  brand,
  label,
  size,
  className,
}: {
  brand: BrandConfig
  label: string
  size?: number
  className?: string
}) {
  const Logo = brand.Logo as ComponentType<
    SVGProps<SVGSVGElement> & { variant?: string }
  >
  // `mono` marks have unfilled paths that inherit the svg fill; tint them by
  // overriding fill (thesvg spreads props over its default). Only pass fill when
  // a color is configured so colored variants keep their own fills.
  const colorProps = brand.color ? { fill: brand.color } : {}
  return (
    <Logo
      variant={brand.variant}
      role="img"
      aria-label={label}
      width={size}
      height={size}
      className={cn("shrink-0 object-contain", className)}
      {...colorProps}
    />
  )
}

export function integrationLogoURL(provider: string): string {
  const localLogo = LOCAL_PROVIDER_LOGOS[provider]
  if (localLogo) return localLogo

  const aliased = LOGO_PROVIDER_ALIASES[provider] ?? provider
  return `${LOGO_BASE}/${aliased}.svg`
}

/**
 * Unframed provider mark: renders the @thesvg/react brand component when one
 * exists for the provider, otherwise falls back to the remote/local logo URL.
 */
export function ProviderLogoMark({
  provider,
  size,
  className,
}: {
  provider: string
  size: number
  className?: string
}) {
  const brand = providerBrand(provider)
  if (brand) {
    return (
      <BrandMark
        brand={brand}
        label={provider}
        size={size}
        className={className}
      />
    )
  }
  return (
    <Image
      src={integrationLogoURL(provider)}
      alt={provider}
      width={size}
      height={size}
      className={cn("shrink-0 object-contain", className)}
    />
  )
}

const sizeClasses: Record<number, string> = {
  16: "size-4",
  20: "size-5",
  24: "size-6",
  28: "size-7",
  32: "size-8",
  40: "size-10",
  48: "size-12",
}

interface IntegrationLogoProps {
  provider: string
  size?: number
  className?: string
}

export function IntegrationLogo({ provider, size = 32, className }: IntegrationLogoProps) {
  const sizeClass = sizeClasses[size] ?? "size-8"

  return (
    <div className={cn("shrink-0 rounded-md bg-white p-0.5", sizeClass, className)}>
      <ProviderLogoMark provider={provider} size={size} className="size-full" />
    </div>
  )
}
