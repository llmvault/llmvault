import type { components } from "@/lib/api/schema"

export type SessionRuntimeStatus =
  | "idle"
  | "queued"
  | "streaming"
  | "waiting_for_user"
  | "stopped"
  | "failed"

export type SessionLastTurnOutcome = "completed" | "stopped" | "failed"

export interface PendingInputRequest {
  requestId: string
  prompt?: string
  options?: unknown
  turnId?: string
  eventId?: string
  requestedAt: string
}

export interface SessionRuntimeSummary {
  status: SessionRuntimeStatus
  lastOutcome?: SessionLastTurnOutcome
  error?: string
  pendingInput?: PendingInputRequest
  serverUpdatedAt?: string
  updatedAt: number
}

export type SessionResponse = components["schemas"]["sessionResponse"]

export const IDLE_RUNTIME_SUMMARY: SessionRuntimeSummary = {
  status: "idle",
  updatedAt: 0,
}
