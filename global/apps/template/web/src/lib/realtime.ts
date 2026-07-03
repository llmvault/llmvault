// Realtime engine — TEMPLATE INFRASTRUCTURE, not agent surface.
//
// Every Hivy app's data is live by default. This module opens a single
// EventSource to /api/_live (the hivycore relay) and turns the sheet-change
// events it streams into precise TanStack Query cache updates, so screens
// re-render the instant data changes with ZERO code in your components. You
// never poll, never build a refresh button, and never import this file from a
// component — read rows through the useRows hook (src/hooks/queries.ts) and
// the updates arrive on their own.
//
// Design: apply the safe, cheap patch immediately (update matching cells,
// remove deleted rows, append provably-safe inserts) and, wherever a change
// could alter which rows belong in a filtered/sorted view, schedule ONE
// debounced invalidation per page as the reconciler. Anything uncertain falls
// back to a refetch — correctness first, patches only when provably right.

import type { QueryClient } from "@tanstack/react-query"

// ---- Query-key convention (the contract the engine keys off) --------------

/** The rows-query input mirrored from the Go hivycore.Query type. */
export type RowsQuery = {
  filter?: unknown
  search?: string
  sorts?: { field: string; desc?: boolean }[]
  cursor?: string
  limit?: number
  resolve_relations?: boolean
}

/** One stored row as the rows API returns it. */
export type Row = {
  id: string
  data: Record<string, unknown>
  position?: number
  created_at?: string
  updated_at?: string
}

/** A page of rows from POST /api/pages/{id}/rows/query. */
export type RowsResult = {
  rows: Row[]
  fields?: unknown[]
  next_cursor?: string
  relations?: Record<string, unknown>
}

const ROWS = "rows"
const PAGES = "pages"

// The hash of the default (unfiltered, default-sorted, unpaged) query. Only
// caches under this hash are safe to append inserts into — every other query
// might filter, sort, or paginate the new row elsewhere, so those refetch.
export const DEFAULT_ROWS_HASH = ""

/**
 * Canonical rows query key: ["rows", pageID, hash(query)]. hash("" | {} |
 * undefined) is DEFAULT_ROWS_HASH; any filter/sort/search/cursor/limit yields
 * a distinct, stable, order-independent hash.
 */
export function rowsKey(pageID: string, query?: RowsQuery) {
  return [ROWS, pageID, stableHash(query)] as const
}

/** Order-independent stable stringify; empty/absent input hashes to "". */
export function stableHash(value: unknown): string {
  return stableStringify(value)
}

function stableStringify(v: unknown): string {
  if (v === null || v === undefined) return ""
  if (Array.isArray(v)) return "[" + v.map(stableStringify).join(",") + "]"
  if (typeof v === "object") {
    const obj = v as Record<string, unknown>
    const keys = Object.keys(obj)
      .filter((k) => obj[k] !== undefined)
      .sort()
    if (keys.length === 0) return ""
    return (
      "{" +
      keys.map((k) => JSON.stringify(k) + ":" + stableStringify(obj[k])).join(",") +
      "}"
    )
  }
  return JSON.stringify(v)
}

// ---- Event shapes (mirror internal/sheets Event) --------------------------

type RowsChanged = {
  page_id: string
  action: "insert" | "update" | "delete"
  row_ids?: string[]
  patches?: Record<string, Record<string, unknown>>
  rows?: Row[]
}

type FieldsChanged = { page_id?: string; field_id?: string }
type PagesChanged = { page_id?: string }
type ProgressOrRevert = { page_id?: string }

// ---- Engine ---------------------------------------------------------------

/** A minimal EventSource surface so tests can inject a stub. */
export interface EventSourceLike {
  addEventListener(type: string, listener: (event: MessageEvent) => void): void
  close(): void
}

export type RealtimeOptions = {
  /** Endpoint; defaults to the hivycore relay. */
  url?: string
  /** Injectable EventSource factory (tests pass a stub). */
  eventSourceFactory?: (url: string) => EventSourceLike
  /** Debounced-invalidation window per page (ms). */
  debounceMs?: number
  /** Pause the stream after the tab is hidden this long (ms); 0 disables. */
  hiddenPauseMs?: number
}

export type RealtimeHandle = { stop: () => void }

const DEFAULT_URL = "/api/_live"
const DEFAULT_DEBOUNCE_MS = 2000
const DEFAULT_HIDDEN_PAUSE_MS = 60_000

/**
 * Start the realtime engine. Call once at app bootstrap (main.tsx already
 * does). Returns a handle whose stop() closes the stream and cancels pending
 * work — you rarely need it outside tests.
 */
export function startRealtime(
  queryClient: QueryClient,
  options: RealtimeOptions = {}
): RealtimeHandle {
  const url = options.url ?? DEFAULT_URL
  const debounceMs = options.debounceMs ?? DEFAULT_DEBOUNCE_MS
  const hiddenPauseMs = options.hiddenPauseMs ?? DEFAULT_HIDDEN_PAUSE_MS
  const factory =
    options.eventSourceFactory ??
    ((u: string) => new EventSource(u) as unknown as EventSourceLike)

  const debouncers = new Map<string, ReturnType<typeof setTimeout>>()

  function scheduleRowsInvalidate(pageID: string) {
    const existing = debouncers.get(pageID)
    if (existing) clearTimeout(existing)
    debouncers.set(
      pageID,
      setTimeout(() => {
        debouncers.delete(pageID)
        void queryClient.invalidateQueries({ queryKey: [ROWS, pageID] })
      }, debounceMs)
    )
  }

  function onRowsChanged(ev: RowsChanged) {
    if (!ev.page_id) return
    if (ev.action === "delete") return applyDelete(queryClient, ev)
    if (ev.action === "insert") return applyInsert(queryClient, ev, scheduleRowsInvalidate)
    return applyUpdate(queryClient, ev, scheduleRowsInvalidate)
  }

  function onFieldsChanged(ev: FieldsChanged) {
    void queryClient.invalidateQueries({ queryKey: [PAGES] })
    // A typed field change (field_id present) can reshape row cells.
    if (ev.page_id && ev.field_id) {
      void queryClient.invalidateQueries({ queryKey: [ROWS, ev.page_id] })
    }
  }

  function onPagesChanged() {
    void queryClient.invalidateQueries({ queryKey: [PAGES] })
  }

  function onProgressOrRevert(ev: ProgressOrRevert) {
    // Imports and reverts change rows in bulk; reconcile the page's rows.
    if (ev.page_id) scheduleRowsInvalidate(ev.page_id)
  }

  function onRefresh() {
    // The universal fallback (subscribe, reconnect, forced resync): drop every
    // sheet-derived cache so screens refetch the truth.
    void queryClient.invalidateQueries({ queryKey: [PAGES] })
    void queryClient.invalidateQueries({ queryKey: [ROWS] })
  }

  let source: EventSourceLike | null = null
  function connect() {
    const es = factory(url)
    const on = <T,>(type: string, handler: (data: T) => void) =>
      es.addEventListener(type, (e: MessageEvent) => handler(parseData<T>(e)))
    on<RowsChanged>("rows_changed", onRowsChanged)
    on<FieldsChanged>("fields_changed", onFieldsChanged)
    on<PagesChanged>("pages_changed", onPagesChanged)
    on<ProgressOrRevert>("import_progress", onProgressOrRevert)
    on<ProgressOrRevert>("operation_reverted", onProgressOrRevert)
    on<unknown>("refresh", onRefresh)
    source = es
  }
  connect()

  // Nice-to-have: after the tab is hidden a while, drop the stream to save a
  // connection; on return, reconnect and force a full resync. Guarded so
  // non-DOM environments (and tests) simply skip it.
  const detachVisibility = wireVisibility(hiddenPauseMs, {
    pause: () => {
      source?.close()
      source = null
    },
    resume: () => {
      if (!source) connect()
      onRefresh()
    },
  })

  return {
    stop() {
      detachVisibility()
      for (const t of debouncers.values()) clearTimeout(t)
      debouncers.clear()
      source?.close()
      source = null
    },
  }
}

// ---- Patch operations (exported for tests) --------------------------------

/** update WITH snapshots → merge matching rows verbatim, then reconcile. */
export function applyUpdate(
  queryClient: QueryClient,
  ev: RowsChanged,
  schedule: (pageID: string) => void
) {
  if (!ev.rows || ev.rows.length === 0) {
    schedule(ev.page_id) // big batch, no snapshots: refetch
    return
  }
  const byId = new Map(ev.rows.map((r) => [r.id, r]))
  queryClient.setQueriesData<RowsResult>({ queryKey: [ROWS, ev.page_id] }, (old) => {
    if (!old || !Array.isArray(old.rows)) return old
    let changed = false
    const rows = old.rows.map((r) => {
      const snap = byId.get(r.id)
      if (!snap) return r
      changed = true
      return { ...r, ...snap }
    })
    return changed ? { ...old, rows } : old
  })
  // A cell edit can move a row in or out of a filtered/sorted view; the
  // debounced invalidation reconciles membership without a refetch storm.
  schedule(ev.page_id)
}

/** delete → remove the ids from every cached page view (always correct). */
export function applyDelete(queryClient: QueryClient, ev: RowsChanged) {
  const ids = new Set(ev.row_ids ?? [])
  if (ids.size === 0) return
  queryClient.setQueriesData<RowsResult>({ queryKey: [ROWS, ev.page_id] }, (old) => {
    if (!old || !Array.isArray(old.rows)) return old
    const rows = old.rows.filter((r) => !ids.has(r.id))
    return rows.length === old.rows.length ? old : { ...old, rows }
  })
}

/** insert WITH snapshots → append only into provably default caches. */
export function applyInsert(
  queryClient: QueryClient,
  ev: RowsChanged,
  schedule: (pageID: string) => void
) {
  if (!ev.rows || ev.rows.length === 0) {
    schedule(ev.page_id) // big batch, no snapshots: refetch
    return
  }
  const matches = queryClient.getQueryCache().findAll({ queryKey: [ROWS, ev.page_id] })
  let undecidable = false
  for (const q of matches) {
    if (q.queryKey[2] === DEFAULT_ROWS_HASH) {
      queryClient.setQueryData<RowsResult>(q.queryKey, (old) => {
        if (!old || !Array.isArray(old.rows)) return old
        const seen = new Set(old.rows.map((r) => r.id))
        const additions = ev.rows!.filter((r) => !seen.has(r.id))
        return additions.length === 0 ? old : { ...old, rows: [...old.rows, ...additions] }
      })
    } else {
      // Filtered/sorted/paged view: can't prove where the row lands. Refetch.
      undecidable = true
    }
  }
  if (undecidable) schedule(ev.page_id)
}

// ---- helpers --------------------------------------------------------------

function parseData<T>(e: MessageEvent): T {
  try {
    return (e.data ? JSON.parse(e.data) : {}) as T
  } catch {
    return {} as T
  }
}

function wireVisibility(
  hiddenPauseMs: number,
  cb: { pause: () => void; resume: () => void }
): () => void {
  if (hiddenPauseMs <= 0 || typeof document === "undefined") return () => {}
  let timer: ReturnType<typeof setTimeout> | null = null
  let paused = false
  const onChange = () => {
    if (document.hidden) {
      timer = setTimeout(() => {
        paused = true
        cb.pause()
      }, hiddenPauseMs)
    } else {
      if (timer) clearTimeout(timer)
      timer = null
      if (paused) {
        paused = false
        cb.resume()
      }
    }
  }
  document.addEventListener("visibilitychange", onChange)
  return () => {
    if (timer) clearTimeout(timer)
    document.removeEventListener("visibilitychange", onChange)
  }
}
