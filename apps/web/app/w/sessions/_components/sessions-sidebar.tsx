"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Loading03Icon,
  Search01Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { EmployeeSession, SessionSegment } from "@/lib/sessions/types"
import { cn } from "@/lib/utils"
import { ErrorLine } from "./error-line"
import {
  groupSessions,
  sessionSegments,
  sessionTitle,
  formatShortTime,
} from "./session-utils"

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
