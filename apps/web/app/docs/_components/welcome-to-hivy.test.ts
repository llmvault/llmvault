import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { WelcomeToHivy } from "./welcome-to-hivy"

describe("WelcomeToHivy", () => {
  it("introduces agents and automations with actionable media briefs", () => {
    const html = renderToString(React.createElement(WelcomeToHivy))

    expect(html).toContain("Agents and automations")
    expect(html).toContain("Start with a real task")
    expect(html).toContain("Give each team the agents it needs")
    expect(html).toContain("Record a 60-second overview")
    expect(html).toContain("Capture the new-session composer at 100% zoom")
    expect(html).toContain("/docs/run-your-first-agent")
  })
})
