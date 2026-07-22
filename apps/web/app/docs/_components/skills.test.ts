import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { Skills } from "./skills"

describe("Skills", () => {
  it("explains skill contents, scope, and lifecycle", () => {
    const html = renderToString(React.createElement(Skills))

    expect(html).toContain("Write a skill an agent can recognize")
    expect(html).toContain("Choose its scope when you create it")
    expect(html).toContain("Let tools and skills do different jobs")
    expect(html).toContain("Archive skills agents should stop using")
    expect(html).toContain("Settings &gt; Skills")
    expect(html).not.toContain("<img")
  })
})
