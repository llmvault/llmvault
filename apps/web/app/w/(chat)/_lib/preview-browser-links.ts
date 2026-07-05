import { clientConfig } from "@/lib/config/public-config"

export interface PreviewBrowserTarget {
  key: string
  url: string
  host: string
  port?: string
  /**
   * App id from the `?app=<id>` hint. Present when the URL points at a Hivy
   * app (preview or deployed): opening it must go through the launch-token
   * handshake instead of loading the raw URL, which would 401 ("not signed in").
   */
  appId?: string
}

const URL_PATTERN = /https?:\/\/[^\s<>"'`]+/g

export function previewBrowserTargets(text: string): PreviewBrowserTarget[] {
  const seen = new Set<string>()
  const targets: PreviewBrowserTarget[] = []

  for (const rawUrl of text.match(URL_PATTERN) ?? []) {
    const target = previewBrowserTargetFromURL(cleanUrl(rawUrl))
    if (!target || seen.has(target.key)) continue
    seen.add(target.key)
    targets.push(target)
  }

  return targets
}

export function previewBrowserTargetFromURL(
  rawUrl: string
): PreviewBrowserTarget | null {
  try {
    const url = new URL(rawUrl)
    if (url.protocol !== "https:" || !isPreviewHost(url.hostname)) return null
    const normalizedUrl = url.toString()
    return {
      key: normalizedUrl,
      url: normalizedUrl,
      host: url.hostname,
      port: previewPort(url.hostname),
      appId: url.searchParams.get("app") ?? undefined,
    }
  } catch {
    return null
  }
}

export function isPreviewBrowserURL(rawUrl: string) {
  return Boolean(previewBrowserTargetFromURL(rawUrl))
}

function isPreviewHost(hostname: string) {
  const previewDomain = clientConfig().previewDomain
  return (
    hostname.endsWith(`.${previewDomain}`) &&
    hostname.length > previewDomain.length + 1
  )
}

function previewPort(hostname: string) {
  const previewDomain = clientConfig().previewDomain
  const prefix = hostname.slice(0, -1 * `.${previewDomain}`.length)
  const [port] = prefix.split("-")
  return port && /^\d+$/.test(port) ? port : undefined
}

function cleanUrl(url: string) {
  return url.replace(/[),.;!?]+$/g, "")
}
