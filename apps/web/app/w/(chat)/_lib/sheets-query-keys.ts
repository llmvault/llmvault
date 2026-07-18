import type {
  SheetFilterNode,
  SheetSort,
} from "@/app/w/(chat)/_lib/sheets-types"

/* ------------------------------------------------------------------ */
/* Query keys                                                          */
/* ------------------------------------------------------------------ */

export const sheetKeys = {
  list: (teamId: string) => ["sheets", teamId] as const,
  structure: (sheetId: string) => ["sheet-structure", sheetId] as const,
  structurePrefix: ["sheet-structure"] as const,
  pageRef: (pageId: string) => ["sheet-page-ref", pageId] as const,
  views: (pageId: string) => ["sheet-views", pageId] as const,
  rows: (pageId: string, querySig: string) =>
    ["sheet-rows", pageId, querySig] as const,
  rowsPrefix: (pageId: string) => ["sheet-rows", pageId] as const,
  attachmentUrls: (pageId: string) =>
    ["sheet-attachment-urls", pageId] as const,
  operations: (pageId: string) => ["sheet-operations", pageId] as const,
  importJob: (jobId: string) => ["sheet-import", jobId] as const,
}

export function rowsQuerySig(input: {
  filter?: SheetFilterNode
  sorts?: SheetSort[]
  search?: string
}): string {
  return JSON.stringify({
    f: input.filter ?? null,
    s: input.sorts ?? [],
    q: input.search ?? "",
  })
}
