"use client"

import { useCallback } from "react"
import { useObserveScrollPosition } from "react-scroll-to-bottom"
import { Streamdown } from "streamdown"
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
import { Input } from "@/components/ui/input"
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
import { Textarea } from "@/components/ui/textarea"
import {
  type EmployeeSessionEvent,
  eventKind,
  eventLabel,
  eventText,
  liveAssistantEventType,
  payloadString,
  toolEventCompleted,
} from "@/lib/sessions/normalize"
import type {
  EmployeeSession,
  SessionSegment,
  StreamState,
} from "@/lib/sessions/types"
import { cn } from "@/lib/utils"

const sessionSegments: Array<{
  id: SessionSegment
  label: string
  source: string
  icon?: typeof SlackIcon
}> = [
  { id: "web", label: "Web", source: "web" },
  { id: "slack", label: "Slack", source: "gateway", icon: SlackIcon },
]

export function SessionsSidebar({
  search,
  onSearchChange,
  selectedSegment,
  onSelectSegment,
  onNewChat,
  sessions,
  selectedSessionID,
  onSelectSession,
  isLoading,
  isError,
  errorMessage,
  hasNextPage,
  isFetchingNextPage,
  onLoadMore,
}: {
  search: string
  onSearchChange: (value: string) => void
  selectedSegment: SessionSegment
  onSelectSegment: (segment: SessionSegment) => void
  onNewChat: () => void
  sessions: EmployeeSession[]
  selectedSessionID: string | null
  onSelectSession: (id: string) => void
  isLoading: boolean
  isError: boolean
  errorMessage?: string
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onLoadMore: () => void
}) {
  return (
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
              onClick={onNewChat}
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
                onClick={() => onSelectSegment(segment.id)}
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
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder="Search sessions"
            className="h-10 pl-9"
          />
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-4">
        {isLoading ? (
          <div className="flex items-center gap-2 px-2 py-3 text-sm text-muted-foreground">
            <HugeiconsIcon icon={Loading03Icon} className="size-4 animate-spin" />
            Loading sessions
          </div>
        ) : isError ? (
          <ErrorLine message={errorMessage ?? "Failed to load sessions"} />
        ) : sessions.length === 0 ? (
          <p className="px-2 py-3 text-sm text-muted-foreground">
            No sessions found.
          </p>
        ) : (
          <SessionGroups
            sessions={sessions}
            selectedSessionID={selectedSessionID}
            onSelect={onSelectSession}
          />
        )}

        {hasNextPage ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3 w-full"
            loading={isFetchingNextPage}
            onClick={onLoadMore}
          >
            Load more
          </Button>
        ) : null}
      </div>
    </aside>
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

export function SessionHeader({ session }: { session?: EmployeeSession }) {
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

export function NewChatDialog({
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

export function EventsScrollObserver({
  onNearTop,
}: {
  onNearTop: () => void
}) {
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

export function SessionEventRow({ event }: { event: EmployeeSessionEvent }) {
  const kind = eventKind(event)
  const text = eventText(event)
  const assistantStreaming =
    event.event_type === liveAssistantEventType &&
    payloadString(event.payload, "status") === "streaming"
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
            <Streamdown
              className="text-sm leading-6"
              mode={assistantStreaming ? "streaming" : "static"}
              isAnimating={assistantStreaming}
              caret={assistantStreaming ? "block" : undefined}
            >
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

export function AssistantStreamRow({ stream }: { stream: StreamState }) {
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
            <HugeiconsIcon icon={Loading03Icon} className="size-4 animate-spin" />
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
              {kind === "tool_call" && !toolEventCompleted(event) ? (
                <HugeiconsIcon
                  icon={Loading03Icon}
                  className="size-3.5 shrink-0 animate-spin text-muted-foreground"
                />
              ) : null}
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

export function ErrorLine({ message }: { message: string }) {
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
