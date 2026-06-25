import { describe, expect, it } from "vitest"
import {
  creditsForCostUSD,
  modelUsageCost,
  sessionUsageSummaryFromSnapshot,
} from "@/app/w/(chat)/_lib/session-usage"

describe("session usage", () => {
  it("converts positive costs to rounded-up credits", () => {
    expect(creditsForCostUSD(0)).toBe(0)
    expect(creditsForCostUSD(0.001)).toBe(1)
    expect(creditsForCostUSD(0.0011)).toBe(2)
  })

  it("normalizes usage snapshots to display totals", () => {
    expect(
      sessionUsageSummaryFromSnapshot({
        cost_usd: 0.0035,
        credits: 4,
      })
    ).toMatchObject({
      costUsd: 0.0035,
      credits: 4,
    })
  })

  it("reads positive model usage costs from stream payloads", () => {
    expect(modelUsageCost({ usage: { cost: 0.0023 } })).toBe(0.0023)
    expect(modelUsageCost({ usage: { cost: -1 } })).toBe(0)
  })
})
