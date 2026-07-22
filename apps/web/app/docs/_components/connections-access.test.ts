import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { ConnectionsAccess } from "./connections-access"

describe("ConnectionsAccess", () => {
  it("explains the complete access chain without real media", () => {
    const html = renderToString(React.createElement(ConnectionsAccess))

    expect(html).toContain("Follow the four layers of access")
    expect(html).toContain("Connections are workspace-level instances")
    expect(html).toContain("Team grants open access; agent switches narrow it")
    expect(html).toContain("Skills are first-class resources")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).toContain("/docs/connections-and-skills/connect-tools")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
