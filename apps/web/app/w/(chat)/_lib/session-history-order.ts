import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"
import {
  eventTurnID,
  parseTimestamp,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history-event-utils"

export function isAutomatedUserBlock(block: ConversationBlock): boolean {
  return (
    block.type === "user" &&
    typeof block.source === "string" &&
    block.source !== "web"
  )
}

export function orderActiveTurnAfterLatestUserMessage(
  events: SessionEventResponse[],
  activeTurnID?: string,
  activeTurnStartedAt?: string
): SessionEventResponse[] {
  const turnID = activeTurnID?.trim()
  if (!turnID) return events

  const activeTurnEvents: SessionEventResponse[] = []
  const otherEvents: SessionEventResponse[] = []
  for (const event of events) {
    if (eventTurnID(event) === turnID && !isUserMessageEvent(event)) {
      activeTurnEvents.push(event)
    } else {
      otherEvents.push(event)
    }
  }
  if (activeTurnEvents.length === 0) return events

  const latestUserIndex = latestUserMessageIndex(otherEvents)
  if (latestUserIndex < 0) return events

  if (turnStartsAfterAnchor(activeTurnStartedAt, otherEvents[latestUserIndex])) {
    return events
  }

  return [
    ...otherEvents.slice(0, latestUserIndex + 1),
    ...activeTurnEvents,
    ...otherEvents.slice(latestUserIndex + 1),
  ]
}

function turnStartsAfterAnchor(
  activeTurnStartedAt: string | undefined,
  anchor: SessionEventResponse | undefined
) {
  const turnStart = parseTimestamp(activeTurnStartedAt ?? "")
  const anchorAt = parseTimestamp(anchor?.event_at ?? "")
  if (turnStart === undefined || anchorAt === undefined) return false
  return turnStart > anchorAt
}

function latestUserMessageIndex(events: SessionEventResponse[]) {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index]
    if (event && isUserMessageEvent(event)) return index
  }
  return -1
}

function isUserMessageEvent(event: SessionEventResponse) {
  return (
    event.event_type === "user.message" ||
    event.event_type === "user.message.received"
  )
}
