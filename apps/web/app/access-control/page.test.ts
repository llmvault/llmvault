import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import AccessControlPage from "./page"

describe("AccessControlPage", () => {
  it("renders a focused access-control landing page", () => {
    const html = renderToString(React.createElement(AccessControlPage))

    expect(html).toContain("Govern every Hivy agent from one workspace.")
    expect(html).toContain("Set access before work starts.")
    expect(html).toContain("Team access settings in Hivy, placeholder")
    expect(html).toContain("Control who can do what, team by team.")
    expect(html).toContain("Workspace roles")
    expect(html).toContain("Teams set the boundary")
    expect(html).toContain("Resources follow the team")
    expect(html).toContain(
      "Give agents what the job needs. Nothing outside it."
    )
    expect(html).toContain("Connections")
    expect(html).toContain("Knowledge sources")
    expect(html).toContain("Skills")
    expect(html).toContain(
      'data-testid="resource-grant-highlights" class="pt-10"'
    )
    expect(html).toContain(
      "grid border-x border-b border-border md:grid-cols-3 border-t"
    )
    expect(html).toContain('aria-label="Choose resources for the Product team"')
    expect(html).toContain('aria-label="github"')
    expect(html).toContain('aria-label="slack"')
    expect(html).toContain('aria-label="notion"')
    expect(html).toContain('aria-label="linear"')
    expect(html).toContain("Set the boundary before your first agent runs.")
    expect(html).toContain("Watch a 2min demo")
    expect(html).not.toContain("People get a role. Work gets a team.")
    expect(html).not.toContain("Route one provider resource")
    expect(html).not.toContain("Build access in the same order")
    expect(html).not.toContain("Atlas workspace")
    expect(html).not.toContain("Review the full access path")
    expect(html).toContain('href="/auth/signup"')
    expect(html).toContain("marketing-link-scope")
    expect(html.match(/>Start for free</g)).toHaveLength(3)
    expect(html).not.toContain("Create free workspace")
    expect(html).not.toContain(String.fromCharCode(8212))
  })
})
