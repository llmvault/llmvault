import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  agentAvatarURL,
  channelRouteSlug,
  dedupeSessions,
  findChannelByRouteSlug,
  groupChannelsByTeam,
  isAmbiguousChannelRouteSlug,
  sessionActivityLabel,
  sessionRouteFromPathname,
  sortChannelsByRecentSession,
  UNGROUPED_TEAM_KEY,
  UNGROUPED_TEAM_LABEL,
  type SidebarAgentResponse,
  type SidebarChannelResponse,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"

function agent(overrides: Partial<SidebarAgentResponse>): SidebarAgentResponse {
  return { id: "agent_1", name: "Hivy", ...overrides }
}

function channel(
  overrides: Partial<SidebarChannelResponse>
): SidebarChannelResponse {
  return { id: "channel_1", name: "General", ...overrides }
}

function session(
  overrides: Partial<SidebarSessionResponse>
): SidebarSessionResponse {
  return { id: "session_1", name: "Ship sidebar", ...overrides }
}

describe("sidebar route helpers", () => {
  it("keeps channel routes name-derived for existing /w/channels/general links", () => {
    expect(channelRouteSlug(channel({ name: "General" }))).toBe("general")
  })

  it("treats duplicate name slugs as ambiguous", () => {
    const channels = [
      channel({ id: "channel_1", name: "General" }),
      channel({ id: "channel_2", name: "general!" }),
    ]

    expect(isAmbiguousChannelRouteSlug(channels, "general")).toBe(true)
    expect(findChannelByRouteSlug(channels, "general")).toBeUndefined()
  })

  it("finds a channel by route slug when the slug is unique", () => {
    const channels = [
      channel({ id: "channel_1", name: "General" }),
      channel({ id: "channel_2", name: "Support" }),
    ]

    expect(findChannelByRouteSlug(channels, "support")?.id).toBe("channel_2")
  })

  it("parses only concrete session routes", () => {
    expect(sessionRouteFromPathname("/w/channels/general")).toBeNull()
    expect(sessionRouteFromPathname("/w/channels/general/session_1")).toEqual({
      channelSlug: "general",
      sessionId: "session_1",
    })
  })

  it("sorts channels by newest latest session and keeps empty channels last", () => {
    const channels = [
      channel({
        id: "empty",
        name: "Empty",
        updated_at: "2026-06-17T10:00:00.000Z",
      }),
      channel({ id: "old", name: "Old" }),
      channel({ id: "recent", name: "Recent" }),
    ]
    const latestSessionsByChannelID = new Map([
      [
        "old",
        session({
          last_activity_at: "2026-06-16T10:00:00.000Z",
        }),
      ],
      [
        "recent",
        session({
          last_activity_at: "2026-06-17T09:00:00.000Z",
        }),
      ],
    ])

    expect(
      sortChannelsByRecentSession(channels, latestSessionsByChannelID).map(
        (entry) => entry.id
      )
    ).toEqual(["recent", "old", "empty"])
  })
})

describe("groupChannelsByTeam", () => {
  it("buckets channels into one group per team, ordered by team name", () => {
    const channels = [
      channel({ id: "c1", name: "Roadmap", team_id: "team_b" }),
      channel({ id: "c2", name: "Support", team_id: "team_a" }),
      channel({ id: "c3", name: "Billing", team_id: "team_b" }),
    ]
    const names = new Map([
      ["team_a", "Alpha"],
      ["team_b", "Beta"],
    ])

    const groups = groupChannelsByTeam(channels, names)

    expect(groups.map((group) => group.name)).toEqual(["Alpha", "Beta"])
    expect(groups[0].channels.map((c) => c.id)).toEqual(["c2"])
    // Preserves incoming channel order within a group.
    expect(groups[1].channels.map((c) => c.id)).toEqual(["c1", "c3"])
  })

  it("falls back to a team-id label when the name is unknown", () => {
    const groups = groupChannelsByTeam([
      channel({ id: "c1", name: "Roadmap", team_id: "team_x" }),
    ])

    expect(groups).toHaveLength(1)
    expect(groups[0].teamId).toBe("team_x")
    expect(groups[0].name).toBe("Team team_x")
  })

  it("collapses channels with no team into a trailing Other group", () => {
    const groups = groupChannelsByTeam(
      [
        channel({ id: "c1", name: "Roadmap", team_id: "team_a" }),
        channel({ id: "c2", name: "Random" }),
        channel({ id: "c3", name: "Ideas", team_id: "" }),
      ],
      new Map([["team_a", "Alpha"]])
    )

    const last = groups[groups.length - 1]
    expect(last.key).toBe(UNGROUPED_TEAM_KEY)
    expect(last.teamId).toBeNull()
    expect(last.name).toBe(UNGROUPED_TEAM_LABEL)
    expect(last.channels.map((c) => c.id)).toEqual(["c2", "c3"])
  })

  it("returns a single ungrouped group when no channel has a team", () => {
    const groups = groupChannelsByTeam([
      channel({ id: "c1", name: "Roadmap" }),
      channel({ id: "c2", name: "Random" }),
    ])

    expect(groups).toHaveLength(1)
    expect(groups[0].teamId).toBeNull()
    expect(groups.some((group) => group.teamId)).toBe(false)
  })
})

describe("sidebar session helpers", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-06-16T01:00:00.000Z"))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it("dedupes paginated sessions by id", () => {
    expect(
      dedupeSessions([
        session({ id: "session_1", name: "First" }),
        session({ id: "session_1", name: "Duplicate" }),
        session({ id: "session_2", name: "Second" }),
        session({ id: undefined, name: "Missing id" }),
      ]).map((entry) => entry.name)
    ).toEqual(["First", "Second"])
  })

  it("formats recent activity labels", () => {
    expect(
      sessionActivityLabel(
        session({ last_activity_at: "2026-06-16T00:57:00.000Z" })
      )
    ).toBe("3m")
    expect(
      sessionActivityLabel(
        session({ last_activity_at: "2026-06-15T22:00:00.000Z" })
      )
    ).toBe("3h")
  })
})

describe("sidebar agent helpers", () => {
  it("uses direct agent avatars before catalog fallback avatars", () => {
    expect(
      agentAvatarURL(
        agent({
          avatar_url: " /assets/hivy.png ",
          catalog: { avatar_url: "/assets/hakaree.png" },
        })
      )
    ).toBe("/assets/hivy.png")

    expect(
      agentAvatarURL(
        agent({
          avatar_url: "",
          catalog: { avatar_url: " /assets/hakaree.png " },
        })
      )
    ).toBe("/assets/hakaree.png")
  })
})
