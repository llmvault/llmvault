import type { ComponentType, SVGProps } from "react"
import Image from "next/image"
import ThesvgChrome from "@thesvg/react/chrome"
import ThesvgGithub from "@thesvg/react/github"
import ThesvgGoogle from "@thesvg/react/google"
import ThesvgLinear from "@thesvg/react/linear"
import ThesvgMongodb from "@thesvg/react/mongodb"
import ThesvgMysql from "@thesvg/react/mysql"
import ThesvgNotion from "@thesvg/react/notion"
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

type BrandLogo = ComponentType<SVGProps<SVGSVGElement>>

/**
 * Providers whose brand mark ships in @thesvg/react. These render the vector
 * component directly (full-color "default" variant); anything not listed here
 * falls back to `integrationLogoURL`.
 */
const PROVIDER_BRAND_LOGOS: Record<string, BrandLogo> = {
  "github-app": ThesvgGithub,
  "github-app-code-reviews": ThesvgGithub,
  slack: ThesvgSlack,
  notion: ThesvgNotion,
  linear: ThesvgLinear,
  vercel: ThesvgVercel,
  railway: ThesvgRailway,
  postgres: ThesvgPostgresql,
  mysql: ThesvgMysql,
  mongodb: ThesvgMongodb,
  redis: ThesvgRedis,
  google: ThesvgGoogle,
  chrome: ThesvgChrome,
}

export function integrationLogoURL(provider: string): string {
  const localLogo = LOCAL_PROVIDER_LOGOS[provider]
  if (localLogo) return localLogo

  const aliased = LOGO_PROVIDER_ALIASES[provider] ?? provider
  return `${LOGO_BASE}/${aliased}.svg`
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
  const BrandLogo = PROVIDER_BRAND_LOGOS[provider]

  return (
    <div className={cn("shrink-0 rounded-md bg-white p-0.5", sizeClass, className)}>
      {BrandLogo ? (
        <BrandLogo
          role="img"
          aria-label={provider}
          className="size-full object-contain"
        />
      ) : (
        <Image
          src={integrationLogoURL(provider)}
          alt={provider}
          width={size}
          height={size}
          className="size-full object-contain"
        />
      )}
    </div>
  )
}
