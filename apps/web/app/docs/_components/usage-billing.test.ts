import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { UsageBilling } from "./usage-billing"

describe("UsageBilling", () => {
  it("explains usage visibility and one-time credit purchases", () => {
    const html = renderToString(React.createElement(UsageBilling))

    expect(html).toContain("Know what a credit buys")
    expect(html).toContain("Find the workspace total")
    expect(html).toContain("Read a session price")
    expect(html).toContain("Choose the currency once")
    expect(html).toContain("Who gets billing access")
    expect(html).toContain("Usage &amp; billing")
    expect(html).toContain("Buy the credits you need")
    expect(html).toContain("one-time deposits")
    expect(html).toContain("1,000 welcome credits")
    expect(html).toContain("12% deposit fee")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).toContain("/w/settings/billing")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("Gumloop")
    expect(html).not.toContain("—")
  })
})
