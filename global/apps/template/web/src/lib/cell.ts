// Cell rendering helpers. Row data comes back typed `Record<string,
// unknown>` (src/lib/realtime.ts's Row.data) because a sheet cell can hold
// any field type — text, number, boolean, a date string, an unresolved
// relation object, and so on. Putting that `unknown` value straight into
// JSX does not type-check (`TS2322: Type 'unknown' is not assignable to
// type 'ReactNode'`). Route every cell value through `cell()` (or
// `cellText()` for a plain string, e.g. in an <input value={…}>) instead of
// reaching for `as string` / `as any` — those silence the type error without
// deciding how the value should actually render.
import type { ReactNode } from "react"

/**
 * Render any row-cell value as a plain string: strings/numbers/bigints
 * print as themselves, booleans as "true"/"false" (React renders a bare
 * boolean as nothing, which reads as a mysteriously blank cell), null/
 * undefined as "" (so the cell stays present but empty rather than
 * collapsing), and anything else (an unresolved relation cell, a nested
 * object) as JSON so you see the shape instead of "[object Object]".
 */
export function cellText(value: unknown): string {
  if (value === null || value === undefined) return ""
  if (typeof value === "string") return value
  if (typeof value === "number" || typeof value === "bigint") return String(value)
  if (typeof value === "boolean") return value ? "true" : "false"
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

/** JSX-ready alias of cellText — use directly inside `{}` in a cell. */
export function cell(value: unknown): ReactNode {
  return cellText(value)
}
