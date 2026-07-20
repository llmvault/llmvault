import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import TagPage from "./page"

describe("TagPage", () => {
  it("renders the Slack tagging landing page and its primary journeys", () => {
    const html = renderToString(React.createElement(TagPage))

    expect(html).toContain("Put Hivy to work from Slack.")
    expect(html).toContain("Tag it in. Keep the thread moving.")
    expect(html).toContain("Hivy Tag in Slack product screenshot, placeholder")
    expect(html).toContain("A Slack message becomes a working session.")
    expect(html).toContain("Put a channel on watch.")
    expect(html).toContain("One emoji can start the work.")
    expect(html).toContain("Give every channel the right agent.")
    expect(html).toContain("The next tag starts with what the agent learned.")
    expect(html).toContain("Hivy answers where you put it.")
    expect(html).toContain("Your next agent request can start in Slack.")
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
    expect(html).not.toContain("@Hivy")
    expect(html).toContain("@hivy")
  })
})
