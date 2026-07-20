import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import PricingPage from "./page"
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

describe("pricing page", () => {
  it("renders the manifesto and the complete included feature list", () => {
    const html = renderToString(React.createElement(PricingPage)).replaceAll(
      "<!-- -->",
      ""
    )

    expect(html).toContain("Pay for agent work.")
    expect(html).toContain("Not another subscription.")
    expect(html).toContain("Add $100 in credits and pay $112 once")
    expect(html).toContain("Hivy deposit fee (12%)")
    expect(html).toContain('aria-label="Agent credit balance"')
    expect(html).toContain("data-slider-origin")
    expect(html).toContain("$0")
    expect(html).not.toContain('aria-label="Select $0 deposit"')
    expect(html).toContain('aria-label="Select $5 deposit"')
    expect(html).toContain('aria-label="Select $500 deposit"')
    expect(html).not.toContain('aria-label="Select $2 deposit"')
    expect(html).toContain("transition-[width]")
    expect(html).toContain(
      "transition-[left,background-color,transform,box-shadow]"
    )
    expect(html).toContain("$11.20")
    expect(html).not.toContain("Deposit presets")
    expect(html).toContain("Unlimited users")
    expect(html).toContain("Unlimited teams")
    expect(html).toContain("Unlimited agents")
    expect(html).toContain("Unlimited agent sessions")
    expect(html).toContain("Unlimited sandboxes")
    expect(html).toContain("Knowledge storage that grows with your tier")
    expect(html).toContain("Unlimited knowledge sources")
    expect(html).toContain("Unlimited connections")
    expect(html).toContain("Access to every available model")
    expect(html).toContain("Role-based access control")
    expect(html).toContain("API and MCP access")
    expect(html).not.toContain("Everything included")
    expect(html).not.toContain("Build the whole company")
    expect(html).not.toContain("No feature tiers")
    expect(html).toContain("Hivy adds")
    expect(html).toContain("0%")
    expect(html).toContain("Kept in your balance")
    expect(html).toContain("Will Hivy charge me every month?")
    expect(html).toContain("What capacity do deposits unlock?")
    expect(html).toContain("Unlocks are permanent and never downgrade.")
    expect(html).toContain('data-slot="accordion"')
    expect(html).toContain('data-slot="accordion-panel"')
    expect(html).toContain("pricing-faq-panel")
    expect(html).toContain("pricing-faq-indicator")
    expect(html).toContain("transition-[height,opacity]!")
    expect(html).toContain("motion-reduce:transition-none!")
    expect(html).toContain("Start free. Add $5 when you’re ready.")
    expect(html).not.toContain("Pricing explorations")
    expect(html).not.toContain("10%")
    expect(html).toContain("marketing-link-scope")
  })
})
