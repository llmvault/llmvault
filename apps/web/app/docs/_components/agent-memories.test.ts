import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AgentMemories } from "./agent-memories"

describe("AgentMemories", () => {
  it("explains learned memory and the controls that exist in the agent UI", () => {
    const html = renderToString(React.createElement(AgentMemories))

    expect(html).toContain("How agent memory builds")
    expect(html).toContain("Review one agent at a time")
    expect(html).toContain("Edit wording without inventing a new fact")
    expect(html).toContain("Forget context that should stop applying")
    expect(html).toContain("Memory belongs to the agent")
    expect(html).toContain("/w/agents")
    expect(html).not.toContain("Memory mission")
    expect(html).not.toContain("/w/settings/memories")
    expect(html).not.toContain("<img")
  })
})
