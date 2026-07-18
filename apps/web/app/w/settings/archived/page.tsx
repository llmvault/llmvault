"use client"

import { useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  CHAT_QUERY_STALE_TIME_MS,
  invalidateSessionListQueries,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  dedupeSessions,
  sessionActivityLabel,
  sessionDisplayName,
} from "@/app/w/(chat)/_lib/sidebar-data"

const PAGE_LIMIT = 50

export default function ArchivedSettingsPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const [restoringID, setRestoringID] = useState<string | null>(null)
  const sessionsQuery = $api.useInfiniteQuery(
    "get",
    "/v1/sessions",
    {
      params: {
        query: { status: "archived", limit: PAGE_LIMIT, sort: "activity" },
      },
    },
    {
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const restoreSession = $api.useMutation("patch", "/v1/sessions/{id}")
  const sessions = useMemo(
    () =>
      dedupeSessions(
        sessionsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []
      ),
    [sessionsQuery.data?.pages]
  )

  function restore(id: string) {
    setRestoringID(id)
    restoreSession.mutate(
      { params: { path: { id } }, body: { status: "active" } },
      {
        onSuccess: () => {
          invalidateSessionListQueries(queryClient)
          void sessionsQuery.refetch()
          toast.success("Chat restored")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not restore chat")),
        onSettled: () => setRestoringID(null),
      }
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-lg font-semibold text-foreground">
          Archived chats
        </h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Restore an archived chat to bring it back to your sidebar.
        </p>
      </div>
      {sessionsQuery.isLoading ? (
        <div className="flex min-h-40 items-center justify-center">
          <Spinner size="sm" />
        </div>
      ) : sessionsQuery.isError ? (
        <p className="text-muted-foreground text-sm">
          Could not load archived chats.
        </p>
      ) : sessions.length ? (
        <div className="rounded-2xl border border-border bg-surface">
          {sessions.map((session, index) => {
            const id = session.id ?? ""
            return (
              <div
                key={id}
                className={`flex items-center gap-4 px-4 py-3 ${index ? "border-t border-border" : ""}`}
              >
                <button
                  type="button"
                  className="min-w-0 flex-1 text-left"
                  onClick={() => router.push(`/w/sessions/${id}`)}
                >
                  <span className="block truncate text-sm font-medium">
                    {sessionDisplayName(session)}
                  </span>
                  <span className="text-xs text-muted">
                    {sessionActivityLabel(session)}
                  </span>
                </button>
                <Button
                  variant="tertiary"
                  size="sm"
                  isDisabled={restoringID === id}
                  onPress={() => restore(id)}
                >
                  {restoringID === id ? (
                    <Spinner color="current" size="sm" />
                  ) : null}
                  Restore
                </Button>
              </div>
            )
          })}
        </div>
      ) : (
        <div className="flex min-h-56 flex-col items-center justify-center gap-3 rounded-2xl border border-border bg-surface px-6 text-center">
          <AppIcon icon="archive" className="h-6 w-6 text-muted" />
          <p className="text-sm text-muted">No archived chats</p>
        </div>
      )}
    </div>
  )
}
