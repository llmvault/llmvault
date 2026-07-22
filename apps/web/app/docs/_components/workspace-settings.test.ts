import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { WorkspaceSettings } from "./workspace-settings"

describe("WorkspaceSettings", () => {
  it("separates shared context, team secrets, and preview ports", () => {
    const html = renderToString(React.createElement(WorkspaceSettings))

    expect(html).toContain("Give every agent a common starting point")
    expect(html).toContain("does not crawl or index the site")
    expect(html).toContain("Keep secret values with the team")
    expect(html).toContain("Expose only the preview ports you use")
    expect(html).toContain("Port 7080")
    expect(html).not.toContain("<img")
  })
})
