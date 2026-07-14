import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { Channels } from "./channels"

describe("Channels", () => {
  it("explains channel scope, memory, sessions, and planned media", () => {
    const html = renderToString(React.createElement(Channels))

    expect(html).toContain("Set the scope before work starts")
    expect(html).toContain("Choose the team and category")
    expect(html).toContain("Start the first session")
    expect(html).toContain("Create channel form")
    expect(html).toContain("Create a channel and run its first session")
    expect(html).toContain("/docs/workspace-and-access/slack-channels")
    expect(html).not.toContain(".jpg")
    expect(html).not.toContain("conversation")
  })
})
