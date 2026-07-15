import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { GeneratedWork } from "./generated-work"

describe("GeneratedWork", () => {
  it("explains every generated-work view and durable artifact type", () => {
    const html = renderToString(React.createElement(GeneratedWork))

    expect(html).toContain("Open the work beside the session")
    expect(html).toContain("Inspect files and code changes")
    expect(html).toContain("Keep long-term information in Sheets")
    expect(html).toContain("Ricky, App builder")
    expect(html).toContain("Inspect delegated work separately")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
