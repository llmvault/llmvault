"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowDown01Icon,
  ArrowUp01Icon,
  Loading03Icon,
  Search01Icon,
} from "@hugeicons/core-free-icons"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
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

export default function SessionsPage() {
  const [search, setSearch] = useState("")
  const [selectedSessionID, setSelectedSessionID] = useState<string | null>(null)
  const eventsScrollRef = useRef<HTMLDivElement | null>(null)
  const autoScrolledSessionRef = useRef<string | null>(null)

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })
  const employee = employeesQuery.data?.data?.[0]
  const employeeID = employee?.id ?? ""

  const sessionsQuery = useInfiniteQuery({
    queryKey: ["employee-sessions", employeeID, search.trim()],
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

  const sessions = useMemo(
    () => sessionsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? [],
    [sessionsQuery.data]
  )

  useEffect(() => {
    if (sessions.length === 0) {
      setSelectedSessionID(null)
      return
    }
    if (!selectedSessionID || !sessions.some((session) => session.id === selectedSessionID)) {
      setSelectedSessionID(sessions[0].id)
    }
  }, [selectedSessionID, sessions])

  const selectedSession = sessions.find((session) => session.id === selectedSessionID)

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

  useEffect(() => {
    const node = eventsScrollRef.current
    if (
      !node ||
      !selectedSessionID ||
      events.length === 0 ||
      autoScrolledSessionRef.current === selectedSessionID
    ) {
      return
    }
    node.scrollTop = node.scrollHeight
    autoScrolledSessionRef.current = selectedSessionID
  }, [selectedSessionID, events.length])

  const handleEventsScroll = useCallback(
    (event: React.UIEvent<HTMLDivElement>) => {
      if (
        event.currentTarget.scrollTop < 96 &&
        eventsQuery.hasNextPage &&
        !eventsQuery.isFetchingNextPage
      ) {
        eventsQuery.fetchNextPage()
      }
    },
    [eventsQuery]
  )

  return (
    <div className="flex min-h-0 flex-1 border-t border-border bg-background">
      <aside className="flex w-[360px] shrink-0 flex-col border-r border-border bg-sidebar/40">
        <div className="border-b border-border p-4">
          <h1 className="font-heading text-xl font-medium text-foreground">
            Sessions
          </h1>
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
              <HugeiconsIcon icon={Loading03Icon} className="size-4 animate-spin" />
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
              onSelect={setSelectedSessionID}
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

        <div
          ref={eventsScrollRef}
          onScroll={handleEventsScroll}
          className="min-h-0 flex-1 overflow-y-auto px-6 py-5"
        >
          {eventsQuery.isFetchingNextPage ? (
            <div className="mb-4 flex items-center justify-center gap-2 text-xs text-muted-foreground">
              <HugeiconsIcon icon={Loading03Icon} className="size-3.5 animate-spin" />
              Loading earlier events
            </div>
          ) : null}

          {!selectedSession ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              Select a session.
            </div>
          ) : eventsQuery.isLoading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <HugeiconsIcon icon={Loading03Icon} className="size-4 animate-spin" />
              Loading events
            </div>
          ) : eventsQuery.isError ? (
            <ErrorLine message={eventsQuery.error.message} />
          ) : events.length === 0 ? (
            <p className="text-sm text-muted-foreground">No events recorded.</p>
          ) : (
            <div className="mx-auto flex w-full max-w-4xl flex-col gap-3">
              {events.map((event) => (
                <SessionEventRow key={event.id} event={event} />
              ))}
            </div>
          )}
        </div>

        <form
          className="border-t border-border bg-background px-6 py-4"
          onSubmit={(event) => event.preventDefault()}
        >
          <div className="mx-auto flex w-full max-w-4xl items-end gap-3">
            <Textarea
              placeholder="Message Hivy"
              className="max-h-32 min-h-12 flex-1 bg-input/10"
            />
            <Button type="button" size="sm" className="h-10">
              Send
            </Button>
          </div>
        </form>
      </section>
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
                    {formatShortTime(session.last_activity_at ?? session.updated_at ?? session.created_at)}
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
            {session.source || "manual"} · {session.source_resource_key || session.runtime_conversation_id}
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
          <div className="whitespace-pre-wrap text-sm leading-6">
            {text || event.event_type || "Message"}
          </div>
          <EventMeta event={event} compact />
        </div>
      </div>
    )
  }

  return <ExpandableEvent event={event} kind={kind} text={text} />
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
            <HugeiconsIcon icon={ArrowDown01Icon} className="size-4 group-aria-expanded:hidden" />
            <HugeiconsIcon icon={ArrowUp01Icon} className="hidden size-4 group-aria-expanded:block" />
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
    const label = dateGroup(session.last_activity_at ?? session.updated_at ?? session.created_at)
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
  const startDate = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  const diffDays = Math.floor((startToday.getTime() - startDate.getTime()) / 86400000)
  if (diffDays === 0) return "Today"
  if (diffDays === 1) return "Yesterday"
  if (diffDays < 7) return "This week"
  return new Intl.DateTimeFormat(undefined, { month: "long", year: "numeric" }).format(date)
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
  if (type === "user.message.received" || type === "message_received") return "user"
  if (type === "agent.message.sent" || type === "response_completed") return "assistant"
  if (type.includes("thinking")) return "thinking"
  if (type.includes("tool.result") || type === "tool_result") return "tool_result"
  if (type.includes("tool.call") || type === "tool_call") return "tool_call"
  if (type.includes("error") || payloadString(event.payload, "error")) return "error"
  if (type.includes("token")) return "token"
  return "system"
}

function eventLabel(event: EmployeeSessionEvent, kind: ReturnType<typeof eventKind>) {
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
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) return ""
  const value = (payload as Record<string, unknown>)[key]
  return typeof value === "string" ? value : ""
}

function nestedPayloadString(payload: unknown, path: string[]) {
  let value = payload
  for (const key of path) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return ""
    value = (value as Record<string, unknown>)[key]
  }
  return typeof value === "string" ? value : ""
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
