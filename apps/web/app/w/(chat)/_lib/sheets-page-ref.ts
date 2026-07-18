import type { QueryClient } from "@tanstack/react-query"
import {
  fetchSheets,
  fetchSheetStructure,
} from "@/app/w/(chat)/_lib/sheets-api"
import { sheetKeys } from "@/app/w/(chat)/_lib/sheets-query-keys"
import type {
  SheetPageRef,
  SheetStructure,
} from "@/app/w/(chat)/_lib/sheets-types"

/* ------------------------------------------------------------------ */
/* Cross-sheet page resolver                                           */
/*                                                                     */
/* Relation fields target a page by ID, which may live in a different  */
/* sheet than the one currently open. Rows can only be queried through */
/* the page's OWNING sheet path, so this resolver maps any org page ID */
/* to its owning sheet — lazily, reusing the TanStack caches for the   */
/* sheets list and per-sheet structures.                               */
/* ------------------------------------------------------------------ */

function pageRefFromStructure(
  structure: SheetStructure | null | undefined,
  pageId: string
): SheetPageRef | null {
  for (const page of structure?.pages ?? []) {
    if (page.id !== pageId) continue
    const sheetId = structure?.sheet?.id ?? page.sheet_id ?? ""
    if (!sheetId) return null
    return { sheetId, sheetName: structure?.sheet?.name ?? "", page }
  }
  return null
}

/**
 * Resolves a page ID to its owning sheet + page structure. Checks every
 * cached sheet structure first, then falls back to fetching the sheets
 * list for the given team and the structures that are not cached yet.
 * Results flow through `ensureQueryData`, so every fetch also warms the
 * regular caches. Sheets are team-scoped, so this can only resolve pages
 * belonging to sheets in `teamId`.
 */
export async function resolvePageRef(
  queryClient: QueryClient,
  teamId: string,
  pageId: string
): Promise<SheetPageRef | null> {
  if (!pageId) return null

  const cachedStructures = queryClient.getQueriesData<SheetStructure>({
    queryKey: sheetKeys.structurePrefix,
  })
  const cachedSheetIds = new Set<string>()
  for (const [, structure] of cachedStructures) {
    const ref = pageRefFromStructure(structure, pageId)
    if (ref) return ref
    if (structure?.sheet?.id) cachedSheetIds.add(structure.sheet.id)
  }

  if (!teamId) return null

  const list = await queryClient.ensureQueryData({
    queryKey: sheetKeys.list(teamId),
    queryFn: ({ signal }) => fetchSheets(teamId, signal),
  })
  const candidates = (list.sheets ?? [])
    .map((sheet) => sheet.id)
    .filter((id): id is string => Boolean(id) && !cachedSheetIds.has(id ?? ""))

  const structures = await Promise.all(
    candidates.map((sheetId) =>
      queryClient
        .ensureQueryData({
          queryKey: sheetKeys.structure(sheetId),
          queryFn: ({ signal }) => fetchSheetStructure(sheetId, signal),
        })
        .catch(() => null)
    )
  )
  for (const structure of structures) {
    const ref = pageRefFromStructure(structure, pageId)
    if (ref) return ref
  }
  return null
}
