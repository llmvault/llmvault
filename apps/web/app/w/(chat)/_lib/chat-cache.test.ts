import { QueryClient } from "@tanstack/react-query"
import { describe, expect, it } from "vitest"
import {
  appendSessionEvents,
  chatQueryKeys,
  insertSessionIntoChannelCache,
  optimisticThinkingEvent,
  optimisticUserEvent,
  patchSessionInChatCaches,
  removeSessionEvent,
  replaceOptimisticEvent,
  replaceOptimisticSessionID,
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

  it("replaces optimistic events with real server events", () => {
    const queryClient = client()
    const optimistic = optimisticUserEvent(
      "session-1",
      "Ship it",
      "optimistic-1"
    )
    seedSessionEvents(queryClient, "session-1", [optimistic])

    replaceOptimisticEvent(queryClient, "session-1", "optimistic-1", {
      ...optimistic,
      id: "real-1",
      event_id: "real-1",
      payload: { text: "Ship it" },
    })

    const cached = queryClient.getQueryData<{
      pages: PaginatedSessionEvents[]
    }>(chatQueryKeys.sessionEvents("session-1"))

    expect(cached?.pages[0].data?.[0]).toMatchObject({
      event_id: "real-1",
      payload: { text: "Ship it" },
    })
  })

  it("moves temporary session event cache to the real session id", () => {
    const queryClient = client()

    seedSessionDetail(queryClient, {
      id: "tmp_1",
      channel_id: "channel-1",
      name: "Temporary",
    })
    insertSessionIntoChannelCache(queryClient, {
      id: "tmp_1",
      channel_id: "channel-1",
      name: "Temporary",
    })
    seedSessionEvents(queryClient, "tmp_1", [
      optimisticUserEvent("tmp_1", "First message", "optimistic-1"),
    ])

    replaceOptimisticSessionID(queryClient, "tmp_1", {
      id: "session-1",
      channel_id: "channel-1",
      name: "Real",
    })

    expect(
      queryClient.getQueryData(chatQueryKeys.sessionEvents("tmp_1"))
    ).toBeUndefined()
    const realEvents = queryClient.getQueryData<{
      pages: PaginatedSessionEvents[]
    }>(chatQueryKeys.sessionEvents("session-1"))
    expect(realEvents?.pages[0].data?.[0].session_id).toBe("session-1")
  })

  it("removes the temporary thinking event after a new session is created", () => {
    const queryClient = client()
    const optimisticMessage = optimisticUserEvent(
      "tmp_1",
      "Research this",
      "optimistic-1"
    )
    const optimisticThinking = optimisticThinkingEvent(
      "tmp_1",
      "optimistic-turn"
    )

    seedSessionEvents(queryClient, "tmp_1", [
      optimisticMessage,
      optimisticThinking,
    ])
    replaceOptimisticSessionID(queryClient, "tmp_1", {
      id: "session-1",
      channel_id: "channel-1",
      name: "Real",
    })
    replaceOptimisticEvent(queryClient, "session-1", "optimistic-1", {
      ...optimisticMessage,
      id: "real-1",
      event_id: "real-1",
      session_id: "session-1",
      payload: { text: "Research this" },
    })
    removeSessionEvent(
      queryClient,
      "session-1",
      optimisticThinking.event_id ?? optimisticThinking.id ?? ""
    )

    const cached = queryClient.getQueryData<{
      pages: PaginatedSessionEvents[]
    }>(chatQueryKeys.sessionEvents("session-1"))

    expect(cached?.pages[0].data).toHaveLength(1)
    expect(cached?.pages[0].data?.[0]).toMatchObject({
      event_id: "real-1",
      session_id: "session-1",
      payload: { text: "Research this" },
    })
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
