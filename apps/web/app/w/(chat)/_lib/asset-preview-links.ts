import type { MediaAttachment } from "@/app/w/(chat)/_lib/static-data"
import { clientConfig } from "@/lib/config/public-config"

const URL_PATTERN = /https?:\/\/[^\s<>"'`]+/g
const PREVIEW_PATH = "/v1/assets/preview"

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
    const filename = assetPreviewFilename(url)
    if (!filename) continue
    attachments.push({
      id: `asset-preview:${hashString(keyPrefix)}:${attachments.length}:${hashString(url)}`,
      filename,
      kind: "image",
      url,
    })
  }

  return attachments
}

export function isAssetPreviewUrl(rawUrl: string) {
  try {
    const url = new URL(cleanUrl(rawUrl))
    const previewHost = new URL(clientConfig().apiUrl).hostname
    return (
      url.hostname === previewHost && url.pathname.startsWith(PREVIEW_PATH)
    )
  } catch {
    return false
  }
}

function assetPreviewFilename(rawUrl: string): string | null {
  const url = new URL(rawUrl)
  const rawPath = rawSearchParam(url.search, "path")
  const path =
    rawPath === null
      ? safeDecodeURIComponent(url.pathname)
      : hasMalformedPercentEscape(rawPath)
        ? null
        : url.searchParams.get("path")
  if (path === null) return null
  const filename = path.split("/").filter(Boolean).at(-1)
  return filename || "asset-preview.png"
}

function rawSearchParam(search: string, key: string): string | null {
  const query = search.startsWith("?") ? search.slice(1) : search
  for (const part of query.split("&")) {
    const separator = part.indexOf("=")
    const rawKey = separator >= 0 ? part.slice(0, separator) : part
    if (rawKey === key) {
      return separator >= 0 ? part.slice(separator + 1) : ""
    }
  }
  return null
}

function hasMalformedPercentEscape(value: string) {
  for (let index = 0; index < value.length; index += 1) {
    if (
      value[index] === "%" &&
      !/^[0-9a-fA-F]{2}$/.test(value.slice(index + 1, index + 3))
    ) {
      return true
    }
  }
  return false
}

function safeDecodeURIComponent(value: string) {
  try {
    return decodeURIComponent(value)
  } catch {
    return null
  }
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
