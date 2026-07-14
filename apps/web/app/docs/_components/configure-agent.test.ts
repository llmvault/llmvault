import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { ConfigureAgent } from "./configure-agent"

describe("ConfigureAgent", () => {
  it("explains a cost-aware custom agent setup with media placeholders", () => {
    const html = renderToString(React.createElement(ConfigureAgent))

    expect(html).toContain("Make three decisions first")
    expect(html).toContain("DeepSeek V4 Flash")
    expect(html).toContain("Give it only the tools it needs")
    expect(html).toContain("Set the runtime for the workload")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).toContain("/w/agents/new")
    expect(html).toContain("/docs/agents/tools-and-sub-agents")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
