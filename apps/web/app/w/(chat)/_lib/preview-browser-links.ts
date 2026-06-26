export interface PreviewBrowserTarget {
  key: string
  url: string
  host: string
  port?: string
}

const PREVIEW_DOMAIN = "preview.usehivy.com"
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
    }
  } catch {
    return null
  }
}

export function isPreviewBrowserURL(rawUrl: string) {
  return Boolean(previewBrowserTargetFromURL(rawUrl))
}

function isPreviewHost(hostname: string) {
  return (
    hostname.endsWith(`.${PREVIEW_DOMAIN}`) &&
    hostname.length > PREVIEW_DOMAIN.length + 1
  )
}

function previewPort(hostname: string) {
  const prefix = hostname.slice(0, -1 * `.${PREVIEW_DOMAIN}`.length)
  const [port] = prefix.split("-")
  return port && /^\d+$/.test(port) ? port : undefined
}

function cleanUrl(url: string) {
  return url.replace(/[),.;!?]+$/g, "")
}
