import { describe, expect, it } from "vitest"
import { shouldArmHistoryPagination } from "./session-history-top-loader"

describe("session history top loader", () => {
  it("arms pagination only after upward scroll intent", () => {
    expect(shouldArmHistoryPagination(0, 0)).toBe(false)
    expect(shouldArmHistoryPagination(120, 150)).toBe(false)
    expect(shouldArmHistoryPagination(120, 119)).toBe(false)
    expect(shouldArmHistoryPagination(120, 80)).toBe(true)
  })
})
