import type {
  FilterRuleState,
  SheetField,
  SheetFilterNode,
} from "@/app/w/(chat)/_lib/sheets-types"

/* ------------------------------------------------------------------ */
/* Filter toolbar state → filter AST                                   */
/* ------------------------------------------------------------------ */

const VALUELESS_OPS = new Set(["is_empty", "is_not_empty"])
const MULTI_VALUE_OPS = new Set(["in"])

export function filterOpsForType(type: string | undefined): string[] {
  switch (type) {
    case "number":
      return [
        "eq",
        "neq",
        "gt",
        "gte",
        "lt",
        "lte",
        "in",
        "is_empty",
        "is_not_empty",
      ]
    case "date":
      return ["eq", "gt", "gte", "lt", "lte", "is_empty", "is_not_empty"]
    case "checkbox":
      return ["eq"]
    case "select":
      return ["eq", "neq", "in", "is_empty", "is_not_empty"]
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
        "in",
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
    case "in":
      return "is any of"
    default:
      return op
  }
}

export function filterOpNeedsValue(op: string): boolean {
  return !VALUELESS_OPS.has(op)
}

export function filterOpIsMulti(op: string): boolean {
  return MULTI_VALUE_OPS.has(op)
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

    if (filterOpIsMulti(rule.op)) {
      let values: unknown[] = (rule.values ?? [])
        .map((entry) => entry.trim())
        .filter(Boolean)
      if (field.type === "number") {
        values = values
          .map((entry) => Number(entry))
          .filter((entry) => Number.isFinite(entry))
      }
      if (values.length === 0) continue
      nodes.push({ field: field.id, op: rule.op, value: values })
      continue
    }

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
