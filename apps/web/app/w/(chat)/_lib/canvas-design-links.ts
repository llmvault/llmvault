export interface CanvasDesignTarget {
  key: string
  fileId: string
  pageId?: string
  sourceUrl: string
}

export interface CanvasSessionURLEntry {
  url: string
  expiresAt: number
  cachedAt: number
}

const CANVAS_HOST = "canvas.usehivy.com"
const URL_PATTERN = /https?:\/\/[^\s<>"'`]+/g
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const SESSION_URL_REFRESH_SKEW_MS = 60_000

export function canvasDesignTargets(
  text: string
): CanvasDesignTarget[] {
  const seen = new Set<string>()
  const targets: CanvasDesignTarget[] = []

  for (const rawUrl of text.match(URL_PATTERN) ?? []) {
    const target = canvasDesignTargetFromURL(cleanUrl(rawUrl))
    if (!target || seen.has(target.key)) continue
    seen.add(target.key)
    targets.push(target)
  }

  return targets
}

export function canvasDesignTargetKey(fileId: string, pageId?: string) {
  return pageId ? `${fileId}:${pageId}` : fileId
}

export function isFreshCanvasSessionURL(
  entry: CanvasSessionURLEntry | undefined,
  now = Date.now()
) {
  return Boolean(
    entry?.url && entry.expiresAt - SESSION_URL_REFRESH_SKEW_MS > now
  )
}

function canvasDesignTargetFromURL(rawUrl: string): CanvasDesignTarget | null {
  try {
    const url = new URL(rawUrl)
    if (url.protocol !== "https:" || url.hostname !== CANVAS_HOST) return null
    const hash = url.hash.startsWith("#") ? url.hash.slice(1) : url.hash
    if (!hash.startsWith("/workspace")) return null
    const queryIndex = hash.indexOf("?")
    if (queryIndex < 0) return null
    const params = new URLSearchParams(hash.slice(queryIndex + 1))
    const fileId = normalizedUUID(params.get("file-id"))
    if (!fileId) return null
    const pageId = normalizedUUID(params.get("page-id")) ?? undefined
    return {
      key: canvasDesignTargetKey(fileId, pageId),
      fileId,
      pageId,
      sourceUrl: url.toString(),
    }
  } catch {
    return null
  }
}

function normalizedUUID(value: string | null) {
  const trimmed = value?.trim()
  return trimmed && UUID_PATTERN.test(trimmed) ? trimmed.toLowerCase() : null
}

function cleanUrl(url: string) {
  return url.replace(/[),.;!?]+$/g, "")
}
