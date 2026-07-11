"use client"

import { useEffect, useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { SESSION_HISTORY_PAGE_LIMIT } from "@/app/w/(chat)/_lib/chat-cache"
import { sessionHistoryPagesToEvents } from "@/app/w/(chat)/_lib/session-history"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"

export function useSubagentHistory(
  sessionId: string | undefined,
  run: SessionSubagentRun | undefined
) {
  const childSessionId = run?.childSessionId?.trim() ?? ""
  const enabled = Boolean(
    sessionId && childSessionId && run && run.status !== "running"
  )
  const query = $api.useInfiniteQuery(
    "get",
    "/v1/sessions/{id}/subagents/{childSessionID}/events",
    {
      params: {
        path: {
          id: sessionId ?? "",
          childSessionID: childSessionId,
        },
        query: { limit: SESSION_HISTORY_PAGE_LIMIT },
      },
    },
    {
      enabled,
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      retry: false,
    }
  )
  const fetchNextPage = query.fetchNextPage
  const hasNextPage = query.hasNextPage
  const isFetchingNextPage = query.isFetchingNextPage

  useEffect(() => {
    if (!enabled || !hasNextPage || isFetchingNextPage) return
    void fetchNextPage()
  }, [enabled, fetchNextPage, hasNextPage, isFetchingNextPage])

  const events = useMemo(
    () =>
      sessionHistoryPagesToEvents([
        { data: run?.events ?? [] },
        ...(query.data?.pages ?? []),
      ]),
    [query.data?.pages, run?.events]
  )

  return {
    events,
    failed: enabled && query.isError,
    retry: query.refetch,
  }
}
