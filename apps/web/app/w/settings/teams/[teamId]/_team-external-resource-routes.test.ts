import { describe, expect, it } from "vitest"
import {
  slackChannelsForRouting,
  slackConnectionsForRouting,
  slackRouteSummary,
} from "./_team-external-resource-routes"

describe("slackConnectionsForRouting", () => {
  it("returns only granted Slack integration connections", () => {
    expect(
      slackConnectionsForRouting([
        {
          id: "slack-2",
          kind: "integration",
          name: "Support workspace",
          provider: "slack",
        },
        {
          id: "github-1",
          kind: "integration",
          name: "GitHub",
          provider: "github-app",
        },
        {
          id: "slack-db",
          kind: "database",
          name: "Slack archive",
          provider: "slack",
        },
        {
          id: "slack-1",
          kind: "integration",
          name: "Engineering workspace",
          provider: "slack",
        },
      ]).map((connection) => connection.id)
    ).toEqual(["slack-1", "slack-2"])
  })
})

describe("slackChannelsForRouting", () => {
  it("keeps every unique Slack channel and orders them by name", () => {
    expect(
      slackChannelsForRouting([
        { id: "C2", name: "support", type: "slack_channel" },
        { id: "R1", name: "repository", type: "repository" },
        { id: "C1", name: "engineering", type: "slack_channel" },
        { id: "C2", name: "support", type: "slack_channel" },
      ]).map((channel) => channel.id)
    ).toEqual(["C1", "C2"])
  })
})

describe("slackRouteSummary", () => {
  it("names the selected workspace, channel, and agent", () => {
    expect(slackRouteSummary("Hivy", "#support", "Triage agent")).toBe(
      "Any @hivy pings in Slack workspace Hivy, channel #support, will be routed to Triage agent."
    )
  })
})
