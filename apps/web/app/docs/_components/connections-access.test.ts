import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { ConnectionsAccess } from "./connections-access"

describe("ConnectionsAccess", () => {
  it("explains the complete access chain without real media", () => {
    const html = renderToString(React.createElement(ConnectionsAccess))

    expect(html).toContain("Follow the three layers of access")
    expect(html).toContain("Connections are workspace-level instances")
    expect(html).toContain("Teams decide which agents receive a connection")
    expect(html).toContain("Skills are first-class resources")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).toContain("/docs/connections-and-skills/connect-tools")
    expect(html).toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
