"use client"

import { useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  fetchAttachmentDownloadURLs,
  sheetKeys,
  stringArrayValue,
  type SheetField,
  type SheetRow,
} from "@/app/w/(chat)/_lib/sheets"
import { isImageAttachmentKey } from "./cells"

/** Presigned URLs expire after ~5 minutes; refresh a little before that. */
const URL_TTL_MS = 4 * 60_000

/**
 * Collects every image attachment key visible in the loaded rows and
 * resolves them to presigned download URLs in one batch, cached and
 * refreshed before expiry. Returns key → URL for resolved entries only.
 */
export function useAttachmentImageUrls(
  sheetId: string,
  pageId: string,
  fields: SheetField[],
  rows: SheetRow[]
): Record<string, string> {
  const imageKeys = useMemo(() => {
    const attachmentFieldIds = fields
      .filter((field) => field.type === "attachment" && field.id)
      .map((field) => field.id ?? "")
    if (attachmentFieldIds.length === 0) return []
    const keys = new Set<string>()
    for (const row of rows) {
      for (const fieldId of attachmentFieldIds) {
        for (const key of stringArrayValue(row.data?.[fieldId])) {
          if (isImageAttachmentKey(key)) keys.add(key)
        }
      }
    }
    return Array.from(keys).sort()
  }, [fields, rows])

  const query = useQuery({
    enabled: imageKeys.length > 0,
    queryKey: [...sheetKeys.attachmentUrls(pageId), imageKeys.join("\n")],
    queryFn: () => fetchAttachmentDownloadURLs(sheetId, pageId, imageKeys),
    staleTime: URL_TTL_MS,
    refetchInterval: URL_TTL_MS,
    // Keep serving the previous (still valid) URLs while a changed key set
    // fetches, so thumbnails do not flicker back to chips on scroll.
    placeholderData: (previous) => previous,
  })

  return query.data?.urls ?? {}
}
