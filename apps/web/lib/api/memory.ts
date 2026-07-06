/**
 * Memory rework API client — observations, directives, and channel memory
 * settings (category + mission).
 *
 * All endpoint paths for the memory rework live HERE and only here, so they
 * can be reconciled with the backend in one place. Paths and shapes are
 * reconciled against the server routes: observations list uses
 * GET /v1/observations?channel_id=&limit=&offset= (offset pagination),
 * corrections use POST /v1/observations/{id}/correct, and directives list
 * uses GET /v1/directives?channel_id=.
 */

export const CHANNEL_CATEGORIES = [
  {
    value: "customer-support",
    label: "Customer Support",
    description:
      "Remembers issue patterns and playbooks — never individual customers.",
  },
  {
    value: "account",
    label: "Account / Client",
    description: "Remembers everything durable about this client.",
  },
  {
    value: "engineering",
    label: "Engineering",
    description: "Decisions, conventions, incident learnings.",
  },
  {
    value: "operations",
    label: "Operations",
    description: "Processes, vendors, ownership.",
  },
  {
    value: "sales",
    label: "Sales",
    description: "Pricing decisions, deal learnings, competitor facts.",
  },
  {
    value: "marketing",
    label: "Marketing",
    description: "Positioning, campaign outcomes, audience learnings.",
  },
  {
    value: "people",
    label: "People / HR",
    description: "Policies and structure — never individual personal details.",
  },
  {
    value: "general",
    label: "General",
    description: "Balanced memory defaults for everything else.",
  },
] as const

export type ChannelCategory = (typeof CHANNEL_CATEGORIES)[number]["value"]

export function categoryLabel(value: string): string {
  return (
    CHANNEL_CATEGORIES.find((category) => category.value === value)?.label ??
    value
  )
}

/** Channel fields added by the memory rework, not yet in the OpenAPI schema. */
export type ChannelMemorySettings = {
  category: string
  memoryMission: string
}

export function channelMemorySettings(channel: unknown): ChannelMemorySettings {
  const record = (channel ?? {}) as Record<string, unknown>
  return {
    category: typeof record.category === "string" ? record.category : "",
    memoryMission:
      typeof record.memory_mission === "string" ? record.memory_mission : "",
  }
}

export const OBSERVATION_KINDS: Record<
  string,
  { label: string; chipClass: string }
> = {
  preference: { label: "Preference", chipClass: "bg-sky-500/15 text-sky-500" },
  rule: { label: "Rule", chipClass: "bg-warning/15 text-warning" },
  decision: {
    label: "Decision",
    chipClass: "bg-violet-500/15 text-violet-500",
  },
  convention: {
    label: "Convention",
    chipClass: "bg-teal-500/15 text-teal-500",
  },
  "org-fact": { label: "Org fact", chipClass: "bg-primary/15 text-primary" },
  person: { label: "Person", chipClass: "bg-pink-500/15 text-pink-500" },
  workaround: {
    label: "Workaround",
    chipClass: "bg-orange-500/15 text-orange-500",
  },
  commitment: {
    label: "Commitment",
    chipClass: "bg-emerald-500/15 text-emerald-600",
  },
  finding: {
    label: "Finding",
    chipClass: "bg-indigo-500/15 text-indigo-500",
  },
}

export function observationKind(kind: string): {
  label: string
  chipClass: string
} {
  return (
    OBSERVATION_KINDS[kind] ?? {
      label: kind || "Memory",
      chipClass: "bg-default text-muted-foreground",
    }
  )
}

export type Observation = {
  id: string
  content: string
  kind: string
  entities: string[]
  proofCount: number
  lastMentionedAt: string
  expiresAt: string | null
  humanVerified: boolean
  createdAt: string
  archivedAt: string | null
}

export type ObservationPage = {
  observations: Observation[]
  hasMore: boolean
}

export type DirectiveSource = "user-pinned" | "extracted-confirmed"

export type Directive = {
  id: string
  channelId: string | null
  content: string
  source: DirectiveSource
  active: boolean
  createdAt: string
}

export const memoryQueryKeys = {
  observations: (channelId: string) =>
    ["memory", "observations", channelId] as const,
  directives: (channelId: string) =>
    ["memory", "directives", channelId] as const,
}

// ---------------------------------------------------------------------------
// Requests
// ---------------------------------------------------------------------------

const PROXY_BASE = "/api/proxy"

async function request<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  const response = await fetch(`${PROXY_BASE}${path}`, {
    method,
    headers:
      body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const text = await response.text()
  const data = text ? safeParseJSON(text) : null
  if (!response.ok) {
    throw data ?? new Error(`Request failed (${response.status})`)
  }
  return (data ?? {}) as T
}

function safeParseJSON(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

// ---------------------------------------------------------------------------
// Observations
// ---------------------------------------------------------------------------

export async function listObservations(
  channelId: string,
  options: { offset?: number; limit?: number } = {}
): Promise<ObservationPage> {
  const params = new URLSearchParams()
  params.set("channel_id", channelId)
  if (options.offset) params.set("offset", String(options.offset))
  if (options.limit) params.set("limit", String(options.limit))
  const data = await request<{
    data?: unknown[]
    has_more?: boolean
  }>("GET", `/v1/observations?${params.toString()}`)
  return {
    observations: (data.data ?? []).map(toObservation),
    hasMore: Boolean(data.has_more),
  }
}

export function confirmObservation(id: string): Promise<unknown> {
  return request("POST", `/v1/observations/${encodeURIComponent(id)}/confirm`)
}

export function correctObservation(
  id: string,
  content: string
): Promise<unknown> {
  return request(
    "POST",
    `/v1/observations/${encodeURIComponent(id)}/correct`,
    { content }
  )
}

export function deleteObservation(id: string): Promise<unknown> {
  return request("DELETE", `/v1/observations/${encodeURIComponent(id)}`)
}

export function pinObservation(id: string): Promise<unknown> {
  return request("POST", `/v1/observations/${encodeURIComponent(id)}/pin`)
}

// ---------------------------------------------------------------------------
// Directives
// ---------------------------------------------------------------------------

export async function listDirectives(channelId: string): Promise<Directive[]> {
  const params = new URLSearchParams()
  params.set("channel_id", channelId)
  const data = await request<{ data?: unknown[] }>(
    "GET",
    `/v1/directives?${params.toString()}`
  )
  return (data.data ?? []).map(toDirective)
}

export function createDirective(
  channelId: string,
  content: string
): Promise<unknown> {
  return request("POST", "/v1/directives", {
    channel_id: channelId,
    content,
  })
}

/**
 * Directive content is immutable once created; PATCH only toggles the active
 * flag. Delete and re-add a directive to change its text.
 */
export function updateDirective(
  id: string,
  patch: { active: boolean }
): Promise<unknown> {
  return request("PATCH", `/v1/directives/${encodeURIComponent(id)}`, patch)
}

export function deleteDirective(id: string): Promise<unknown> {
  return request("DELETE", `/v1/directives/${encodeURIComponent(id)}`)
}

// ---------------------------------------------------------------------------
// Normalizers
// ---------------------------------------------------------------------------

function toObservation(raw: unknown): Observation {
  const record = (raw ?? {}) as Record<string, unknown>
  return {
    id: stringField(record.id),
    content: stringField(record.content),
    kind: stringField(record.kind),
    entities: Array.isArray(record.entities)
      ? record.entities.filter(
          (entity): entity is string => typeof entity === "string"
        )
      : [],
    proofCount:
      typeof record.proof_count === "number" ? record.proof_count : 1,
    lastMentionedAt: stringField(record.last_mentioned_at),
    expiresAt: stringField(record.expires_at) || null,
    humanVerified: Boolean(record.human_verified),
    createdAt: stringField(record.created_at),
    archivedAt: stringField(record.archived_at) || null,
  }
}

function toDirective(raw: unknown): Directive {
  const record = (raw ?? {}) as Record<string, unknown>
  const source = stringField(record.source)
  return {
    id: stringField(record.id),
    channelId: stringField(record.channel_id) || null,
    content: stringField(record.content),
    source: source === "extracted-confirmed" ? source : "user-pinned",
    active: record.active === undefined ? true : Boolean(record.active),
    createdAt: stringField(record.created_at),
  }
}

function stringField(value: unknown): string {
  return typeof value === "string" ? value : ""
}
