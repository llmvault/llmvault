import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AutomationRuns } from "./automation-runs"

describe("AutomationRuns", () => {
  it("documents how to find and troubleshoot automated sessions", () => {
    const html = renderToString(React.createElement(AutomationRuns))

    expect(html).toContain("Find the run in its channel")
    expect(html).toContain("Start with four checks")
    expect(html).toContain("If a connected event does not run")
    expect(html).toContain("If a schedule does not run")
    expect(html).toContain("If Hivy rejects an HTTP webhook")
    expect(html).toContain("401:")
    expect(html).toContain("404:")
    expect(html).toContain("413:")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
