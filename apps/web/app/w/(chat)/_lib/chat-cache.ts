import type { InfiniteData, QueryClient } from "@tanstack/react-query"
import { createStore, clear } from "idb-keyval"
import type { components } from "@/lib/api/schema"

export const CHAT_QUERY_STALE_TIME_MS = 5 * 60 * 1000
export const SESSION_HISTORY_PAGE_LIMIT = 100
export const SIDEBAR_SESSION_PAGE_LIMIT = 5
export const SIDEBAR_SESSION_SORT = "activity"
const CHAT_CACHE_STORE = createStore("hivy-chat-query-cache", "queries")
export const CHANNEL_SESSIONS_INFINITE_KEY = "channel-sessions-infinite-v1"
export const SESSION_EVENTS_INFINITE_KEY = "session-events-infinite-v1"

export type ChannelResponse = components["schemas"]["channelResponse"]
export type ChannelDetailResponse =
  components["schemas"]["channelDetailResponse"]
export type SessionResponse = components["schemas"]["sessionResponse"]
export type SessionEventResponse = components["schemas"]["sessionEventResponse"]
export type PaginatedChannels =
  components["schemas"]["paginatedResponse-channelResponse"]
export type SessionDetailResponse =
  components["schemas"]["sessionDetailResponse"]
export type PaginatedSessions =
  components["schemas"]["paginatedResponse-sessionResponse"]
export type PaginatedSessionEvents =
  components["schemas"]["paginatedResponse-sessionEventResponse"]

export const chatQueryKeys = {
  channels: (limit = 100) =>
    ["get", "/v1/channels", { params: { query: { limit } } }] as const,
  channel: (channelID: string) =>
    [
      "get",
      "/v1/channels/{id}",
      { params: { path: { id: channelID } } },
    ] as const,
  channelSessions: (
    channelID: string,
    limit = SIDEBAR_SESSION_PAGE_LIMIT,
    sort = SIDEBAR_SESSION_SORT
  ) =>
    [
      "get",
      "/v1/channels/{id}/sessions",
      {
        _hivyQueryKey: CHANNEL_SESSIONS_INFINITE_KEY,
        params: {
          path: { id: channelID },
          query: { limit, sort },
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
  queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
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

export function patchSessionInChatCaches(
  queryClient: QueryClient,
  session: SessionResponse
) {
  if (!session.id) return
  queryClient.setQueryData<SessionDetailResponse>(
    chatQueryKeys.session(session.id),
    (current) => ({
      ...current,
      session: mergeSession(current?.session, session),
    })
  )
  queryClient.setQueriesData<unknown>(
    { queryKey: ["get", "/v1/channels/{id}/sessions"] },
    (current: unknown) => patchSessionInfiniteData(current, session)
  )
  queryClient.setQueriesData<unknown>(
    { queryKey: ["get", "/v1/sessions"] },
    (current: unknown) => patchSessionInfiniteData(current, session)
  )
  queryClient.setQueriesData<unknown>(
    { queryKey: ["get", "/v1/channels"] },
    (current: unknown) => patchChannelInfiniteData(current, session)
  )
}

export function patchChannelInChatCaches(
  queryClient: QueryClient,
  channel: ChannelResponse
) {
  if (!channel.id) return
  queryClient.setQueryData<ChannelDetailResponse>(
    chatQueryKeys.channel(channel.id),
    (current) =>
      current
        ? {
            ...current,
            channel: mergeChannel(current.channel, channel),
          }
        : current
  )
  queryClient.setQueriesData<unknown>(
    { queryKey: ["get", "/v1/channels"] },
    (current: unknown) => patchChannelListData(current, channel)
  )
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

function patchSessionInfiniteData(
  current: unknown,
  session: SessionResponse
): unknown {
  if (!isInfiniteData<PaginatedSessions>(current)) return current
  let changed = false
  const pages = current.pages.map((page) => {
    const next = patchSessionPage(page, session)
    changed ||= next !== page
    return next
  })
  return changed ? { ...current, pages } : current
}

function patchChannelInfiniteData(
  current: unknown,
  session: SessionResponse
): unknown {
  if (!isInfiniteData<PaginatedChannels>(current)) return current
  let changed = false
  const pages = current.pages.map((page) => {
    const data = page.data
    if (!data) return page
    let pageChanged = false
    const nextData = data.map((channel) => {
      const recentSessions = patchSessionArray(channel.recent_sessions, session)
      if (recentSessions === channel.recent_sessions) return channel
      pageChanged = true
      return { ...channel, recent_sessions: recentSessions }
    })
    if (!pageChanged) return page
    changed = true
    return { ...page, data: nextData }
  })
  return changed ? { ...current, pages } : current
}

function patchChannelListData(
  current: unknown,
  channel: ChannelResponse
): unknown {
  if (isInfiniteData<PaginatedChannels>(current)) {
    let changed = false
    const pages = current.pages.map((page) => {
      const next = patchChannelPage(page, channel)
      changed ||= next !== page
      return next
    })
    return changed ? { ...current, pages } : current
  }
  if (isPaginatedChannels(current)) {
    return patchChannelPage(current, channel)
  }
  return current
}

function patchChannelPage(
  page: PaginatedChannels,
  channel: ChannelResponse
): PaginatedChannels {
  const data = patchChannelArray(page.data, channel)
  return data === page.data ? page : { ...page, data }
}

function patchChannelArray(
  channels: ChannelResponse[] | undefined,
  channel: ChannelResponse
) {
  if (!channels) return channels
  let changed = false
  const next = channels.map((entry) => {
    if (entry.id !== channel.id) return entry
    changed = true
    return mergeChannel(entry, channel)
  })
  return changed ? next : channels
}

function patchSessionPage(
  page: PaginatedSessions,
  session: SessionResponse
): PaginatedSessions {
  const data = patchSessionArray(page.data, session)
  return data === page.data ? page : { ...page, data }
}

function patchSessionArray(
  sessions: SessionResponse[] | undefined,
  session: SessionResponse
) {
  if (!sessions) return sessions
  let changed = false
  const next = sessions.map((entry) => {
    if (entry.id !== session.id) return entry
    changed = true
    return mergeSession(entry, session)
  })
  return changed ? next : sessions
}

function mergeSession(
  current: SessionResponse | undefined,
  incoming: SessionResponse
): SessionResponse {
  return { ...current, ...incoming }
}

function mergeChannel(
  current: ChannelResponse | undefined,
  incoming: ChannelResponse
): ChannelResponse {
  return { ...current, ...incoming }
}

function isPaginatedChannels(value: unknown): value is PaginatedChannels {
  return (
    Boolean(value) &&
    typeof value === "object" &&
    Array.isArray((value as PaginatedChannels).data)
  )
}

function isInfiniteData<T>(value: unknown): value is InfiniteData<T> {
  return (
    Boolean(value) &&
    typeof value === "object" &&
    Array.isArray((value as InfiniteData<T>).pages)
  )
}
