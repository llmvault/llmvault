import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { SlackChannels } from "./slack-channels"

describe("SlackChannels", () => {
  it("explains setup, team scope, session continuity, and Slack limits", () => {
    const html = renderToString(React.createElement(SlackChannels))

    expect(html).toContain("Install and connect Slack")
    expect(html).toContain("Enable Slack for the team")
    expect(html).toContain("Link the Slack channel")
    expect(html).toContain("Start a session from Slack")
    expect(html).toContain("One Slack thread, one Hivy session")
    expect(html).toContain("A linked Slack thread and its Hivy session")
    expect(html).toContain("Connect Slack and complete a session")
    expect(html).toContain("Direct messages and group direct messages")
    expect(html).toContain("/w/connections/slack")
    expect(html).not.toContain(".jpg")
    expect(html).not.toContain("conversation")
  })
})
