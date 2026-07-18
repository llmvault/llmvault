import type { components } from "@/lib/api/schema"

export type SidebarSessionResponse = components["schemas"]["sessionResponse"]
export type SidebarAgentResponse = components["schemas"]["agentListItem"]
export type SidebarTeamResponse = components["schemas"]["teamResponse"]

export function slugify(value: string): string {
  const slug = value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return slug || "item"
}

interface SidebarTeamGroup {
  key: string
  teamId: string
  name: string
  sessions: SidebarSessionResponse[]
}

function teamGroupFallbackLabel(teamId: string): string {
  return `Team ${teamId}`
}

export function buildSidebarTeamGroups(
  teams: SidebarTeamResponse[],
  sessions: SidebarSessionResponse[]
): SidebarTeamGroup[] {
  const groups: SidebarTeamGroup[] = []
  for (const team of teams) {
    const teamId = team.id?.trim()
    if (!teamId) continue
    groups.push({
      key: teamId,
      teamId,
      name: team.name?.trim() || teamGroupFallbackLabel(teamId),
      sessions: sessions
        .filter((session) => sessionTeamID(session) === teamId)
        .sort((left, right) => sessionTimestamp(right) - sessionTimestamp(left)),
    })
  }
  return groups
}

export function sessionTeamID(session: SidebarSessionResponse): string | undefined {
  return (session as SidebarSessionResponse & { team_id?: string }).team_id
}

export function sessionDisplayName(session: SidebarSessionResponse): string {
  return session.name?.trim() || "Untitled"
}

export function agentDisplayName(agent: SidebarAgentResponse): string {
  return agent.name?.trim() || "Untitled agent"
}

export function agentIcon(agent?: SidebarAgentResponse): string {
  return agent?.icon?.trim() || "bot"
}

export function agentAvatarURL(
  agent?: SidebarAgentResponse
): string | undefined {
  const avatarURL = agent?.avatar_url?.trim()
  if (avatarURL) return avatarURL

  const catalogAvatarURL = agent?.catalog?.avatar_url?.trim()
  return catalogAvatarURL || undefined
}

export function agentModel(agent?: SidebarAgentResponse): string | undefined {
  return agent?.model?.trim() || undefined
}

export function sessionActivityLabel(session: SidebarSessionResponse): string {
  const timestamp =
    session.last_activity_at ?? session.updated_at ?? session.created_at
  if (!timestamp) return ""

  const time = Date.parse(timestamp)
  if (Number.isNaN(time)) return ""

  const diff = Math.max(0, Date.now() - time)
  const minute = 60 * 1000
  const hour = 60 * minute
  const day = 24 * hour
  const week = 7 * day

  if (diff < minute) return "now"
  if (diff < hour) return `${Math.floor(diff / minute)}m`
  if (diff < day) return `${Math.floor(diff / hour)}h`
  if (diff < week) return `${Math.floor(diff / day)}d`
  if (diff < 5 * week) return `${Math.floor(diff / week)}w`

  return new Intl.DateTimeFormat("en", {
    month: "short",
    day: "numeric",
  }).format(new Date(time))
}

function timestampValue(value?: string): number | undefined {
  if (!value) return undefined
  const time = Date.parse(value)
  return Number.isNaN(time) ? undefined : time
}

function sessionTimestamp(session: SidebarSessionResponse): number {
  return (
    timestampValue(
      session.last_activity_at ?? session.updated_at ?? session.created_at
    ) ?? 0
  )
}

export function dedupeSessions(sessions: SidebarSessionResponse[]) {
  const seen = new Set<string>()
  const result: SidebarSessionResponse[] = []
  for (const session of sessions) {
    if (!session.id || seen.has(session.id)) continue
    seen.add(session.id)
    result.push(session)
  }
  return result
}

export function sessionRouteFromPathname(
  pathname: string
): { sessionId: string } | null {
  const match = pathname.match(/^\/w\/sessions\/([^/]+)$/)
  if (!match) return null
  return {
    sessionId: decodeURIComponent(match[1]),
  }
}
