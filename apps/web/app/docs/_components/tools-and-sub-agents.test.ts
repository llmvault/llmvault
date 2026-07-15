import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { ToolsAndSubAgents } from "./tools-and-sub-agents"

describe("ToolsAndSubAgents", () => {
  it("explains independent capabilities, delegation, and session sandboxes", () => {
    const html = renderToString(React.createElement(ToolsAndSubAgents))

    expect(html).toContain("Tools control what the agent can do")
    expect(html).toContain("Grant its tools independently")
    expect(html).toContain("Sandboxes isolate execution by session")
    expect(html).toContain("8 CPU, 16 GB RAM, 60 GB disk")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
