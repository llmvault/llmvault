import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AgentDrive } from "./agent-drive"

describe("AgentDrive", () => {
  it("explains durable agent files and their boundary", () => {
    const html = renderToString(React.createElement(AgentDrive))

    expect(html).toContain("Know which files will last")
    expect(html).toContain("Search, download, then save")
    expect(html).toContain("Attach images to a request")
    expect(html).toContain("Drive is agent-scoped")
    expect(html).toContain("/docs/sheets-and-apps/sheets")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).not.toContain("<img")
  })
})
