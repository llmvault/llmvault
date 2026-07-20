import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import TagPage from "./page"

describe("TagPage", () => {
  it("renders the Slack tagging landing page and its primary journeys", () => {
    const html = renderToString(React.createElement(TagPage))

    expect(html).toContain("Bring @hivy into the conversation.")
    expect(html).toContain("Keep the work in Slack.")
    expect(html).toContain("Hivy working inside a Slack channel, placeholder")
    expect(html).toContain("Stop carrying Slack requests into another app.")
    expect(html).toContain("Some work shouldn’t wait for a mention.")
    expect(html).toContain("Turn a reaction into a handoff.")
    expect(html).toContain(
      "Match each channel with an agent that knows the job."
    )
    expect(html).toContain("Tomorrow’s answer remembers today’s work.")
    expect(html).toContain("How teams use Hivy Tag")
    expect(html).toContain(
      "Hivy agents in Slack can return the work each team needs, right in the thread."
    )
    expect(html).toContain("Customer support")
    expect(html).toContain("Product")
    expect(html).toContain("Finance")
    expect(html).toContain("Revenue")
    expect(html).toContain("The regression starts in workspace_lookup.")
    expect(html).toContain("Suggested fix")
    expect(html).toContain('aria-label="Teams using Hivy Tag"')
    expect(html).not.toContain(
      "Hand off the work without writing a prompt spec."
    )
    expect(html).toContain("@hivy only works where you’ve assigned it.")
    expect(html).toContain("Your next handoff can stay in Slack.")
    expect(html).toContain('href="/auth/signup"')
    expect(html).toContain('href="#how-it-works"')
    expect(html).toContain("marketing-link-scope")
    expect(html).toContain("hivy is watching #customer-voice")
    expect(html).toContain("Reaction trigger")
    expect(html).toContain("Slack workspace showing #product-support")
    expect(html).toContain("Maya Chen profile photo")
    expect(html).toContain("Leah Brooks profile photo")
    expect(html).toContain('src="/logo.png"')
    expect(html).toContain("hivy is typing")
    expect(html).toContain("Reply…")
    expect(html).toContain("Connect Slack")
    expect(html).toContain('aria-label="slack"')
    expect(html).toContain("Read the setup guide")
    expect(html).not.toContain("@Hivy")
    expect(html).toContain("@hivy")
  })
})
