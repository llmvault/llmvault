"use client"

import { Badge } from "@/components/ui/badge"
import type { EmployeeSession } from "@/lib/sessions/types"
import { sessionTitle } from "./session-utils"

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
