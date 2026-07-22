import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { McpServers } from "./mcp-servers"

describe("McpServers", () => {
  it("documents remote transports, identities, grants, and health", () => {
    const html = renderToString(React.createElement(McpServers))

    expect(html).toContain("Streamable HTTP")
    expect(html).toContain("Personal server")
    expect(html).toContain("Organization server")
    expect(html).toContain("OAuth client credentials")
    expect(html).toContain("Grant the smallest useful scope")
    expect(html).toContain("Re-test after endpoint or credential changes")
    expect(html).toContain("does not run a local stdio server")
    expect(html).not.toContain("<img")
  })
})
