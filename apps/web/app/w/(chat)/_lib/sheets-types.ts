import type { components } from "@/lib/api/schema"

export type SheetPage = components["schemas"]["sheetPageView"]
export type SheetField = components["schemas"]["sheetFieldView"]
export type SheetRow = components["schemas"]["sheetRowView"]
export type SheetView = components["schemas"]["sheetViewSummary"]
export type SheetOperation = components["schemas"]["sheetOperationView"]
export type SheetImportJob = components["schemas"]["sheetImportJobView"]
export type SheetStructure = components["schemas"]["sheetStructureResponse"]
export type SheetRowsQueryResponse =
  components["schemas"]["sheetRowsQueryResponse"]
export type SheetRelationRef = components["schemas"]["sheetRelationRef"]
/** Filter AST node. The REST endpoints share the Go `sheets.Filter` shape. */
export type SheetFilterNode = components["schemas"]["Filter"]
export type SheetSort = components["schemas"]["Sort"]
export type SheetFieldSpec = components["schemas"]["sheetFieldSpecRequest"]

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

export interface RowsQueryInput {
  filter?: SheetFilterNode
  search?: string
  sorts?: SheetSort[]
  cursor?: string
  limit?: number
  resolve_relations?: boolean
}

/* ------------------------------------------------------------------ */
/* Cross-sheet page resolver                                           */
/* ------------------------------------------------------------------ */

export interface SheetPageRef {
  sheetId: string
  sheetName: string
  page: SheetPage
}

/* ------------------------------------------------------------------ */
/* Filter toolbar state → filter AST                                   */
/* ------------------------------------------------------------------ */

export interface FilterRuleState {
  id: string
  field: string
  op: string
  value: string
  /** Values for multi-value operators (`in`); entered as tags. */
  values?: string[]
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
