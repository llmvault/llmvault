import { QueryClient } from "@tanstack/react-query"
import { describe, expect, it } from "vitest"
import {
  appendSessionEvents,
  chatQueryKeys,
  insertSessionIntoChannelCache,
  patchChannelInChatCaches,
  patchSessionInChatCaches,
  seedSessionDetail,
  seedSessionEvents,
  type PaginatedChannels,
  type PaginatedSessionEvents,
  type PaginatedSessions,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/chat-cache"

function client() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  })
}

describe("chat cache helpers", () => {
  it("appends session events without duplicating existing events", () => {
    const queryClient = client()
    const first = event("event-1", "hello")

    seedSessionEvents(queryClient, "session-1", [first])
    appendSessionEvents(queryClient, "session-1", [
      first,
      event("event-2", "world"),
    ])

    const cached = queryClient.getQueryData<{
      pages: PaginatedSessionEvents[]
    }>(chatQueryKeys.sessionEvents("session-1"))

    expect(cached?.pages[0].data?.map((entry) => entry.event_id)).toEqual([
      "event-1",
      "event-2",
    ])
  })

  it("patches renamed sessions across detail and sidebar caches", () => {
    const queryClient = client()
    const original = {
      id: "session-1",
      channel_id: "channel-1",
      name: "Old title",
    }
    const renamed = { ...original, name: "New title" }

    seedSessionDetail(queryClient, original)
    queryClient.setQueryData<{
      pages: PaginatedSessions[]
      pageParams: string[]
    }>(["get", "/v1/sessions", { params: { query: { limit: 5 } } }], {
      pageParams: ["0"],
      pages: [{ data: [original], has_more: false }],
    })
    insertSessionIntoChannelCache(queryClient, original)
    queryClient.setQueryData<{
      pages: PaginatedChannels[]
      pageParams: string[]
    }>(["get", "/v1/channels", { params: { query: { limit: 100 } } }], {
      pageParams: ["0"],
      pages: [
        {
          data: [
            {
              id: "channel-1",
              name: "general",
              recent_sessions: [original],
            },
          ],
          has_more: false,
        },
      ],
    })

    patchSessionInChatCaches(queryClient, renamed)

    expect(
      queryClient.getQueryData<{ session?: { name?: string } }>(
        chatQueryKeys.session("session-1")
      )?.session?.name
    ).toBe("New title")
    expect(
      queryClient.getQueryData<{ pages: PaginatedSessions[] }>(
        chatQueryKeys.channelSessions("channel-1")
      )?.pages[0].data?.[0].name
    ).toBe("New title")
    expect(
      queryClient.getQueryData<{ pages: PaginatedSessions[] }>([
        "get",
        "/v1/sessions",
        { params: { query: { limit: 5 } } },
      ])?.pages[0].data?.[0].name
    ).toBe("New title")
    expect(
      queryClient.getQueryData<{ pages: PaginatedChannels[] }>([
        "get",
        "/v1/channels",
        { params: { query: { limit: 100 } } },
      ])?.pages[0].data?.[0].recent_sessions?.[0].name
    ).toBe("New title")
  })

  it("patches renamed channels across detail and channel list caches", () => {
    const queryClient = client()
    const original = {
      id: "channel-1",
      name: "general",
      description: "Workspace updates",
    }
    const renamed = { id: "channel-1", name: "announcements" }

    queryClient.setQueryData(chatQueryKeys.channel("channel-1"), {
      channel: original,
      members: [],
    })
    queryClient.setQueryData<PaginatedChannels>(
      ["get", "/v1/channels", { params: { query: { limit: 100 } } }],
      { data: [original], has_more: false }
    )
    queryClient.setQueryData<{
      pages: PaginatedChannels[]
      pageParams: string[]
    }>(
      [
        "get",
        "/v1/channels",
        {
          _hivyQueryKey: "channels-infinite-v1",
          params: { query: { limit: 100 } },
        },
      ],
      { pageParams: ["0"], pages: [{ data: [original], has_more: false }] }
    )

    patchChannelInChatCaches(queryClient, renamed)

    expect(
      queryClient.getQueryData<{ channel?: { name?: string } }>(
        chatQueryKeys.channel("channel-1")
      )?.channel?.name
    ).toBe("announcements")
    expect(
      queryClient.getQueryData<{ channel?: { description?: string } }>(
        chatQueryKeys.channel("channel-1")
      )?.channel?.description
    ).toBe("Workspace updates")
    expect(
      queryClient.getQueryData<PaginatedChannels>([
        "get",
        "/v1/channels",
        { params: { query: { limit: 100 } } },
      ])?.data?.[0].name
    ).toBe("announcements")
    expect(
      queryClient.getQueryData<{ pages: PaginatedChannels[] }>([
        "get",
        "/v1/channels",
        {
          _hivyQueryKey: "channels-infinite-v1",
          params: { query: { limit: 100 } },
        },
      ])?.pages[0].data?.[0].name
    ).toBe("announcements")
  })

  it("does not seed a partial channel detail cache from a list update", () => {
    const queryClient = client()
    const original = { id: "channel-1", name: "general" }
    const renamed = { id: "channel-1", name: "announcements" }

    queryClient.setQueryData<PaginatedChannels>(
      ["get", "/v1/channels", { params: { query: { limit: 100 } } }],
      { data: [original], has_more: false }
    )

    patchChannelInChatCaches(queryClient, renamed)

    expect(
      queryClient.getQueryData(chatQueryKeys.channel("channel-1"))
    ).toBeUndefined()
    expect(
      queryClient.getQueryData<PaginatedChannels>([
        "get",
        "/v1/channels",
        { params: { query: { limit: 100 } } },
      ])?.data?.[0].name
    ).toBe("announcements")
  })
})

function event(id: string, text: string): SessionEventResponse {
  return {
    id,
    event_id: id,
    event_type: "token",
    event_at: `2026-06-15T12:00:0${id.at(-1)}.000Z`,
    payload: { text },
  }
}
