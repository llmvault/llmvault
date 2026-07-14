import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { Teams } from "./teams"

describe("Teams", () => {
  it("explains team scope, setup, and planned media", () => {
    const html = renderToString(React.createElement(Teams))

    expect(html).toContain("What a team controls")
    expect(html).toContain("Create a team for durable ownership")
    expect(html).toContain("Team details and resource access")
    expect(html).toContain("Create a team and set its access")
    expect(html).toContain("/w/settings/teams")
    expect(html).toContain("/docs/agents/configure-an-agent")
    expect(html).not.toContain(".jpg")
    expect(html).not.toContain("conversation")
  })
})
