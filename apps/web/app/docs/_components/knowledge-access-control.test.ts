import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { KnowledgeAccessControl } from "./knowledge-access-control"

describe("KnowledgeAccessControl", () => {
  it("explains content scope, team grants, health, and planned media", () => {
    const html = renderToString(React.createElement(KnowledgeAccessControl))

    expect(html).toContain("Access follows the team")
    expect(html).toContain("Treat team grants as the access switch")
    expect(html).toContain("Source scope and team grants")
    expect(html).toContain("Change knowledge access safely")
    expect(html).toContain("/w/settings/knowledge")
    expect(html).toContain("/docs/workspace-and-access/teams")
    expect(html).not.toContain(".jpg")
    expect(html).not.toContain("conversation")
  })
})
