import { SlackIcon } from "@hugeicons/core-free-icons"
import type { EmployeeSession, SessionSegment } from "@/lib/sessions/types"

export const sessionSegments: Array<{
  id: SessionSegment
  label: string
  source: string
  icon?: typeof SlackIcon
}> = [
  { id: "web", label: "Web", source: "web" },
  { id: "slack", label: "Slack", source: "gateway", icon: SlackIcon },
]

export function groupSessions(sessions: EmployeeSession[]) {
  const groups = new Map<string, EmployeeSession[]>()
  for (const session of sessions) {
    const label = dateGroup(
      session.last_activity_at ?? session.updated_at ?? session.created_at
    )
    groups.set(label, [...(groups.get(label) ?? []), session])
  }
  return Array.from(groups.entries()).map(([label, groupSessions]) => ({
    label,
    sessions: groupSessions,
  }))
}

function dateGroup(value?: string) {
  if (!value) return "Unknown"
  const date = new Date(value)
  const now = new Date()
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startDate = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate()
  )
  const diffDays = Math.floor(
    (startToday.getTime() - startDate.getTime()) / 86400000
  )
  if (diffDays === 0) return "Today"
  if (diffDays === 1) return "Yesterday"
  if (diffDays < 7) return "This week"
  return new Intl.DateTimeFormat(undefined, {
    month: "long",
    year: "numeric",
  }).format(date)
}

export function sessionTitle(session: EmployeeSession) {
  return (
    session.name ||
    session.source_resource_key ||
    session.runtime_conversation_id ||
    "Untitled session"
  )
}

export function formatDateTime(value?: string) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(date)
}

export function formatShortTime(value?: string) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).format(date)
}
