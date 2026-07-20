import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import HomePage from "./page"

describe("HomePage", () => {
  it("renders the complete landing-page entry point", () => {
    const html = renderToString(React.createElement(HomePage))

    expect(html).toContain("Productive ai agents for your entire team.")
    expect(html).toContain("With no monthly subscriptions")
    expect(html).toContain("Describe it. Hivy builds it.")
    expect(html).toContain("Everything your agents need, in one workspace.")
    expect(html).toContain("Build your first agent today.")
    expect(html).toContain(
      "Main Hivy workspace product screenshot, placeholder"
    )
    expect(html).toContain("Aster placeholder logo")
    expect(html).toContain("Current placeholder logo")
    expect(html).not.toContain("Global work done by Hivy")
    expect(html).not.toContain(">Solutions<")
    expect(html).toContain("Knowledge base")
    expect(html).toContain("Agents")
    expect(html).toContain("Drive")
    expect(html).toContain("Automations")
    expect(html).toContain("Sheets")
    expect(html).toContain("Access control")
    expect(html).toContain('href="/knowledge"')
    expect(html).toContain('href="/agents"')
    expect(html).toContain('href="/drive"')
    expect(html).toContain('href="/automations"')
    expect(html).toContain('href="/sheets"')
    expect(html).toContain('href="/access-control"')
    expect(html).toContain("Docs")
    expect(html).toContain("Blog")
    expect(html).toContain("Changelog")
    expect(html).toContain("Models")
    expect(html).toContain("0.1k")
    expect(html).toContain("Watch a 2min demo")
    expect(html).toContain('aria-label="github"')
    expect(html).toContain("Start for free")
    expect(html).not.toContain("Sign up")
    expect(html).not.toContain("Request a demo")
    expect(html).toContain('href="/auth/signup"')
    expect(html).toContain("marketing-link-scope")
    expect(html).toContain("marketing-menu-link")
  })
})
