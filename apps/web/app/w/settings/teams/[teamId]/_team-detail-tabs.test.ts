import { describe, expect, it } from "vitest"
import { teamDetailTabs } from "./_team-detail-tabs"

describe("teamDetailTabs", () => {
  it("returns every team settings tab for administrators", () => {
    expect(teamDetailTabs(true).map((tab) => tab.id)).toEqual([
      "overview",
      "connections",
      "routing",
      "skills",
      "knowledge",
      "environment-variables",
    ])
  })

  it("keeps knowledge controls hidden from non-administrators", () => {
    expect(teamDetailTabs(false).map((tab) => tab.id)).toEqual([
      "overview",
      "connections",
      "routing",
      "skills",
      "environment-variables",
    ])
  })
})
