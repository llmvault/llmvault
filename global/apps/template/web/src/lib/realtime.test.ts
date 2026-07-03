import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { QueryClient } from "@tanstack/react-query"
import {
  DEFAULT_ROWS_HASH,
  rowsKey,
  stableHash,
  startRealtime,
  type EventSourceLike,
  type RealtimeHandle,
  type Row,
  type RowsQuery,
  type RowsResult,
} from "./realtime"

// FakeEventSource is a scriptable EventSource: tests register listeners the
// same way the engine does, then emit named events with JSON payloads.
class FakeEventSource implements EventSourceLike {
  static last: FakeEventSource | null = null
  listeners = new Map<string, ((event: MessageEvent) => void)[]>()
  closed = false
  constructor(public url: string) {
    FakeEventSource.last = this
  }
  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    const arr = this.listeners.get(type) ?? []
    arr.push(listener)
    this.listeners.set(type, arr)
  }
  close() {
    this.closed = true
  }
  emit(type: string, data: unknown) {
    const payload = data === undefined ? "" : JSON.stringify(data)
    for (const l of this.listeners.get(type) ?? []) {
      l({ data: payload } as MessageEvent)
    }
  }
}

function makeRow(id: string, data: Record<string, unknown>): Row {
  return { id, data, position: 1 }
}

function seedRows(qc: QueryClient, pageID: string, query: RowsQuery | undefined, rows: Row[]) {
  const result: RowsResult = { rows, next_cursor: "" }
  qc.setQueryData(rowsKey(pageID, query), result)
}

function cachedRows(qc: QueryClient, pageID: string, query?: RowsQuery): Row[] {
  return qc.getQueryData<RowsResult>(rowsKey(pageID, query))?.rows ?? []
}

const PAGE = "pg_1"

describe("query-key convention", () => {
  it("hashes the default query to DEFAULT_ROWS_HASH", () => {
    expect(stableHash(undefined)).toBe(DEFAULT_ROWS_HASH)
    expect(stableHash({})).toBe(DEFAULT_ROWS_HASH)
    expect(rowsKey(PAGE)).toEqual(["rows", PAGE, DEFAULT_ROWS_HASH])
  })

  it("is order-independent and distinct for real options", () => {
    const a = stableHash({ search: "x", limit: 5 })
    const b = stableHash({ limit: 5, search: "x" })
    expect(a).toBe(b)
    expect(a).not.toBe(DEFAULT_ROWS_HASH)
    expect(stableHash({ filter: { field: "f", op: "eq", value: 1 } })).not.toBe(DEFAULT_ROWS_HASH)
  })
})

describe("realtime patch engine", () => {
  let qc: QueryClient
  let handle: RealtimeHandle
  let es: FakeEventSource

  beforeEach(() => {
    vi.useFakeTimers()
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    handle = startRealtime(qc, {
      debounceMs: 2000,
      hiddenPauseMs: 0,
      eventSourceFactory: (url) => new FakeEventSource(url),
    })
    es = FakeEventSource.last!
  })

  afterEach(() => {
    handle.stop()
    qc.clear()
    vi.useRealTimers()
  })

  it("subscribes to the relay endpoint", () => {
    expect(es.url).toBe("/api/_live")
  })

  it("update WITH snapshots merges into seeded cache, then debounced invalidation fires", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", { fld_a: "old" }), makeRow("r2", { fld_a: "keep" })])
    const invalidate = vi.spyOn(qc, "invalidateQueries")

    es.emit("rows_changed", {
      page_id: PAGE,
      action: "update",
      rows: [makeRow("r1", { fld_a: "new" })],
    })

    // Immediate patch: r1's cell updated verbatim, r2 untouched.
    const rows = cachedRows(qc, PAGE)
    expect(rows.find((r) => r.id === "r1")?.data.fld_a).toBe("new")
    expect(rows.find((r) => r.id === "r2")?.data.fld_a).toBe("keep")

    // The reconciler is debounced, not immediate.
    expect(invalidate).not.toHaveBeenCalled()
    vi.advanceTimersByTime(2000)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["rows", PAGE] })
  })

  it("coalesces rapid updates into a single debounced invalidation", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", { n: 0 })])
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    for (let i = 1; i <= 5; i++) {
      es.emit("rows_changed", { page_id: PAGE, action: "update", rows: [makeRow("r1", { n: i })] })
      vi.advanceTimersByTime(500) // < debounce window each time
    }
    expect(cachedRows(qc, PAGE)[0].data.n).toBe(5)
    expect(invalidate).not.toHaveBeenCalled()
    vi.advanceTimersByTime(2000)
    const rowsCalls = invalidate.mock.calls.filter(
      (c) => JSON.stringify(c[0]) === JSON.stringify({ queryKey: ["rows", PAGE] })
    )
    expect(rowsCalls).toHaveLength(1)
  })

  it("delete removes ids from every cached view immediately, no refetch", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", {}), makeRow("r2", {})])
    seedRows(qc, PAGE, { search: "x" }, [makeRow("r1", {}), makeRow("r3", {})])
    const invalidate = vi.spyOn(qc, "invalidateQueries")

    es.emit("rows_changed", { page_id: PAGE, action: "delete", row_ids: ["r1"] })

    expect(cachedRows(qc, PAGE).map((r) => r.id)).toEqual(["r2"])
    expect(cachedRows(qc, PAGE, { search: "x" }).map((r) => r.id)).toEqual(["r3"])
    vi.advanceTimersByTime(5000)
    expect(invalidate).not.toHaveBeenCalled()
  })

  it("insert appends only into the decidable default cache; filtered caches refetch", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", {})])
    seedRows(qc, PAGE, { filter: { field: "f", op: "eq", value: 1 } }, [makeRow("r1", {})])
    const invalidate = vi.spyOn(qc, "invalidateQueries")

    es.emit("rows_changed", { page_id: PAGE, action: "insert", rows: [makeRow("r2", { fld_a: "v" })] })

    // Default (unfiltered) cache got the append immediately.
    expect(cachedRows(qc, PAGE).map((r) => r.id)).toEqual(["r1", "r2"])
    // Filtered cache is left alone; a debounced invalidation reconciles it.
    expect(cachedRows(qc, PAGE, { filter: { field: "f", op: "eq", value: 1 } }).map((r) => r.id)).toEqual(["r1"])
    expect(invalidate).not.toHaveBeenCalled()
    vi.advanceTimersByTime(2000)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["rows", PAGE] })
  })

  it("insert does not duplicate a row already present in the default cache", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", {})])
    es.emit("rows_changed", { page_id: PAGE, action: "insert", rows: [makeRow("r1", {})] })
    expect(cachedRows(qc, PAGE).map((r) => r.id)).toEqual(["r1"])
  })

  it("insert with only a decidable cache does not schedule a refetch", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", {})])
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    es.emit("rows_changed", { page_id: PAGE, action: "insert", rows: [makeRow("r2", {})] })
    vi.advanceTimersByTime(5000)
    expect(invalidate).not.toHaveBeenCalled()
  })

  it("big-batch update without snapshots falls back to debounced invalidation", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", { n: 0 })])
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    es.emit("rows_changed", { page_id: PAGE, action: "update", row_ids: ["r1"] }) // no rows[]
    expect(invalidate).not.toHaveBeenCalled()
    vi.advanceTimersByTime(2000)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["rows", PAGE] })
  })

  it("fields_changed invalidates pages, and rows of the page when a field is typed", () => {
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    es.emit("fields_changed", { page_id: PAGE, field_id: "fld_a" })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["pages"] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["rows", PAGE] })
  })

  it("pages_changed invalidates only pages", () => {
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    es.emit("pages_changed", { page_id: PAGE })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["pages"] })
    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: ["rows", PAGE] })
  })

  it("refresh invalidates all sheet-related caches", () => {
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    es.emit("refresh", {})
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["pages"] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["rows"] })
  })

  it("stop() closes the stream and cancels pending debounces", () => {
    seedRows(qc, PAGE, undefined, [makeRow("r1", { n: 0 })])
    es.emit("rows_changed", { page_id: PAGE, action: "update", rows: [makeRow("r1", { n: 1 })] })
    const invalidate = vi.spyOn(qc, "invalidateQueries")
    handle.stop()
    expect(es.closed).toBe(true)
    vi.advanceTimersByTime(5000)
    expect(invalidate).not.toHaveBeenCalled()
  })
})
