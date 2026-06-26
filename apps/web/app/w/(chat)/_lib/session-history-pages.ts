import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history-event-utils"

type SessionHistoryPage = {
  data?: SessionEventResponse[]
}

export function sessionHistoryPagesToEvents(
  pages: SessionHistoryPage[]
): SessionEventResponse[] {
  const seen = new Set<string>()
  const events: SessionEventResponse[] = []

  for (const page of pages) {
    for (const event of page.data ?? []) {
      const keys = [event.id, event.event_id].filter((key): key is string =>
        Boolean(key)
      )
      if (keys.some((key) => seen.has(key))) continue
      for (const key of keys) seen.add(key)
      events.push(event)
    }
  }

  return events
}
