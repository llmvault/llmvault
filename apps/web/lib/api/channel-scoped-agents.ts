"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { useChannelAgents, type ChannelAgent } from "@/lib/api/channel-agents"

// Local stale time mirrors CHAT_QUERY_STALE_TIME_MS without importing the
// chat-layer cache module into the generic API layer.
const AGENTS_STALE_TIME_MS = 60_000

export interface ChannelScopedAgents {
  /** Agents assigned to the channel (or the org fallback when unavailable). */
  agents: ChannelAgent[]
  isLoading: boolean
  /** True only when both the channel-scoped and the org fallback queries fail. */
  isError: boolean
  /**
   * True when the channel-scoped endpoint failed and `agents` is the full org
   * list instead. Callers that reason about channel assignment (e.g. warning
   * that a fixed agent isn't assigned to a channel) must not trust `agents` in
   * this mode — the list is not channel-filtered.
   */
  isFallback: boolean
}

/**
 * Agents scoped to `channelId` — the only agents that can run sessions in that
 * channel. Matches new-session-view's error-fallback semantics: when the
 * channel-agents endpoint is unavailable (e.g. the backend hasn't shipped it
 * yet) we fall back to the full org agent list so forms keep working.
 *
 * Pass an empty `channelId` to disable both queries (no channel selected yet).
 */
export function useChannelScopedAgents(channelId: string): ChannelScopedAgents {
  const channelAgentsQuery = useChannelAgents(channelId, {
    enabled: Boolean(channelId),
  })
  const orgAgentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    { params: { query: { status: "active", limit: 100 } } },
    {
      enabled: channelAgentsQuery.isError,
      retry: false,
      staleTime: AGENTS_STALE_TIME_MS,
    }
  )
  const isFallback = channelAgentsQuery.isError
  const agents = useMemo(
    () =>
      (isFallback
        ? (orgAgentsQuery.data?.data ?? [])
        : (channelAgentsQuery.data?.data ?? [])
      ).filter((agent) => agent.id),
    [isFallback, channelAgentsQuery.data?.data, orgAgentsQuery.data?.data]
  )
  const isLoading = isFallback
    ? orgAgentsQuery.isLoading
    : channelAgentsQuery.isLoading
  const isError = channelAgentsQuery.isError && orgAgentsQuery.isError
  return { agents, isLoading, isError, isFallback }
}

/**
 * Resolves which agent id a channel-scoped form should treat as selected.
 *
 * Selection-preservation rule (consistent across every channel+agent form):
 *   1. Keep the user's current pick if it's still assigned to the channel.
 *   2. Otherwise fall back to the channel's configured default agent.
 *   3. Otherwise the first assigned agent.
 *   4. Otherwise "" (no agent).
 *
 * Because the value is derived (not stored), switching to a channel that lacks
 * the current agent transparently swaps to that channel's default, and
 * switching back restores the original pick while it stays valid.
 */
export function resolveScopedAgentID(
  agents: ReadonlyArray<{ id?: string }>,
  currentAgentID: string,
  channelDefaultAgentID?: string | null
): string {
  if (currentAgentID && agents.some((agent) => agent.id === currentAgentID)) {
    return currentAgentID
  }
  return (
    agents.find((agent) => agent.id === channelDefaultAgentID)?.id ??
    agents[0]?.id ??
    ""
  )
}
