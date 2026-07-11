import type {
  ConversationBlock,
  ToolCallDetail,
  ToolSearchResult,
} from "@/app/w/(chat)/_lib/static-data"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import { detectBrowserBashCommand } from "@/app/w/(chat)/_lib/browser-bash-commands"

type Payload = Record<string, unknown>
type ToolCategory = NonNullable<ToolCallDetail["category"]>

const shellToolNames = new Set([
  "bash",
  "shell",
  "sh",
  "exec",
  "exec_command",
  "run_command",
  "command",
])

const webSearchTools = new Set([
  "web_search",
  "websearch",
  "hivy_web_search",
  "hivy_websearch",
  "search_web",
  "search",
])

const webFetchTools = new Set([
  "web_fetch",
  "webfetch",
  "hivy_web_fetch",
  "hivy_webfetch",
  "fetch_web",
  "fetch",
])

const readFileTools = new Set(["read_file", "builtin_read_file"])
const writeFileTools = new Set(["write_file", "builtin_write_file"])
const editFileTools = new Set(["edit_file", "builtin_edit_file"])
const skillListTools = new Set(["skill_list", "skills_list", "list_skills"])
const skillTools = new Set(["skill_manage", "skill_create", "skill_update"])
const sessionSearchTools = new Set(["search_sessions", "hivy_search_sessions"])
const searchTools = new Set([
  "hivy_search_knowledge_base",
  "search_knowledge_base",
])

function toolStatus(event: SessionEventResponse): string {
  return stringValue(payloadRecord(event), "status")
}

export function toolBlock(
  event: SessionEventResponse
): Extract<ConversationBlock, { type: "tool" }> {
  const detail = toolDetail(event)
  return {
    type: "tool",
    label: toolLabel(event, detail),
    running:
      toolStatus(event) === "running" || event.event_type === "tool_call",
    detail,
  }
}

export function shouldRenderToolBlock(
  block: Extract<ConversationBlock, { type: "tool" }>
) {
  const detail = block.detail
  if (!detail) return true
  return !(
    detail.category === "tool" &&
    detail.tool === "tool" &&
    detail.kind === "tool"
  )
}

export function mergeToolBlocks(
  current: ConversationBlock,
  next: Extract<ConversationBlock, { type: "tool" }>
): ConversationBlock {
  if (current.type !== "tool") return next
  const detail = mergeToolDetails(current.detail, next.detail)
  return {
    type: "tool",
    key: current.key ?? next.key,
    label: next.running ? current.label : toolLabelFromDetail(detail),
    running: next.running,
    detail,
  }
}

export function toolEventID(event: SessionEventResponse): string {
  const payload = payloadRecord(event)
  return stringValue(payload, "id") || event.event_id || event.id || ""
}

function mergeToolDetails(
  current?: ToolCallDetail,
  next?: ToolCallDetail
): ToolCallDetail | undefined {
  if (!current) return next
  if (!next) return current
  const category = mergeToolCategory(current.category, next.category)
  const nextIsGeneric = isGenericToolCategory(next.category)
  return {
    ...current,
    ...next,
    kind:
      nextIsGeneric || next.kind === "unknown" || next.kind === "tool"
        ? current.kind
        : next.kind,
    category,
    tool: next.tool && next.tool !== "tool" ? next.tool : current.tool,
    icon: nextIsGeneric ? current.icon : next.icon || current.icon,
    actionIcon: next.actionIcon || current.actionIcon,
    command: next.command || current.command,
    input: next.input || current.input,
    query: next.query || current.query,
    url: next.url || current.url,
    path: next.path || current.path,
    paths: mergeUniqueStrings(current.paths, next.paths),
    searchResults: mergeSearchResults(
      current.searchResults,
      next.searchResults
    ),
    diff: next.diff || current.diff,
    bytesWritten: next.bytesWritten ?? current.bytesWritten,
    editsApplied: next.editsApplied ?? current.editsApplied,
    output: next.output || current.output,
    error: next.error || current.error,
    expandedLabel: next.expandedLabel || current.expandedLabel,
    preview: next.preview || current.preview,
    status: next.status || current.status,
  }
}

function mergeToolCategory(
  current?: ToolCategory,
  next?: ToolCategory
): ToolCategory | undefined {
  if (isGenericToolCategory(next) && !isGenericToolCategory(current)) {
    return current
  }
  return next || current
}

function isGenericToolCategory(category?: ToolCategory) {
  return !category || category === "tool"
}

function toolLabel(
  event: SessionEventResponse,
  detail?: ToolCallDetail
): string {
  if (detail) return toolLabelFromDetail(detail)
  const payload = payloadRecord(event)
  const tool = stringValue(payload, "tool") || "unknown"
  const status = toolStatus(event)
  const action = status === "running" ? "Running" : "Ran"
  const detailText = toolSummary(payload)

  if (tool === "bash" && detailText) return `${action} ${detailText}`
  if (detailText) return `${action} ${tool}: ${detailText}`
  return `${action} ${tool}`
}

function toolLabelFromDetail(detail?: ToolCallDetail): string {
  if (!detail) return "Ran unknown tool"
  const running = detail.status === "running"
  if (detail.tool === "subagent_task") {
    const agent = detail.input || "subagent"
    if (detail.status === "errored") return `${agent} failed`
    return running ? `Delegating to ${agent}` : `${agent} completed`
  }
  const target =
    detail.command || detail.query || detail.url || detail.path || detail.input
  const browserCommand =
    detail.category === "shell" && detail.command
      ? detectBrowserBashCommand(detail.command)
      : undefined

  if (browserCommand) {
    return running ? browserCommand.runningLabel : browserCommand.label
  }

  switch (detail.category) {
    case "shell":
      return target
        ? `${running ? "Running" : "Ran"} ${target}`
        : `${running ? "Running" : "Ran"} command`
    case "web_search":
      if (detail.status === "errored")
        return `Failed searching ${target || "the web"}`
      return `${running ? "Searching" : "Searched"} ${target || "the web"}`
    case "web_fetch":
      if (detail.status === "errored")
        return `Failed fetching ${target || "a webpage"}`
      return `${running ? "Fetching" : "Fetched"} ${target || "a webpage"}`
    case "file_read":
      return `${running ? "Reading" : "Read"} ${readFileLabel(detail)}`
    case "file_edit":
      return running ? "Editing a file" : "Edited a file"
    case "file_write":
      return running ? "Writing a file" : "Wrote a file"
    case "skill_list":
      return running ? "Listing skills" : "Listed skills"
    case "skill":
      return running ? "Managing skills" : "Managed skills"
    case "session_search":
      return `${running ? "Searching sessions" : "Searched sessions"}${
        target ? `: ${target}` : ""
      }`
    case "search":
      return `${running ? "Searching" : "Searched"}${target ? ` ${target}` : ""}`
    default:
      if (target)
        return `${running ? "Running" : "Ran"} ${detail.kind}: ${target}`
      return `${running ? "Running" : "Ran"} ${detail.kind}`
  }
}

function toolDetail(event: SessionEventResponse): ToolCallDetail | undefined {
  const payload = payloadRecord(event)
  const tool = toolName(payload) || "tool"
  const normalizedTool = normalizeToolName(tool)
  const subagentTask = normalizedTool === "subagent_task"
  const args =
    payloadValueAsSummary(payload.args) ??
    parseSummary(stringValue(payload, "args_summary"))
  const result =
    payloadValueAsSummary(payload.result) ??
    parseSummary(stringValue(payload, "result_summary"))
  const command =
    firstSummaryString(result, ["command", "cmd", "script"]) ||
    firstSummaryString(args, ["command", "cmd", "script"])
  const query =
    firstSummaryString(args, [
      "query",
      "q",
      "goal",
      "term",
      "search",
      "prompt",
      "text",
    ]) || firstSummaryString(result, ["query", "q", "term", "search", "prompt"])
  const url =
    firstSummaryString(args, ["url", "uri", "href"]) ||
    firstSummaryString(result, ["url", "uri", "href"])
  const path =
    firstSummaryString(args, ["path", "file", "file_path", "filename"]) ||
    firstSummaryString(result, ["path", "file", "file_path", "filename"])
  const paths = collectPaths(args, result, path)
  const diff =
    firstSummaryString(result, ["diff", "patch", "unified_diff"]) ||
    firstSummaryString(args, ["diff", "patch", "unified_diff"])
  const output =
    summaryString(result, "output") || summaryContentText(result) || ""
  const error =
    stringValue(payload, "error") ||
    summaryString(result, "error") ||
    errorFromOutput(output)
  const timedOut = summaryBoolean(result, "timed_out")
  const truncated = summaryBoolean(result, "truncated")
  const exitCode = summaryNumberOrNull(result, "exit_code")
  const status =
    toolStatus(event) ||
    (event.event_type === "tool_call"
      ? "running"
      : error
        ? "errored"
        : event.event_type === "tool_result"
          ? "completed"
          : "")
  const durationMs = numberValue(payload, "duration_ms")
  const category = toolCategory(normalizedTool, command, error || output)
  const browserCommand =
    category === "shell" ? detectBrowserBashCommand(command) : undefined
  const kind =
    browserCommand?.kind ??
    (subagentTask ? "Subagent" : toolKind(category, normalizedTool))
  const icon =
    browserCommand?.icon ?? (subagentTask ? "bot" : toolIcon(category))
  const subagentAgent = subagentTask
    ? firstSummaryString(args, ["agent"]) ||
      firstSummaryString(result, ["agent"])
    : ""
  const subagentGoal = subagentTask ? query : ""
  const subagentJobId = subagentTask
    ? firstSummaryString(result, ["job_id"]) ||
      firstSummaryString(args, ["job_id"])
    : ""
  const subagentChildSessionId = subagentTask
    ? firstSummaryString(result, ["child_session_id", "session_id"]) ||
      firstSummaryString(args, ["child_session_id", "session_id"])
    : ""
  const bytesWritten = summaryNumber(result, "bytes_written")
  const editsApplied = summaryNumber(result, "edits_applied")
  const searchResults = collectSearchResults(result, output)
  const preview =
    command ||
    query ||
    url ||
    path ||
    output ||
    error ||
    readableToolName(normalizedTool)

  return {
    tool: normalizedTool,
    category,
    kind,
    icon,
    actionIcon: browserCommand?.actionIcon,
    expandedLabel: preview
      ? toolLabelFromDetail({
          tool: normalizedTool,
          kind,
          category,
          command,
          query,
          url,
          path,
          input: subagentAgent || query,
          status,
        })
      : undefined,
    preview,
    command,
    input: subagentAgent || query,
    query,
    jobId: subagentJobId || undefined,
    agentName: subagentAgent || undefined,
    goal: subagentGoal || undefined,
    childSessionId: subagentChildSessionId || undefined,
    url,
    path,
    paths,
    searchResults,
    diff,
    bytesWritten,
    editsApplied,
    output,
    error,
    exitCode,
    durationMs,
    status,
    timedOut,
    truncated,
  }
}

function toolSummary(payload: Payload): string {
  const args = parseSummary(stringValue(payload, "args_summary"))
  const result = parseSummary(stringValue(payload, "result_summary"))
  return (
    compactSummaryValue(args, "command") ||
    compactSummaryValue(args, "query") ||
    compactSummaryValue(result, "command") ||
    compactSummaryValue(result, "output") ||
    stringValue(payload, "error")
  )
}

function parseSummary(value: string): Payload {
  if (!value) return {}
  const parsed = parseUnknownJSON(value)
  if (parsed === undefined) return { output: value }
  return payloadValueAsSummary(parsed) ?? {}
}

function payloadValueAsSummary(value: unknown): Payload | undefined {
  if (!value) return undefined
  if (Array.isArray(value)) return { results: value }
  if (typeof value === "string") {
    const parsed = parseUnknownJSON(value)
    if (parsed !== undefined) return payloadValueAsSummary(parsed)
    return { output: value }
  }
  if (typeof value !== "object") return undefined
  return value as Payload
}

function firstSummaryString(payload: Payload, keys: string[]): string {
  for (const key of keys) {
    const value = summaryString(payload, key)
    if (value) return value
  }
  return ""
}

function payloadRecord(event: SessionEventResponse): Payload {
  const payload = event.payload
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return {}
  }
  const record = payload as Payload
  const agentEvent = recordValue(record, "agent_event")
  return agentEvent ? { ...record, ...agentEvent } : record
}

function stringValue(payload: Payload, key: string): string {
  const value = payload[key]
  return typeof value === "string" ? value : ""
}

function summaryString(payload: Payload, key: string): string {
  const value = payload[key]
  if (typeof value !== "string") return ""
  return value
}

function summaryNumber(payload: Payload, key: string): number | undefined {
  const value = payload[key]
  return typeof value === "number" ? value : undefined
}

function recordValue(payload: Payload, key: string): Payload | undefined {
  const value = payload[key]
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined
  }
  return value as Payload
}

function toolName(payload: Payload): string {
  const direct =
    stringValue(payload, "tool") ||
    stringValue(payload, "tool_name") ||
    stringValue(payload, "name") ||
    stringValue(payload, "function") ||
    stringValue(payload, "function_name") ||
    stringValue(payload, "display_name") ||
    stringValue(payload, "kind") ||
    stringValue(payload, "title")
  if (direct) return direct

  const nested =
    recordValue(payload, "tool") || recordValue(payload, "function")
  if (!nested) return ""
  return (
    stringValue(nested, "name") ||
    stringValue(nested, "tool_name") ||
    stringValue(nested, "display_name")
  )
}

function compactSummaryValue(payload: Payload, key: string): string {
  const value = summaryString(payload, key)
  if (!value) return ""
  return value.replace(/\s+/g, " ").trim()
}

function numberValue(payload: Payload, key: string): number | undefined {
  const value = payload[key]
  return typeof value === "number" ? value : undefined
}

function summaryNumberOrNull(
  payload: Payload,
  key: string
): number | null | undefined {
  const value = payload[key]
  if (value === null) return null
  return typeof value === "number" ? value : undefined
}

function summaryBoolean(payload: Payload, key: string): boolean | undefined {
  const value = payload[key]
  return typeof value === "boolean" ? value : undefined
}

function summaryContentText(payload: Payload): string {
  const content = payload.content
  if (typeof content === "string") return content
  if (!Array.isArray(content)) return ""
  return content
    .map((item) => {
      if (!item || typeof item !== "object" || Array.isArray(item)) return ""
      const text = (item as Payload).text
      return typeof text === "string" ? text : ""
    })
    .filter(Boolean)
    .join("\n")
}

function collectSearchResults(
  result: Payload,
  output: string
): ToolSearchResult[] | undefined {
  const results = uniqueSearchResults([
    ...searchResultsFromUnknown(result.results),
    ...searchResultsFromUnknown(result.items),
    ...searchResultsFromUnknown(result.data),
    ...searchResultsFromUnknown(result),
    ...searchResultsFromText(output),
  ])
  return results.length > 0 ? results : undefined
}

function searchResultsFromText(text: string): ToolSearchResult[] {
  if (!text.trim()) return []
  const parsed = parseUnknownJSON(text)
  if (parsed !== undefined) return searchResultsFromUnknown(parsed)

  const matches = text.match(/https?:\/\/[^\s"',\]}]+/g) ?? []
  return matches.map((url) => ({ url }))
}

function searchResultsFromUnknown(value: unknown): ToolSearchResult[] {
  if (!value) return []
  if (typeof value === "string") return searchResultsFromText(value)
  if (Array.isArray(value)) return value.flatMap(searchResultsFromUnknown)
  if (typeof value !== "object") return []

  const record = value as Payload
  const directURL =
    unknownString(record.url) ||
    unknownString(record.link) ||
    unknownString(record.href)
  if (directURL) {
    return [
      {
        url: directURL,
        title:
          unknownString(record.title) ||
          unknownString(record.name) ||
          undefined,
      },
    ]
  }

  return [
    ...searchResultsFromUnknown(record.results),
    ...searchResultsFromUnknown(record.items),
    ...searchResultsFromUnknown(record.data),
    ...searchResultsFromUnknown(record.content),
  ]
}

function parseUnknownJSON(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

function unknownString(value: unknown): string {
  return typeof value === "string" ? value : ""
}

function mergeSearchResults(
  current?: ToolSearchResult[],
  next?: ToolSearchResult[]
) {
  const merged = uniqueSearchResults([...(current ?? []), ...(next ?? [])])
  return merged.length > 0 ? merged : undefined
}

function uniqueSearchResults(results: ToolSearchResult[]): ToolSearchResult[] {
  const byURL = new Map<string, ToolSearchResult>()
  for (const result of results) {
    const url = result.url.trim()
    if (!url || byURL.has(url)) continue
    byURL.set(url, {
      url,
      title: result.title?.trim() || undefined,
    })
  }
  return [...byURL.values()]
}

function readableToolName(tool: string): string {
  if (!tool) return "tool"
  return tool.replace(/_/g, " ")
}

function isShellTool(tool: string, command: string): boolean {
  if (command) return true
  return shellToolNames.has(normalizeToolName(tool))
}

function normalizeToolName(tool: string): string {
  return tool
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
}

function errorFromOutput(output: string): string {
  const text = output.trim()
  return /^error[:\s]/i.test(text) ? text : ""
}

function toolCategory(
  tool: string,
  command: string,
  context = ""
): ToolCategory {
  if (isShellTool(tool, command)) return "shell"
  const normalizedContext = normalizeToolName(context)
  if (
    tool.includes("web_search") ||
    tool.includes("websearch") ||
    normalizedContext.includes("web_search") ||
    normalizedContext.includes("websearch")
  )
    return "web_search"
  if (
    tool.includes("web_fetch") ||
    tool.includes("webfetch") ||
    normalizedContext.includes("web_fetch") ||
    normalizedContext.includes("webfetch")
  )
    return "web_fetch"
  if (tool.includes("search_session")) return "session_search"
  if (webSearchTools.has(tool)) return "web_search"
  if (webFetchTools.has(tool)) return "web_fetch"
  if (readFileTools.has(tool)) return "file_read"
  if (writeFileTools.has(tool)) return "file_write"
  if (editFileTools.has(tool)) return "file_edit"
  if (skillListTools.has(tool)) return "skill_list"
  if (skillTools.has(tool)) return "skill"
  if (sessionSearchTools.has(tool)) return "session_search"
  if (searchTools.has(tool)) return "search"
  return "tool"
}

function toolKind(category: ToolCategory, tool: string): string {
  switch (category) {
    case "shell":
      return "Shell"
    case "web_search":
      return "Web search"
    case "web_fetch":
      return "Web fetch"
    case "file_read":
      return "Read file"
    case "file_write":
      return "Write file"
    case "file_edit":
      return "Edit file"
    case "skill_list":
      return "Skills"
    case "skill":
      return "Skills"
    case "session_search":
      return "Session search"
    case "search":
      return "Search"
    default:
      return readableToolName(tool)
  }
}

function toolIcon(category: ToolCategory): string {
  switch (category) {
    case "shell":
      return "square-terminal"
    case "web_search":
    case "web_fetch":
      return "chrome"
    case "file_read":
      return "file-text"
    case "file_write":
      return "file-plus"
    case "file_edit":
      return "pencil"
    case "skill_list":
    case "skill":
      return "sparkles"
    case "session_search":
      return "messages-square"
    case "search":
      return "search"
    default:
      return "square-chevron-right"
  }
}

function readFileLabel(detail: ToolCallDetail): string {
  if (detail.path) return displayFileName(detail.path)
  const paths = detail.paths ?? []
  if (paths.length === 0) return "a file"
  if (paths.length === 1) return displayFileName(paths[0] ?? "")
  return `${paths.length} files`
}

function collectPaths(
  args: Payload,
  result: Payload,
  fallback: string
): string[] {
  const paths = [
    firstSummaryString(args, ["path", "file", "file_path", "filename"]),
    firstSummaryString(result, ["path", "file", "file_path", "filename"]),
    ...stringArrayValue(args, "paths"),
    ...stringArrayValue(args, "files"),
    ...stringArrayValue(result, "paths"),
    ...stringArrayValue(result, "files"),
  ]
  if (fallback) paths.unshift(fallback)
  return uniqueStrings(paths.filter(Boolean))
}

function stringArrayValue(payload: Payload, key: string): string[] {
  const value = payload[key]
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === "string")
}

function mergeUniqueStrings(
  current?: string[],
  next?: string[]
): string[] | undefined {
  const merged = uniqueStrings([...(current ?? []), ...(next ?? [])])
  return merged.length > 0 ? merged : undefined
}

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))]
}

function displayFileName(path: string): string {
  const normalized = path.replace(/\\/g, "/")
  return normalized.split("/").filter(Boolean).pop() || path
}
