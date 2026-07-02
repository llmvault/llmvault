import { HelpSquareIcon } from "@hugeicons/core-free-icons"
import { HugeiconsIcon } from "@hugeicons/react"
import type { SVGProps } from "react"
import { iconRegistry, type RegisteredIcon } from "@/lib/icons/registry"

/**
 * Legacy @iconify names whose base (the part after the ":") differs from the
 * registry key. Lucide names ("lucide:archive" -> "archive") resolve by simply
 * stripping the prefix, so only the odd ones out need an explicit alias.
 */
const LEGACY_BRAND_ALIASES: Record<string, string> = {
  "simple-icons:postgresql": "postgresql",
  "mdi:github": "github",
  "ri:twitter-x-fill": "twitter-x",
  "logos:chrome": "chrome",
  "devicon:google": "google",
}

function resolveIconName(name: string): string {
  if (!name.includes(":")) return name
  // Tolerate stale @iconify names (persisted state, un-migrated JSON):
  // map known brand aliases, otherwise strip everything up to the colon.
  return LEGACY_BRAND_ALIASES[name] ?? name.slice(name.indexOf(":") + 1)
}

/**
 * Look up a registered icon by name (legacy "prefix:name" forms tolerated).
 * Returns the hugeicons `IconSvgElement` or custom brand component, or
 * `undefined` when the name is unknown.
 */
export function getAppIcon(name: string): RegisteredIcon | undefined {
  return iconRegistry[resolveIconName(name)]
}

const warnedNames = new Set<string>()

function warnUnknownIcon(name: string) {
  if (process.env.NODE_ENV === "production" || warnedNames.has(name)) return
  warnedNames.add(name)
  console.warn(`[AppIcon] Unknown icon name "${name}" — rendering fallback. Add it to lib/icons/registry.ts.`)
}

export interface AppIconProps extends Omit<SVGProps<SVGSVGElement>, "ref" | "strokeWidth"> {
  /** Registry key, e.g. "archive". Legacy "lucide:archive" forms still resolve. */
  icon: string
  className?: string
  /**
   * Explicit pixel/CSS size. Defaults to "1em" for parity with the old
   * @iconify <Icon>, which sized via font-size / `size-*` classes.
   * (CSS classes still win over the width/height attributes this sets.)
   */
  size?: number | string
  strokeWidth?: number
}

export function AppIcon({ icon, className, size = "1em", strokeWidth = 1.5, ...rest }: AppIconProps) {
  const resolved = getAppIcon(icon)

  if (!resolved) {
    warnUnknownIcon(icon)
    return (
      <HugeiconsIcon icon={HelpSquareIcon} className={className} size={size} strokeWidth={strokeWidth} {...rest} />
    )
  }

  if (typeof resolved === "function") {
    // Custom brand component (plain SVG, fill="currentColor").
    const BrandIcon = resolved
    return <BrandIcon className={className} width={size} height={size} {...rest} />
  }

  // Hugeicons IconSvgElement (readonly tuple array).
  return <HugeiconsIcon icon={resolved} className={className} size={size} strokeWidth={strokeWidth} {...rest} />
}
