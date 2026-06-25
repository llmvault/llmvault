import type { GoSessionStreamReplayMode } from "@/app/w/(chat)/_lib/go-session-stream"
import {
  eventTurnID,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history-event-utils"

const sandboxOwnedTurnEventTypes = new Set([
  "turn_started",
  "thinking",
  "token",
  "tool_call",
  "tool_result",
  "plan_updated",
  "final",
  "error",
  "turn_completed",
  "turn_failed",
  "turn_interrupted",
  "done",
  "model_usage",
  "model_request_started",
  "model_request_completed",
  "question_requested",
  "question_answered",
  "session_waiting",
  "subagent_started",
  "subagent_completed",
  "subagent_errored",
])

export function replayModeForLoadedSession(activeTurnID?: string) {
  return activeTurnID
    ? ({
        mode: "from_turn_id_follow",
        turnId: activeTurnID,
      } satisfies GoSessionStreamReplayMode)
    : ({ mode: "none" } satisfies GoSessionStreamReplayMode)
}

export function suppressBackendEventsForLiveTurn(
  events: SessionEventResponse[],
  activeTurnID?: string
) {
  return suppressBackendEventsForLiveTurns(events, [activeTurnID])
}

export function suppressBackendEventsForLiveTurns(
  events: SessionEventResponse[],
  turnIDs: Iterable<string | undefined>
) {
  const suppressedTurnIDs = new Set(
    [...turnIDs].map((turnID) => turnID?.trim()).filter(Boolean)
  )
  if (suppressedTurnIDs.size === 0 || events.length === 0) return events
  const next = events.filter(
    (event) =>
      !suppressedTurnIDs.has(eventTurnID(event)) ||
      !sandboxOwnedTurnEventTypes.has(event.event_type ?? "")
  )
  return next.length === events.length ? events : next
}

export function eventTurnIDs(events: SessionEventResponse[]) {
  const turnIDs = new Set<string>()
  for (const event of events) {
    const turnID = eventTurnID(event).trim()
    if (turnID) turnIDs.add(turnID)
  }
  return [...turnIDs]
}

export type TerminalTurnOutcome = "completed" | "stopped" | "failed"

export function terminalOutcomeForTurnEvents(
  events: SessionEventResponse[],
  turnID?: string
): TerminalTurnOutcome | undefined {
  const targetTurnID = turnID?.trim()
  if (!targetTurnID) return undefined
  let outcome: TerminalTurnOutcome | undefined
  for (const event of events) {
    if (eventTurnID(event).trim() !== targetTurnID) continue
    switch (event.event_type) {
      case "turn_failed":
      case "error":
        outcome = "failed"
        break
      case "turn_interrupted":
        outcome = "stopped"
        break
      case "final":
      case "turn_completed":
      case "done":
        outcome ??= "completed"
        break
    }
  }
  return outcome
}
