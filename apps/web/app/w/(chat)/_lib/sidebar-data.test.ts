import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  agentAvatarURL,
  buildSidebarTeamGroups,
  channelRouteSlug,
  dedupeSessions,
  findChannelByRouteSlug,
  isAmbiguousChannelRouteSlug,
  sessionActivityLabel,
  sessionRouteFromPathname,
  sortChannelsByRecentSession,
  type SidebarAgentResponse,
  type SidebarChannelResponse,
  type SidebarSessionResponse,
  type SidebarTeamResponse,
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

describe("buildSidebarTeamGroups", () => {
  function team(overrides: Partial<SidebarTeamResponse>): SidebarTeamResponse {
    return { id: "team_1", name: "Alpha", ...overrides }
  }

  it("builds one group per team with its channels sorted by recent session", () => {
    const groups = buildSidebarTeamGroups(
      [
        team({
          id: "team_a",
          name: "Alpha",
          channels: [
            channel({ id: "old", name: "Old" }),
            channel({ id: "recent", name: "Recent" }),
          ],
        }),
        team({ id: "team_b", name: "Beta", channels: [] }),
      ],
      new Map([
        ["recent", session({ last_activity_at: "2026-06-17T09:00:00.000Z" })],
        ["old", session({ last_activity_at: "2026-06-16T09:00:00.000Z" })],
      ])
    )

    expect(groups.map((group) => group.name)).toEqual(["Alpha", "Beta"])
    expect(groups[0].channels.map((c) => c.id)).toEqual(["recent", "old"])
    expect(groups[1].channels).toEqual([])
  })

  it("falls back to a team-id label and skips teams without an id", () => {
    const groups = buildSidebarTeamGroups(
      [
        team({ id: "team_x", name: "  ", channels: [] }),
        team({ id: undefined, name: "No id", channels: [] }),
      ],
      new Map()
    )

    expect(groups).toHaveLength(1)
    expect(groups[0].teamId).toBe("team_x")
    expect(groups[0].name).toBe("Team team_x")
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
