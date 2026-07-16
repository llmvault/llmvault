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
import ThesvgRailway from "@thesvg/react/railway"
import ThesvgRedis from "@thesvg/react/redis"
import ThesvgSlack from "@thesvg/react/slack"
import ThesvgVercel from "@thesvg/react/vercel"
import { clientConfig } from "@/lib/config/public-config"
import { cn } from "@/lib/utils"

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
  // (postgres uses a custom local logomark instead — see LOCAL_PROVIDER_LOGOS.)
  notion: { Logo: ThesvgNotion, variant: "mono", color: "#000000" },
}

/** Integration provider slugs -> canonical brand key. */
const PROVIDER_BRAND_ALIASES: Record<string, string> = {
  "github-app": "github",
  "github-app-code-reviews": "github",
}

/** Brand config for an integration provider, if @thesvg/react ships one. */
function providerBrand(provider: string): BrandConfig | undefined {
  return BRANDS[PROVIDER_BRAND_ALIASES[provider] ?? provider]
}

/** Renders a brand mark with its configured variant and tint. */
function BrandMark({
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

function integrationLogoURL(provider: string): string {
  const localLogo = LOCAL_PROVIDER_LOGOS[provider]
  if (localLogo) return localLogo

  const aliased = LOGO_PROVIDER_ALIASES[provider] ?? provider
  const logoBase = `${clientConfig().connectionsHost.replace(/\/$/, "")}/images/template-logos`
  return `${logoBase}/${aliased}.svg`
}

/**
 * Unframed provider mark: renders the @thesvg/react brand component when one
 * exists for the provider, otherwise falls back to the remote/local logo URL.
 */
function ProviderLogoMark({
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
  16: "size-4 p-0.5",
  20: "size-5 p-0.5",
  24: "size-6 p-1",
  28: "size-7 p-1",
  32: "size-8 p-1",
  36: "size-9 p-1.5",
  40: "size-10 p-1.5",
  48: "size-12 p-2",
}

interface IntegrationLogoProps {
  provider: string
  size?: number
  className?: string
}

export function IntegrationLogo({
  provider,
  size = 32,
  className,
}: IntegrationLogoProps) {
  const sizeClass = sizeClasses[size] ?? "size-8"

  return (
    <div className={cn("shrink-0 rounded-md bg-white", sizeClass, className)}>
      <ProviderLogoMark provider={provider} size={size} className="size-full" />
    </div>
  )
}
