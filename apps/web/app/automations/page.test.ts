import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import AutomationsPage from "./page"

describe("AutomationsPage", () => {
  it("renders a focused Automations landing page", () => {
    const html = renderToString(React.createElement(AutomationsPage))

    expect(html).toContain("Run Hivy agents when the work arrives.")
    expect(html).toContain("No one has to press go.")
    expect(html).toContain(
      "Automation trigger and completed agent session, placeholder"
    )
    expect(html).toContain("Put any agent on the signal that starts its job.")
    expect(html).toContain("Pull requests")
    expect(html).toContain("Slack reactions")
    expect(html).toContain('aria-label="github"')
    expect(html).toContain('aria-label="slack"')
    expect(html).toContain("Schedules")
    expect(html).toContain("Webhooks")
    expect(html).toContain('aria-label="Choose what starts the automation"')
    expect(html).toContain(
      "Check the diff and CI results. Post only actionable review comments."
    )
    expect(html).toContain('data-testid="automation-trigger-grid"')
    expect(html).toContain("lg:grid-cols-[1fr_2fr]")
    expect(html).toContain("See what ran, what it cost, and what happened.")
    expect(html).toContain('data-testid="automation-history-grid"')
    expect(html).toContain("The OAuth callback test needs a fix.")
    expect(html).toContain("Keep the run history")
    expect(html).toContain("Put one repeatable job on autopilot.")
    expect(html).toContain("Watch a 2min demo")
    expect(html).not.toContain("Let the event carry the request.")
    expect(html).not.toContain("Put the work on the clock, not the reminder.")
    expect(html).not.toContain("Decide who owns the run before it starts.")
    expect(html).toContain('href="/auth/signup"')
    expect(html).toContain("marketing-link-scope")
    expect(html.match(/>Start for free</g)).toHaveLength(3)
    expect(html).not.toContain("Create free workspace")
    expect(html).not.toContain('href="/docs/automations/overview"')
  })
})
