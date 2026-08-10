import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"

import { SessionSpendPill } from "./session-spend-pill"

describe("SessionSpendPill", () => {
  it("shows model and sandbox spend in credits and USD", () => {
    const html = renderToStaticMarkup(
      createElement(SessionSpendPill, {
        usage: {
          costUsd: 0.004,
          credits: 4.5,
          modelCostUsd: 0.0035,
          modelCredits: 4,
          sandboxCostUsd: 0.0005,
          sandboxCredits: 0.5,
          sandboxVCPUSeconds: 30,
          updatedAt: 0,
        },
      })
    )

    expect(html).toContain("Model")
    expect(html).toContain("4 cr")
    expect(html).toContain("$0.0035")
    expect(html).toContain("Sandbox")
    expect(html).toContain("0.5 cr")
    expect(html).toContain("$0.0005")
    expect(html).toContain("Sandbox: 0.5 credits, $0.0005")
  })
})
