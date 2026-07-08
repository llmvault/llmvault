"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

export type TeamAgent = components["schemas"]["agentListItem"]

// Local stale time mirrors CHAT_QUERY_STALE_TIME_MS without importing the
// chat-layer cache module into the generic API layer.
const AGENTS_STALE_TIME_MS = 60_000

/**
 * The agents that can run in a channel owned by `teamId`.
 *
 * Agents are team members now — there is no per-channel assignment — so the
 * candidate set for any channel is exactly the agents whose team owns it.
 * Agents without an id are dropped. An empty `teamId` yields an empty list
 * (no channel/team selected yet).
 */
export function agentsForTeam(
  agents: readonly TeamAgent[],
  teamId: string | null | undefined
): TeamAgent[] {
  const team = teamId?.trim()
  if (!team) return []
  return agents.filter((agent) => agent.id && agent.team_id === team)
}

export interface TeamAgents {
  /** The team's agents (empty until a team is known / the list has loaded). */
  agents: TeamAgent[]
  isLoading: boolean
  isError: boolean
}

/**
 * Agents belonging to the team that owns the selected channel. The `/v1/agents`
 * list is already scoped to the caller's visible teams by the backend; we
 * filter it down to the single team that owns the channel the form targets.
 *
 * Pass an empty/undefined `teamId` (no channel selected yet) to get an empty
 * agent list without disabling the shared, cached agents query.
 */
export function useTeamAgents(teamId: string | null | undefined): TeamAgents {
  const query = $api.useQuery(
    "get",
    "/v1/agents",
    { params: { query: { status: "active", limit: 200 } } },
    { retry: false, staleTime: AGENTS_STALE_TIME_MS }
  )
  const agents = useMemo(
    () => agentsForTeam(query.data?.data ?? [], teamId),
    [query.data?.data, teamId]
  )
  return {
    agents,
    isLoading: query.isLoading,
    isError: query.isError,
  }
}

/**
 * Resolves which agent id a channel-scoped form should treat as selected.
 *
 * Selection-preservation rule (consistent across every channel+agent form):
 *   1. Keep the user's current pick if it's still one of the team's agents.
 *   2. Otherwise fall back to the channel's configured default agent.
 *   3. Otherwise the first team agent.
 *   4. Otherwise "" (no agent).
 *
 * Because the value is derived (not stored), switching to a channel on a
 * different team transparently swaps to that channel's default, and switching
 * back restores the original pick while it stays valid.
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
