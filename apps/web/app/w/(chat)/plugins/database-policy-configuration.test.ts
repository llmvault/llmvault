import { describe, expect, it } from "vitest"
import {
  policyForUpdate,
  setAllValues,
} from "@/app/w/(chat)/plugins/database-policy-configuration"

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

describe("database policy updates", () => {
  it("keeps the configured row limit while replacing editable SQL access", () => {
    const policy = policyForUpdate(
      {
        access_policy: {
          allowed_schemas: ["private"],
          allowed_tables: ["private.audit_log"],
          max_rows: 250,
        },
      },
      {
        allowed_schemas: ["public"],
        allowed_tables: ["public.users"],
        masked_fields: ["email"],
      }
    )

    expect(policy).toEqual({
      allowed_schemas: ["public"],
      allowed_tables: ["public.users"],
      masked_fields: ["email"],
      max_rows: 250,
    })
  })
})
