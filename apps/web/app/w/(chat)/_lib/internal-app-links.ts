export interface InternalAppLinkTarget {
  key: string
  /** Path (+ search + hash) for client-side next/link navigation. */
  href: string
  label: string
  /** Optional secondary line under the label. */
  subtitle?: string
  icon: string
}

const URL_PATTERN = /https?:\/\/[^\s<>"'`*]+/g

// Curated templates for the app links agents actually share. Only links that
// match a template render a card — we intentionally do NOT card every /w/ URL.
// Add a template here to support a new agent-facing destination.
interface LinkTemplate {
  test: (pathname: string) => boolean
  label: string
  icon: string
  subtitle?: string
}

const LINK_TEMPLATES: LinkTemplate[] = [
  {
    // /w/agents/edit/<id>
    test: (pathname) => /^\/w\/agents\/edit\/[^/]+\/?$/.test(pathname),
    label: "Edit agent details",
    icon: "bot",
  },
]

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

    const template = LINK_TEMPLATES.find((t) => t.test(url.pathname))
    if (!template) return null

    const href = url.pathname + url.search + url.hash
    const target: InternalAppLinkTarget = {
      key: href,
      href,
      label: template.label,
      icon: template.icon,
    }
    if (template.subtitle) target.subtitle = template.subtitle
    return target
  } catch {
    return null
  }
}

export function isInternalAppURL(rawUrl: string) {
  return Boolean(internalAppLinkFromURL(rawUrl))
}

// Only same-origin links (whatever host the app is actually served from,
// including localhost in dev) are treated as internal app links — matched at
// runtime via window.location.origin.
function isAppOrigin(url: URL) {
  if (url.protocol !== "https:" && url.protocol !== "http:") return false
  if (typeof window !== "undefined" && url.origin === window.location.origin) {
    return true
  }
  return false
}

function cleanUrl(url: string) {
  return url.replace(/[),.;!?*]+$/g, "")
}
