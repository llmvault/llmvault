import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import PricingPage from "./page"
import { pricingComparisons } from "./_components/pricing-comparison-data"
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
    expect(html).not.toContain("Watch a 2min demo")
    expect(html).toContain("Hivy deposit fee (12%)")
    expect(html).toContain('aria-label="Agent credit balance"')
    expect(html).toContain("data-slider-origin")
    expect(html).toContain("$0")
    expect(html).not.toContain('aria-label="Select $0 deposit"')
    expect(html).toContain('aria-label="Select $5 deposit"')
    expect(html).toContain('aria-label="Select $250 deposit"')
    expect(html).toContain('aria-label="Select $500 deposit"')
    expect(html).not.toContain('aria-label="Select $2 deposit"')
    expect(html).not.toContain("data-tier-indicator")
    expect(html).not.toContain("lifetime deposits")
    expect(html).toContain("one credit per active vCPU-minute")
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
    expect(html).toContain("Unlimited knowledge storage")
    expect(html).toContain("Unlimited knowledge sources")
    expect(html).toContain("Unlimited connections")
    expect(html).toContain("Access to every available model")
    expect(html).toContain("Unlimited agent drive storage")
    expect(html).toContain("Unlimited agent sheets")
    expect(html).toContain("Unlimited automations")
    expect(html).toContain("Role-based access control")
    expect(html).toContain("API and MCP access")
    expect(html).toContain("Every sandbox size is available.")
    expect(html).toContain("Nano and Small cost 1 credit per active minute")
    expect(html).not.toContain("Everything included")
    expect(html).not.toContain("Build the whole company")
    expect(html).not.toContain("No feature tiers")
    expect(html).toContain("Hivy adds")
    expect(html).toContain("0%")
    expect(html).toContain("Kept in your balance")
    expect(html).toContain("Will Hivy charge me every month?")
    expect(html).toContain("How is sandbox compute charged?")
    expect(html).toContain("Can I choose any sandbox size?")
    expect(html).toContain(
      "There are no deposit tiers or sandbox-size unlocks."
    )
    expect(html).not.toContain("capacity tiers")
    expect(html).not.toContain("Unlocks are permanent")
    expect(html).toContain('data-slot="accordion"')
    expect(html).toContain('data-slot="accordion-panel"')
    expect(html).toContain("pricing-faq-panel")
    expect(html).toContain("pricing-faq-indicator")
    expect(html).toContain("transition-[height,opacity]!")
    expect(html).toContain("motion-reduce:transition-none!")
    expect(html).toContain("Start free. Add $5 when you’re ready.")
    expect(html.match(/>Start for free</g)).toHaveLength(2)
    expect(html).not.toContain("Ask a pricing question")
    expect(html).not.toContain("Pricing explorations")
    expect(html).not.toContain("10%")
    expect(html).toContain("marketing-link-scope")
    expect(html).toContain("Your business is overpaying for AI work.")
    expect(html).toContain(
      "These providers charge recurring platform or seat fees before your agents do any work."
    )
    expect(html).toContain("Choose a provider to compare with Hivy")
    expect(html).toContain("Hivy vs. Claude Team")
    expect(html).toContain("$50 / month")
    expect(html).toContain("Public prices checked August 10, 2026")
    expect(html).toContain(
      "This compares billing mechanics, not feature equivalence."
    )
  })
})

describe("provider pricing comparisons", () => {
  it("keeps one complete comparison for each requested provider", () => {
    expect(pricingComparisons.map((comparison) => comparison.id)).toEqual([
      "claude",
      "chatgpt",
      "gumloop",
      "notion",
    ])
    expect(pricingComparisons.at(-1)?.tabLabel).toBe("Notion agents")

    for (const comparison of pricingComparisons) {
      expect(comparison.rows.map((row) => row.label)).toEqual([
        "Monthly floor",
        "Five-person team",
        "Agent usage",
        "When agents are idle",
        "Seat model",
      ])
      expect(comparison.sources.length).toBeGreaterThan(0)
    }
  })

  it("uses the published monthly cost for a five-person team", () => {
    const fivePersonCosts = Object.fromEntries(
      pricingComparisons.map((comparison) => [
        comparison.id,
        comparison.rows.find((row) => row.label === "Five-person team")
          ?.competitor,
      ])
    )

    expect(fivePersonCosts).toEqual({
      claude: "$125 / month",
      chatgpt: "$125 / month",
      gumloop: "$37 / month",
      notion: "$100 / month + credits",
    })
  })
})
