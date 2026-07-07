import { describe, expect, it } from "vitest"
import { type ChannelAgent } from "@/lib/api/channel-agents"
import { channelAgentsErrorMessage, unassignedAgents } from "./_agents-tab-lib"

function agent(id: string, name = id): ChannelAgent {
  return { id, name }
}

describe("unassignedAgents", () => {
  it("returns org agents that are not already assigned", () => {
    const org = [agent("a"), agent("b"), agent("c")]
    const assigned = [agent("b")]
    expect(unassignedAgents(org, assigned).map((entry) => entry.id)).toEqual([
      "a",
      "c",
    ])
  })

  it("returns an empty list when every org agent is assigned", () => {
    const org = [agent("a"), agent("b")]
    expect(unassignedAgents(org, [agent("a"), agent("b")])).toEqual([])
  })

  it("drops agents without an id", () => {
    const org = [agent("a"), { name: "ghost" }]
    expect(unassignedAgents(org, []).map((entry) => entry.id)).toEqual(["a"])
  })
})

describe("channelAgentsErrorMessage", () => {
  it("surfaces friendly copy for the membership (403) server messages", () => {
    // openapi-react-query surfaces only the error body: `{ error: string }`.
    expect(
      channelAgentsErrorMessage(
        { error: "join the channel before managing its agents" },
        "fallback"
      )
    ).toBe(
      "You're not a member of this channel, so you can't change its agents."
    )
    expect(channelAgentsErrorMessage({ error: "forbidden" }, "fallback")).toBe(
      "You're not a member of this channel, so you can't change its agents."
    )
  })

  it("uses the descriptive server message for other known failures", () => {
    expect(
      channelAgentsErrorMessage(
        { error: "agent is already assigned to this channel" },
        "fallback"
      )
    ).toBe("agent is already assigned to this channel")
    expect(
      channelAgentsErrorMessage({ error: "agent is archived" }, "fallback")
    ).toBe("agent is archived")
  })

  it("falls back to the generic message when the error has none", () => {
    expect(channelAgentsErrorMessage({}, "oops")).toBe("oops")
    expect(channelAgentsErrorMessage(null, "oops")).toBe("oops")
  })
})
