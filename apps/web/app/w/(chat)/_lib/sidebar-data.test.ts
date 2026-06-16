import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  channelRouteSlug,
  dedupeSessions,
  findChannelByRouteSlug,
  isAmbiguousChannelRouteSlug,
  sessionActivityLabel,
  sessionRouteFromPathname,
  type SidebarChannelResponse,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"

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
