import { describe, expect, it } from "vitest"
import { isModelNew } from "@/lib/model-new"

const WINDOW = {
  new_from: "2026-07-24T00:00:00Z",
  new_to: "2026-09-24T00:00:00Z",
}

describe("isModelNew", () => {
  it("includes the start and excludes the end of a model's new window", () => {
    expect(isModelNew(WINDOW, new Date("2026-07-24T00:00:00Z"))).toBe(true)
    expect(isModelNew(WINDOW, new Date("2026-09-23T23:59:59Z"))).toBe(true)
    expect(isModelNew(WINDOW, new Date("2026-09-24T00:00:00Z"))).toBe(false)
  })

  it("rejects missing or invalid windows", () => {
    expect(isModelNew(undefined, new Date("2026-08-01T00:00:00Z"))).toBe(false)
    expect(
      isModelNew(
        { new_from: "not-a-date", new_to: WINDOW.new_to },
        new Date("2026-08-01T00:00:00Z")
      )
    ).toBe(false)
  })
})
