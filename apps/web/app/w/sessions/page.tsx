"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query"
import ScrollToBottom from "react-scroll-to-bottom"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  type EmployeeSessionEvent,
  type LocalMessage,
  assistantTextMatchesStream,
  eventKind,
  eventText,
  localUserMessageToEvent,
  normalizeSessionEvents,
} from "@/lib/sessions/normalize"
import type {
  EmployeeSession,
  Paginated,
  SendSessionMessageResponse,
  SessionSegment,
} from "@/lib/sessions/types"
import { useDebouncedValue } from "@/lib/sessions/use-debounced-value"
import { useSessionStream } from "@/lib/sessions/use-session-stream"
import {
  AssistantStreamRow,
  ErrorLine,
  EventsScrollObserver,
  NewChatDialog,
  SessionEventRow,
  SessionHeader,
  SessionsSidebar,
} from "./components"

function segmentFromParam(value: string | null): SessionSegment {
  return value === "slack" ? "slack" : "web"
}

export default function SessionsPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const searchParams = useSearchParams()
  const querySessionID = searchParams.get("session")
  const [search, setSearch] = useState("")
  // Debounce the value that drives the query so a burst of keystrokes does not
  // fire one request + mint one infinite-query cache entry per character (P2-9).
  const debouncedSearch = useDebouncedValue(search.trim(), 300)
  const [selectedSegment, setSelectedSegment] = useState<SessionSegment>(() =>
    segmentFromParam(searchParams.get("source"))
  )
  const [selectedSessionID, setSelectedSessionID] = useState<string | null>(
    null
  )
  const [newChatOpen, setNewChatOpen] = useState(false)
  const [newChatPrompt, setNewChatPrompt] = useState("")
  const [composerText, setComposerText] = useState("")
  const [isSending, setIsSending] = useState(false)
  const [pendingMessages, setPendingMessages] = useState<
    Record<string, LocalMessage[]>
  >({})
  const [optimisticSessions, setOptimisticSessions] = useState<
    Record<string, EmployeeSession>
  >({})

  const selectedSource = selectedSegment === "slack" ? "gateway" : "web"

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })
  const employee = employeesQuery.data?.data?.[0]
  const employeeID = employee?.id ?? ""

  const sessionsQuery = useInfiniteQuery({
    queryKey: ["employee-sessions", employeeID, selectedSource, debouncedSearch],
    enabled: Boolean(employeeID),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/v1/employees/{id}/sessions", {
        params: {
          path: { id: employeeID },
          query: {
            limit: 30,
            cursor: pageParam,
            q: debouncedSearch || undefined,
            channel: selectedSource,
          },
        },
      })
      if (error) {
        throw new Error(extractErrorMessage(error, "Failed to load sessions"))
      }
      return data as Paginated<EmployeeSession>
    },
    getNextPageParam: (page) =>
      page.has_more ? (page.next_cursor ?? undefined) : undefined,
  })

  useEffect(() => {
    setSelectedSegment(segmentFromParam(searchParams.get("source")))
  }, [searchParams])

  const sessions = useMemo(() => {
    const loaded =
      sessionsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []
    const optimistic = Object.values(optimisticSessions).filter(
      (session) => session.source === selectedSource
    )
    const existing = new Set(loaded.map((session) => session.id))
    return [
      ...optimistic.filter((session) => !existing.has(session.id)),
      ...loaded,
    ]
  }, [optimisticSessions, selectedSource, sessionsQuery.data])

  useEffect(() => {
    if (querySessionID) {
      setSelectedSessionID(querySessionID)
      return
    }
    if (sessions.length === 0) {
      setSelectedSessionID(null)
      return
    }
    if (
      !selectedSessionID ||
      !sessions.some((session) => session.id === selectedSessionID)
    ) {
      setSelectedSessionID(sessions[0].id)
    }
  }, [querySessionID, selectedSessionID, sessions])

  const selectedSession =
    sessions.find((session) => session.id === selectedSessionID) ??
    (selectedSessionID ? optimisticSessions[selectedSessionID] : undefined)

  const eventsQuery = useInfiniteQuery({
    queryKey: ["employee-session-events", employeeID, selectedSessionID],
    enabled: Boolean(employeeID && selectedSessionID),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET(
        "/v1/employees/{id}/sessions/{sessionID}/events",
        {
          params: {
            path: { id: employeeID, sessionID: selectedSessionID ?? "" },
            query: { limit: 80, cursor: pageParam },
          },
        }
      )
      if (error) {
        throw new Error(extractErrorMessage(error, "Failed to load events"))
      }
      return data as Paginated<EmployeeSessionEvent>
    },
    getNextPageParam: (page) =>
      page.has_more ? (page.next_cursor ?? undefined) : undefined,
  })

  const events = useMemo(() => {
    const newestFirst =
      eventsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []
    return [...newestFirst].reverse()
  }, [eventsQuery.data])
  const displayEvents = useMemo(() => normalizeSessionEvents(events), [events])

  const refetchPersistedSession = useCallback(
    (sessionID: string) => {
      queryClient.invalidateQueries({
        queryKey: ["employee-session-events", employeeID, sessionID],
      })
      queryClient.invalidateQueries({
        queryKey: ["employee-sessions", employeeID],
      })
    },
    [employeeID, queryClient]
  )

  // The SSE-protocol half of this page lives in useSessionStream (P2-21): it
  // owns the live `streams` map, the abort controllers, and the reconnect loop.
  const { streams, startSessionStream, pruneStream } = useSessionStream(
    refetchPersistedSession
  )

  const selectedPendingMessages = useMemo(() => {
    if (!selectedSessionID) return []
    const persistedUserTexts = new Set(
      events.filter((event) => eventKind(event) === "user").map(eventText)
    )
    return (pendingMessages[selectedSessionID] ?? []).filter(
      (message) => !persistedUserTexts.has(message.text)
    )
  }, [events, pendingMessages, selectedSessionID])

  const selectedStream = selectedSessionID
    ? streams[selectedSessionID]
    : undefined
  const selectedStreamEvents = useMemo(
    () => normalizeSessionEvents(selectedStream?.events ?? []),
    [selectedStream?.events]
  )
  const streamPersisted = useMemo(() => {
    if (!selectedStream?.text) return false
    return events.some(
      (event) =>
        eventKind(event) === "assistant" &&
        assistantTextMatchesStream(eventText(event), selectedStream.text)
    )
  }, [events, selectedStream])
  const hasLocalStream = selectedStream
    ? selectedStream.isStreaming ||
      Boolean(selectedStream.text) ||
      selectedStreamEvents.length > 0 ||
      Boolean(selectedStream.error)
    : false
  const hasVisibleMessages =
    displayEvents.length > 0 ||
    selectedPendingMessages.length > 0 ||
    hasLocalStream

  const handleLoadEarlier = useCallback(() => {
    if (eventsQuery.hasNextPage && !eventsQuery.isFetchingNextPage) {
      eventsQuery.fetchNextPage()
    }
  }, [eventsQuery])

  useEffect(() => {
    if (
      !selectedSessionID ||
      !selectedStream ||
      selectedStream.isStreaming ||
      !streamPersisted
    ) {
      return
    }
    pruneStream(selectedSessionID)
  }, [pruneStream, selectedSessionID, selectedStream, streamPersisted])

  // Prune the pending-message and optimistic-session maps once the backend has
  // persisted the corresponding rows. Without this, both maps grew without
  // bound for the page's lifetime (P2-5): pending messages stayed even after
  // their persisted user event arrived, and optimistic sessions stayed even
  // after the real session loaded.
  const loadedSessionIDs = useMemo(
    () =>
      new Set(
        (sessionsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []).map(
          (session) => session.id
        )
      ),
    [sessionsQuery.data]
  )

  useEffect(() => {
    if (!selectedSessionID) return
    const persistedUserTexts = new Set(
      events.filter((event) => eventKind(event) === "user").map(eventText)
    )
    setPendingMessages((current) => {
      const pending = current[selectedSessionID]
      if (!pending) return current
      const remaining = pending.filter(
        (message) => !persistedUserTexts.has(message.text)
      )
      if (remaining.length === pending.length) return current
      const next = { ...current }
      if (remaining.length === 0) delete next[selectedSessionID]
      else next[selectedSessionID] = remaining
      return next
    })
  }, [events, selectedSessionID])

  useEffect(() => {
    setOptimisticSessions((current) => {
      const next: Record<string, EmployeeSession> = {}
      let changed = false
      for (const [id, session] of Object.entries(current)) {
        if (loadedSessionIDs.has(id)) {
          changed = true
          continue
        }
        next[id] = session
      }
      return changed ? next : current
    })
  }, [loadedSessionIDs])

  const navigateToSession = useCallback(
    (sessionID: string, segment: SessionSegment = selectedSegment) => {
      setSelectedSessionID(sessionID)
      router.push(`/w/sessions?source=${segment}&session=${sessionID}`)
    },
    [router, selectedSegment]
  )

  const sendMessage = useCallback(
    async (text: string, sessionID?: string | null) => {
      if (!employeeID) return null
      const trimmed = text.trim()
      if (!trimmed) return null

      setIsSending(true)
      try {
        const { data, error } = await api.POST(
          "/v1/employees/{id}/sessions/messages",
          {
            params: { path: { id: employeeID } },
            body: { text: trimmed, session_id: sessionID || undefined },
          }
        )
        if (error) {
          throw new Error(extractErrorMessage(error, "Failed to send message"))
        }

        const result = data as SendSessionMessageResponse
        const nextSessionID = result.employee_session_id
        if (!nextSessionID) {
          throw new Error("Session was not returned by the backend")
        }

        const now = new Date().toISOString()
        setPendingMessages((current) => ({
          ...current,
          [nextSessionID]: [
            ...(current[nextSessionID] ?? []),
            {
              id: `local-${Date.now()}`,
              sessionID: nextSessionID,
              text: trimmed,
              createdAt: now,
            },
          ],
        }))

        if (result.created) {
          setOptimisticSessions((current) => ({
            ...current,
            [nextSessionID]: {
              id: nextSessionID,
              source: "web",
              status: "active",
              name: trimmed,
              source_resource_key: result.source_resource_key,
              runtime_conversation_id: result.runtime_conversation_id,
              event_count: 1,
              created_at: now,
              updated_at: now,
              last_activity_at: now,
            },
          }))
        }

        navigateToSession(nextSessionID, "web")
        // startSessionStream is fire-and-forget: its own try/finally clears
        // isStreaming on any failure path, so .catch() here only silences the
        // unhandled-rejection warning — errors are already surfaced in the UI.
        startSessionStream(
          nextSessionID,
          result.stream_url ?? result.response_stream_url
        ).catch(() => undefined)
        return result
      } finally {
        setIsSending(false)
      }
    },
    [employeeID, navigateToSession, startSessionStream]
  )

  const handleCreateChat = useCallback(async () => {
    try {
      const result = await sendMessage(newChatPrompt)
      if (!result) return
      setNewChatOpen(false)
      setNewChatPrompt("")
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to start chat"
      )
    }
  }, [newChatPrompt, sendMessage])

  const handleSendFollowUp = useCallback(async () => {
    if (!selectedSessionID || selectedSegment !== "web") return
    try {
      const sent = await sendMessage(composerText, selectedSessionID)
      if (!sent) return
      setComposerText("")
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to send message"
      )
    }
  }, [composerText, selectedSegment, selectedSessionID, sendMessage])

  const handleSelectSegment = useCallback(
    (segment: SessionSegment) => {
      setSelectedSegment(segment)
      setSelectedSessionID(null)
      router.push(`/w/sessions?source=${segment}`)
    },
    [router]
  )

  const inputDisabled =
    !selectedSessionID ||
    selectedSegment !== "web" ||
    Boolean(selectedStream?.isStreaming) ||
    isSending

  return (
    <div className="flex min-h-0 flex-1 border-t border-border bg-background">
      <SessionsSidebar
        search={search}
        onSearchChange={setSearch}
        selectedSegment={selectedSegment}
        onSelectSegment={handleSelectSegment}
        onNewChat={() => setNewChatOpen(true)}
        sessions={sessions}
        selectedSessionID={selectedSessionID}
        onSelectSession={(id) => navigateToSession(id)}
        isLoading={sessionsQuery.isLoading || employeesQuery.isLoading}
        isError={sessionsQuery.isError}
        errorMessage={sessionsQuery.error?.message}
        hasNextPage={sessionsQuery.hasNextPage}
        isFetchingNextPage={sessionsQuery.isFetchingNextPage}
        onLoadMore={() => sessionsQuery.fetchNextPage()}
      />

      <section className="flex min-w-0 flex-1 flex-col bg-background">
        <SessionHeader session={selectedSession} />

        <ScrollToBottom
          className="min-h-0 flex-1"
          scrollViewClassName="px-6 py-5"
          followButtonClassName="hidden"
          initialScrollBehavior="auto"
        >
          <EventsScrollObserver onNearTop={handleLoadEarlier} />
          {eventsQuery.isFetchingNextPage ? (
            <div className="mb-4 flex items-center justify-center gap-2 text-xs text-muted-foreground">
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-3.5 animate-spin"
              />
              Loading earlier events
            </div>
          ) : null}

          {!selectedSession ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              Select a session.
            </div>
          ) : eventsQuery.isLoading && !hasVisibleMessages ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-4 animate-spin"
              />
              Loading events
            </div>
          ) : eventsQuery.isError ? (
            <ErrorLine message={eventsQuery.error.message} />
          ) : !hasVisibleMessages ? (
            <p className="text-sm text-muted-foreground">No events recorded.</p>
          ) : (
            <div className="mx-auto flex w-full max-w-4xl flex-col gap-3">
              {displayEvents.map((event) => (
                <SessionEventRow key={event.id} event={event} />
              ))}
              {selectedPendingMessages.map((message) => (
                <SessionEventRow
                  key={message.id}
                  event={localUserMessageToEvent(message)}
                />
              ))}
              {selectedStreamEvents.map((event) => (
                <SessionEventRow key={event.id} event={event} />
              ))}
              {selectedStream &&
              hasLocalStream &&
              selectedStreamEvents.length === 0 ? (
                <AssistantStreamRow stream={selectedStream} />
              ) : null}
              {selectedStream?.error && selectedStreamEvents.length > 0 ? (
                <ErrorLine message={selectedStream.error} />
              ) : null}
            </div>
          )}
        </ScrollToBottom>

        <form
          className="border-t border-border bg-background px-6 py-4"
          onSubmit={(event) => {
            event.preventDefault()
            handleSendFollowUp()
          }}
        >
          <div className="mx-auto flex w-full max-w-4xl items-end gap-3">
            <Textarea
              value={composerText}
              onChange={(event) => setComposerText(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault()
                  handleSendFollowUp()
                }
              }}
              disabled={inputDisabled}
              placeholder={
                selectedSegment !== "web"
                  ? "Slack sessions are read-only here"
                  : selectedStream?.isStreaming
                    ? "Hivy is responding"
                    : "Message Hivy"
              }
              className="max-h-32 min-h-12 flex-1 bg-input/10"
            />
            <Button
              type="submit"
              size="sm"
              className="h-10"
              loading={isSending}
              disabled={inputDisabled || composerText.trim() === ""}
            >
              Send
            </Button>
          </div>
        </form>
      </section>
      <NewChatDialog
        open={newChatOpen}
        prompt={newChatPrompt}
        loading={isSending}
        onOpenChange={setNewChatOpen}
        onPromptChange={setNewChatPrompt}
        onSubmit={handleCreateChat}
      />
    </div>
  )
}
