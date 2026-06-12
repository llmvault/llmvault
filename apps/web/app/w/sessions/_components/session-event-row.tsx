"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  ArrowUp01Icon,
  Loading03Icon,
} from "@hugeicons/core-free-icons"
import { Streamdown } from "streamdown"
import { Badge } from "@/components/ui/badge"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  type EmployeeSessionEvent,
  eventKind,
  eventLabel,
  eventText,
  liveAssistantEventType,
  payloadString,
  toolEventCompleted,
} from "@/lib/sessions/normalize"
import type { StreamState } from "@/lib/sessions/types"
import { cn } from "@/lib/utils"
import { formatDateTime } from "./session-utils"

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
