import type { InfiniteData, QueryClient } from "@tanstack/react-query"
import { rowsQuerySig, sheetKeys } from "@/app/w/(chat)/_lib/sheets-query-keys"
import type {
  SheetLiveEvent,
  SheetRow,
  SheetRowsQueryResponse,
} from "@/app/w/(chat)/_lib/sheets-types"

/* ------------------------------------------------------------------ */
/* SSE events → TanStack Query cache                                   */
/* ------------------------------------------------------------------ */

type RowsCache = InfiniteData<SheetRowsQueryResponse>

/**
 * Applies a rows_changed event to every cached rows window for the page.
 * Deletes always apply (ids are always present); updates apply when the
 * event carries patches; inserts with full row data are appended into
 * fully-loaded default-position-order windows and invalidate the rest
 * (custom sorts/filters/searches are ambiguous — events are hints, REST
 * is truth). All invalidations are already scoped to the event's page.
 */
export function applyRowsChangedEvent(
  queryClient: QueryClient,
  event: SheetLiveEvent
): void {
  const pageId = event.page_id
  if (!pageId) return
  const prefix = sheetKeys.rowsPrefix(pageId)

  if (event.action === "delete") {
    const ids = new Set(event.row_ids ?? [])
    if (ids.size === 0) return
    mapCachedRows(queryClient, prefix, (rows) =>
      rows.filter((row) => !row.id || !ids.has(row.id))
    )
    return
  }

  if (event.action === "update" && event.patches) {
    const patches = event.patches
    mapCachedRows(queryClient, prefix, (rows) =>
      rows.map((row) => {
        const patch = row.id ? patches[row.id] : undefined
        if (!patch) return row
        return { ...row, data: { ...(row.data ?? {}), ...patch } }
      })
    )
    return
  }

  if (event.action === "insert" && event.patches && event.row_ids?.length) {
    const patches = event.patches
    const ids = event.row_ids
    if (ids.every((id) => patches[id] !== undefined)) {
      spliceInsertedRows(
        queryClient,
        prefix,
        ids.map((id) => ({ id, data: patches[id] }))
      )
      return
    }
  }

  void queryClient.invalidateQueries({ queryKey: prefix })
}

const DEFAULT_ORDER_SIG = rowsQuerySig({})

/**
 * Appends freshly inserted rows into cached windows that use the default
 * position order AND have their tail loaded (no next cursor) — new rows
 * always land at the end there. Every other cached window for the page
 * (custom sort/filter/search, or a partially-loaded window) is invalidated
 * individually.
 */
function spliceInsertedRows(
  queryClient: QueryClient,
  prefix: readonly unknown[],
  newRows: SheetRow[]
): void {
  const entries = queryClient.getQueriesData<RowsCache>({ queryKey: prefix })
  for (const [key, data] of entries) {
    const sig = Array.isArray(key) ? key[2] : undefined
    const lastPage = data?.pages?.[data.pages.length - 1]
    if (sig === DEFAULT_ORDER_SIG && lastPage && !lastPage.next_cursor) {
      const existing = new Set(
        data.pages.flatMap((page) => (page.rows ?? []).map((row) => row.id))
      )
      const toAppend = newRows.filter((row) => row.id && !existing.has(row.id))
      if (toAppend.length === 0) continue
      queryClient.setQueryData<RowsCache>(key, {
        ...data,
        pages: data.pages.map((page, index) =>
          index === data.pages.length - 1
            ? { ...page, rows: [...(page.rows ?? []), ...toAppend] }
            : page
        ),
      })
      continue
    }
    void queryClient.invalidateQueries({ queryKey: key, exact: true })
  }
}

function mapCachedRows(
  queryClient: QueryClient,
  queryKey: readonly unknown[],
  mapRows: (rows: SheetRow[]) => SheetRow[]
): void {
  const entries = queryClient.getQueriesData<RowsCache>({ queryKey })
  for (const [key, data] of entries) {
    if (!data?.pages) continue
    queryClient.setQueryData<RowsCache>(key, {
      ...data,
      pages: data.pages.map((page) => ({
        ...page,
        rows: mapRows(page.rows ?? []),
      })),
    })
  }
}
