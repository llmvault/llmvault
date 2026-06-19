import type { MediaAttachment } from "@/app/w/(chat)/_lib/static-data"

const URL_PATTERN = /https?:\/\/[^\s<>"'`]+/g
const PREVIEW_PATH = "/v1/assets/preview"
const PREVIEW_HOST = "api.usehivy.com"

export function assetPreviewAttachments(
  text: string,
  keyPrefix = "asset-preview"
): MediaAttachment[] {
  const seen = new Set<string>()
  const attachments: MediaAttachment[] = []

  for (const rawUrl of text.match(URL_PATTERN) ?? []) {
    const url = cleanUrl(rawUrl)
    if (seen.has(url) || !isAssetPreviewUrl(url)) continue
    seen.add(url)
    attachments.push({
      id: `asset-preview:${hashString(keyPrefix)}:${attachments.length}:${hashString(url)}`,
      filename: assetPreviewFilename(url),
      kind: "image",
      url,
    })
  }

  return attachments
}

export function isAssetPreviewUrl(rawUrl: string) {
  try {
    const url = new URL(cleanUrl(rawUrl))
    return (
      url.hostname === PREVIEW_HOST && url.pathname.startsWith(PREVIEW_PATH)
    )
  } catch {
    return false
  }
}

function assetPreviewFilename(rawUrl: string) {
  const url = new URL(rawUrl)
  const path = url.searchParams.get("path") || url.pathname
  const filename = path.split("/").filter(Boolean).at(-1)
  return filename ? decodeURIComponent(filename) : "asset-preview.png"
}

function cleanUrl(url: string) {
  return url.replace(/[),.;!?]+$/g, "")
}

function hashString(value: string) {
  let hash = 0
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0
  }
  return hash.toString(36)
}
