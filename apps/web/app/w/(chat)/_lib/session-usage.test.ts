import { describe, expect, it } from "vitest"
import {
  creditsForCostUSD,
  formatSessionCredits,
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
        cost_usd: 0.004,
        credits: 4.5,
        model_cost_usd: 0.0035,
        model_credits: 4,
        sandbox_cost_usd: 0.0005,
        sandbox_credits: 0.5,
        sandbox_vcpu_seconds: 30,
      })
    ).toMatchObject({
      costUsd: 0.004,
      credits: 4.5,
      modelCostUsd: 0.0035,
      modelCredits: 4,
      sandboxCostUsd: 0.0005,
      sandboxCredits: 0.5,
      sandboxVCPUSeconds: 30,
    })
  })

  it("formats fractional sandbox credits without unnecessary zeroes", () => {
    expect(formatSessionCredits(0)).toBe("0")
    expect(formatSessionCredits(0.5)).toBe("0.5")
    expect(formatSessionCredits(12.345)).toBe("12.35")
  })

  it("reads positive model usage costs from stream payloads", () => {
    expect(modelUsageCost({ usage: { cost: 0.0023 } })).toBe(0.0023)
    expect(modelUsageCost({ usage: { cost: -1 } })).toBe(0)
  })
})
