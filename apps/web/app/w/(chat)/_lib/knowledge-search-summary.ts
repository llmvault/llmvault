/**
 * Grouped summary for the `search_knowledge_base` MCP tool output.
 *
 * The tool (internal/rag/mcptools.go) returns JSON shaped like:
 *
 *   {
 *     "success": true,
 *     "query": "...",
 *     "total_results": 7,
 *     "result_groups": 3,
 *     "results": [
 *       {
 *         "source_id": "...",
 *         "source": { "id": "...", "name": "...", "kind": "INTEGRATION" |
 *                     "WEBSITE" | "FILE_UPLOAD", "provider": "github" | ... },
 *         "result_count": 3,
 *         "chunks": [{ "id", "score", "doc_id", "semantic_id", "link",
 *                      "title", "part_index", "excerpt" }]
 *       }
 *     ]
 *   }
 *
 * Live stream frames carry the full MCP result, but durable session history
 * only stores `result_summary`, which the runtime truncates to 2,000 chars
 * with a trailing "..." (TOOL_SUMMARY_CHARS in
 * sandboxes/runtime/crates/runtime/src/handler.rs). Any non-trivial search
 * result exceeds that, so this module parses BOTH the pristine payload and a
 * truncated MCP envelope ({"content":[{"type":"text","text":"<json>"}]} cut
 * mid-string). Truncated payloads yield lower-bound counts ("Found 2+ Slack
 * messages") for the aggregate the cut landed in.
 *
 * Everything here is pure so the grouping/pluralization stays unit-testable;
 * rendering lives in tool-knowledge-search-detail.tsx.
 */

const KNOWLEDGE_SEARCH_TOOLS = new Set([
  "search_knowledge_base",
  "hivy_search_knowledge_base",
])

export function isKnowledgeSearchTool(tool?: string): boolean {
  return Boolean(tool && KNOWLEDGE_SEARCH_TOOLS.has(tool))
}

export interface KnowledgeSearchSummaryLine {
  /** Stable key for the rendered row ("family:type", e.g. "linear:issue"). */
  key: string
  /** Logo key for <IntegrationLogo> (github, slack, linear, notion, chrome). */
  provider?: string
  /** AppIcon registry name fallback when there is no brand logo. */
  icon?: string
  /** Full display text, e.g. "Found 2 Slack messages". */
  text: string
}

type SourceFamily =
  | "github"
  | "linear"
  | "slack"
  | "notion"
  | "website"
  | "file_upload"
  | "unknown"

/** One aggregate per (source family, document type), e.g. linear+issue. */
interface TypeBucket {
  family: SourceFamily
  type: string
  docIDs: Set<string>
  /** Display name for unknown families (from source.name/provider). */
  fallbackName: string
  /** True when this bucket's count is a lower bound (truncated payload). */
  approximate: boolean
}

/**
 * Parses the raw tool output into aggregate summary lines — one line per
 * source kind + document type ("Found 4 Linear issues", "Found 2 Linear
 * initiatives", "Found 3 website pages").
 *
 * Returns `null` when the payload does not match the expected shape (tool
 * error, schema drift, still running) so callers can fall back to the raw
 * rendering. Returns `[]` for a successful search with zero results.
 */
export function summarizeKnowledgeSearchOutput(
  output?: string
): KnowledgeSearchSummaryLine[] | null {
  const parsed = parseKnowledgeSearchPayload(output)
  if (!parsed) return null
  const { payload, truncated } = parsed
  // Pristine payloads must self-identify; truncated ones lose trailing keys
  // ("results" sorts before "success" in Go's map marshalling), so shape
  // trust comes from the caller keying on the tool name.
  if (!truncated && payload.success !== true) return null
  const groups = Array.isArray(payload.results)
    ? payload.results.filter(isRecord)
    : undefined
  if (groups === undefined) return null

  const buckets = new Map<string, TypeBucket>()
  let lastBucket: TypeBucket | undefined
  for (const group of groups) {
    lastBucket = collectGroup(buckets, group) ?? lastBucket
  }
  if (truncated && lastBucket) {
    // The cut lands inside the final parsed group, so the count of the last
    // aggregate touched by it is a lower bound.
    lastBucket.approximate = true
  }

  const lines = orderBuckets([...buckets.values()]).map(bucketToLine)
  if (lines.length === 0 && truncated) {
    // Nothing usable survived the cut — the raw fallback is more honest
    // than an empty state that would read as "no results".
    return null
  }
  return lines
}

/**
 * Lines keep source order of first appearance, with each family's types in
 * canonical order (pull requests before issues, issues before projects).
 */
function orderBuckets(buckets: TypeBucket[]): TypeBucket[] {
  const familyOrder = [...new Set(buckets.map((bucket) => bucket.family))]
  return buckets
    .filter((bucket) => bucket.docIDs.size > 0)
    .sort((a, b) => {
      if (a.family !== b.family) {
        return familyOrder.indexOf(a.family) - familyOrder.indexOf(b.family)
      }
      const order = TYPE_ORDER[a.family]
      if (!order) return 0
      return order.indexOf(a.type) - order.indexOf(b.type)
    })
}

function collectGroup(
  buckets: Map<string, TypeBucket>,
  group: Record<string, unknown>
): TypeBucket | undefined {
  const source = recordField(group, "source")
  const provider = stringField(source, "provider")
  const kind = stringField(source, "kind")
  const chunks = Array.isArray(group.chunks) ? group.chunks : []

  const family = resolveFamily(provider, kind, chunks)
  const fallbackName =
    stringField(source, "name") || provider || kind || "knowledge base"

  let lastBucket: TypeBucket | undefined
  for (const [index, chunk] of chunks.entries()) {
    const record = isRecord(chunk) ? chunk : undefined
    const docID =
      stringField(record, "doc_id") ||
      idField(record) ||
      `${stringField(group, "source_id")}:${index}`
    const type = docTypeForFamily(family, docID)
    const key = `${family}:${type}`
    let bucket = buckets.get(key)
    if (!bucket) {
      bucket = {
        family,
        type,
        docIDs: new Set(),
        fallbackName,
        approximate: false,
      }
      buckets.set(key, bucket)
    }
    bucket.docIDs.add(docID)
    lastBucket = bucket
  }
  return lastBucket
}

/** Slack chunk doc IDs look like "C0123ABC__1699999999.000100". */
const SLACK_DOC_ID = /^[A-Z0-9]{5,}__\d+(\.\d+)?$/

/** Maps the source payload (or doc_id prefixes) to a source family. */
function resolveFamily(
  provider: string,
  kind: string,
  chunks: unknown[]
): SourceFamily {
  const normalized = provider.toLowerCase()
  if (normalized.startsWith("github")) return "github"
  if (normalized === "linear") return "linear"
  if (normalized === "slack") return "slack"
  if (normalized === "notion") return "notion"
  if (normalized === "website" || kind === "WEBSITE") return "website"
  if (normalized === "file_upload" || kind === "FILE_UPLOAD") {
    return "file_upload"
  }
  // Truncated/legacy groups may lack a source payload ("source" sorts after
  // "chunks" in the marshalled JSON) — infer from doc_id shapes instead.
  for (const chunk of chunks) {
    if (!isRecord(chunk)) continue
    const docID = stringField(chunk, "doc_id")
    if (docID.startsWith("github_")) return "github"
    if (docID.startsWith("linear_")) return "linear"
    if (docID.startsWith("notion_")) return "notion"
    if (SLACK_DOC_ID.test(docID)) return "slack"
    if (/^https?:\/\//.test(docID)) return "website"
  }
  return "unknown"
}

/**
 * Singular doc-type nouns per family. GitHub and Linear doc IDs are prefixed
 * by type (github_pr_*, linear_issue_*, ...) by their connectors.
 */
function docTypeForFamily(family: SourceFamily, docID: string): string {
  switch (family) {
    case "github":
      if (docID.startsWith("github_pr_")) return "pull request"
      if (docID.startsWith("github_issue_")) return "issue"
      return "document"
    case "linear":
      if (docID.startsWith("linear_issue_")) return "issue"
      if (docID.startsWith("linear_project_")) return "project"
      if (docID.startsWith("linear_initiative_")) return "initiative"
      return "document"
    case "slack":
      return "message"
    case "notion":
      return "page"
    case "website":
      return "page"
    default:
      return "document"
  }
}

/** Canonical type ordering per family so lines read consistently. */
const TYPE_ORDER: Partial<Record<SourceFamily, string[]>> = {
  github: ["pull request", "issue", "document"],
  linear: ["issue", "project", "initiative", "document"],
}

const FAMILY_DISPLAY: Record<
  SourceFamily,
  { label?: string; provider?: string; icon?: string }
> = {
  github: { label: "GitHub", provider: "github" },
  linear: { label: "Linear", provider: "linear" },
  slack: { label: "Slack", provider: "slack" },
  notion: { label: "Notion", provider: "notion" },
  // Website pages render with the Chrome mark per design.
  website: { label: "website", provider: "chrome" },
  file_upload: { label: "uploaded", icon: "file-text" },
  unknown: { icon: "database" },
}

function bucketToLine(bucket: TypeBucket): KnowledgeSearchSummaryLine {
  const display = FAMILY_DISPLAY[bucket.family]
  const total = bucket.docIDs.size
  const label = display.label ?? bucket.fallbackName
  // Approximate counts are lower bounds — surface "2+" and plural nouns.
  const noun = pluralize(bucket.type, bucket.approximate ? 2 : total)
  const count = bucket.approximate ? `${total}+` : `${total}`

  return {
    key: `${bucket.family}:${bucket.type}`,
    provider: display.provider,
    icon: display.icon,
    text: `Found ${count} ${label} ${noun}`,
  }
}

function pluralize(noun: string, count: number): string {
  return count === 1 ? noun : `${noun}s`
}

// --- payload parsing -------------------------------------------------------

interface ParsedPayload {
  payload: Record<string, unknown>
  /** True when the payload was recovered from a truncated result_summary. */
  truncated: boolean
}

function parseKnowledgeSearchPayload(
  output?: string
): ParsedPayload | null {
  const raw = output?.trim()
  if (!raw) return null

  const direct = tryParseJSON(raw)
  if (direct !== undefined) {
    const payload = unwrapEnvelope(direct)
    return payload ? { payload, truncated: false } : null
  }

  // The durable result_summary is the serialized MCP envelope cut at 2,000
  // chars — the cut virtually always lands inside the escaped inner JSON
  // string, so peel the envelope lexically before repairing.
  const inner = raw.includes('"content"')
    ? extractEnvelopeText(raw)
    : undefined
  const candidate = stripTruncationMarker(inner ?? raw)
  const repaired = repairTruncatedJSON(candidate)
  if (!isRecord(repaired)) return null
  const payload = unwrapEnvelope(repaired)
  return payload ? { payload, truncated: true } : null
}

/** Unwraps {"content":[{"type":"text","text":"<json>"}]} envelopes. */
function unwrapEnvelope(value: unknown): Record<string, unknown> | null {
  if (!isRecord(value)) return null
  if (!Array.isArray(value.content)) return value
  const text = value.content
    .map((item) => (isRecord(item) ? stringField(item, "text") : ""))
    .filter(Boolean)
    .join("\n")
  if (!text) return null
  const parsed =
    tryParseJSON(text) ?? repairTruncatedJSON(stripTruncationMarker(text))
  return isRecord(parsed) ? parsed : null
}

/** Drops the "..." the runtime appends after cutting a summary. */
function stripTruncationMarker(text: string): string {
  return text.endsWith("...") ? text.slice(0, -3) : text
}

/**
 * Extracts the (possibly truncated) inner JSON string from a serialized MCP
 * envelope by finding the first `"text":"` and JSON-unescaping until the
 * closing quote or end of input.
 */
function extractEnvelopeText(raw: string): string | undefined {
  const marker = '"text":"'
  const start = raw.indexOf(marker)
  if (start < 0) return undefined
  let out = ""
  for (let i = start + marker.length; i < raw.length; i++) {
    const char = raw[i]
    if (char === '"') return out
    if (char !== "\\") {
      out += char
      continue
    }
    const next = raw[i + 1]
    if (next === undefined) break // cut mid-escape
    i++
    switch (next) {
      case "n":
        out += "\n"
        break
      case "t":
        out += "\t"
        break
      case "r":
        out += "\r"
        break
      case "b":
        out += "\b"
        break
      case "f":
        out += "\f"
        break
      case "u": {
        const hex = raw.slice(i + 1, i + 5)
        if (hex.length < 4 || !/^[0-9a-fA-F]{4}$/.test(hex)) return out
        out += String.fromCharCode(parseInt(hex, 16))
        i += 4
        break
      }
      default:
        out += next // covers \" \\ \/ and tolerates unknown escapes
    }
  }
  return out
}

interface RepairCandidate {
  end: number
  closers: string
}

const MAX_REPAIR_ATTEMPTS = 100

/**
 * Best-effort parse of JSON cut off mid-stream: scans for positions where a
 * value just completed, then retries `prefix + missing closers` from the
 * latest such position backwards until one parses.
 */
function repairTruncatedJSON(text: string): unknown | undefined {
  const raw = text.trim()
  if (!raw.startsWith("{") && !raw.startsWith("[")) return undefined

  const stack: string[] = []
  const candidates: RepairCandidate[] = []
  let inString = false
  let escaped = false
  for (let i = 0; i < raw.length; i++) {
    const char = raw[i]
    if (inString) {
      if (escaped) {
        escaped = false
      } else if (char === "\\") {
        escaped = true
      } else if (char === '"') {
        inString = false
        // May be a key rather than a value — parse validation below
        // rejects those candidates.
        pushCandidate(candidates, i + 1, stack)
      }
      continue
    }
    switch (char) {
      case '"':
        inString = true
        break
      case "{":
        stack.push("}")
        break
      case "[":
        stack.push("]")
        break
      case "}":
      case "]":
        if (stack.pop() !== char) return undefined // malformed, not truncated
        pushCandidate(candidates, i + 1, stack)
        break
      case ",":
        pushCandidate(candidates, i, stack)
        break
      default:
        break
    }
  }

  for (const candidate of candidates.slice(-MAX_REPAIR_ATTEMPTS).reverse()) {
    const prefix = raw.slice(0, candidate.end).replace(/,\s*$/, "")
    const parsed = tryParseJSON(prefix + candidate.closers)
    if (parsed !== undefined) return parsed
  }
  return undefined
}

function pushCandidate(
  candidates: RepairCandidate[],
  end: number,
  stack: string[]
) {
  candidates.push({ end, closers: [...stack].reverse().join("") })
  if (candidates.length > MAX_REPAIR_ATTEMPTS * 2) {
    candidates.splice(0, candidates.length - MAX_REPAIR_ATTEMPTS)
  }
}

function tryParseJSON(text: string): unknown | undefined {
  try {
    const parsed: unknown = JSON.parse(text)
    return parsed ?? undefined
  } catch {
    return undefined
  }
}

// --- field helpers ---------------------------------------------------------

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value)
}

function recordField(
  payload: Record<string, unknown> | undefined,
  key: string
): Record<string, unknown> | undefined {
  const value = payload?.[key]
  return isRecord(value) ? value : undefined
}

function stringField(
  payload: Record<string, unknown> | undefined,
  key: string
): string {
  const value = payload?.[key]
  return typeof value === "string" ? value.trim() : ""
}

/** Chunk point IDs may be strings or numbers in Qdrant. */
function idField(payload: Record<string, unknown> | undefined): string {
  const value = payload?.id
  if (typeof value === "string") return value
  if (typeof value === "number") return String(value)
  return ""
}
