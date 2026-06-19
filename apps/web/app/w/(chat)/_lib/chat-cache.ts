import type { InfiniteData, QueryClient } from "@tanstack/react-query"
import { createStore, clear } from "idb-keyval"
import type { components } from "@/lib/api/schema"
import type { ImageAttachmentMetadata } from "@/app/w/(chat)/_lib/image-attachments"
import type { CodeLineCommentPayload } from "@/app/w/(chat)/_lib/code-line-comments"

export const CHAT_QUERY_STALE_TIME_MS = 5 * 60 * 1000
export const SESSION_HISTORY_PAGE_LIMIT = 100
export const SIDEBAR_SESSION_PAGE_LIMIT = 5
const CHAT_CACHE_STORE = createStore("hivy-chat-query-cache", "queries")
export const CHANNEL_SESSIONS_INFINITE_KEY = "channel-sessions-infinite-v1"
export const SESSION_EVENTS_INFINITE_KEY = "session-events-infinite-v1"

export type ChannelResponse = components["schemas"]["channelResponse"]
export type SessionResponse = components["schemas"]["sessionResponse"]
export type SessionEventResponse = components["schemas"]["sessionEventResponse"]
export type PaginatedChannels =
  components["schemas"]["paginatedResponse-channelResponse"]
export type PaginatedSessions =
  components["schemas"]["paginatedResponse-sessionResponse"]
export type PaginatedSessionEvents =
  components["schemas"]["paginatedResponse-sessionEventResponse"]

export const chatQueryKeys = {
  channels: (limit = 100) =>
    ["get", "/v1/channels", { params: { query: { limit } } }] as const,
  channelSessions: (channelID: string, limit = SIDEBAR_SESSION_PAGE_LIMIT) =>
    [
      "get",
      "/v1/channels/{id}/sessions",
      {
        _hivyQueryKey: CHANNEL_SESSIONS_INFINITE_KEY,
        params: {
          path: { id: channelID },
          query: { limit },
        },
      },
    ] as const,
  session: (sessionID: string) =>
    [
      "get",
      "/v1/sessions/{id}",
      { params: { path: { id: sessionID } } },
    ] as const,
  sessionEvents: (sessionID: string, limit = SESSION_HISTORY_PAGE_LIMIT) =>
    [
      "get",
      "/v1/sessions/{id}/events",
      {
        _hivyQueryKey: SESSION_EVENTS_INFINITE_KEY,
        params: {
          path: { id: sessionID },
          query: { limit },
        },
      },
    ] as const,
  agents: (limit = 100, status = "active") =>
    ["get", "/v1/agents", { params: { query: { status, limit } } }] as const,
}

export function clearPersistedChatQueries() {
  return clear(CHAT_CACHE_STORE)
}

export function invalidateSessionListQueries(queryClient: QueryClient) {
  queryClient.invalidateQueries({
    queryKey: ["get", "/v1/channels/{id}/sessions"],
  })
  queryClient.invalidateQueries({ queryKey: ["get", "/v1/sessions"] })
  queryClient.invalidateQueries({
    queryKey: ["sidebar-channel-latest-session"],
  })
}

export function seedSessionDetail(
  queryClient: QueryClient,
  session: SessionResponse
) {
  if (!session.id) return
  queryClient.setQueryData(chatQueryKeys.session(session.id), { session })
}

export function insertSessionIntoChannelCache(
  queryClient: QueryClient,
  session: SessionResponse
) {
  if (!session.id || !session.channel_id) return
  queryClient.setQueryData<InfiniteData<PaginatedSessions>>(
    chatQueryKeys.channelSessions(session.channel_id),
    (current) => {
      if (!current) {
        return {
          pageParams: ["0"],
          pages: [{ data: [session], has_more: false }],
        }
      }
      const pages = current.pages.map((page, index) => {
        const data = page.data ?? []
        const withoutDuplicate = data.filter((entry) => entry.id !== session.id)
        return index === 0
          ? { ...page, data: [session, ...withoutDuplicate] }
          : { ...page, data: withoutDuplicate }
      })
      return { ...current, pages }
    }
  )
}

export function removeSessionFromChannelCache(
  queryClient: QueryClient,
  channelID: string | undefined,
  sessionID: string
) {
  if (!channelID) return
  queryClient.setQueryData<InfiniteData<PaginatedSessions>>(
    chatQueryKeys.channelSessions(channelID),
    (current) => {
      if (!current) return current
      return {
        ...current,
        pages: current.pages.map((page) => ({
          ...page,
          data: (page.data ?? []).filter((session) => session.id !== sessionID),
        })),
      }
    }
  )
}

export function seedSessionEvents(
  queryClient: QueryClient,
  sessionID: string,
  events: SessionEventResponse[]
) {
  queryClient.setQueryData<InfiniteData<PaginatedSessionEvents>>(
    chatQueryKeys.sessionEvents(sessionID),
    (current) => {
      const page = mergeEventsIntoPage(current?.pages[0], events)
      return {
        pageParams: current?.pageParams ?? ["0"],
        pages: current?.pages.length
          ? [page, ...current.pages.slice(1)]
          : [page],
      }
    }
  )
}

export function appendSessionEvents(
  queryClient: QueryClient,
  sessionID: string,
  events: SessionEventResponse[]
) {
  queryClient.setQueryData<InfiniteData<PaginatedSessionEvents>>(
    chatQueryKeys.sessionEvents(sessionID),
    (current) => {
      const firstPage = mergeEventsIntoPage(current?.pages[0], events)
      return {
        pageParams: current?.pageParams ?? ["0"],
        pages: current?.pages.length
          ? [firstPage, ...current.pages.slice(1)]
          : [firstPage],
      }
    }
  )
}

export function replaceOptimisticSessionID(
  queryClient: QueryClient,
  optimisticID: string,
  session: SessionResponse
) {
  if (!session.id || optimisticID === session.id) return
  const optimisticEvents = queryClient.getQueryData<
    InfiniteData<PaginatedSessionEvents>
  >(chatQueryKeys.sessionEvents(optimisticID))
  queryClient.removeQueries({
    queryKey: chatQueryKeys.sessionEvents(optimisticID),
    exact: true,
  })
  queryClient.removeQueries({
    queryKey: chatQueryKeys.session(optimisticID),
    exact: true,
  })
  if (optimisticEvents) {
    queryClient.setQueryData(chatQueryKeys.sessionEvents(session.id), {
      ...optimisticEvents,
      pages: optimisticEvents.pages.map((page) => ({
        ...page,
        data: (page.data ?? []).map((event) => ({
          ...event,
          session_id: session.id,
        })),
      })),
    })
  }
}

export function markOptimisticEventFailed(
  queryClient: QueryClient,
  sessionID: string,
  eventID: string,
  message: string
) {
  updateOptimisticEventPayload(queryClient, sessionID, eventID, (event) => ({
    ...payloadRecord(event),
    client_status: "failed",
    client_error: message,
  }))
}

export function markOptimisticEventPending(
  queryClient: QueryClient,
  sessionID: string,
  eventID: string
) {
  updateOptimisticEventPayload(queryClient, sessionID, eventID, (event) => {
    const payload: Record<string, unknown> = {
      ...payloadRecord(event),
      client_status: "pending",
    }
    delete payload.client_error
    return payload
  })
}

export function removeSessionEvent(
  queryClient: QueryClient,
  sessionID: string,
  eventID: string
) {
  queryClient.setQueryData<InfiniteData<PaginatedSessionEvents>>(
    chatQueryKeys.sessionEvents(sessionID),
    (current) => {
      if (!current) return current
      return {
        ...current,
        pages: current.pages.map((page) => ({
          ...page,
          data: (page.data ?? []).filter(
            (event) => event.event_id !== eventID && event.id !== eventID
          ),
        })),
      }
    }
  )
}

export function replaceOptimisticEvent(
  queryClient: QueryClient,
  sessionID: string,
  optimisticEventID: string,
  event: SessionEventResponse
) {
  queryClient.setQueryData<InfiniteData<PaginatedSessionEvents>>(
    chatQueryKeys.sessionEvents(sessionID),
    (current) => {
      if (!current) return current
      return {
        ...current,
        pages: current.pages.map((page) => ({
          ...page,
          data: (page.data ?? []).map((entry) =>
            entry.id === optimisticEventID ||
            entry.event_id === optimisticEventID
              ? event
              : entry
          ),
        })),
      }
    }
  )
}

export function optimisticUserEvent(
  sessionID: string,
  text: string,
  eventID = `optimistic_msg_${crypto.randomUUID()}`,
  attachments: ImageAttachmentMetadata[] = [],
  codeLineComments: CodeLineCommentPayload[] = []
): SessionEventResponse {
  const now = new Date().toISOString()
  return {
    id: eventID,
    event_id: eventID,
    event_type: "user.message",
    event_at: now,
    session_id: sessionID,
    source: "web",
    payload: {
      text,
      ...(attachments.length ? { attachments } : {}),
      ...(codeLineComments.length
        ? { code_line_comments: codeLineComments }
        : {}),
      client_status: "pending",
    },
  }
}

export function optimisticThinkingEvent(
  sessionID: string,
  turnID = `optimistic_turn_${crypto.randomUUID()}`
): SessionEventResponse {
  const now = new Date().toISOString()
  return {
    id: `optimistic_thinking_${turnID}`,
    event_id: `optimistic_thinking_${turnID}`,
    event_type: "thinking",
    event_at: now,
    session_id: sessionID,
    source: "web",
    payload: {
      text: "",
      turn_id: turnID,
      client_status: "pending",
    },
  }
}

function mergeEventsIntoPage(
  page: PaginatedSessionEvents | undefined,
  incoming: SessionEventResponse[]
): PaginatedSessionEvents {
  const existing = page?.data ?? []
  const byID = new Map<string, SessionEventResponse>()
  for (const event of existing) byID.set(eventKey(event), event)
  for (const event of incoming) byID.set(eventKey(event), event)
  const data = [...byID.values()].sort(compareSessionEvents)
  return { ...page, data, has_more: page?.has_more ?? false }
}

function compareSessionEvents(
  left: SessionEventResponse,
  right: SessionEventResponse
) {
  const byTime = eventTime(left) - eventTime(right)
  if (byTime !== 0) return byTime
  return (left.sequence_number ?? 0) - (right.sequence_number ?? 0)
}

function eventTime(event: SessionEventResponse) {
  const time = event.event_at ? Date.parse(event.event_at) : 0
  return Number.isNaN(time) ? 0 : time
}

function eventKey(event: SessionEventResponse) {
  return event.id ?? event.event_id ?? `${event.event_type}:${event.event_at}`
}

function updateOptimisticEventPayload(
  queryClient: QueryClient,
  sessionID: string,
  eventID: string,
  update: (event: SessionEventResponse) => Record<string, unknown>
) {
  queryClient.setQueryData<InfiniteData<PaginatedSessionEvents>>(
    chatQueryKeys.sessionEvents(sessionID),
    (current) => {
      if (!current) return current
      return {
        ...current,
        pages: current.pages.map((page) => ({
          ...page,
          data: (page.data ?? []).map((event) =>
            event.event_id === eventID || event.id === eventID
              ? { ...event, payload: update(event) }
              : event
          ),
        })),
      }
    }
  )
}

function payloadRecord(event: SessionEventResponse): Record<string, unknown> {
  const payload = event.payload
  return payload && typeof payload === "object" && !Array.isArray(payload)
    ? (payload as Record<string, unknown>)
    : {}
}
