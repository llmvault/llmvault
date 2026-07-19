import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { MemoriesAndRules } from "./memories-and-rules"

describe("MemoriesAndRules", () => {
  it("explains agent memory, rules, controls, and planned media", () => {
    const html = renderToString(React.createElement(MemoriesAndRules))

    expect(html).toContain("Use memory for context and rules for control")
    expect(html).toContain("How agent memory builds")
    expect(html).toContain("Memory mission")
    expect(html).toContain("Review and correct what an agent remembers")
    expect(html).toContain("/w/settings/memories")
    expect(html).toContain("/w/agents")
    expect(html).not.toContain(".jpg")
    expect(html).not.toContain("conversation")
  })
})
