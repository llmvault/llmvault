import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { SkillsHeader } from "./_skills-page"

describe("SkillsHeader", () => {
  it("starts skill creation in a new chat", () => {
    const html = renderToString(React.createElement(SkillsHeader))

    expect(html).toContain("Add skill")
    expect(html).toContain('href="/w"')
  })
})
