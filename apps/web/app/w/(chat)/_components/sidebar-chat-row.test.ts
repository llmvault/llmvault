import { describe, expect, it } from "vitest"
import { sessionRowAccessoryDisplay } from "./sidebar-chat-row"

describe("sessionRowAccessoryDisplay", () => {
  it("shows active and failed runtime status in the timestamp slot", () => {
    expect(sessionRowAccessoryDisplay("streaming", "now")).toMatchObject({
      kind: "status",
      indicator: { icon: "loader-2" },
    })
    expect(sessionRowAccessoryDisplay("failed", "20m")).toMatchObject({
      kind: "status",
      indicator: { icon: "triangle-alert" },
    })
  })

  it("falls back to activity time when no runtime status is visible", () => {
    expect(sessionRowAccessoryDisplay("idle", "2h")).toEqual({
      kind: "meta",
      meta: "2h",
    })
    expect(sessionRowAccessoryDisplay("idle")).toEqual({ kind: "empty" })
  })
})
