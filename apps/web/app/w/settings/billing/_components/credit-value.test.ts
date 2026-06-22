import { describe, expect, it } from "vitest"
import {
  creditsToUsageUSD,
  formatCreditLabel,
  formatCreditsWithUsage,
  formatUsageValue,
} from "./credit-value"

describe("credit value formatting", () => {
  it("formats pricing slider labels", () => {
    expect(formatCreditLabel(15000)).toBe("15K")
    expect(formatCreditLabel(180000)).toBe("180K")
    expect(formatCreditLabel(2500)).toBe("2.5K")
  })

  it("converts credits to transparent metered usage value", () => {
    expect(creditsToUsageUSD(1000)).toBe(1)
    expect(creditsToUsageUSD(15000)).toBe(15)
    expect(creditsToUsageUSD(12500)).toBe(12.5)
  })

  it("formats usage value and balance summaries", () => {
    expect(formatUsageValue(15000)).toBe("$15")
    expect(formatUsageValue(12500)).toBe("$12.50")
    expect(formatCreditsWithUsage(12500)).toBe(
      "12,500 credits · $12.50 usage value"
    )
  })
})
