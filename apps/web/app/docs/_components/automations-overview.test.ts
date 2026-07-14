import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AutomationsOverview } from "./automations-overview"

describe("AutomationsOverview", () => {
  it("explains each automation entry point and its session boundary", () => {
    const html = renderToString(React.createElement(AutomationsOverview))

    expect(html).toContain("Choose how the work starts")
    expect(html).toContain("A connected app event")
    expect(html).toContain("A recurring schedule")
    expect(html).toContain("An HTTP request")
    expect(html).toContain("Each run becomes a session")
    expect(html).toContain("Video placeholder")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
