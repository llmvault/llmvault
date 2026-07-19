import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AgentSessions } from "./agent-sessions"

describe("AgentSessions", () => {
  it("teaches the complete team and agent-scoped session workflow", () => {
    const html = renderToString(React.createElement(AgentSessions))

    expect(html).toContain("Start with the right scope")
    expect(html).toContain("Choose the agent before the model")
    expect(html).toContain("Review cost as the session grows")
    expect(html).toContain("Share, rename, or archive the session")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
