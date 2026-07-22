import React from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import AgentsLayout from "./layout"

describe("AgentsLayout", () => {
  it("keeps the background pinned to the workspace viewport while scrolling", () => {
    const html = renderToStaticMarkup(
      React.createElement(
        AgentsLayout,
        null,
        React.createElement("div", null, "Agent settings")
      )
    )

    expect(html).toContain(
      'class="absolute inset-0 overflow-y-auto bg-background text-foreground"'
    )
  })
})
