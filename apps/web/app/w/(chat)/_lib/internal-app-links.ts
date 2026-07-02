export interface InternalAppLinkTarget {
  key: string
  /** Path (+ search + hash) for client-side next/link navigation. */
  href: string
  label: string
  /** The origin-stripped path, shown as the card subtitle. */
  subtitle: string
  icon: string
}

// Known canonical hosts for the Hivy app. Same-origin links (whatever host the
// app is actually served from, including localhost in dev) are also matched at
// runtime via window.location.origin.
const APP_HOSTS = new Set([
  "usehivy.com",
  "www.usehivy.com",
  "app.usehivy.com",
])

const URL_PATTERN = /https?:\/\/[^\s<>"'`]+/g

export function internalAppLinkTargets(text: string): InternalAppLinkTarget[] {
  const seen = new Set<string>()
  const targets: InternalAppLinkTarget[] = []

  for (const rawUrl of text.match(URL_PATTERN) ?? []) {
    const target = internalAppLinkFromURL(cleanUrl(rawUrl))
    if (!target || seen.has(target.key)) continue
    seen.add(target.key)
    targets.push(target)
  }

  return targets
}

export function internalAppLinkFromURL(
  rawUrl: string
): InternalAppLinkTarget | null {
  try {
    const url = new URL(rawUrl)
    if (!isAppOrigin(url)) return null
    // Only link into the authenticated workspace, so marketing/home URLs don't
    // turn into navigation cards.
    if (!url.pathname.startsWith("/w/")) return null

    const href = url.pathname + url.search + url.hash
    const { label, icon } = describeInternalPath(url.pathname)
    return { key: href, href, label, subtitle: href, icon }
  } catch {
    return null
  }
}

export function isInternalAppURL(rawUrl: string) {
  return Boolean(internalAppLinkFromURL(rawUrl))
}

function isAppOrigin(url: URL) {
  if (url.protocol !== "https:" && url.protocol !== "http:") return false
  if (APP_HOSTS.has(url.hostname)) return true
  if (typeof window !== "undefined" && url.origin === window.location.origin) {
    return true
  }
  return false
}

function describeInternalPath(pathname: string): {
  label: string
  icon: string
} {
  if (pathname.startsWith("/w/settings/agents")) {
    return { label: "Agent", icon: "lucide:bot" }
  }
  if (pathname.startsWith("/w/plugins")) {
    return { label: "Plugin", icon: "lucide:blocks" }
  }
  if (pathname.startsWith("/w/automations")) {
    return { label: "Automation", icon: "lucide:workflow" }
  }
  if (pathname.startsWith("/w/channels")) {
    return { label: "Channel", icon: "lucide:hash" }
  }
  if (
    pathname.startsWith("/w/billing") ||
    pathname.startsWith("/w/settings/billing")
  ) {
    return { label: "Billing", icon: "lucide:credit-card" }
  }
  if (pathname.startsWith("/w/settings")) {
    return { label: "Settings", icon: "lucide:settings" }
  }
  return { label: "Open in Hivy", icon: "lucide:arrow-up-right" }
}

function cleanUrl(url: string) {
  return url.replace(/[),.;!?]+$/g, "")
}
