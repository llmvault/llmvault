import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { AgentInboxView } from "./_agent-inbox-section"

describe("AgentInboxView", () => {
  it("offers to provision an inbox when the agent does not have one", () => {
    const html = renderToString(
      React.createElement(AgentInboxView, {
        inbox: { available: false, message_count: 0 },
        onProvision: () => {},
      })
    )

    expect(html).toContain("Give this agent an email address")
    expect(html).toContain("Add inbox")
  })

  it("shows a copyable active address and received message count", () => {
    const html = renderToString(
      React.createElement(AgentInboxView, {
        inbox: {
          available: true,
          email: "operator-abcd1234@agents.example.test",
          message_count: 12,
        },
        onProvision: () => {},
      })
    )

    expect(html).toContain("operator-abcd1234@agents.example.test")
    expect(html).toContain("Inbox active")
    expect(html).toContain("12 messages received")
    expect(html).toContain("Copy address")
  })

  it("turns a zero count into a useful empty state", () => {
    const html = renderToString(
      React.createElement(AgentInboxView, {
        inbox: {
          available: true,
          email: "operator-abcd1234@agents.example.test",
          message_count: 0,
        },
        onProvision: () => {},
      })
    )

    expect(html).toContain("No messages received yet")
  })
})
