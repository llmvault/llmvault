import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import PricingPage from "./page"
import PricingUnlimitedPage from "./variant-2/page"
import PricingReceiptPage from "./variant-3/page"
import PricingManifestoPage from "./variant-4/page"
import { calculateDeposit } from "./_components/pricing-model"

describe("deposit calculator", () => {
  it("adds the 12% fee without reducing the credit value", () => {
    const estimate = calculateDeposit(100)

    expect(estimate.creditValue).toBe(100)
    expect(estimate.creditsAdded).toBe(100_000)
    expect(estimate.depositFee).toBe(12)
    expect(estimate.checkoutTotal).toBe(112)
  })

  it("does not produce negative values", () => {
    expect(calculateDeposit(-25)).toEqual({
      creditValue: 0,
      creditsAdded: 0,
      depositFee: 0,
      checkoutTotal: 0,
    })
  })
})

describe("pricing page variants", () => {
  it.each([
    ["one fee", PricingPage, "One fee. Everything else stays simple."],
    ["unlimited", PricingUnlimitedPage, "Unlimited means unlimited."],
    ["receipt", PricingReceiptPage, "The whole price fits on one receipt."],
    ["manifesto", PricingManifestoPage, "No plans."],
  ])("renders the complete %s entry point", (_, Page, heading) => {
    const html = renderToString(React.createElement(Page)).replaceAll(
      "<!-- -->",
      ""
    )

    expect(html).toContain(heading)
    expect(html).toContain("Hivy deposit fee (12%)")
    expect(html).toContain("Unlimited Agents")
    expect(html).toContain("Unlimited Sessions")
    expect(html).toContain("Unlimited Sandboxes")
    expect(html).toContain("Unlimited Knowledge base storage")
    expect(html).toContain("Hivy markup on agent costs")
    expect(html).toContain("0%")
    expect(html).toContain("Is there a monthly subscription?")
    expect(html).not.toContain("Editable planning rates")
    expect(html).not.toContain("Sandbox rate")
    expect(html).not.toContain("Storage rate")
    expect(html).not.toContain("10%")
  })
})
