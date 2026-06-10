"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { useInfiniteQuery } from "@tanstack/react-query"
import { fetchEventSource } from "@microsoft/fetch-event-source"
import ScrollToBottom, {
  useObserveScrollPosition,
} from "react-scroll-to-bottom"
import { Streamdown } from "streamdown"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Alert02Icon,
  ArrowDown01Icon,
  ArrowUp01Icon,
  Loading03Icon,
  Search01Icon,
  SlackIcon,
} from "@hugeicons/core-free-icons"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"

type Paginated<T> = {
  data?: T[]
  next_cursor?: string | null
  has_more?: boolean
}

type EmployeeSession = {
  id: string
  runtime_conversation_id?: string
  source?: string
  source_resource_key?: string
  status?: string
  name?: string
  event_count?: number
  created_at?: string
  updated_at?: string
  last_activity_at?: string
}

type EmployeeSessionEvent = {
  id: string
  employee_session_id?: string
  runtime_session_id?: string
  event_id?: string
  event_type?: string
  source?: string
  mode?: string
  specialist_slug?: string
  specialist_task_id?: string
  sequence_number?: number
  payload?: unknown
  event_at?: string
  created_at?: string
}

type SessionSegment = "web" | "slack"

type LocalMessage = {
  id: string
  sessionID: string
  text: string
  createdAt: string
}

type StreamState = {
  text: string
  isStreaming: boolean
  events: EmployeeSessionEvent[]
  error?: string
}

type SendSessionMessageResponse = {
  created?: boolean
  employee_session_id?: string
  stream_url?: string
  response_stream_url?: string
  runtime_session_id?: string
  source?: string
  source_resource_key?: string
  runtime_conversation_id?: string
}

const sessionSegments: Array<{
  id: SessionSegment
  label: string
  source: string
  icon?: typeof SlackIcon
}> = [
  { id: "web", label: "Web", source: "web" },
  { id: "slack", label: "Slack", source: "gateway", icon: SlackIcon },
]

function segmentFromParam(value: string | null): SessionSegment {
  return value === "slack" ? "slack" : "web"
}

export default function SessionsPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const querySessionID = searchParams.get("session")
  const [search, setSearch] = useState("")
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
  const [streams, setStreams] = useState<Record<string, StreamState>>({})
  const streamControllersRef = useRef<Record<string, AbortController>>({})

  const selectedSegmentConfig = sessionSegments.find(
    (segment) => segment.id === selectedSegment
  )
  const selectedSource = selectedSegmentConfig?.source ?? "web"

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })
  const employee = employeesQuery.data?.data?.[0]
  const employeeID = employee?.id ?? ""

  const sessionsQuery = useInfiniteQuery({
    queryKey: ["employee-sessions", employeeID, selectedSource, search.trim()],
    enabled: Boolean(employeeID),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/v1/employees/{id}/sessions", {
        params: {
          path: { id: employeeID },
          query: {
            limit: 30,
            cursor: pageParam,
            q: search.trim() || undefined,
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
  const streamPersisted = useMemo(() => {
    if (!selectedStream?.text) return false
    return events.some(
      (event) =>
        eventKind(event) === "assistant" &&
        eventText(event).trim() === selectedStream.text.trim()
    )
  }, [events, selectedStream])
  const hasLocalStream = selectedStream
    ? selectedStream.isStreaming ||
      Boolean(selectedStream.text) ||
      selectedStream.events.length > 0 ||
      Boolean(selectedStream.error)
    : false
  const hasVisibleMessages =
    events.length > 0 || selectedPendingMessages.length > 0 || hasLocalStream

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
    setStreams((current) => {
      const next = { ...current }
      delete next[selectedSessionID]
      return next
    })
  }, [selectedSessionID, selectedStream, streamPersisted])

  const navigateToSession = useCallback(
    (sessionID: string, segment: SessionSegment = selectedSegment) => {
      setSelectedSessionID(sessionID)
      router.push(`/w/sessions?source=${segment}&session=${sessionID}`)
    },
    [router, selectedSegment]
  )

  const startSessionStream = useCallback(
    async (sessionID: string, sessionStreamURL?: string) => {
      if (!sessionStreamURL) return

      streamControllersRef.current[sessionID]?.abort()
      const controller = new AbortController()
      streamControllersRef.current[sessionID] = controller
      setStreams((current) => ({
        ...current,
        [sessionID]: { text: "", events: [], isStreaming: true },
      }))

      let buffer = ""
      let sequence = 0
      const streamURL = proxiedStreamURL(sessionStreamURL)

      try {
        await fetchEventSource(streamURL, {
          method: "GET",
          headers: { Accept: "text/event-stream" },
          credentials: "include",
          signal: controller.signal,
          openWhenHidden: true,
          async onopen(response) {
            const contentType = response.headers.get("content-type") ?? ""
            if (response.ok && contentType.includes("text/event-stream")) return
            let message = `Stream failed with HTTP ${response.status}`
            try {
              const envelope = (await response.json()) as { error?: string }
              if (envelope.error) message = envelope.error
            } catch {
              /* non-JSON stream error */
            }
            throw new Error(message)
          },
          onmessage(event) {
            if (!event.data) return
            const frame = parseStreamFrame(event.data)
            if (!frame) return

            if (event.event === "token" && typeof frame.text === "string") {
              buffer += frame.text
              setStreams((current) => ({
                ...current,
                [sessionID]: {
                  ...(current[sessionID] ?? {
                    events: [],
                    isStreaming: true,
                  }),
                  text: buffer,
                  isStreaming: true,
                },
              }))
              return
            }
            if (event.event === "final" && typeof frame.text === "string") {
              buffer = frame.text
              setStreams((current) => ({
                ...current,
                [sessionID]: {
                  ...(current[sessionID] ?? {
                    events: [],
                    isStreaming: true,
                  }),
                  text: buffer,
                  isStreaming: true,
                },
              }))
              return
            }
            if (event.event === "error") {
              const message =
                typeof frame.message === "string"
                  ? frame.message
                  : "The response stream failed."
              setStreams((current) => ({
                ...current,
                [sessionID]: {
                  ...(current[sessionID] ?? { events: [] }),
                  text: buffer,
                  isStreaming: false,
                  error: message,
                },
              }))
              return
            }
            if (event.event === "done") {
              controller.abort()
              return
            }
            sequence += 1
            const liveEvent = streamFrameToSessionEvent(
              sessionID,
              event.event,
              frame,
              sequence
            )
            setStreams((current) => {
              const existing = current[sessionID] ?? {
                text: buffer,
                events: [],
                isStreaming: true,
              }
              return {
                ...current,
                [sessionID]: {
                  ...existing,
                  text: existing.text || buffer,
                  events: [...existing.events, liveEvent],
                  isStreaming: true,
                },
              }
            })
          },
          onerror(error) {
            throw error
          },
        })
      } catch (error) {
        if (!controller.signal.aborted) {
          const message =
            error instanceof Error
              ? error.message
              : "The response stream failed."
          setStreams((current) => ({
            ...current,
            [sessionID]: {
              ...(current[sessionID] ?? { events: [] }),
              text: buffer,
              isStreaming: false,
              error: message,
            },
          }))
        }
      } finally {
        if (streamControllersRef.current[sessionID] === controller) {
          delete streamControllersRef.current[sessionID]
        }
        setStreams((current) => ({
          ...current,
          [sessionID]: {
            ...(current[sessionID] ?? { events: [] }),
            text: current[sessionID]?.text ?? buffer,
            isStreaming: false,
          },
        }))
      }
    },
    []
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
        startSessionStream(
          nextSessionID,
          result.stream_url ?? result.response_stream_url
        )
        return result
      } finally {
        setIsSending(false)
      }
    },
    [employeeID, navigateToSession, startSessionStream]
  )

  useEffect(() => {
    return () => {
      for (const controller of Object.values(streamControllersRef.current)) {
        controller.abort()
      }
      streamControllersRef.current = {}
    }
  }, [])

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
      <aside className="flex w-[360px] shrink-0 flex-col border-r border-border bg-sidebar/40">
        <div className="border-b border-border p-4">
          <div className="flex items-center justify-between gap-3">
            <h1 className="font-heading text-xl font-medium text-foreground">
              Sessions
            </h1>
            {selectedSegment === "web" ? (
              <Button
                type="button"
                size="sm"
                className="h-8 gap-1.5"
                onClick={() => setNewChatOpen(true)}
              >
                <HugeiconsIcon icon={Add01Icon} className="size-3.5" />
                New chat
              </Button>
            ) : null}
          </div>
          <div className="mt-4 grid grid-cols-2 rounded-md border border-border bg-muted/30 p-1">
            {sessionSegments.map((segment) => {
              const active = selectedSegment === segment.id
              return (
                <button
                  key={segment.id}
                  type="button"
                  onClick={() => handleSelectSegment(segment.id)}
                  className={cn(
                    "flex h-8 items-center justify-center gap-1.5 rounded-sm text-sm font-medium transition-colors",
                    active
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  {segment.icon ? (
                    <HugeiconsIcon icon={segment.icon} className="size-3.5" />
                  ) : null}
                  {segment.label}
                </button>
              )
            })}
          </div>
          <div className="relative mt-4">
            <HugeiconsIcon
              icon={Search01Icon}
              className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search sessions"
              className="h-10 pl-9"
            />
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-3 py-4">
          {sessionsQuery.isLoading || employeesQuery.isLoading ? (
            <div className="flex items-center gap-2 px-2 py-3 text-sm text-muted-foreground">
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-4 animate-spin"
              />
              Loading sessions
            </div>
          ) : sessionsQuery.isError ? (
            <ErrorLine message={sessionsQuery.error.message} />
          ) : sessions.length === 0 ? (
            <p className="px-2 py-3 text-sm text-muted-foreground">
              No sessions found.
            </p>
          ) : (
            <SessionGroups
              sessions={sessions}
              selectedSessionID={selectedSessionID}
              onSelect={(id) => navigateToSession(id)}
            />
          )}

          {sessionsQuery.hasNextPage ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-3 w-full"
              loading={sessionsQuery.isFetchingNextPage}
              onClick={() => sessionsQuery.fetchNextPage()}
            >
              Load more
            </Button>
          ) : null}
        </div>
      </aside>

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
              {events.map((event) => (
                <SessionEventRow key={event.id} event={event} />
              ))}
              {selectedPendingMessages.map((message) => (
                <SessionEventRow
                  key={message.id}
                  event={localUserMessageToEvent(message)}
                />
              ))}
              {selectedStream?.events.map((event) => (
                <SessionEventRow key={event.id} event={event} />
              ))}
              {selectedStream && hasLocalStream ? (
                <AssistantStreamRow stream={selectedStream} />
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

function SessionGroups({
  sessions,
  selectedSessionID,
  onSelect,
}: {
  sessions: EmployeeSession[]
  selectedSessionID: string | null
  onSelect: (id: string) => void
}) {
  const groups = groupSessions(sessions)
  return (
    <div className="space-y-5">
      {groups.map((group) => (
        <div key={group.label}>
          <p className="px-2 pb-1.5 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
            {group.label}
          </p>
          <div className="space-y-1">
            {group.sessions.map((session) => (
              <button
                key={session.id}
                type="button"
                onClick={() => onSelect(session.id)}
                className={cn(
                  "w-full rounded-md px-3 py-2 text-left transition-colors hover:bg-muted",
                  selectedSessionID === session.id && "bg-muted text-foreground"
                )}
              >
                <div className="flex items-center justify-between gap-2">
                  <p className="truncate text-sm font-medium text-foreground">
                    {sessionTitle(session)}
                  </p>
                  <span className="shrink-0 text-[11px] text-muted-foreground">
                    {formatShortTime(
                      session.last_activity_at ??
                        session.updated_at ??
                        session.created_at
                    )}
                  </span>
                </div>
                <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                  <span className="truncate">{session.source || "manual"}</span>
                  <span>·</span>
                  <span>{session.event_count ?? 0} events</span>
                </div>
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function SessionHeader({ session }: { session?: EmployeeSession }) {
  return (
    <div className="flex min-h-16 items-center justify-between gap-4 border-b border-border px-6">
      <div className="min-w-0">
        <h2 className="truncate font-heading text-lg font-medium text-foreground">
          {session ? sessionTitle(session) : "Session"}
        </h2>
        {session ? (
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {session.source || "manual"} ·{" "}
            {session.source_resource_key || session.runtime_conversation_id}
          </p>
        ) : null}
      </div>
      {session ? (
        <div className="flex shrink-0 items-center gap-2">
          <Badge variant="outline">{session.status || "active"}</Badge>
          <Badge variant="secondary">{session.event_count ?? 0} events</Badge>
        </div>
      ) : null}
    </div>
  )
}

function NewChatDialog({
  open,
  prompt,
  loading,
  onOpenChange,
  onPromptChange,
  onSubmit,
}: {
  open: boolean
  prompt: string
  loading: boolean
  onOpenChange: (open: boolean) => void
  onPromptChange: (prompt: string) => void
  onSubmit: () => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>New web chat</DialogTitle>
          <DialogDescription>
            Start a fresh web session with Hivy.
          </DialogDescription>
        </DialogHeader>
        <Textarea
          value={prompt}
          onChange={(event) => onPromptChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
              event.preventDefault()
              onSubmit()
            }
          }}
          placeholder="Ask Hivy a question"
          className="min-h-32"
          autoFocus
        />
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            loading={loading}
            disabled={prompt.trim() === ""}
            onClick={onSubmit}
          >
            Start chat
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function EventsScrollObserver({ onNearTop }: { onNearTop: () => void }) {
  useObserveScrollPosition(
    useCallback(
      ({ scrollTop }: { scrollTop: number }) => {
        if (scrollTop < 96) onNearTop()
      },
      [onNearTop]
    )
  )
  return null
}

function SessionEventRow({ event }: { event: EmployeeSessionEvent }) {
  const kind = eventKind(event)
  const text = eventText(event)
  if (kind === "user" || kind === "assistant") {
    return (
      <div
        className={cn(
          "flex",
          kind === "user" ? "justify-end" : "justify-start"
        )}
      >
        <div
          className={cn(
            "max-w-[78%] rounded-md border px-4 py-3",
            kind === "user"
              ? "border-primary/20 bg-primary/10 text-foreground"
              : "border-border bg-card text-foreground"
          )}
        >
          {kind === "assistant" ? (
            <Streamdown className="text-sm leading-6" mode="static">
              {text || event.event_type || "Message"}
            </Streamdown>
          ) : (
            <div className="text-sm leading-6 whitespace-pre-wrap">
              {text || event.event_type || "Message"}
            </div>
          )}
          <EventMeta event={event} compact />
        </div>
      </div>
    )
  }

  return <ExpandableEvent event={event} kind={kind} text={text} />
}

function AssistantStreamRow({ stream }: { stream: StreamState }) {
  return (
    <div className="flex justify-start">
      <div className="max-w-[78%] rounded-md border border-border bg-card px-4 py-3 text-foreground">
        {stream.text ? (
          <Streamdown
            className="text-sm leading-6"
            mode="streaming"
            isAnimating={stream.isStreaming}
            caret={stream.isStreaming ? "block" : undefined}
          >
            {stream.text}
          </Streamdown>
        ) : (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <HugeiconsIcon
              icon={Loading03Icon}
              className="size-4 animate-spin"
            />
            Hivy is responding
          </div>
        )}
        {stream.error ? (
          <p className="mt-2 text-xs text-destructive">{stream.error}</p>
        ) : null}
      </div>
    </div>
  )
}

function ExpandableEvent({
  event,
  kind,
  text,
}: {
  event: EmployeeSessionEvent
  kind: ReturnType<typeof eventKind>
  text: string
}) {
  return (
    <Collapsible>
      <div
        className={cn(
          "rounded-md border bg-card",
          kind === "error" ? "border-destructive/30" : "border-border"
        )}
      >
        <CollapsibleTrigger className="group flex w-full items-center justify-between gap-3 px-4 py-3 text-left">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <Badge variant={kind === "error" ? "destructive" : "outline"}>
                {eventLabel(event, kind)}
              </Badge>
              <span className="truncate text-sm text-foreground">
                {text || event.event_type || "Event"}
              </span>
            </div>
            <EventMeta event={event} />
          </div>
          <span className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground">
            <HugeiconsIcon
              icon={ArrowDown01Icon}
              className="size-4 group-aria-expanded:hidden"
            />
            <HugeiconsIcon
              icon={ArrowUp01Icon}
              className="hidden size-4 group-aria-expanded:block"
            />
          </span>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <pre className="max-h-80 overflow-auto border-t border-border bg-muted/30 p-4 text-xs leading-5 text-muted-foreground">
            {JSON.stringify(event.payload ?? {}, null, 2)}
          </pre>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

function EventMeta({
  event,
  compact = false,
}: {
  event: EmployeeSessionEvent
  compact?: boolean
}) {
  return (
    <div
      className={cn(
        "mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground",
        compact && "justify-end"
      )}
    >
      <span>{formatDateTime(event.event_at ?? event.created_at)}</span>
      {event.sequence_number ? <span>seq {event.sequence_number}</span> : null}
      {event.source ? <span>{event.source}</span> : null}
      {event.specialist_slug ? <span>{event.specialist_slug}</span> : null}
    </div>
  )
}

function ErrorLine({ message }: { message: string }) {
  return (
    <div className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
      <HugeiconsIcon icon={Alert02Icon} className="size-4" />
      {message}
    </div>
  )
}

function groupSessions(sessions: EmployeeSession[]) {
  const groups = new Map<string, EmployeeSession[]>()
  for (const session of sessions) {
    const label = dateGroup(
      session.last_activity_at ?? session.updated_at ?? session.created_at
    )
    groups.set(label, [...(groups.get(label) ?? []), session])
  }
  return Array.from(groups.entries()).map(([label, groupSessions]) => ({
    label,
    sessions: groupSessions,
  }))
}

function dateGroup(value?: string) {
  if (!value) return "Unknown"
  const date = new Date(value)
  const now = new Date()
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startDate = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate()
  )
  const diffDays = Math.floor(
    (startToday.getTime() - startDate.getTime()) / 86400000
  )
  if (diffDays === 0) return "Today"
  if (diffDays === 1) return "Yesterday"
  if (diffDays < 7) return "This week"
  return new Intl.DateTimeFormat(undefined, {
    month: "long",
    year: "numeric",
  }).format(date)
}

function sessionTitle(session: EmployeeSession) {
  return (
    session.name ||
    session.source_resource_key ||
    session.runtime_conversation_id ||
    "Untitled session"
  )
}

function eventKind(event: EmployeeSessionEvent) {
  const type = event.event_type ?? ""
  if (type === "user.message.received" || type === "message_received") {
    return "user"
  }
  if (type === "agent.message.sent" || type === "response_completed") {
    return "assistant"
  }
  if (type.includes("thinking")) return "thinking"
  if (type.includes("tool.result") || type === "tool_result") {
    return "tool_result"
  }
  if (type.includes("tool.call") || type === "tool_call") return "tool_call"
  if (type.includes("error") || payloadString(event.payload, "error")) {
    return "error"
  }
  if (type.includes("token")) return "token"
  return "system"
}

function eventLabel(
  event: EmployeeSessionEvent,
  kind: ReturnType<typeof eventKind>
) {
  if (kind === "thinking") return "Thinking"
  if (kind === "tool_call") return "Tool call"
  if (kind === "tool_result") return "Tool result"
  if (kind === "token") return "Token"
  if (kind === "error") return "Error"
  return event.event_type || "Event"
}

function eventText(event: EmployeeSessionEvent) {
  const payload = event.payload
  return (
    payloadString(payload, "text") ||
    payloadString(payload, "message") ||
    payloadString(payload, "content") ||
    payloadString(payload, "markdown") ||
    payloadString(payload, "result_summary") ||
    nestedPayloadString(payload, ["agent_event", "text"]) ||
    nestedPayloadString(payload, ["agent_event", "content"]) ||
    nestedPayloadString(payload, ["error", "message"]) ||
    payloadString(payload, "tool") ||
    ""
  )
}

function payloadString(payload: unknown, key: string) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return ""
  }
  const value = (payload as Record<string, unknown>)[key]
  return typeof value === "string" ? value : ""
}

function nestedPayloadString(payload: unknown, path: string[]) {
  let value = payload
  for (const key of path) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return ""
    }
    value = (value as Record<string, unknown>)[key]
  }
  return typeof value === "string" ? value : ""
}

function localUserMessageToEvent(message: LocalMessage): EmployeeSessionEvent {
  return {
    id: message.id,
    employee_session_id: message.sessionID,
    event_type: "user.message.received",
    source: "web",
    payload: { text: message.text },
    event_at: message.createdAt,
    created_at: message.createdAt,
  }
}

function streamFrameToSessionEvent(
  sessionID: string,
  eventType: string,
  payload: Record<string, unknown>,
  sequence: number
): EmployeeSessionEvent {
  const now = new Date().toISOString()
  return {
    id: `live-${sessionID}-${sequence}-${eventType}`,
    employee_session_id: sessionID,
    event_type: eventType,
    source: payloadString(payload, "source") || "web",
    sequence_number: sequence,
    payload,
    event_at: now,
    created_at: now,
  }
}

function proxiedStreamURL(url: string) {
  if (url.startsWith("/api/proxy")) return url
  if (url.startsWith("/")) return `/api/proxy${url}`
  return url
}

function parseStreamFrame(data: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(data)
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    return null
  }
  return null
}

function formatDateTime(value?: string) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(date)
}

function formatShortTime(value?: string) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).format(date)
}
