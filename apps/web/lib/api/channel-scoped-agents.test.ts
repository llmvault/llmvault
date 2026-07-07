import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  useChannelAgents: vi.fn(),
  useQuery: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: { useQuery: mocks.useQuery },
}))

vi.mock("@/lib/api/channel-agents", () => ({
  useChannelAgents: mocks.useChannelAgents,
}))

import {
  resolveScopedAgentID,
  useChannelScopedAgents,
  type ChannelScopedAgents,
} from "@/lib/api/channel-scoped-agents"

function probe(channelId: string): ChannelScopedAgents {
  const captured: ChannelScopedAgents[] = []
  function Probe() {
    captured.push(useChannelScopedAgents(channelId))
    return null
  }
  renderToString(React.createElement(Probe))
  const result = captured[0]
  if (!result) throw new Error("hook did not run")
  return result
}

describe("useChannelScopedAgents", () => {
  beforeEach(() => {
    mocks.useChannelAgents.mockReset()
    mocks.useQuery.mockReset()
    // Default: org fallback query is inert (not enabled).
    mocks.useQuery.mockReturnValue({
      data: undefined,
      isError: false,
      isLoading: false,
    })
  })

  it("returns channel-scoped agents when the endpoint succeeds", () => {
    mocks.useChannelAgents.mockReturnValue({
      data: {
        data: [
          { id: "a1", name: "Agent One" },
          { id: "a2", name: "Agent Two" },
        ],
      },
      isError: false,
      isLoading: false,
    })

    const result = probe("channel-1")

    expect(result.agents.map((a) => a.id)).toEqual(["a1", "a2"])
    expect(result.isLoading).toBe(false)
    expect(result.isError).toBe(false)
    expect(result.isFallback).toBe(false)
    // The org fallback query must stay disabled while the scoped one works.
    expect(mocks.useQuery.mock.calls[0]?.[3]).toMatchObject({ enabled: false })
  })

  it("drops agents without an id", () => {
    mocks.useChannelAgents.mockReturnValue({
      data: { data: [{ id: "a1" }, { name: "no id" }] },
      isError: false,
      isLoading: false,
    })

    expect(probe("channel-1").agents.map((a) => a.id)).toEqual(["a1"])
  })

  it("surfaces channel loading state", () => {
    mocks.useChannelAgents.mockReturnValue({
      data: undefined,
      isError: false,
      isLoading: true,
    })

    const result = probe("channel-1")
    expect(result.isLoading).toBe(true)
    expect(result.agents).toEqual([])
  })

  it("falls back to the org agent list when the channel endpoint errors", () => {
    mocks.useChannelAgents.mockReturnValue({
      data: undefined,
      isError: true,
      isLoading: false,
    })
    mocks.useQuery.mockReturnValue({
      data: { data: [{ id: "org-1", name: "Org Agent" }] },
      isError: false,
      isLoading: false,
    })

    const result = probe("channel-1")

    expect(result.agents.map((a) => a.id)).toEqual(["org-1"])
    expect(result.isFallback).toBe(true)
    expect(result.isError).toBe(false)
    // The fallback query is enabled precisely because the scoped one errored.
    expect(mocks.useQuery.mock.calls[0]?.[3]).toMatchObject({ enabled: true })
  })

  it("reports isError only when both queries fail", () => {
    mocks.useChannelAgents.mockReturnValue({
      data: undefined,
      isError: true,
      isLoading: false,
    })
    mocks.useQuery.mockReturnValue({
      data: undefined,
      isError: true,
      isLoading: false,
    })

    const result = probe("channel-1")
    expect(result.isError).toBe(true)
    expect(result.isFallback).toBe(true)
  })
})

describe("resolveScopedAgentID", () => {
  const agents = [{ id: "a1" }, { id: "a2" }, { id: "a3" }]

  it("preserves the current pick while it's assigned to the channel", () => {
    expect(resolveScopedAgentID(agents, "a2", "a1")).toBe("a2")
  })

  it("falls back to the channel default when the pick is not assigned", () => {
    expect(resolveScopedAgentID(agents, "gone", "a3")).toBe("a3")
  })

  it("falls back to the first agent when there is no valid default", () => {
    expect(resolveScopedAgentID(agents, "gone", "also-gone")).toBe("a1")
    expect(resolveScopedAgentID(agents, "", null)).toBe("a1")
  })

  it("returns empty string when the channel has no agents", () => {
    expect(resolveScopedAgentID([], "a1", "a1")).toBe("")
  })

  it("ignores an empty current pick even if listed defaults are absent", () => {
    expect(resolveScopedAgentID(agents, "", "a2")).toBe("a2")
  })
})
