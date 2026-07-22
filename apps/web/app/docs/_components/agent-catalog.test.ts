import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AgentCatalog } from "./agent-catalog"

describe("AgentCatalog", () => {
  it("explains team-scoped catalog installation without shipping image assets", () => {
    const html = renderToString(React.createElement(AgentCatalog))

    expect(html).toContain("Choose and install an agent")
    expect(html).toContain("Each team gets its own agent")
    expect(html).toContain("Hivy checks required connections first")
    expect(html).toContain("required are locked")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).toContain("/docs/agents/configure-an-agent")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
