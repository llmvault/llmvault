import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { CollectionState } from "./collection-state"

describe("CollectionState", () => {
  it("explains the empty state and offers its next action", () => {
    const html = renderToString(
      React.createElement(CollectionState, {
        icon: "table",
        title: "No sheets yet",
        description:
          "Ask an agent to collect or organise data in a sheet. It will appear here when it is ready.",
        action: {
          label: "Start a chat",
          icon: "square-pen",
          variant: "primary",
          onPress: () => {},
        },
      })
    )

    expect(html).toContain("No sheets yet")
    expect(html).toContain("Ask an agent to collect or organise data")
    expect(html).toContain("Start a chat")
    expect(html).toContain('role="status"')
  })
})
