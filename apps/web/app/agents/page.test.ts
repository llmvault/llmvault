import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import AgentsPage from "./page"

describe("AgentsPage", () => {
  it("renders a focused agents landing page", () => {
    const html = renderToString(React.createElement(AgentsPage))

    expect(html).toContain("Build a Hivy agent for every repeatable job.")
    expect(html).toContain("Give it tools, context, and limits.")
    expect(html).toContain("Install a specialist. Or define the role yourself.")
    expect(html).toContain("Run real work without losing control of the agent.")
    expect(html).toContain("Every session inspectable")
    expect(html).toContain("Build the first agent your team can keep using.")
    expect(html).toContain('aria-label="Browse agents by team"')
    expect(html).toContain("Operations")
    expect(html).toContain("Support")
    expect(html).toContain("Create free workspace")
    expect(html).not.toContain("Pick the model. Limit the tools.")
    expect(html).not.toContain("One agent can call in the right specialist.")
    expect(html).not.toContain("See every step behind the answer.")
    expect(html).toContain("marketing-link-scope")
    expect(html).toContain('href="/auth/signup"')
    expect(html).toContain('href="#catalog"')
  })
})
