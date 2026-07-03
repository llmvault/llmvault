import type {
  SheetField,
  SheetPage,
  SheetRow,
} from "@/app/w/(chat)/_lib/sheets-types"

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
