import { SESSION_HISTORY_PAGE_LIMIT } from "@/app/w/(chat)/_lib/chat-cache"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"
import { useSessionRuntimeStore } from "@/app/w/(chat)/_stores/session-runtime-store"

interface FetchSubagentSessionEventsOptions {
  limit?: number
  signal?: AbortSignal
}

interface SubagentSessionEventsPage {
  data?: SessionEventResponse[]
  next_cursor?: string
  has_more?: boolean
}

export async function fetchSubagentSessionEvents(
  parentSessionId: string,
  childSessionId: string,
  options: FetchSubagentSessionEventsOptions = {}
) {
  const events: SessionEventResponse[] = []
  const limit = boundedLimit(options.limit)
  let cursor: string | undefined

  do {
    const page = await fetchSubagentSessionEventsPage(
      parentSessionId,
      childSessionId,
      limit,
      cursor,
      options.signal
    )
    events.push(...(page.data ?? []))
    cursor = page.has_more ? page.next_cursor : undefined
    if (page.has_more && !cursor) {
      throw new Error("Subagent events response did not include a next cursor.")
    }
  } while (cursor)

  return events
}

export async function hydrateCompletedSubagentRun({
  parentSessionId,
  run,
}: {
  parentSessionId: string
  run: SessionSubagentRun
}) {
  if (run.status !== "completed" || !run.childSessionId) return
  const events = await fetchSubagentSessionEvents(
    parentSessionId,
    run.childSessionId
  )
  useSessionRuntimeStore.getState().mergeSubagentRuns(parentSessionId, [
    {
      ...run,
      events,
      updatedAt: Date.now(),
    },
  ])
}

async function fetchSubagentSessionEventsPage(
  parentSessionId: string,
  childSessionId: string,
  limit: number,
  cursor: string | undefined,
  signal?: AbortSignal
): Promise<SubagentSessionEventsPage> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set("cursor", cursor)
  const response = await fetch(
    `/api/proxy/v1/sessions/${encodeURIComponent(
      parentSessionId
    )}/subagents/${encodeURIComponent(childSessionId)}/events?${params}`,
    { headers: { Accept: "application/json" }, signal }
  )
  if (!response.ok) {
    throw new Error(
      (await responseErrorMessage(response)) ||
        `Subagent events failed with HTTP ${response.status}`
    )
  }
  return (await response.json()) as SubagentSessionEventsPage
}

function boundedLimit(value: unknown) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return SESSION_HISTORY_PAGE_LIMIT
  }
  return Math.min(100, Math.max(1, Math.trunc(value)))
}

async function responseErrorMessage(response: Response) {
  const text = await response.text().catch(() => "")
  if (!text.trim()) return ""
  try {
    const parsed = JSON.parse(text) as { error?: unknown; message?: unknown }
    return stringValue(parsed.error) || stringValue(parsed.message) || text
  } catch {
    return text
  }
}

function stringValue(value: unknown) {
  return typeof value === "string" && value.trim() ? value : ""
}
