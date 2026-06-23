import { fetchEventSource } from "@microsoft/fetch-event-source"
import type { QueryClient } from "@tanstack/react-query"
import {
  invalidateSessionListQueries,
  patchSessionInChatCaches,
  type SessionResponse,
} from "@/app/w/(chat)/_lib/chat-cache"

const activeNameWatchers = new Map<string, AbortController>()

export function watchGeneratedSessionName(
  queryClient: QueryClient,
  session: SessionResponse
) {
  const sessionID = session.id?.trim()
  if (!sessionID) return

  activeNameWatchers.get(sessionID)?.abort()
  const controller = new AbortController()
  activeNameWatchers.set(sessionID, controller)

  void fetchEventSource(
    `/api/proxy/v1/sessions/${encodeURIComponent(sessionID)}/name-updates`,
    {
      method: "GET",
      headers: { Accept: "text/event-stream" },
      credentials: "include",
      openWhenHidden: true,
      signal: controller.signal,
      async onopen(response) {
        const contentType = response.headers.get("content-type") ?? ""
        if (response.ok && contentType.includes("text/event-stream")) return
        throw new Error(
          `Session name stream failed with HTTP ${response.status}`
        )
      },
      onmessage(event) {
        if (event.event !== "session.name") return
        const updated = parseSessionNameUpdate(event.data)
        if (!updated?.id) return
        patchSessionInChatCaches(queryClient, updated)
        invalidateSessionListQueries(queryClient)
        controller.abort()
      },
      onerror(error) {
        throw error
      },
    }
  )
    .catch(() => {
      /* Name generation is opportunistic; stale titles are corrected by normal refetches. */
    })
    .finally(() => {
      if (activeNameWatchers.get(sessionID) === controller) {
        activeNameWatchers.delete(sessionID)
      }
    })
}

function parseSessionNameUpdate(raw: string): SessionResponse | null {
  try {
    const decoded = JSON.parse(raw) as { session?: SessionResponse }
    return decoded.session ?? null
  } catch {
    return null
  }
}
