import type { components } from "@/lib/api/schema"
import {
  currentCollaborator,
  type ConversationBlock,
  type MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"
import {
  mergeToolBlocks,
  shouldRenderToolBlock,
  toolBlock,
  toolEventID,
} from "@/app/w/(chat)/_lib/session-tools"
import { codeLineCommentReferenceFromPayload } from "@/app/w/(chat)/_lib/code-line-comments"

export type SessionEventResponse = components["schemas"]["sessionEventResponse"]

type Payload = Record<string, unknown>
type SessionHistoryPage = {
  data?: SessionEventResponse[]
}
type SessionBlocksMode = "history" | "live"
type TimelineRole = "work" | "final" | "standalone"
type SessionBlocksOptions = {
  mode?: SessionBlocksMode
  activeTurnID?: string
  activeTurnStartedAt?: string
}

interface TimelineItem {
  block: ConversationBlock
  role: TimelineRole
  turnID?: string
  at: number
}

interface TurnInfo {
  startedAt?: number
  endedAt?: number
}

const hiddenEventTypes = new Set([
  "turn_started",
  "turn_completed",
  "model_usage",
  "model_request_started",
  "model_request_completed",
  "plan_updated",
])

export function sessionHistoryPagesToEvents(
  pages: SessionHistoryPage[]
): SessionEventResponse[] {
  const seen = new Set<string>()
  const events: SessionEventResponse[] = []

  for (const page of pages) {
    for (const event of page.data ?? []) {
      const keys = [event.id, event.event_id].filter((key): key is string =>
        Boolean(key)
      )
      if (keys.some((key) => seen.has(key))) continue
      for (const key of keys) seen.add(key)
      events.push(event)
    }
  }

  return events
}

export function sessionEventsToConversationBlocks(
  events: SessionEventResponse[],
  options: SessionBlocksOptions = {}
): ConversationBlock[] {
  const mode = options.mode ?? "history"
  const ordered = [...events].sort(compareSessionEvents)
  const finalTurns = new Set<string>()
  const finalTextByTurn = new Map<string, string>()
  const thinkingTurns = new Set<string>()
  const toolTurns = new Set<string>()
  const turnInfoByID = new Map<string, TurnInfo>()

  for (const event of ordered) {
    const turn = eventTurnID(event)
    if (!turn) continue
    captureTurnInfo(turnInfoByID, turn, event)
    if (event.event_type === "final" && eventText(event)) {
      finalTurns.add(turn)
      finalTextByTurn.set(turn, eventText(event))
    }
    if (event.event_type === "thinking") thinkingTurns.add(turn)
    if (
      event.event_type === "tool_call" ||
      event.event_type === "tool_result"
    ) {
      toolTurns.add(turn)
    }
  }
  seedActiveTurnInfo(turnInfoByID, options)

  const items: TimelineItem[] = []
  const toolBlockIndexByID = new Map<string, number>()

  for (const event of ordered) {
    const type = event.event_type ?? ""
    if (hiddenEventTypes.has(type)) continue

    if (type === "user.message" || type === "user.message.received") {
      const text = stripAttachmentTags(eventText(event))
      const attachments = eventAttachments(event)
      const codeLineComments = eventCodeLineComments(event)
      if (text || attachments.length || codeLineComments.length) {
        items.push({
          block: {
            type: "user",
            key: eventBlockKey(event, "user"),
            text,
            ...(attachments.length ? { attachments } : {}),
            ...(codeLineComments.length ? { codeLineComments } : {}),
            author: currentCollaborator,
            clientEventID: event.id ?? event.event_id ?? undefined,
            clientStatus: clientStatus(event),
            clientError: clientError(event),
          },
          role: "standalone",
          at: eventTime(event),
        })
      }
      continue
    }

    if (type === "thinking") {
      const text = eventText(event).trim()
      const turn = eventTurnID(event)
      const hasTool = Boolean(turn && toolTurns.has(turn))
      const eventMode = turnMode(turn, mode, finalTurns, options)
      items.push({
        block: thinkingBlock(event, text, eventMode),
        role: hasTool || eventMode === "live" ? "work" : "standalone",
        turnID: turn,
        at: eventTime(event),
      })
      continue
    }

    if (type === "tool_call" || type === "tool_result") {
      const toolID = toolEventID(event)
      const next = {
        ...toolBlock(event),
        key: toolID ? `tool:${toolID}` : eventBlockKey(event, "tool"),
      }
      if (toolID && toolBlockIndexByID.has(toolID)) {
        const index = toolBlockIndexByID.get(toolID)
        if (index !== undefined) {
          items[index] = {
            ...items[index],
            block: mergeToolBlocks(items[index].block, next),
            at: eventTime(event),
          }
        }
        continue
      }
      if (!shouldRenderToolBlock(next)) continue
      items.push({
        block: next,
        role: "work",
        turnID: eventTurnID(event),
        at: eventTime(event),
      })
      if (toolID) toolBlockIndexByID.set(toolID, items.length - 1)
      continue
    }

    if (type === "error") {
      const text = eventErrorText(event)
      if (text) {
        items.push({
          block: {
            type: "error",
            key: eventBlockKey(event, "error"),
            text,
          },
          role: "standalone",
          turnID: eventTurnID(event),
          at: eventTime(event),
        })
      }
      continue
    }

    if (type === "final") {
      appendAssistantItem(items, eventText(event), mode, "final", event)
      continue
    }

    if (type === "token") {
      const turn = eventTurnID(event)
      const hasTool = Boolean(turn && toolTurns.has(turn))
      const hasWork = hasTool || Boolean(turn && thinkingTurns.has(turn))
      const eventMode = turnMode(turn, mode, finalTurns, options)
      if (turn && finalTurns.has(turn) && !hasTool) continue
      if (
        turn &&
        hasTool &&
        finalTurns.has(turn) &&
        finalTextByTurn.get(turn)?.trim() === eventText(event).trim()
      ) {
        continue
      }
      appendAssistantItem(
        items,
        eventText(event),
        eventMode,
        (eventMode === "live" && hasWork) || (hasTool && finalTurns.has(turn))
          ? "work"
          : "final",
        event
      )
    }
  }

  return buildConversationBlocks(items, turnInfoByID, mode, options)
}

function turnMode(
  turnID: string,
  mode: SessionBlocksMode,
  finalTurns: Set<string>,
  options: SessionBlocksOptions
): SessionBlocksMode {
  if (!turnID || !options.activeTurnID || turnID !== options.activeTurnID) {
    return mode
  }
  return finalTurns.has(turnID) ? "history" : "live"
}

function appendAssistantItem(
  items: TimelineItem[],
  text: string,
  mode: SessionBlocksMode,
  role: TimelineRole,
  event: SessionEventResponse
) {
  const trimmed = text.trim()
  if (!trimmed) return
  items.push({
    block: {
      type: "assistant",
      key: eventBlockKey(event, role === "final" ? "final" : "assistant"),
      text: trimmed,
      streaming: mode === "live",
    },
    role,
    turnID: eventTurnID(event),
    at: eventTime(event),
  })
}

function buildConversationBlocks(
  items: TimelineItem[],
  turnInfoByID: Map<string, TurnInfo>,
  mode: SessionBlocksMode,
  options: SessionBlocksOptions
): ConversationBlock[] {
  const blocks: ConversationBlock[] = []
  let workTurnID: string | undefined
  let workItems: TimelineItem[] = []

  const flushWork = (completed: boolean) => {
    if (workItems.length === 0) return
    const hasPendingWork = workItems.some(
      (item) => item.block.type === "thinking" && item.block.active
    )
    const hasActiveTurnWork = Boolean(
      !completed && workTurnID && workTurnID === options.activeTurnID
    )
    const shouldShowActiveWork =
      hasPendingWork || hasActiveTurnWork || (!completed && mode === "live")
    const workBlocks = resolveLiveThinkingState(
      workItems.map((item) => item.block),
      shouldShowActiveWork
    )
    blocks.push({
      type: "worklog",
      key: worklogBlockKey(workTurnID, workItems),
      duration: workDuration(workTurnID, workItems, turnInfoByID),
      startedAt: workStartedAt(workTurnID, workItems, turnInfoByID),
      blocks: workBlocks,
      active: shouldShowActiveWork,
      defaultExpanded: shouldShowActiveWork,
    })
    workItems = []
    workTurnID = undefined
  }

  for (const item of items) {
    if (item.role === "work") {
      if (workItems.length > 0 && workTurnID !== item.turnID) {
        flushWork(false)
      }
      workTurnID = item.turnID
      workItems.push(item)
      continue
    }

    flushWork(item.role === "final")
    blocks.push(item.block)
    if (
      item.role === "final" &&
      mode !== "live" &&
      item.block.type === "assistant" &&
      !item.block.streaming
    )
      blocks.push({
        type: "actions",
        key: `actions:${item.turnID ?? item.block.key ?? item.at}`,
      })
  }

  flushWork(false)
  return blocks
}

function eventBlockKey(event: SessionEventResponse, prefix: string) {
  return `${prefix}:${event.id ?? event.event_id ?? event.sequence_number ?? eventTime(event)}`
}

function worklogBlockKey(turnID: string | undefined, items: TimelineItem[]) {
  if (turnID) return `worklog:${turnID}`
  return `worklog:${items.map((item) => item.block.key).join("|")}`
}

function compareSessionEvents(
  left: SessionEventResponse,
  right: SessionEventResponse
) {
  const byTime = eventTime(left) - eventTime(right)
  if (byTime !== 0) return byTime
  return (left.sequence_number ?? 0) - (right.sequence_number ?? 0)
}

function eventTime(event: SessionEventResponse): number {
  const time = event.event_at ? Date.parse(event.event_at) : 0
  return Number.isNaN(time) ? 0 : time
}

function payloadRecord(event: SessionEventResponse): Payload {
  const payload = event.payload
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return {}
  }
  return payload as Payload
}

function stringValue(payload: Payload, key: string): string {
  const value = payload[key]
  return typeof value === "string" ? value : ""
}

function eventText(event: SessionEventResponse): string {
  const payload = payloadRecord(event)
  return (
    stringValue(payload, "text") ||
    stringValue(payload, "message") ||
    stringValue(payload, "content") ||
    stringValue(payload, "markdown") ||
    stringValue(payload, "result_summary")
  )
}

function eventAttachments(event: SessionEventResponse): MediaAttachment[] {
  const value = payloadRecord(event).attachments
  if (!Array.isArray(value)) return []
  return value.flatMap((entry, index) => {
    if (!entry || typeof entry !== "object" || Array.isArray(entry)) return []
    const record = entry as Record<string, unknown>
    const url =
      stringRecordValue(record, "asset_url") || stringRecordValue(record, "url")
    const filename =
      stringRecordValue(record, "filename") || `Image ${index + 1}`
    const contentType =
      stringRecordValue(record, "content_type") ||
      stringRecordValue(record, "mime_type")
    if (!url || !contentType.startsWith("image/")) return []
    const id =
      stringRecordValue(record, "drive_asset_id") ||
      stringRecordValue(record, "id") ||
      `attachment:${event.id ?? event.event_id ?? eventTime(event)}:${index}`
    return [
      {
        id,
        filename,
        kind: "image" as const,
        url,
      },
    ]
  })
}

function eventCodeLineComments(event: SessionEventResponse) {
  const value = payloadRecord(event).code_line_comments
  if (!Array.isArray(value)) return []
  return value.flatMap((entry, index) => {
    const comment = codeLineCommentReferenceFromPayload(
      entry,
      `code-comment:${event.id ?? event.event_id ?? eventTime(event)}:${index}`
    )
    return comment ? [comment] : []
  })
}

function stringRecordValue(
  record: Record<string, unknown>,
  key: string
): string {
  const value = record[key]
  return typeof value === "string" ? value : ""
}

function stripAttachmentTags(text: string): string {
  return text.replace(/<attachment\b[\s\S]*?<\/attachment>/gi, "").trim()
}

function eventTurnID(event: SessionEventResponse): string {
  return stringValue(payloadRecord(event), "turn_id")
}

function clientStatus(
  event: SessionEventResponse
): "pending" | "failed" | undefined {
  const status = stringValue(payloadRecord(event), "client_status")
  return status === "pending" || status === "failed" ? status : undefined
}

function clientError(event: SessionEventResponse): string | undefined {
  return stringValue(payloadRecord(event), "client_error") || undefined
}

function eventErrorText(event: SessionEventResponse): string {
  const payload = payloadRecord(event)
  return (
    stringValue(payload, "error") ||
    stringValue(payload, "message") ||
    stringValue(payload, "text") ||
    "The session stream failed. Try sending your message again."
  )
}

function captureTurnInfo(
  infoByID: Map<string, TurnInfo>,
  turnID: string,
  event: SessionEventResponse
) {
  const info = infoByID.get(turnID) ?? {}
  const type = event.event_type ?? ""
  const at = eventTime(event)
  const payload = payloadRecord(event)
  const startedAt = timestampValue(payload, "started_at")
  const endedAt =
    timestampValue(payload, "ended_at") ||
    timestampValue(payload, "occurred_at") ||
    at

  if (type === "turn_started") {
    info.startedAt = startedAt || at
  } else if (type === "turn_completed") {
    info.endedAt = endedAt
  }

  infoByID.set(turnID, info)
}

function seedActiveTurnInfo(
  infoByID: Map<string, TurnInfo>,
  options: SessionBlocksOptions
) {
  if (!options.activeTurnID || !options.activeTurnStartedAt) return
  const startedAt = parseTimestamp(options.activeTurnStartedAt)
  if (startedAt === undefined) return
  const info = infoByID.get(options.activeTurnID) ?? {}
  if (info.startedAt === undefined) info.startedAt = startedAt
  infoByID.set(options.activeTurnID, info)
}

function timestampValue(payload: Payload, key: string): number | undefined {
  const value = stringValue(payload, key)
  return parseTimestamp(value)
}

function parseTimestamp(value: string): number | undefined {
  if (!value) return undefined
  const time = Date.parse(value)
  return Number.isNaN(time) ? undefined : time
}

function workDuration(
  turnID: string | undefined,
  items: TimelineItem[],
  turnInfoByID: Map<string, TurnInfo>
): string | undefined {
  const info = turnID ? turnInfoByID.get(turnID) : undefined
  const startedAt = workStartedAt(turnID, items, turnInfoByID)
  const itemTimes = itemEventTimes(items)
  const endedAt = info?.endedAt ?? Math.max(...itemTimes)

  if (
    typeof startedAt !== "number" ||
    !Number.isFinite(startedAt) ||
    !Number.isFinite(endedAt)
  ) {
    return undefined
  }
  if (endedAt < startedAt) return undefined
  return formatDuration(endedAt - startedAt)
}

function workStartedAt(
  turnID: string | undefined,
  items: TimelineItem[],
  turnInfoByID: Map<string, TurnInfo>
): number | undefined {
  const info = turnID ? turnInfoByID.get(turnID) : undefined
  const itemTimes = itemEventTimes(items)
  const startedAt = info?.startedAt ?? Math.min(...itemTimes)
  return Number.isFinite(startedAt) ? startedAt : undefined
}

function itemEventTimes(items: TimelineItem[]): number[] {
  return items.map((item) => item.at).filter((time) => time > 0)
}

function thinkingBlock(
  event: SessionEventResponse,
  text: string,
  mode: SessionBlocksMode
): Extract<ConversationBlock, { type: "thinking" }> {
  if (mode === "live" || clientStatus(event) === "pending") {
    return {
      type: "thinking",
      key: eventBlockKey(event, "thinking"),
      label: clientStatus(event) === "pending" ? "Thinking" : "Thought",
      text: text || undefined,
      active: clientStatus(event) === "pending",
      defaultExpanded: true,
    }
  }

  return {
    type: "thinking",
    key: eventBlockKey(event, "thinking"),
    label: thoughtLabel(event),
    duration: thoughtDuration(event),
    text: text || undefined,
  }
}

function resolveLiveThinkingState(
  blocks: ConversationBlock[],
  allowActive = true
): ConversationBlock[] {
  const activeThinkingIndex = allowActive ? liveActiveThinkingIndex(blocks) : -1

  return blocks.map((block, index) => {
    if (block.type !== "thinking") return block

    const active = index === activeThinkingIndex
    return {
      ...block,
      label: active ? "Thinking" : inactiveThoughtLabel(block),
      active,
      defaultExpanded: active ? true : block.defaultExpanded,
    }
  })
}

function liveActiveThinkingIndex(blocks: ConversationBlock[]) {
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index]
    if (block.type === "actions") continue
    return block.type === "thinking" ? index : -1
  }
  return -1
}

function inactiveThoughtLabel(
  block: Extract<ConversationBlock, { type: "thinking" }>
) {
  return block.duration ? `Thought for ${block.duration}` : "Thought"
}

function thoughtLabel(event: SessionEventResponse): string {
  const duration = thoughtDuration(event)
  return duration ? `Thought for ${duration}` : "Thought"
}

function thoughtDuration(event: SessionEventResponse): string | undefined {
  const payload = payloadRecord(event)
  const startedAt = stringValue(payload, "started_at")
  const endedAt =
    stringValue(payload, "ended_at") ||
    stringValue(payload, "occurred_at") ||
    event.event_at ||
    ""
  const durationMs = durationBetween(startedAt, endedAt)
  if (durationMs === undefined) return undefined
  return formatDuration(durationMs)
}

function durationBetween(startedAt: string, endedAt: string) {
  if (!startedAt || !endedAt) return undefined
  const started = Date.parse(startedAt)
  const ended = Date.parse(endedAt)
  if (Number.isNaN(started) || Number.isNaN(ended) || ended < started) {
    return undefined
  }
  return ended - started
}

function formatDuration(durationMs: number): string {
  if (durationMs < 1000) {
    return `${Math.max(0.1, Math.round(durationMs / 100) / 10)} seconds`
  }
  if (durationMs < 60_000) {
    const seconds = Math.round(durationMs / 1000)
    return seconds === 1 ? "1 second" : `${seconds} seconds`
  }
  if (durationMs < 3_600_000) {
    const totalSeconds = Math.round(durationMs / 1000)
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
  }
  const totalMinutes = Math.round(durationMs / 60_000)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return minutes === 0 ? `${hours}h` : `${hours}h ${minutes}m`
}
