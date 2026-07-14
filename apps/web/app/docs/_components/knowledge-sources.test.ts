import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { KnowledgeSources } from "./knowledge-sources"

describe("KnowledgeSources", () => {
  it("explains source setup, indexing, and planned media", () => {
    const html = renderToString(React.createElement(KnowledgeSources))

    expect(html).toContain("Knowledge is context, not a live tool")
    expect(html).toContain("Add a source in four steps")
    expect(html).toContain("Select a focused source scope")
    expect(html).toContain("Add your first knowledge source")
    expect(html).toContain("/w/settings/knowledge/new")
    expect(html).toContain("/docs/agents/sessions")
    expect(html).not.toContain(".jpg")
    expect(html).not.toContain("conversation")
  })
})
