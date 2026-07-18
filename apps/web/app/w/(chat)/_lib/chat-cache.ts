import type { InfiniteData, QueryClient } from "@tanstack/react-query"
import type { components } from "@/lib/api/schema"

export const CHAT_QUERY_STALE_TIME_MS = 5 * 60 * 1000
export const SESSION_HISTORY_PAGE_LIMIT = 100
export const SESSION_EVENTS_INFINITE_KEY = "session-events-infinite-v1"

export type SessionResponse = components["schemas"]["sessionResponse"]
type SessionDetailResponse =
  components["schemas"]["sessionDetailResponse"]
type PaginatedSessions =
  components["schemas"]["paginatedResponse-sessionResponse"]

export const chatQueryKeys = {
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

export function invalidateSessionListQueries(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: ["get", "/v1/sessions"] })
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
    { queryKey: ["get", "/v1/sessions"] },
    (current: unknown) => patchSessionInfiniteData(current, session)
  )
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

function isInfiniteData<T>(value: unknown): value is InfiniteData<T> {
  return (
    Boolean(value) &&
    typeof value === "object" &&
    Array.isArray((value as InfiniteData<T>).pages)
  )
}
