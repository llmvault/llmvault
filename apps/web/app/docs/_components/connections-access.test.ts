import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { ConnectionsAccess } from "./connections-access"

describe("ConnectionsAccess", () => {
  it("explains the complete access chain without real media", () => {
    const html = renderToString(React.createElement(ConnectionsAccess))

    expect(html).toContain("Follow the four layers of access")
    expect(html).toContain("A connection is workspace-level")
    expect(html).toContain("Teams decide which agents receive a plugin")
    expect(html).toContain("Narrow one agent")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).toContain("/docs/plugins-and-connections/connect-tools")
    expect(html).toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
