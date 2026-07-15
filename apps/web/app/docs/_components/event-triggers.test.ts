import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { EventTriggers } from "./event-triggers"

describe("EventTriggers", () => {
  it("documents supported events, scope, instructions, and lifecycle", () => {
    const html = renderToString(React.createElement(EventTriggers))

    expect(html).toContain("Start from a supported event")
    expect(html).toContain("Slack reaction")
    expect(html).toContain("GitHub mention")
    expect(html).toContain("Write instructions for the event")
    expect(html).toContain("Disable a trigger")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
