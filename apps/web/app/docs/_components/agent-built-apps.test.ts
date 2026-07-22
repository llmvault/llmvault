import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AgentBuiltApps } from "./agent-built-apps"

describe("AgentBuiltApps", () => {
  it("documents the Ricky workflow without embedding final media", () => {
    const html = renderToString(React.createElement(AgentBuiltApps))

    expect(html).toContain("Ricky - App builder")
    expect(html).toContain("one Hivy Sheet")
    expect(html).toContain("Review the preview before you publish")
    expect(html).toContain("Approve each production deployment")
    expect(html).toContain("App access follows its team")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
