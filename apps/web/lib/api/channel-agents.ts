"use client"

import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import type { paths } from "./schema"

// Thin composition over the generated `$api` hooks for the channel-agents
// endpoints. No fetch, no manual URLs, no hand-written types — request and
// response shapes come straight from the OpenAPI `paths`, so backend contract
// drift fails typecheck here.

type ChannelAgentsListResponse =
  paths["/v1/channels/{id}/agents"]["get"]["responses"]["200"]["content"]["application/json"]

export type ChannelAgent = NonNullable<ChannelAgentsListResponse["data"]>[number]

/**
 * openapi-react-query cache key for a channel's assigned-agents query, matching
 * the `[method, path, init]` convention the generated hooks emit. Used for
 * targeted invalidation after an assign/unassign mutation.
 */
export function channelAgentsQueryKey(channelId: string) {
  return [
    "get",
    "/v1/channels/{id}/agents",
    { params: { path: { id: channelId } } },
  ] as const
}

/** Assigned-agents query for a channel. Data is the raw `{ data: [...] }` body. */
export function useChannelAgents(
  channelId: string,
  options?: { enabled?: boolean }
) {
  return $api.useQuery(
    "get",
    "/v1/channels/{id}/agents",
    { params: { path: { id: channelId } } },
    { enabled: options?.enabled ?? Boolean(channelId), retry: false }
  )
}

/**
 * Assigns an agent to the channel and invalidates its agent list on success.
 * `mutate` takes the generated init: `{ params: { path: { id } }, body: { agent_id } }`.
 */
export function useAssignChannelAgent(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("post", "/v1/channels/{id}/agents", {
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: channelAgentsQueryKey(channelId),
      }),
  })
}

/**
 * Removes an agent from the channel and invalidates its agent list on success.
 * `mutate` takes the generated init: `{ params: { path: { id, agentID } } }`.
 */
export function useUnassignChannelAgent(channelId: string) {
  const queryClient = useQueryClient()
  return $api.useMutation("delete", "/v1/channels/{id}/agents/{agentID}", {
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: channelAgentsQueryKey(channelId),
      }),
  })
}
