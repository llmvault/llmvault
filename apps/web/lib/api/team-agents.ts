"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

export type TeamAgent = components["schemas"]["agentListItem"]

// Local stale time mirrors CHAT_QUERY_STALE_TIME_MS without importing the
// chat-layer cache module into the generic API layer.
const AGENTS_STALE_TIME_MS = 60_000

/** The active top-level agents owned by `teamId`. */
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

/** Filters the caller's visible agents to one selected team. */
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
 * Resolves which agent id a team-scoped form should treat as selected.
 *
 * Selection-preservation rule (consistent across every team+agent form):
 *   1. Keep the user's current pick if it's still one of the team's agents.
 *   2. Otherwise fall back to a supplied preferred agent.
 *   3. Otherwise the first team agent.
 *   4. Otherwise "" (no agent).
 *
 * Because the value is derived (not stored), switching teams updates the
 * available agent list without preserving invalid choices.
 */
export function resolveTeamAgentID(
  agents: ReadonlyArray<{ id?: string }>,
  currentAgentID: string,
  preferredAgentID?: string | null
): string {
  if (currentAgentID && agents.some((agent) => agent.id === currentAgentID)) {
    return currentAgentID
  }
  return (
    agents.find((agent) => agent.id === preferredAgentID)?.id ??
    agents[0]?.id ??
    ""
  )
}
