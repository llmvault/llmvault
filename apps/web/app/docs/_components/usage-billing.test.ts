import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { UsageBilling } from "./usage-billing"

describe("UsageBilling", () => {
  it("explains usage visibility and the subscription lifecycle", () => {
    const html = renderToString(React.createElement(UsageBilling))

    expect(html).toContain("Know what a credit buys")
    expect(html).toContain("Find the workspace total")
    expect(html).toContain("Read a session price")
    expect(html).toContain("Plan changes use different clocks")
    expect(html).toContain("Who gets billing access")
    expect(html).toContain("Usage &amp; billing")
    expect(html).toContain("View plans")
    expect(html).toContain("Cancel plan")
    expect(html).toContain("Resume plan")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).toContain("/w/settings/billing")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("Gumloop")
    expect(html).not.toContain("—")
  })
})
