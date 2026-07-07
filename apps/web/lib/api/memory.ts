"use client"

/**
 * Memory rework API — observations, directives, and channel memory settings
 * (category + mission).
 *
 * Every backend call goes through the generated `$api` hooks (openapi-react-query
 * over openapi-fetch); there is no hand-written fetch or `/api/proxy` URL here.
 * Request/response shapes are derived from the OpenAPI `paths`, so backend
 * contract drift fails typecheck. The only bespoke layer is a pure transform
 * from the generated snake_case response into the camelCase domain shapes
 * (`Observation`/`Directive`) the UI consumes.
 */

import { useMemo } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import type { components } from "./schema"

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
  /**
   * Curated default mission for the channel's category ("" for general).
   * When memoryMission is empty this is the mission extraction actually
   * uses, so the UI shows it as the effective value.
   */
  defaultMemoryMission: string
}

export function channelMemorySettings(channel: unknown): ChannelMemorySettings {
  const record = (channel ?? {}) as Record<string, unknown>
  return {
    category: typeof record.category === "string" ? record.category : "",
    memoryMission:
      typeof record.memory_mission === "string" ? record.memory_mission : "",
    defaultMemoryMission:
      typeof record.default_memory_mission === "string"
        ? record.default_memory_mission
        : "",
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

// ---------------------------------------------------------------------------
// Domain shapes + normalizers (pure transform over the generated responses)
// ---------------------------------------------------------------------------

type ObservationResponse = components["schemas"]["observationResponse"]
type DirectiveResponse = components["schemas"]["directiveResponse"]

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

export type DirectiveSource = "user-pinned" | "extracted-confirmed"

export type Directive = {
  id: string
  channelId: string | null
  content: string
  source: DirectiveSource
  active: boolean
  createdAt: string
}

function toObservation(raw: ObservationResponse): Observation {
  return {
    id: raw.id ?? "",
    content: raw.content ?? "",
    kind: raw.kind ?? "",
    entities: (raw.entities ?? []).filter(
      (entity): entity is string => typeof entity === "string"
    ),
    proofCount: typeof raw.proof_count === "number" ? raw.proof_count : 1,
    lastMentionedAt: raw.last_mentioned_at ?? "",
    expiresAt: raw.expires_at || null,
    humanVerified: Boolean(raw.human_verified),
    createdAt: raw.created_at ?? "",
    archivedAt: raw.archived_at || null,
  }
}

function toDirective(raw: DirectiveResponse): Directive {
  const source = raw.source ?? ""
  return {
    id: raw.id ?? "",
    channelId: raw.channel_id || null,
    content: raw.content ?? "",
    source: source === "extracted-confirmed" ? source : "user-pinned",
    active: raw.active === undefined ? true : Boolean(raw.active),
    createdAt: raw.created_at ?? "",
  }
}

// ---------------------------------------------------------------------------
// Query keys + invalidation (openapi-react-query `[method, path, init]` shape)
// ---------------------------------------------------------------------------

/** Observations page size for the paginated list. */
export const OBSERVATIONS_PAGE_SIZE = 30

/**
 * openapi-react-query cache-key prefix for a channel's observation list. The
 * generated hooks key on `["get", path, init]`; scoping the init to the channel
 * lets a mutation invalidate exactly that channel's list (any page size / page).
 */
function observationsKey(channelId: string) {
  return [
    "get",
    "/v1/observations",
    { params: { query: { channel_id: channelId } } },
  ] as const
}

function directivesKey(channelId: string) {
  return [
    "get",
    "/v1/directives",
    { params: { query: { channel_id: channelId } } },
  ] as const
}

// ---------------------------------------------------------------------------
// Observations
// ---------------------------------------------------------------------------

export type ObservationsController = {
  observations: Observation[]
  hasMore: boolean
  isLoading: boolean
  isError: boolean
  isLoadingMore: boolean
  loadMore: () => void
}

/**
 * Paginated observations for a channel, flattened + mapped to the domain shape.
 * Offset pagination is handled by openapi-react-query's infinite query: each
 * page's `offset` is derived from the running count of already-loaded rows.
 */
export function useObservations(channelId: string): ObservationsController {
  const query = $api.useInfiniteQuery(
    "get",
    "/v1/observations",
    {
      params: {
        query: { channel_id: channelId, limit: OBSERVATIONS_PAGE_SIZE },
      },
    },
    {
      pageParamName: "offset",
      initialPageParam: 0,
      getNextPageParam: (lastPage, allPages) => {
        if (!lastPage.has_more) return undefined
        return allPages.reduce(
          (total, page) => total + (page.data?.length ?? 0),
          0
        )
      },
      enabled: Boolean(channelId),
    }
  )

  const observations = useMemo(() => {
    const seen = new Set<string>()
    const result: Observation[] = []
    for (const page of query.data?.pages ?? []) {
      for (const raw of page.data ?? []) {
        const observation = toObservation(raw)
        if (observation.id) {
          if (seen.has(observation.id)) continue
          seen.add(observation.id)
        }
        result.push(observation)
      }
    }
    return result
  }, [query.data])

  return {
    observations,
    hasMore: Boolean(query.hasNextPage),
    isLoading: query.isLoading,
    isError: query.isError,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      if (query.hasNextPage && !query.isFetchingNextPage) {
        void query.fetchNextPage()
      }
    },
  }
}

export function useConfirmObservation(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("post", "/v1/observations/{id}/confirm", {
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: observationsKey(channelId) }),
  })
}

export function useCorrectObservation(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("post", "/v1/observations/{id}/correct", {
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: observationsKey(channelId) }),
  })
}

export function useDeleteObservation(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("delete", "/v1/observations/{id}", {
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: observationsKey(channelId) }),
  })
}

/** Pinning promotes an observation to a directive, so both lists refresh. */
export function usePinObservation(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("post", "/v1/observations/{id}/pin", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: observationsKey(channelId) })
      queryClient.invalidateQueries({ queryKey: directivesKey(channelId) })
    },
  })
}

// ---------------------------------------------------------------------------
// Directives
// ---------------------------------------------------------------------------

export type DirectivesController = {
  directives: Directive[]
  isLoading: boolean
  isError: boolean
}

/** Channel directives, mapped to the domain shape. */
export function useDirectives(channelId: string): DirectivesController {
  const query = $api.useQuery(
    "get",
    "/v1/directives",
    { params: { query: { channel_id: channelId } } },
    { enabled: Boolean(channelId) }
  )

  const directives = useMemo(
    () => (query.data?.data ?? []).map(toDirective),
    [query.data]
  )

  return {
    directives,
    isLoading: query.isLoading,
    isError: query.isError,
  }
}

export function useCreateDirective(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("post", "/v1/directives", {
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: directivesKey(channelId) }),
  })
}

/**
 * Directive content is immutable once created; PATCH only toggles the active
 * flag. Delete and re-add a directive to change its text.
 */
export function useUpdateDirective(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("patch", "/v1/directives/{id}", {
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: directivesKey(channelId) }),
  })
}

export function useDeleteDirective(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("delete", "/v1/directives/{id}", {
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: directivesKey(channelId) }),
  })
}
