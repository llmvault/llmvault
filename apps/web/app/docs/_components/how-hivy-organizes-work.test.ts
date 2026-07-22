import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { HowHivyOrganizesWork } from "./how-hivy-organizes-work"

describe("HowHivyOrganizesWork", () => {
  it("explains the hierarchy and requests a focused workspace capture", () => {
    const html = renderToString(React.createElement(HowHivyOrganizesWork))

    expect(html).toContain("Where a session lives")
    expect(html).toContain("Agents belong to teams")
    expect(html).toContain("See where work lives")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).toContain(">Session<")
    expect(html).not.toContain("workspace-overview-light.jpg")
    expect(html).not.toContain("DocsImage")
    expect(html).not.toContain("<img")
  })
})
