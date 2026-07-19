import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import HomeVariantOnePage from "./variant-1/page"
import HomeVariantTwoPage from "./variant-2/page"
import HomeVariantThreePage from "./variant-3/page"
import HomeVariantFourPage from "./variant-4/page"
import HomeVariantFivePage from "./variant-5/page"
import HomeVariantSixPage from "./variant-6/page"

const sharedHeroCopy = "Productive ai agents for your entire team."
const sharedHeroPlaceholder =
  "Main Hivy workspace product screenshot, placeholder"

describe("landing page variants", () => {
  it.each([
    ["workspace canvas", HomeVariantOnePage, "Every surface your agents need"],
    ["sticky chapters", HomeVariantTwoPage, "Four focused chapters"],
    [
      "lifecycle timeline",
      HomeVariantThreePage,
      "Follow the work from first input",
    ],
    ["editorial ledger", HomeVariantFourPage, "A precise account"],
    ["workspace layers", HomeVariantFivePage, "A full stack for agent work"],
    ["control room", HomeVariantSixPage, "See the entire agent system"],
  ])(
    "renders the %s entry point with the shared hero",
    (_, Page, layoutCopy) => {
      const html = renderToString(React.createElement(Page))

      expect(html.match(/<section/g)).toHaveLength(9)
      expect(html).toContain(sharedHeroCopy)
      expect(html).toContain(sharedHeroPlaceholder)
      expect(html).toContain(layoutCopy)
      expect(html).toContain("Describe it. Hivy builds it.")
      expect(html).toContain("Connect the tools your work runs on.")
      expect(html).toContain("Give Hivy data it can reason over.")
      expect(html).toContain("Build agents that solve real problems.")
      expect(html).toContain("Watch every run, end to end.")
      expect(html).toContain("Everything your agents need")
      expect(html).toContain("Build your first agent today.")
    }
  )
})
