import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import KnowledgePage from "./page"

describe("KnowledgePage", () => {
  it("renders a focused knowledge landing page", () => {
    const html = renderToString(React.createElement(KnowledgePage))

    expect(html).toContain("Give Hivy agents memory of your company’s work.")
    expect(html).toContain("Get every answer with its source.")
    expect(html).toContain(
      "Connected sources and an answer with citations, placeholder"
    )
    expect(html).toContain("Give agents company context they can actually use.")
    expect(html).toContain('aria-label="Choose a knowledge source"')
    expect(html).toContain("GitHub")
    expect(html).toContain("Slack")
    expect(html).toContain("Notion")
    expect(html).toContain("Linear")
    expect(html).toContain("Website")
    expect(html).toContain('aria-label="github"')
    expect(html).toContain('aria-label="slack"')
    expect(html).toContain('aria-label="notion"')
    expect(html).toContain('aria-label="linear"')
    expect(html).toContain('aria-label="chrome"')
    expect(html).toContain("Answers stay grounded as your sources change.")
    expect(html).toContain("Changed sources re-index")
    expect(html).toContain("Evidence")
    expect(html).toContain("Audit export decision")
    expect(html).toContain("Give your first agent company memory.")
    expect(html).toContain("Create free workspace")
    expect(html).not.toContain(
      "Know what’s ready before an agent relies on it."
    )
    expect(html).not.toContain(
      "Open the index instead of trusting a green dot."
    )
    expect(html).not.toContain("Company knowledge changes.")
    expect(html).toContain('href="/auth/signup"')
    expect(html).toContain('href="#source-setup"')
    expect(html).toContain("marketing-link-scope")
  })
})
