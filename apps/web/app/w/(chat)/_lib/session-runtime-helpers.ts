import type {
  GoSessionStreamCursor,
  GoSessionStreamFrame,
} from "@/app/w/(chat)/_lib/go-session-stream"
import { isTerminalStreamFrame } from "@/app/w/(chat)/_lib/live-session-stream"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import {
  creditsForCostUSD,
  modelUsageCost,
  modelUsageEventKey,
  type SessionUsageSummary,
} from "@/app/w/(chat)/_lib/session-usage"
import {
  IDLE_RUNTIME_SUMMARY,
  type PendingInputRequest,
  type SessionLastTurnOutcome,
  type SessionResponse,
  type SessionRuntimeStatus,
  type SessionRuntimeSummary,
} from "@/app/w/(chat)/_lib/session-runtime-types"

interface SessionRuntimeUsageState {
  usageBySessionId: Record<string, SessionUsageSummary | undefined>
  usageEventKeysBySessionId: Record<string, Record<string, boolean> | undefined>
}

export function sessionRuntimeStatusFromResponse(
  session: SessionResponse
): SessionRuntimeStatus {
  const turnStatus = stringValue(session, "agent_turn_status")
  if (turnStatus === "active") return "streaming"
  if (turnStatus === "queued") return "queued"
  if (turnStatus === "waiting_for_user") return "waiting_for_user"
  const outcome = sessionLastOutcomeFromResponse(session)
  if (outcome === "stopped") return "stopped"
  if (outcome === "failed") return "failed"
  return "idle"
}

export function sessionLastOutcomeFromResponse(
  session: SessionResponse
): SessionLastTurnOutcome | undefined {
  const value = stringValue(session, "last_turn_outcome")
  return value === "completed" || value === "stopped" || value === "failed"
    ? value
    : undefined
}

export function sessionServerUpdatedAt(session: SessionResponse) {
  return (
    stringValue(session, "updated_at") ||
    stringValue(session, "last_activity_at")
  )
}

export function shouldHydrateSessionRuntime(
  existing: SessionRuntimeSummary | undefined,
  status: SessionRuntimeStatus,
  lastOutcome: SessionLastTurnOutcome | undefined,
  serverUpdatedAt: string
) {
  if (!existing) return true
  if (serverSnapshotIsOlder(serverUpdatedAt, existing.serverUpdatedAt)) {
    return false
  }
  if (isRuntimeActive(existing.status) && status === "idle") {
    return Boolean(lastOutcome || serverUpdatedAt)
  }
  return true
}

export function summaryForFrame(
  current: SessionRuntimeSummary | undefined,
  frame: GoSessionStreamFrame
): SessionRuntimeSummary {
  const pendingInput = pendingInputFromFrame(frame)
  const serverUpdatedAt =
    frameServerUpdatedAt(frame) ?? current?.serverUpdatedAt
  if (pendingInput) {
    return {
      ...current,
      status: "waiting_for_user",
      pendingInput,
      serverUpdatedAt,
      updatedAt: Date.now(),
    }
  }
  if (frame.event === "question_answered") {
    return {
      ...current,
      status: "streaming",
      pendingInput: undefined,
      serverUpdatedAt,
      updatedAt: Date.now(),
    }
  }
  if (isTerminalStreamFrame(frame)) {
    const message = terminalFrameErrorMessage(frame)
    if (message) {
      return {
        ...current,
        status: "failed",
        lastOutcome: "failed",
        error: message,
        pendingInput: undefined,
        serverUpdatedAt,
        updatedAt: Date.now(),
      }
    }
    return {
      ...current,
      status: "idle",
      lastOutcome: "completed",
      pendingInput: undefined,
      serverUpdatedAt,
      updatedAt: Date.now(),
    }
  }
  if (isActiveStreamFrame(frame.event)) {
    return {
      ...current,
      status: "streaming",
      error: undefined,
      serverUpdatedAt,
      updatedAt: Date.now(),
    }
  }
  return {
    ...(current ?? IDLE_RUNTIME_SUMMARY),
    updatedAt: current?.updatedAt ?? Date.now(),
  }
}

export function usagePatchForFrame(
  state: SessionRuntimeUsageState,
  sessionId: string,
  frame: GoSessionStreamFrame
): Partial<SessionRuntimeUsageState> {
  if (frame.event !== "model_usage") return {}
  const payload = payloadRecord(frame.data)
  const eventKey = modelUsageEventKey(payload)
  const seen = state.usageEventKeysBySessionId[sessionId] ?? {}
  if (seen[eventKey]) return {}
  const cost = modelUsageCost(payload)
  if (cost <= 0) {
    return {
      usageEventKeysBySessionId: {
        ...state.usageEventKeysBySessionId,
        [sessionId]: {
          ...seen,
          [eventKey]: true,
        },
      },
    }
  }
  const current = state.usageBySessionId[sessionId]
  const modelCostUsd = (current?.modelCostUsd ?? 0) + cost
  const modelCredits = creditsForCostUSD(modelCostUsd)
  const sandboxCostUsd = current?.sandboxCostUsd ?? 0
  const sandboxCredits = current?.sandboxCredits ?? 0
  return {
    usageBySessionId: {
      ...state.usageBySessionId,
      [sessionId]: {
        costUsd: modelCostUsd + sandboxCostUsd,
        credits: modelCredits + sandboxCredits,
        modelCostUsd,
        modelCredits,
        sandboxCostUsd,
        sandboxCredits,
        sandboxVCPUSeconds: current?.sandboxVCPUSeconds ?? 0,
        updatedAt: Date.now(),
      },
    },
    usageEventKeysBySessionId: {
      ...state.usageEventKeysBySessionId,
      [sessionId]: {
        ...seen,
        [eventKey]: true,
      },
    },
  }
}

export function shouldAdvanceCursor(
  current: GoSessionStreamCursor | undefined,
  next: GoSessionStreamCursor
) {
  return (
    !current ||
    current.streamId !== next.streamId ||
    next.sequence > current.sequence
  )
}

export function isModelRequestMarker(event: SessionEventResponse) {
  return payloadRecord(event.payload).model_request_pending === true
}

export function reconcileLiveSessionEvents(
  current: SessionEventResponse[],
  historyEvents: SessionEventResponse[]
) {
  const historyByKey = historyEventMap(historyEvents)
  const next = current.filter(
    (event) =>
      !historyContainsLiveEvent(historyByKey, event) &&
      !historyContainsClientUserMessage(historyEvents, event)
  )
  return next.length === current.length ? current : next
}

export function streamErrorEvent(
  sessionId: string,
  message: string
): SessionEventResponse {
  const now = new Date().toISOString()
  const id = `stream-error:${Date.now()}`
  return {
    id,
    session_id: sessionId,
    event_id: id,
    event_type: "error",
    sequence_number: Number.MAX_SAFE_INTEGER,
    payload: {
      message,
    },
    event_at: now,
  } as SessionEventResponse
}

function pendingInputFromFrame(
  frame: GoSessionStreamFrame
): PendingInputRequest | undefined {
  if (frame.event !== "question_requested") return undefined
  const data = payloadRecord(frame.data)
  const firstQuestion = requestInputQuestions(data)[0]
  const requestId =
    stringValue(data, "question_request_id") ||
    stringValue(data, "request_id") ||
    stringValue(data, "question_id") ||
    stringValue(data, "id") ||
    frame.id
  return {
    requestId,
    prompt:
      firstQuestion?.question ||
      stringValue(data, "prompt") ||
      stringValue(data, "question") ||
      stringValue(data, "message") ||
      stringValue(data, "text") ||
      undefined,
    options: firstQuestion?.options ?? data.options,
    turnId: stringValue(data, "turn_id") || undefined,
    eventId: stringValue(data, "event_id") || frame.id || undefined,
    requestedAt:
      stringValue(data, "occurred_at") ||
      stringValue(data, "requested_at") ||
      new Date().toISOString(),
  }
}

function requestInputQuestions(data: Record<string, unknown>) {
  const questions = data.questions
  if (!Array.isArray(questions)) return []
  return questions
    .map((question) =>
      question && typeof question === "object" && !Array.isArray(question)
        ? (question as Record<string, unknown>)
        : undefined
    )
    .filter((question): question is Record<string, unknown> =>
      Boolean(question)
    )
    .map((question) => ({
      question: stringValue(question, "question"),
      options: Array.isArray(question.options) ? question.options : undefined,
    }))
}

function serverSnapshotIsOlder(incoming: string, current?: string) {
  if (!incoming || !current) return false
  const incomingTime = Date.parse(incoming)
  const currentTime = Date.parse(current)
  if (Number.isNaN(incomingTime) || Number.isNaN(currentTime)) return false
  return incomingTime < currentTime
}

function isRuntimeActive(status: SessionRuntimeStatus) {
  return (
    status === "queued" ||
    status === "streaming" ||
    status === "waiting_for_user"
  )
}

function frameServerUpdatedAt(frame: GoSessionStreamFrame) {
  return stringValue(payloadRecord(frame.data), "occurred_at") || undefined
}

function isActiveStreamFrame(event: string) {
  return (
    event === "turn_started" ||
    event === "model_request_started" ||
    event === "thinking" ||
    event === "token" ||
    event === "tool_call" ||
    event === "tool_result" ||
    event === "plan_updated"
  )
}

function historyEventMap(events: SessionEventResponse[]) {
  const byKey = new Map<string, SessionEventResponse>()
  for (const event of events) {
    for (const key of eventKeys(event)) {
      byKey.set(key, event)
    }
  }
  return byKey
}

function historyContainsLiveEvent(
  historyByKey: Map<string, SessionEventResponse>,
  live: SessionEventResponse
) {
  for (const key of eventKeys(live)) {
    const history = historyByKey.get(key)
    if (!history) continue
    if (isMergedTextEvent(live)) {
      return (
        eventText(history).length >= eventText(live).length ||
        (history.sequence_number ?? 0) >= (live.sequence_number ?? 0)
      )
    }
    return true
  }
  return false
}

function historyContainsClientUserMessage(
  historyEvents: SessionEventResponse[],
  live: SessionEventResponse
) {
  if (!isClientUserMessage(live)) return false
  const text = eventText(live).trim()
  if (!text) return false
  return historyEvents.some(
    (event) =>
      isUserMessage(event) &&
      event.session_id === live.session_id &&
      eventText(event).trim() === text
  )
}

function eventKeys(event: SessionEventResponse) {
  return [event.id, event.event_id].filter((key): key is string => Boolean(key))
}

function isClientUserMessage(event: SessionEventResponse) {
  return (
    isUserMessage(event) &&
    (event.id?.startsWith("client:") || event.event_id?.startsWith("client:"))
  )
}

function isUserMessage(event: SessionEventResponse) {
  return (
    event.event_type === "user.message" ||
    event.event_type === "user.message.received"
  )
}

function isMergedTextEvent(event: SessionEventResponse) {
  return event.event_type === "thinking" || event.event_type === "token"
}

function eventText(event: SessionEventResponse) {
  const payload = payloadRecord(event.payload)
  return (
    stringValue(payload, "text") ||
    stringValue(payload, "message") ||
    stringValue(payload, "content") ||
    stringValue(payload, "markdown") ||
    stringValue(payload, "result_summary")
  )
}

function terminalFrameErrorMessage(frame: GoSessionStreamFrame): string {
  if (frame.event !== "error" && frame.event !== "turn_failed") return ""
  const fallback =
    frame.event === "turn_failed"
      ? "The agent turn failed."
      : "The live session stream failed."
  const record = payloadRecord(frame.data)
  if (record.interrupted === true) return ""
  const value = record.error ?? record.message ?? record.text
  if (
    typeof value === "string" &&
    value.trim().toLowerCase() === "interrupted by user"
  ) {
    return ""
  }
  return typeof value === "string" && value.trim() ? value : fallback
}

function payloadRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function stringValue(value: unknown, key: string) {
  const raw = payloadRecord(value)[key]
  return typeof raw === "string" ? raw : ""
}
