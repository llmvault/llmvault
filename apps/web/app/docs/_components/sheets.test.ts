import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { Sheets } from "./sheets"

describe("Sheets", () => {
  it("explains durable team data and the complete Sheets workflow", () => {
    const html = renderToString(React.createElement(Sheets))

    expect(html).toContain("information that should outlive one session")
    expect(html).toContain("Use a Sheet when the information should last")
    expect(html).toContain("Create it yourself or ask an agent")
    expect(html).toContain("Bring data in and take it out")
    expect(html).toContain("Sheet access follows the team")
    expect(html).toContain("Turn a Sheet into an app")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
