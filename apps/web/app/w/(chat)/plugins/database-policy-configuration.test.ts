import { describe, expect, it } from "vitest"
import { setAllValues } from "@/app/w/(chat)/plugins/database-policy-configuration"

describe("database policy table selection", () => {
  it("selects and clears every visible table without changing hidden selections", () => {
    const visibleTables = ["public.accounts", "public.orders"]
    const initialSelection = new Set(["private.audit_log"])

    const selected = setAllValues(initialSelection, visibleTables, true)
    expect(Array.from(selected).sort()).toEqual([
      "private.audit_log",
      "public.accounts",
      "public.orders",
    ])

    const cleared = setAllValues(selected, visibleTables, false)
    expect(Array.from(cleared)).toEqual(["private.audit_log"])
  })
})
