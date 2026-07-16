import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { ConnectTools } from "./connect-tools"

describe("ConnectTools", () => {
  it("documents the admin setup and maintenance path with media placeholders", () => {
    const html = renderToString(React.createElement(ConnectTools))

    expect(html).toContain("Connect a tool in five steps")
    expect(html).toContain("Start with the provider")
    expect(html).toContain("Connect databases with a read boundary")
    expect(html).toContain("Enable the connection for the owning team")
    expect(html).toContain("Reconnect, remove, or disconnect")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).toContain("/w/connections")
    expect(html).toContain("/w/settings/teams")
    expect(html).toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
