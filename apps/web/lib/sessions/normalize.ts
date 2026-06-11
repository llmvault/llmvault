/**
 * Pure session-event normalization helpers for the web session timeline.
 *
 * These functions take the raw `EmployeeSessionEvent`s persisted by the backend
 * (and the synthetic live events produced by the SSE client) and collapse them
 * into the de-duplicated, render-ready timeline shown in the sessions UI. They
 * are intentionally free of React / network dependencies so the normalization
 * contract can be unit-tested directly (P2-3, P2-4).
 */

export type EmployeeSessionEvent = {
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

export type LocalMessage = {
  id: string
  sessionID: string
  text: string
  createdAt: string
}

export const liveAssistantEventType = "agent.message.streaming"

export function eventKind(event: EmployeeSessionEvent) {
  const type = event.event_type ?? ""
  if (type === "user.message.received" || type === "message_received") {
    return "user"
  }
  if (
    type === "agent.message.sent" ||
    type === liveAssistantEventType ||
    type === "response_completed"
  ) {
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

/**
 * Collapse a raw persisted/live event stream into the render-ready timeline.
 *
 * A single turn can contain several *spans* — interleaved assistant text,
 * thinking, and tool calls — and the runtime persists both the streamed
 * `token` deltas and the authoritative `agent.message.sent` for each span.
 * We:
 *   - drop raw `token` deltas (their text is carried by the assistant span that
 *     follows), but accumulate them *per span* so a multi-span turn is not
 *     collapsed to a single bubble (P2-3),
 *   - coalesce consecutive `thinking` deltas into one bubble,
 *   - merge a `tool_call`/`tool_result` pair into one tool bubble, and
 *   - keep every assistant span as its own completed bubble.
 */
export function normalizeSessionEvents(events: EmployeeSessionEvent[]) {
  const normalized: EmployeeSessionEvent[] = []
  const toolIndexByID = new Map<string, number>()
  // Tokens accumulate per assistant span only: they are reset once their span's
  // assistant message is emitted so a later span's bubble is deduped against its
  // own tokens, not the whole turn's (which collapsed multi-span turns, P2-3).
  let tokenTexts: string[] = []
  let thinkingIndex: number | null = null

  for (const event of events) {
    const kind = eventKind(event)

    if (kind === "token") {
      const text = eventText(event)
      if (text) tokenTexts.push(text)
      continue
    }

    if (isHiddenSessionEvent(event)) {
      continue
    }

    if (kind === "thinking") {
      if (thinkingIndex === null) {
        thinkingIndex = normalized.length
        normalized.push(normalizedThinkingEvent(event))
      } else {
        normalized[thinkingIndex] = appendThinkingEvent(
          normalized[thinkingIndex],
          event
        )
      }
      continue
    }

    thinkingIndex = null

    if (kind === "tool_call" || kind === "tool_result") {
      const toolID = toolEventID(event)
      if (toolID && toolIndexByID.has(toolID)) {
        const index = toolIndexByID.get(toolID)
        if (index !== undefined) {
          normalized[index] = mergeToolEvent(normalized[index], event)
        }
        continue
      }

      const normalizedTool = normalizedToolEvent(event)
      normalized.push(normalizedTool)
      if (toolID) toolIndexByID.set(toolID, normalized.length - 1)
      continue
    }

    if (kind === "assistant") {
      thinkingIndex = null
      normalized.push(normalizedAssistantEvent(event, tokenTexts))
      // Each assistant span owns the tokens that streamed it; reset so a
      // following span dedupes against its own tokens rather than the whole turn.
      tokenTexts = []
      continue
    }

    normalized.push(event)
  }

  return normalized
}

export function assistantTextMatchesStream(
  persistedText: string,
  streamText: string
) {
  const persisted = persistedText.trim()
  const streamed = streamText.trim()
  return persisted === streamed || persisted.endsWith(streamed)
}

export function normalizedAssistantEvent(
  event: EmployeeSessionEvent,
  tokenTexts: string[]
) {
  const cleanedText = cleanPersistedAssistantText(eventText(event), tokenTexts)
  if (cleanedText === eventText(event)) return event
  return {
    ...event,
    payload: {
      ...payloadRecord(event.payload),
      text: cleanedText,
    },
  }
}

/**
 * The runtime sometimes persists an assistant message whose text is the *whole*
 * concatenation of its streamed tokens, and sometimes only the final fragment.
 * When the persisted text exactly equals the concatenation of this span's
 * tokens we keep it verbatim — the previous logic returned only the last token,
 * which collapsed a multi-token span to its tail (P2-3).
 */
export function cleanPersistedAssistantText(
  text: string,
  tokenTexts: string[]
) {
  const visibleTokens = tokenTexts.filter((token) => token !== "")
  if (visibleTokens.length < 2 || text === "") return text

  const allTokens = visibleTokens.join("")
  // If the persisted text is exactly the concatenation of this span's tokens it
  // is already the full answer for the span — keep it as-is.
  if (text === allTokens) return text

  // If the persisted text carries the full concatenation followed by extra
  // (duplicated) trailing content, strip the leading duplicate.
  if (text.startsWith(allTokens)) {
    const candidate = text.slice(allTokens.length)
    if (candidate.trim() !== "") return candidate
  }
  return text
}

export function cleanLiveFinalText(text: string, assistantTexts: string[]) {
  const previousText = assistantTexts
    .filter((assistantText) => assistantText !== "")
    .join("")
  if (previousText !== "" && text.startsWith(previousText)) {
    const candidate = text.slice(previousText.length)
    if (candidate.trim() !== "") return candidate
  }
  return cleanPersistedAssistantText(text, assistantTexts)
}

export function isHiddenSessionEvent(event: EmployeeSessionEvent) {
  switch (event.event_type) {
    case "turn_started":
    case "turn_completed":
    case "model_request_started":
    case "model_request_completed":
    case "model_usage":
    case "final":
    case "done":
      return true
    default:
      return false
  }
}

export function normalizedThinkingEvent(event: EmployeeSessionEvent) {
  return {
    ...event,
    event_type: "thinking",
    payload: {
      ...payloadRecord(event.payload),
      text: eventText(event),
    },
  }
}

export function appendThinkingEvent(
  current: EmployeeSessionEvent,
  next: EmployeeSessionEvent
) {
  return {
    ...current,
    sequence_number: next.sequence_number ?? current.sequence_number,
    event_at: next.event_at ?? current.event_at,
    created_at: next.created_at ?? current.created_at,
    payload: {
      ...payloadRecord(current.payload),
      text: eventText(current) + eventText(next),
    },
  }
}

export function normalizedToolEvent(event: EmployeeSessionEvent) {
  const result = toolEventResult(event)
  return {
    ...event,
    event_type: "tool_call",
    payload: {
      ...payloadRecord(event.payload),
      id: toolEventID(event),
      tool: toolEventName(event),
      args: toolEventArgs(event),
      result,
      status: result === undefined ? "running" : "completed",
    },
  }
}

export function mergeToolEvent(
  current: EmployeeSessionEvent,
  next: EmployeeSessionEvent
) {
  const result = toolEventResult(next) ?? toolEventResult(current)
  return {
    ...current,
    sequence_number: next.sequence_number ?? current.sequence_number,
    event_at: next.event_at ?? current.event_at,
    created_at: next.created_at ?? current.created_at,
    payload: {
      ...payloadRecord(current.payload),
      result,
      result_payload: next.payload,
      status: result === undefined ? "running" : "completed",
    },
  }
}

export function eventLabel(
  event: EmployeeSessionEvent,
  kind: ReturnType<typeof eventKind>
) {
  if (kind === "thinking") return "Thinking"
  if (kind === "tool_call") {
    return toolEventCompleted(event) ? "Tool completed" : "Tool running"
  }
  if (kind === "tool_result") return "Tool completed"
  if (kind === "token") return "Token"
  if (kind === "error") return "Error"
  return event.event_type || "Event"
}

export function eventText(event: EmployeeSessionEvent) {
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
    nestedPayloadString(payload, ["agent_event", "tool"]) ||
    payloadString(payload, "tool") ||
    ""
  )
}

export function toolEventID(event: EmployeeSessionEvent) {
  return (
    payloadString(event.payload, "id") ||
    nestedPayloadString(event.payload, ["agent_event", "id"]) ||
    event.event_id ||
    ""
  )
}

export function toolEventName(event: EmployeeSessionEvent) {
  return (
    payloadString(event.payload, "tool") ||
    nestedPayloadString(event.payload, ["agent_event", "tool"]) ||
    ""
  )
}

export function toolEventArgs(event: EmployeeSessionEvent) {
  return (
    payloadValue(event.payload, "args") ??
    nestedPayloadValue(event.payload, ["agent_event", "args"])
  )
}

export function toolEventResult(event: EmployeeSessionEvent) {
  return (
    payloadValue(event.payload, "result") ??
    nestedPayloadValue(event.payload, ["agent_event", "result"])
  )
}

export function toolEventCompleted(event: EmployeeSessionEvent) {
  return payloadString(event.payload, "status") === "completed"
}

export function payloadRecord(payload: unknown): Record<string, unknown> {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return {}
  }
  return payload as Record<string, unknown>
}

export function payloadValue(payload: unknown, key: string) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return undefined
  }
  return (payload as Record<string, unknown>)[key]
}

export function nestedPayloadValue(payload: unknown, path: string[]) {
  let value = payload
  for (const key of path) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return undefined
    }
    value = (value as Record<string, unknown>)[key]
  }
  return value
}

export function payloadString(payload: unknown, key: string) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return ""
  }
  const value = (payload as Record<string, unknown>)[key]
  return typeof value === "string" ? value : ""
}

export function nestedPayloadString(payload: unknown, path: string[]) {
  let value = payload
  for (const key of path) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return ""
    }
    value = (value as Record<string, unknown>)[key]
  }
  return typeof value === "string" ? value : ""
}

export function localUserMessageToEvent(
  message: LocalMessage
): EmployeeSessionEvent {
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

export function streamFrameToSessionEvent(
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

export function appendLiveTokenEvent(
  events: EmployeeSessionEvent[],
  sessionID: string,
  token: string,
  sequence: number
) {
  if (token === "") return events
  const last = events.at(-1)
  if (
    last?.event_type === liveAssistantEventType &&
    payloadString(last.payload, "status") === "streaming"
  ) {
    const now = new Date().toISOString()
    return [
      ...events.slice(0, -1),
      {
        ...last,
        event_at: now,
        payload: {
          ...payloadRecord(last.payload),
          text: eventText(last) + token,
          status: "streaming",
        },
      },
    ]
  }
  return [
    ...events,
    liveAssistantEvent(sessionID, token, sequence, "streaming"),
  ]
}

/**
 * Append a live `thinking` delta, coalescing consecutive deltas into the single
 * trailing thinking event instead of pushing one event per SSE frame.
 *
 * Without this the live `events` array grew by one entry per reasoning token
 * (P2-5); `normalizeSessionEvents` collapsed them for display but the underlying
 * array still ballooned for a long-thinking turn. Coalescing keeps the array
 * bounded by the number of distinct thinking *spans*, not frames.
 */
export function appendLiveThinkingEvent(
  events: EmployeeSessionEvent[],
  event: EmployeeSessionEvent
) {
  const delta = eventText(event)
  if (delta === "") return events
  const last = events.at(-1)
  if (last?.event_type === "thinking") {
    return [...events.slice(0, -1), appendThinkingEvent(last, event)]
  }
  return [...events, normalizedThinkingEvent(event)]
}

export function reconcileLiveFinalEvent(
  events: EmployeeSessionEvent[],
  sessionID: string,
  finalText: string,
  sequence: number
) {
  const completed = completeTrailingLiveAssistant(events)
  const assistantTexts = completed
    .filter((event) => event.event_type === liveAssistantEventType)
    .map((event) => eventText(event))

  // If the final text is already fully rendered across the existing assistant
  // bubbles, there is nothing new to show. Previously this still appended a
  // fresh bubble whenever the turn ended on a non-assistant event (e.g. a tool
  // call), duplicating the answer (P2-4). Return the timeline unchanged.
  if (finalTextAlreadyRendered(finalText, assistantTexts)) return completed

  const displayText = cleanLiveFinalText(finalText, assistantTexts)
  if (displayText.trim() === "") return completed

  const last = completed.at(-1)
  // Fold the new text into the trailing streamed assistant bubble only when the
  // turn ended on that assistant span. If it ended on a non-assistant event the
  // trailing entry is something else, so we append a fresh completed bubble.
  if (last?.event_type === liveAssistantEventType) {
    return [
      ...completed.slice(0, -1),
      {
        ...last,
        event_at: new Date().toISOString(),
        payload: {
          ...payloadRecord(last.payload),
          text: displayText,
          status: "completed",
        },
      },
    ]
  }
  return [
    ...completed,
    liveAssistantEvent(sessionID, displayText, sequence, "completed"),
  ]
}

/**
 * Returns true when `finalText` is the same content the existing assistant
 * bubbles already rendered (verbatim or as the concatenation of their text), so
 * the `final` event carries no new content and must not add a bubble (P2-4).
 */
function finalTextAlreadyRendered(finalText: string, assistantTexts: string[]) {
  const visible = assistantTexts.filter((assistantText) => assistantText !== "")
  if (visible.length === 0) return false
  const final = finalText.trim()
  const concatenated = visible.join("").trim()
  if (final === concatenated) return true
  // A single-span turn often persists the same text in both the streamed bubble
  // and the final event.
  return visible.some((assistantText) => assistantText.trim() === final)
}

export function completeTrailingLiveAssistant(events: EmployeeSessionEvent[]) {
  const last = events.at(-1)
  if (
    last?.event_type !== liveAssistantEventType ||
    payloadString(last.payload, "status") !== "streaming"
  ) {
    return events
  }
  return [
    ...events.slice(0, -1),
    {
      ...last,
      payload: {
        ...payloadRecord(last.payload),
        status: "completed",
      },
    },
  ]
}

export function parseStreamFrame(data: string): Record<string, unknown> | null {
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

export function liveAssistantEvent(
  sessionID: string,
  text: string,
  sequence: number,
  status: "streaming" | "completed"
): EmployeeSessionEvent {
  const now = new Date().toISOString()
  return {
    id: `live-${sessionID}-${sequence}-assistant`,
    employee_session_id: sessionID,
    event_type: liveAssistantEventType,
    source: "web",
    payload: { text, status },
    event_at: now,
    created_at: now,
  }
}
