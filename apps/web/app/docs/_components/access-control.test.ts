import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AccessControl } from "./access-control"

describe("AccessControl", () => {
  it("explains workspace roles, team scope, and invitations", () => {
    const html = renderToString(React.createElement(AccessControl))

    expect(html).toContain("Access has two layers")
    expect(html).toContain("Workspace role")
    expect(html).toContain("Team membership")
    expect(html).toContain("Invite people into the right scope")
    expect(html).toContain("Members, roles, and pending invitations")
    expect(html).toContain("Invite a member with the right access")
    expect(html).toContain("Owner")
    expect(html).toContain("Admin")
    expect(html).toContain("Member")
    expect(html).toContain("/docs/workspace-and-access/teams")
    expect(html).not.toContain(".jpg")
    expect(html).not.toContain("conversation")
  })
})
