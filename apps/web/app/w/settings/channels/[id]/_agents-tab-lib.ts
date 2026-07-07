import { type ChannelAgent } from "@/lib/api/channel-agents"
import { extractErrorMessage } from "@/lib/api/error"

// The backend returns a distinct, self-explanatory message for the membership
// failure (a 403). openapi-react-query surfaces only the error body — not the
// HTTP status — so we recognise that failure by its message and swap in
// friendlier copy. All other messages (409 already-assigned, 422 archived /
// default-agent) are already descriptive and surface as-is.
const MEMBERSHIP_ERROR_MESSAGES = new Set([
  "join the channel before managing its agents",
  "forbidden",
])

/**
 * Org agents that are not yet assigned to the channel — the candidates the
 * "add agent" picker should offer. Agents without an id (shouldn't happen) are
 * treated as unassignable and dropped.
 */
export function unassignedAgents(
  orgAgents: ChannelAgent[],
  assignedAgents: ChannelAgent[]
): ChannelAgent[] {
  const assigned = new Set(
    assignedAgents
      .map((agent) => agent.id)
      .filter((id): id is string => Boolean(id))
  )
  return orgAgents.filter((agent) => agent.id && !assigned.has(agent.id))
}

/**
 * Human-readable toast copy for an assign/unassign failure. The 403 membership
 * error is surfaced with clear, self-explanatory copy (recognised by its server
 * message, since the status is not available); other known failures defer to
 * the descriptive server message, falling back to a generic one when the body
 * has none.
 */
export function channelAgentsErrorMessage(
  error: unknown,
  fallback: string
): string {
  const message = extractErrorMessage(error, fallback)
  if (MEMBERSHIP_ERROR_MESSAGES.has(message.trim().toLowerCase())) {
    return "You're not a member of this channel, so you can't change its agents."
  }
  return message
}
