import type { components } from "@/lib/api/schema"

export type SidebarChannelResponse = components["schemas"]["channelResponse"]
export type SidebarSessionResponse = components["schemas"]["sessionResponse"]
export type SidebarAgentResponse = components["schemas"]["agentListItem"]

export function slugify(value: string): string {
  const slug = value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return slug || "channel"
}

export function channelDisplayName(channel: SidebarChannelResponse): string {
  return channel.name?.trim() || "Untitled channel"
}

export function channelRouteSlug(channel: SidebarChannelResponse): string {
  return slugify(channelDisplayName(channel))
}

export function sortChannelsByRecentSession(
  channels: SidebarChannelResponse[],
  latestSessionsByChannelID: Map<string, SidebarSessionResponse | null>
): SidebarChannelResponse[] {
  return channels
    .map((channel, index) => {
      const latestSession = channel.id
        ? latestSessionsByChannelID.get(channel.id)
        : undefined
      return {
        channel,
        index,
        hasSession: Boolean(latestSession),
        timestamp:
          timestampValue(latestSession?.last_activity_at) ??
          timestampValue(latestSession?.updated_at) ??
          timestampValue(latestSession?.created_at) ??
          timestampValue(channel.updated_at) ??
          timestampValue(channel.created_at) ??
          0,
      }
    })
    .sort((left, right) => {
      if (left.hasSession !== right.hasSession) {
        return left.hasSession ? -1 : 1
      }
      if (left.timestamp !== right.timestamp) {
        return right.timestamp - left.timestamp
      }
      return left.index - right.index
    })
    .map(({ channel }) => channel)
}

export function channelRouteSlugCounts(channels: SidebarChannelResponse[]) {
  const counts = new Map<string, number>()
  for (const channel of channels) {
    const slug = channelRouteSlug(channel)
    counts.set(slug, (counts.get(slug) ?? 0) + 1)
  }
  return counts
}

export function isAmbiguousChannelRouteSlug(
  channels: SidebarChannelResponse[],
  slug: string
): boolean {
  return (channelRouteSlugCounts(channels).get(slug) ?? 0) > 1
}

export function findChannelByRouteSlug(
  channels: SidebarChannelResponse[],
  slug: string
): SidebarChannelResponse | undefined {
  if (isAmbiguousChannelRouteSlug(channels, slug)) return undefined
  return channels.find((channel) => channelRouteSlug(channel) === slug)
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
): { channelSlug: string; sessionId: string } | null {
  const match = pathname.match(/^\/w\/channels\/([^/]+)\/([^/]+)$/)
  if (!match) return null
  return {
    channelSlug: decodeURIComponent(match[1]),
    sessionId: decodeURIComponent(match[2]),
  }
}
