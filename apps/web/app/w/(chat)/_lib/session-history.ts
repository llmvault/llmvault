import {
  currentCollaborator,
  type ConversationBlock,
  type MediaAttachment,
  type ToolConversationBlock,
} from "@/app/w/(chat)/_lib/static-data"
import {
  isSubagentToolBlock,
  mergeSubagentBlocks,
  subagentBlockFromTool,
  type SessionSubagentBlockOptions,
} from "@/app/w/(chat)/_lib/session-subagent-blocks"
import {
  mergeToolBlocks,
  shouldRenderToolBlock,
  toolBlock,
  toolEventID,
} from "@/app/w/(chat)/_lib/session-tools"
import { codeLineCommentReferenceFromPayload } from "@/app/w/(chat)/_lib/code-line-comments"
import {
  isAutomatedUserBlock,
  orderActiveTurnAfterLatestUserMessage,
} from "@/app/w/(chat)/_lib/session-history-order"
import {
  compareSessionEvents,
  eventBlockKey,
  eventErrorText,
  eventText,
  eventTime,
  eventTurnID,
  formatDuration,
  parseTimestamp,
  payloadRecord,
  stringRecordValue,
  stringValue,
  stripAttachmentTags,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history-event-utils"

export type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history-event-utils"
export { sessionHistoryPagesToEvents } from "@/app/w/(chat)/_lib/session-history-pages"

type SessionBlocksMode = "history" | "live"

type SessionBlocksOptions = {
  mode?: SessionBlocksMode
  activeTurnID?: string
  activeTurnStartedAt?: string
} & SessionSubagentBlockOptions

type TimelineItem = {
  block: ConversationBlock
  mergeKind?: "assistant-token" | "thinking"
  turnID?: string
  terminal?: boolean
  at: number
}

type TurnInfo = {
  startedAt?: number
  endedAt?: number
}

const hiddenEventTypes = new Set(
  "turn_started turn_completed turn_failed turn_interrupted done model_usage model_request_started model_request_completed plan_updated question_requested question_answered session_waiting".split(
    " "
  )
)

export function sessionEventsToConversationBlocks(
  events: SessionEventResponse[],
  options: SessionBlocksOptions = {}
): ConversationBlock[] {
  const mode = options.mode ?? "history"
  const ordered = orderActiveTurnAfterLatestUserMessage(
    [...events].sort(compareSessionEvents),
    options.activeTurnID,
    options.activeTurnStartedAt
  )
  const duplicatedFinalTokens = duplicatedFinalTokenEvents(ordered)
  const turnInfoByID = new Map<string, TurnInfo>()
  const items: TimelineItem[] = []
  const toolItemIndexByID = new Map<string, number>()

  for (const event of ordered) {
    captureTurnInfo(turnInfoByID, event)
    const type = event.event_type ?? ""
    if (hiddenEventTypes.has(type)) continue

    if (type === "user.message" || type === "user.message.received") {
      const block = userBlock(event)
      if (
        !block.text &&
        !block.attachments?.length &&
        !block.codeLineComments?.length
      ) {
        continue
      }
      appendItem(items, {
        block,
        at: eventTime(event),
      })
      continue
    }

    if (type === "thinking") {
      appendItem(items, {
        block: thinkingBlock(event, options.activeTurnID),
        mergeKind: "thinking",
        turnID: eventTurnID(event),
        at: eventTime(event),
      })
      continue
    }

    if (type === "token") {
      if (duplicatedFinalTokens.has(event)) continue
      appendAssistantItem(items, event, {
        streaming: eventIsLive(event, mode, options),
        mergeKind: "assistant-token",
      })
      continue
    }

    if (type === "final") {
      appendAssistantItem(items, event, { streaming: false, terminal: true })
      continue
    }

    if (type === "tool_call" || type === "tool_result") {
      appendToolItem(items, toolItemIndexByID, event, options)
      continue
    }

    if (type === "error") {
      const text = eventErrorText(event)
      if (text) {
        appendItem(items, {
          block: {
            type: "error",
            key: eventBlockKey(event, "error"),
            text,
          },
          terminal: true,
          turnID: eventTurnID(event),
          at: eventTime(event),
        })
      }
    }
  }

  return groupToolChains(groupAgentWork(items, options, turnInfoByID))
}

function appendAssistantItem(
  items: TimelineItem[],
  event: SessionEventResponse,
  options: {
    streaming: boolean
    mergeKind?: "assistant-token"
    terminal?: boolean
  }
) {
  const rawText = eventText(event)
  const text = options.mergeKind ? rawText : rawText.trim()
  if (!text.trim()) return

  appendItem(items, {
    block: {
      type: "assistant",
      key: eventBlockKey(event, options.mergeKind ? "token" : "final"),
      text,
      streaming: options.streaming,
      completedAt: options.terminal ? event.event_at : undefined,
    },
    mergeKind: options.mergeKind,
    terminal: options.terminal,
    turnID: eventTurnID(event),
    at: eventTime(event),
  })
}

function appendToolItem(
  items: TimelineItem[],
  toolItemIndexByID: Map<string, number>,
  event: SessionEventResponse,
  options: SessionBlocksOptions
) {
  const toolID = toolEventID(event)
  const nextTool: ToolConversationBlock = {
    ...toolBlock(event),
    key: toolID ? `tool:${toolID}` : eventBlockKey(event, "tool"),
  }
  const next = isSubagentToolBlock(nextTool)
    ? subagentBlockFromTool(nextTool, options)
    : nextTool

  if (toolID && toolItemIndexByID.has(toolID)) {
    const index = toolItemIndexByID.get(toolID)
    if (index === undefined) return
    const current = items[index]
    if (!current) return
    current.block = mergeToolTimelineBlocks(current.block, next, options)
    current.turnID ||= eventTurnID(event)
    current.at = eventTime(event)
    return
  }

  if (next.type === "tool" && !shouldRenderToolBlock(next)) return
  appendItem(items, {
    block: next,
    turnID: eventTurnID(event),
    at: eventTime(event),
  })
  if (toolID) toolItemIndexByID.set(toolID, items.length - 1)
}

function mergeToolTimelineBlocks(
  current: ConversationBlock,
  next: ConversationBlock,
  options: SessionBlocksOptions
): ConversationBlock {
  if (next.type === "subagent") {
    return mergeSubagentBlocks(current, next, options)
  }
  return next.type === "tool" ? mergeToolBlocks(current, next) : next
}

function appendItem(items: TimelineItem[], item: TimelineItem) {
  const previous = items.at(-1)
  if (previous && canMergeTimelineItems(previous, item)) {
    previous.block = mergeTimelineBlocks(previous.block, item.block)
    return
  }
  items.push(item)
}

function canMergeTimelineItems(left: TimelineItem, right: TimelineItem) {
  return (
    Boolean(left.mergeKind) &&
    left.mergeKind === right.mergeKind &&
    left.turnID === right.turnID
  )
}

function mergeTimelineBlocks(
  left: ConversationBlock,
  right: ConversationBlock
): ConversationBlock {
  if (left.type === "assistant" && right.type === "assistant") {
    return {
      ...left,
      text: left.text + right.text,
      streaming: left.streaming || right.streaming,
      completedAt: right.completedAt ?? left.completedAt,
    }
  }

  if (left.type === "thinking" && right.type === "thinking") {
    return {
      ...left,
      text: `${left.text ?? ""}${right.text ?? ""}` || undefined,
      active: left.active || right.active,
    }
  }

  return right
}

function groupToolChains(blocks: ConversationBlock[]): ConversationBlock[] {
  const grouped: ConversationBlock[] = []
  let tools: ToolConversationBlock[] = []

  const flushTools = () => {
    if (tools.length === 0) return
    if (tools.length > 2) {
      grouped.push({
        type: "tool_chain",
        key: `tool-chain:${tools.map((tool) => tool.key ?? tool.label).join("|")}`,
        tools,
        running: tools.some((tool) => tool.running),
      })
    } else {
      grouped.push(...tools)
    }
    tools = []
  }

  for (const block of blocks) {
    if (block.type === "tool") {
      tools.push(block)
      continue
    }
    flushTools()
    grouped.push(block)
  }

  flushTools()
  return grouped
}

function groupAgentWork(
  items: TimelineItem[],
  options: SessionBlocksOptions,
  turnInfoByID: Map<string, TurnInfo>
): ConversationBlock[] {
  const blocks: ConversationBlock[] = []
  let workTurnID: string | undefined
  let workItems: TimelineItem[] = []
  let deferredItems: TimelineItem[] = []

  const flushDeferred = () => {
    for (const deferred of deferredItems) blocks.push(deferred.block)
    deferredItems = []
  }

  const flushWork = (completed: boolean, terminalAt?: number) => {
    if (workItems.length === 0) return
    const groupedWorkBlocks = groupToolChains(
      workItems.map((item) => item.block)
    )
    const active =
      !completed && isActiveWork(workTurnID, groupedWorkBlocks, options)
    blocks.push({
      type: "agent_work",
      key: agentWorkBlockKey(workTurnID, groupedWorkBlocks),
      duration: workDuration(workTurnID, workItems, terminalAt, turnInfoByID),
      blocks: groupedWorkBlocks,
      active,
      defaultExpanded: active,
    })
    workTurnID = undefined
    workItems = []
  }

  for (const item of items) {
    if (item.terminal) {
      flushWork(true, item.at)
      blocks.push(item.block)
      flushDeferred()
      continue
    }

    if (isAgentWorkItem(item)) {
      if (workItems.length > 0 && workTurnID !== item.turnID) {
        flushWork(false)
        flushDeferred()
      }
      workTurnID = item.turnID
      workItems.push(item)
      continue
    }

    if (workItems.length > 0 && isAutomatedUserBlock(item.block)) {
      deferredItems.push(item)
      continue
    }

    flushWork(false)
    flushDeferred()
    blocks.push(item.block)
  }

  flushWork(false)
  flushDeferred()
  return blocks
}

function isAgentWorkItem(item: TimelineItem) {
  return Boolean(item.turnID) && isAgentWorkBlock(item.block)
}

function isAgentWorkBlock(block: ConversationBlock) {
  return (
    block.type === "assistant" ||
    block.type === "thinking" ||
    block.type === "subagent" ||
    block.type === "tool"
  )
}

function isActiveWork(
  turnID: string | undefined,
  blocks: ConversationBlock[],
  options: SessionBlocksOptions
) {
  return (
    options.mode === "live" ||
    Boolean(turnID && turnID === options.activeTurnID) ||
    blocks.some((block) => {
      if (block.type === "assistant") return Boolean(block.streaming)
      if (block.type === "thinking") return Boolean(block.active)
      if (block.type === "subagent") return block.status === "running"
      if (block.type === "tool") return Boolean(block.running)
      if (block.type === "tool_chain") return Boolean(block.running)
      return false
    })
  )
}

function agentWorkBlockKey(
  turnID: string | undefined,
  blocks: ConversationBlock[]
) {
  if (turnID) return `agent-work:${turnID}`
  return `agent-work:${blocks.map((block) => block.key).join("|")}`
}

function workDuration(
  turnID: string | undefined,
  items: TimelineItem[],
  terminalAt: number | undefined,
  turnInfoByID: Map<string, TurnInfo>
) {
  const startedAt = workStartedAt(turnID, items, turnInfoByID)
  const itemTimes = items.map((item) => item.at).filter((time) => time > 0)
  const info = turnID ? turnInfoByID.get(turnID) : undefined
  const endedAt = info?.endedAt ?? terminalAt ?? Math.max(...itemTimes)

  if (
    typeof startedAt !== "number" ||
    !Number.isFinite(startedAt) ||
    !Number.isFinite(endedAt) ||
    endedAt < startedAt
  ) {
    return undefined
  }

  return formatDuration(endedAt - startedAt)
}

function workStartedAt(
  turnID: string | undefined,
  items: TimelineItem[],
  turnInfoByID: Map<string, TurnInfo>
) {
  const info = turnID ? turnInfoByID.get(turnID) : undefined
  const itemTimes = items.map((item) => item.at).filter((time) => time > 0)
  const startedAt = info?.startedAt ?? Math.min(...itemTimes)
  return Number.isFinite(startedAt) ? startedAt : undefined
}

function captureTurnInfo(
  infoByID: Map<string, TurnInfo>,
  event: SessionEventResponse
) {
  const turnID = eventTurnID(event)
  if (!turnID) return

  const info = infoByID.get(turnID) ?? {}
  const type = event.event_type ?? ""
  const payload = payloadRecord(event)
  const at = eventTime(event)

  if (type === "turn_started") {
    info.startedAt = timestampValue(payload, "started_at") ?? at
  } else if (
    type === "turn_completed" ||
    type === "turn_failed" ||
    type === "turn_interrupted"
  ) {
    info.endedAt =
      timestampValue(payload, "ended_at") ??
      timestampValue(payload, "occurred_at") ??
      at
  } else if (
    (type === "final" || type === "error") &&
    info.endedAt === undefined
  ) {
    info.endedAt = at
  }

  infoByID.set(turnID, info)
}

function timestampValue(payload: Record<string, unknown>, key: string) {
  return parseTimestamp(stringValue(payload, key))
}

function userBlock(
  event: SessionEventResponse
): Extract<ConversationBlock, { type: "user" }> {
  const text = stripAttachmentTags(eventText(event))
  const attachments = eventAttachments(event)
  const codeLineComments = eventCodeLineComments(event)
  const source = event.source
  const provider = stringValue(payloadRecord(event), "provider")

  return {
    type: "user",
    key: eventBlockKey(event, "user"),
    text,
    ...(attachments.length ? { attachments } : {}),
    ...(codeLineComments.length ? { codeLineComments } : {}),
    ...(source ? { source } : {}),
    ...(provider ? { provider } : {}),
    author: currentCollaborator,
  }
}

function thinkingBlock(
  event: SessionEventResponse,
  activeTurnID?: string
): Extract<ConversationBlock, { type: "thinking" }> {
  const active = Boolean(activeTurnID && eventTurnID(event) === activeTurnID)
  const label = stringValue(payloadRecord(event), "label")

  return {
    type: "thinking",
    key: eventBlockKey(event, "thinking"),
    label: label || (active ? "Thinking" : "Thought"),
    text: eventText(event).trim() ? eventText(event) : undefined,
    active,
  }
}

function duplicatedFinalTokenEvents(events: SessionEventResponse[]) {
  const duplicated = new Set<SessionEventResponse>()

  events.forEach((event, index) => {
    if (event.event_type !== "final") return
    const text = eventText(event).trim()
    if (!text) return
    const turnID = eventTurnID(event)
    const previous = previousVisibleEvent(events, index)
    if (
      previous?.event_type === "token" &&
      eventTurnID(previous) === turnID &&
      eventText(previous).trim() === text
    ) {
      duplicated.add(previous)
    }
  })

  return duplicated
}

function previousVisibleEvent(
  events: SessionEventResponse[],
  beforeIndex: number
) {
  for (let index = beforeIndex - 1; index >= 0; index -= 1) {
    const event = events[index]
    if (!event || hiddenEventTypes.has(event.event_type ?? "")) continue
    return event
  }
  return undefined
}

function eventIsLive(
  event: SessionEventResponse,
  mode: SessionBlocksMode,
  options: SessionBlocksOptions
) {
  const turnID = eventTurnID(event)
  return mode === "live" || Boolean(turnID && turnID === options.activeTurnID)
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
      `attachment:${event.id ?? event.event_id ?? event.event_at ?? index}`
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
      `code-comment:${event.id ?? event.event_id ?? event.event_at ?? index}`
    )
    return comment ? [comment] : []
  })
}
