import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import HomePage from "./page"

describe("HomePage", () => {
  it("renders the complete landing-page entry point", () => {
    const html = renderToString(React.createElement(HomePage))

    expect(html).toContain("Hivy is the AI workspace")
    expect(html).toContain("Describe it. Hivy builds it.")
    expect(html).toContain("Everything your agents need, in one workspace.")
    expect(html).toContain("Build your first agent today.")
    expect(html).toContain(
      "Main Hivy workspace product screenshot, placeholder"
    )
    expect(html).toContain("Aster placeholder logo")
    expect(html).toContain("Current placeholder logo")
    expect(html).not.toContain("Global work done by Hivy")
    expect(html).toContain('href="/auth/signup"')
  })
})
