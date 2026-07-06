import { AppIcon } from "@/components/icon"

// Brand marks (github, notion, slack, ...) come from @thesvg via the icon
// registry's mono variant, which bakes in the brand's own dark color instead of
// tinting via currentColor — so they disappear on a dark background. Forcing
// fill="currentColor" makes them follow the text color like every other icon.
// The globe (website) is a stroke-based UI icon, not a brand, so it renders
// normally.
const NON_BRAND = new Set(["globe"])

export function ProviderIcon({
  icon,
  className,
}: {
  icon: string
  className?: string
}) {
  if (NON_BRAND.has(icon)) {
    return <AppIcon icon={icon} className={className} />
  }
  return <AppIcon icon={icon} className={className} fill="currentColor" />
}
