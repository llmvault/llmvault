import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: { useQuery: mocks.useQuery },
}))

import {
  agentsForTeam,
  resolveScopedAgentID,
  useTeamAgents,
  type TeamAgent,
  type TeamAgents,
} from "@/lib/api/team-agents"

function agent(id: string, teamId?: string): TeamAgent {
  return { id, name: id, team_id: teamId }
}

describe("agentsForTeam", () => {
  const agents = [
    agent("a1", "team-1"),
    agent("a2", "team-2"),
    agent("a3", "team-1"),
    { name: "no id", team_id: "team-1" } as TeamAgent,
  ]

  it("returns only the agents whose team owns the channel", () => {
    expect(agentsForTeam(agents, "team-1").map((a) => a.id)).toEqual([
      "a1",
      "a3",
    ])
    expect(agentsForTeam(agents, "team-2").map((a) => a.id)).toEqual(["a2"])
  })

  it("drops agents without an id", () => {
    // The id-less "team-1" agent above must never appear in the result.
    expect(
      agentsForTeam(agents, "team-1").every((a) => Boolean(a.id))
    ).toBe(true)
  })

  it("returns an empty list when no team is selected", () => {
    expect(agentsForTeam(agents, "")).toEqual([])
    expect(agentsForTeam(agents, null)).toEqual([])
    expect(agentsForTeam(agents, undefined)).toEqual([])
    expect(agentsForTeam(agents, "   ")).toEqual([])
  })

  it("returns an empty list when no agent belongs to the team", () => {
    expect(agentsForTeam(agents, "team-unknown")).toEqual([])
  })
})

function probe(teamId: string | null | undefined): TeamAgents {
  const captured: TeamAgents[] = []
  function Probe() {
    captured.push(useTeamAgents(teamId))
    return null
  }
  renderToString(React.createElement(Probe))
  const result = captured[0]
  if (!result) throw new Error("hook did not run")
  return result
}

describe("useTeamAgents", () => {
  beforeEach(() => {
    mocks.useQuery.mockReset()
  })

  it("filters the visible agents to the selected channel's team", () => {
    mocks.useQuery.mockReturnValue({
      data: { data: [agent("a1", "team-1"), agent("a2", "team-2")] },
      isError: false,
      isLoading: false,
    })

    const result = probe("team-1")
    expect(result.agents.map((a) => a.id)).toEqual(["a1"])
    expect(result.isLoading).toBe(false)
    expect(result.isError).toBe(false)
  })

  it("surfaces loading and error state from the agents query", () => {
    mocks.useQuery.mockReturnValue({
      data: undefined,
      isError: true,
      isLoading: false,
    })
    expect(probe("team-1").isError).toBe(true)

    mocks.useQuery.mockReturnValue({
      data: undefined,
      isError: false,
      isLoading: true,
    })
    const loading = probe("team-1")
    expect(loading.isLoading).toBe(true)
    expect(loading.agents).toEqual([])
  })

  it("returns no agents until a team is known", () => {
    mocks.useQuery.mockReturnValue({
      data: { data: [agent("a1", "team-1")] },
      isError: false,
      isLoading: false,
    })
    expect(probe("").agents).toEqual([])
    expect(probe(undefined).agents).toEqual([])
  })
})

describe("resolveScopedAgentID", () => {
  const agents = [{ id: "a1" }, { id: "a2" }, { id: "a3" }]

  it("preserves the current pick while it belongs to the team", () => {
    expect(resolveScopedAgentID(agents, "a2", "a1")).toBe("a2")
  })

  it("falls back to the channel default when the pick is gone", () => {
    expect(resolveScopedAgentID(agents, "gone", "a3")).toBe("a3")
  })

  it("falls back to the first agent when there is no valid default", () => {
    expect(resolveScopedAgentID(agents, "gone", "also-gone")).toBe("a1")
    expect(resolveScopedAgentID(agents, "", null)).toBe("a1")
  })

  it("returns empty string when the team has no agents", () => {
    expect(resolveScopedAgentID([], "a1", "a1")).toBe("")
  })

  it("ignores an empty current pick even if listed defaults are absent", () => {
    expect(resolveScopedAgentID(agents, "", "a2")).toBe("a2")
  })
})
