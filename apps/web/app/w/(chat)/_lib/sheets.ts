import type { InfiniteData, QueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

export type SheetSummary = components["schemas"]["SheetSummary"]
export type SheetPage = components["schemas"]["SheetPage"]
export type SheetField = components["schemas"]["SheetField"]
export type SheetRow = components["schemas"]["SheetRow"]
export type SheetView = components["schemas"]["SheetView"]
export type SheetOperation = components["schemas"]["SheetOperation"]
export type SheetImportJob = components["schemas"]["SheetImportJob"]
export type SheetRelationRef = components["schemas"]["SheetRelationRef"]
export type SheetStructure = components["schemas"]["SheetStructureResponse"]
export type SheetRowsQueryResponse =
  components["schemas"]["SheetRowsQueryResponse"]
export type SheetFilterNode = components["schemas"]["sheetFilterNode"]
export type SheetSort = components["schemas"]["sheetSort"]
export type SheetFieldSpec = components["schemas"]["sheetFieldSpec"]

export const ROWS_PAGE_SIZE = 200

export const SHEET_FIELD_TYPES = [
  "text",
  "long_text",
  "number",
  "checkbox",
  "select",
  "multi_select",
  "date",
  "url",
  "email",
  "phone",
  "attachment",
  "relation",
] as const

export type SheetFieldType = (typeof SHEET_FIELD_TYPES)[number]

/* ------------------------------------------------------------------ */
/* Self-echo suppression: every mutation carries a client-generated    */
/* mutation_id that the backend echoes on SSE events.                  */
/* ------------------------------------------------------------------ */

const recentMutationIds = new Set<string>()

export function newMutationId(): string {
  const id =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `mut_${Date.now()}_${Math.random().toString(36).slice(2)}`
  recentMutationIds.add(id)
  if (recentMutationIds.size > 200) {
    for (const old of recentMutationIds) {
      if (recentMutationIds.size <= 100) break
      recentMutationIds.delete(old)
    }
  }
  return id
}

export function isOwnMutation(id: string | null | undefined): boolean {
  return Boolean(id && recentMutationIds.has(id))
}

/* ------------------------------------------------------------------ */
/* Query keys                                                          */
/* ------------------------------------------------------------------ */

export const sheetKeys = {
  list: ["sheets"] as const,
  structure: (sheetId: string) => ["sheet-structure", sheetId] as const,
  structurePrefix: ["sheet-structure"] as const,
  pageRef: (pageId: string) => ["sheet-page-ref", pageId] as const,
  views: (pageId: string) => ["sheet-views", pageId] as const,
  rows: (pageId: string, querySig: string) =>
    ["sheet-rows", pageId, querySig] as const,
  rowsPrefix: (pageId: string) => ["sheet-rows", pageId] as const,
  attachmentUrls: (pageId: string) => ["sheet-attachment-urls", pageId] as const,
  operations: (pageId: string) => ["sheet-operations", pageId] as const,
  importJob: (jobId: string) => ["sheet-import", jobId] as const,
}

export interface RowsQueryInput {
  filter?: SheetFilterNode
  search?: string
  sorts?: SheetSort[]
  cursor?: string
  limit?: number
  resolve_relations?: boolean
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

/* ------------------------------------------------------------------ */
/* API calls (typed openapi-fetch client through /api/proxy)           */
/* ------------------------------------------------------------------ */

async function unwrap<T>(
  result: Promise<{ data?: T; error?: unknown }>
): Promise<T> {
  const { data, error } = await result
  if (error !== undefined) {
    throw new Error(extractErrorMessage(error, "Sheets request failed"))
  }
  return data as T
}

export function fetchSheets(signal?: AbortSignal) {
  return unwrap(
    api.GET("/v1/sheets", { params: { query: { limit: 200 } }, signal })
  )
}

export function createSheet(name: string) {
  return unwrap(api.POST("/v1/sheets", { body: { name } }))
}

export function fetchSheetStructure(sheetID: string, signal?: AbortSignal) {
  return unwrap(
    api.GET("/v1/sheets/{sheetID}", { params: { path: { sheetID } }, signal })
  )
}

export function createSheetPage(sheetID: string, name: string) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages", {
      params: { path: { sheetID } },
      body: { name, mutation_id: newMutationId() },
    })
  )
}

export function createSheetField(
  sheetID: string,
  pageID: string,
  spec: SheetFieldSpec
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/fields", {
      params: { path: { sheetID, pageID } },
      body: { ...spec, mutation_id: newMutationId() },
    })
  )
}

export function queryRows(
  sheetID: string,
  pageID: string,
  input: RowsQueryInput,
  signal?: AbortSignal
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/rows/query", {
      params: { path: { sheetID, pageID } },
      body: {
        filter: input.filter,
        search: input.search || undefined,
        sorts: input.sorts?.length ? input.sorts : undefined,
        cursor: input.cursor || undefined,
        limit: input.limit ?? ROWS_PAGE_SIZE,
        resolve_relations: input.resolve_relations ?? true,
      },
      signal,
    })
  )
}

export function insertRows(
  sheetID: string,
  pageID: string,
  rows: { data: Record<string, unknown>; position?: number }[],
  mutationId = newMutationId()
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/rows", {
      params: { path: { sheetID, pageID } },
      body: { rows, mutation_id: mutationId },
    })
  )
}

export function updateRows(
  sheetID: string,
  pageID: string,
  rows: { id: string; data?: Record<string, unknown>; position?: number }[],
  mutationId = newMutationId()
) {
  return unwrap(
    api.PATCH("/v1/sheets/{sheetID}/pages/{pageID}/rows", {
      params: { path: { sheetID, pageID } },
      body: { rows, mutation_id: mutationId },
    })
  )
}

export function deleteRows(
  sheetID: string,
  pageID: string,
  ids: string[],
  mutationId = newMutationId()
) {
  return unwrap(
    api.DELETE("/v1/sheets/{sheetID}/pages/{pageID}/rows", {
      params: { path: { sheetID, pageID } },
      body: { ids, mutation_id: mutationId },
    })
  )
}

export function fetchViews(
  sheetID: string,
  pageID: string,
  signal?: AbortSignal
) {
  return unwrap(
    api.GET("/v1/sheets/{sheetID}/pages/{pageID}/views", {
      params: { path: { sheetID, pageID } },
      signal,
    })
  )
}

export function createView(
  sheetID: string,
  pageID: string,
  body: { name: string; type?: string; config?: Record<string, unknown> }
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/views", {
      params: { path: { sheetID, pageID } },
      body,
    })
  )
}

export function updateView(
  sheetID: string,
  pageID: string,
  viewID: string,
  body: { config?: Record<string, unknown>; name?: string }
) {
  return unwrap(
    api.PATCH("/v1/sheets/{sheetID}/pages/{pageID}/views/{viewID}", {
      params: { path: { sheetID, pageID, viewID } },
      body,
    })
  )
}

export function fetchOperations(
  sheetID: string,
  pageID: string,
  signal?: AbortSignal
) {
  return unwrap(
    api.GET("/v1/sheets/{sheetID}/pages/{pageID}/operations", {
      params: { path: { sheetID, pageID }, query: { limit: 25 } },
      signal,
    })
  )
}

export function revertOperation(
  sheetID: string,
  pageID: string,
  operationID: string
) {
  return unwrap(
    api.POST(
      "/v1/sheets/{sheetID}/pages/{pageID}/operations/{operationID}/revert",
      {
        params: { path: { sheetID, pageID, operationID } },
        body: { mutation_id: newMutationId() },
      }
    )
  )
}

export function createImport(
  sheetID: string,
  pageID: string,
  objectKey: string,
  options: Record<string, unknown>
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/imports", {
      params: { path: { sheetID, pageID } },
      body: {
        object_key: objectKey,
        options,
        mutation_id: newMutationId(),
      },
    })
  )
}

export function fetchImportJob(jobID: string, signal?: AbortSignal) {
  return unwrap(
    api.GET("/v1/sheets/imports/{jobID}", {
      params: { path: { jobID } },
      signal,
    })
  )
}

export function fetchAttachmentDownloadURLs(
  sheetID: string,
  pageID: string,
  keys: string[]
) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/pages/{pageID}/attachments/download-url", {
      params: { path: { sheetID, pageID } },
      body: { keys },
    })
  )
}

export function mintLiveToken(sheetID: string) {
  return unwrap(
    api.POST("/v1/sheets/{sheetID}/live-token", {
      params: { path: { sheetID } },
    })
  )
}

export function exportCsvUrl(sheetID: string, pageID: string): string {
  return `/api/proxy/v1/sheets/${encodeURIComponent(
    sheetID
  )}/pages/${encodeURIComponent(pageID)}/export.csv`
}

/** Signs an upload for a sheets asset type and PUTs the file to storage. */
export async function uploadSheetObject(
  assetType: "sheet_import" | "sheet_attachment",
  file: File
): Promise<string> {
  const contentType = file.type || "application/octet-stream"
  const signed = await unwrap(
    api.POST("/v1/uploads/sign", {
      body: {
        asset_type: assetType,
        content_type: contentType,
        filename: file.name,
        size_bytes: file.size,
      },
    })
  )
  if (!signed.upload_url || !signed.key) {
    throw new Error("Upload could not be signed")
  }
  const putRes = await fetch(signed.upload_url, {
    method: signed.upload_method || "PUT",
    headers: {
      "Content-Type": contentType,
      ...(signed.required_headers ?? {}),
    },
    body: file,
  })
  if (!putRes.ok) throw new Error(`Upload failed with HTTP ${putRes.status}`)
  return signed.key
}

/* ------------------------------------------------------------------ */
/* Cross-sheet page resolver                                           */
/*                                                                     */
/* Relation fields target a page by ID, which may live in a different  */
/* sheet than the one currently open. Rows can only be queried through */
/* the page's OWNING sheet path, so this resolver maps any org page ID */
/* to its owning sheet — lazily, reusing the TanStack caches for the   */
/* sheets list and per-sheet structures.                               */
/* ------------------------------------------------------------------ */

export interface SheetPageRef {
  sheetId: string
  sheetName: string
  page: SheetPage
}

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
 * list and the structures that are not cached yet. Results flow through
 * `ensureQueryData`, so every fetch also warms the regular caches.
 */
export async function resolvePageRef(
  queryClient: QueryClient,
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

  const list = await queryClient.ensureQueryData({
    queryKey: sheetKeys.list,
    queryFn: ({ signal }) => fetchSheets(signal),
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

/* ------------------------------------------------------------------ */
/* Field helpers                                                       */
/* ------------------------------------------------------------------ */

export function selectChoices(field: SheetField): string[] {
  const raw = field.options?.choices
  if (!Array.isArray(raw)) return []
  return raw.filter((c): c is string => typeof c === "string")
}

export function relationTargetPageId(field: SheetField): string | null {
  const raw = field.options?.target_page_id
  return typeof raw === "string" && raw ? raw : null
}

export function stringArrayValue(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((v): v is string => typeof v === "string")
}

/** Human label for a row on a page, using display_field_id or first text field. */
export function rowDisplayLabel(row: SheetRow, page: SheetPage): string {
  const fields = page.fields ?? []
  const displayField =
    fields.find((f) => f.id === page.display_field_id) ??
    fields.find((f) => f.type === "text") ??
    fields[0]
  const value = displayField?.id ? row.data?.[displayField.id] : undefined
  if (typeof value === "string" && value.trim()) return value
  if (typeof value === "number") return String(value)
  return "Untitled row"
}

/* ------------------------------------------------------------------ */
/* Filter toolbar state → filter AST                                   */
/* ------------------------------------------------------------------ */

export interface FilterRuleState {
  id: string
  field: string
  op: string
  value: string
}

const VALUELESS_OPS = new Set(["is_empty", "is_not_empty"])

export function filterOpsForType(type: string | undefined): string[] {
  switch (type) {
    case "number":
      return ["eq", "neq", "gt", "gte", "lt", "lte", "is_empty", "is_not_empty"]
    case "date":
      return ["eq", "gt", "gte", "lt", "lte", "is_empty", "is_not_empty"]
    case "checkbox":
      return ["eq"]
    case "select":
      return ["eq", "neq", "is_empty", "is_not_empty"]
    case "multi_select":
      return ["contains", "not_contains", "is_empty", "is_not_empty"]
    case "attachment":
    case "relation":
      return ["is_empty", "is_not_empty"]
    default:
      return [
        "eq",
        "neq",
        "contains",
        "not_contains",
        "starts_with",
        "is_empty",
        "is_not_empty",
      ]
  }
}

export function filterOpLabel(op: string): string {
  switch (op) {
    case "eq":
      return "is"
    case "neq":
      return "is not"
    case "contains":
      return "contains"
    case "not_contains":
      return "doesn't contain"
    case "starts_with":
      return "starts with"
    case "gt":
      return ">"
    case "gte":
      return "≥"
    case "lt":
      return "<"
    case "lte":
      return "≤"
    case "is_empty":
      return "is empty"
    case "is_not_empty":
      return "is not empty"
    default:
      return op
  }
}

export function filterOpNeedsValue(op: string): boolean {
  return !VALUELESS_OPS.has(op)
}

export function compileFilter(
  rules: FilterRuleState[],
  fields: SheetField[]
): SheetFilterNode | undefined {
  const byId = new Map(fields.map((f) => [f.id ?? "", f]))
  const nodes: Record<string, unknown>[] = []
  for (const rule of rules) {
    const field = byId.get(rule.field)
    if (!field?.id || !rule.op) continue
    if (filterOpNeedsValue(rule.op) && rule.value.trim() === "") continue
    let value: unknown = rule.value
    if (field.type === "number") {
      const parsed = Number(rule.value)
      if (!Number.isFinite(parsed)) continue
      value = parsed
    } else if (field.type === "checkbox") {
      value = rule.value === "true"
    }
    nodes.push(
      filterOpNeedsValue(rule.op)
        ? { field: field.id, op: rule.op, value }
        : { field: field.id, op: rule.op }
    )
  }
  if (nodes.length === 0) return undefined
  if (nodes.length === 1) return nodes[0] as SheetFilterNode
  return { and: nodes }
}

/* ------------------------------------------------------------------ */
/* SSE events → TanStack Query cache                                   */
/* ------------------------------------------------------------------ */

export interface SheetLiveEvent {
  type: string
  sheet_id?: string
  page_id?: string
  action?: string
  row_ids?: string[]
  patches?: Record<string, Record<string, unknown>>
  field_id?: string
  operation_id?: string
  job_id?: string
  processed_rows?: number
  total_rows?: number
  status?: string
  actor?: { agent_id?: string; user_id?: string }
  mutation_id?: string
}

type RowsCache = InfiniteData<SheetRowsQueryResponse>

/**
 * Applies a rows_changed event to every cached rows window for the page.
 * Deletes always apply (ids are always present); updates apply when the
 * event carries patches; inserts and patch-less updates invalidate the
 * window instead (events are hints, REST is truth).
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

  void queryClient.invalidateQueries({ queryKey: prefix })
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
