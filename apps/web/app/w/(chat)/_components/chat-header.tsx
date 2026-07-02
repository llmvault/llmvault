"use client"

import { memo, useMemo } from "react"
import { Avatar, Button, Tooltip } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { CHAT_QUERY_STALE_TIME_MS } from "@/app/w/(chat)/_lib/chat-cache"
import { ChatHeaderAgentLogo } from "./chat-header-agent-logo"
import type { ChatHeaderAgent } from "./chat-header-types"
import { SessionActionsMenu } from "./session-actions-menu"

type OrgMember = components["schemas"]["orgMemberResponse"]

const PRESENCE_MAX_AVATARS = 4

export const ChatHeader = memo(function ChatHeader({
  title,
  agent,
  sessionId,
  sidebarOpen,
  onExpandSidebar,
  onRename,
  onShare,
  onArchive,
  rightOpen,
  onToggleRight,
}: {
  title: string
  agent: ChatHeaderAgent | null
  sessionId?: string
  sidebarOpen: boolean
  onExpandSidebar: () => void
  onRename?: () => void
  onShare?: () => void
  onArchive?: () => void
  rightOpen: boolean
  onToggleRight: () => void
}) {
  return (
    <div className="flex h-12 shrink-0 items-center gap-1 px-3">
      {!sidebarOpen ? (
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Expand sidebar"
          onPress={onExpandSidebar}
        >
          <Icon icon="lucide:panel-left-open" className="h-4 w-4" />
        </Button>
      ) : null}

      <span className="truncate px-1 text-sm font-medium">{title}</span>
      {agent ? (
        // The agent is fixed once a session exists, so this is a read-only
        // chip rather than a switcher.
        <span
          title="The agent can't be changed after a session starts"
          className="flex shrink-0 cursor-default items-center gap-1.5 rounded-full border border-border px-2 py-0.5 text-xs text-muted"
        >
          <ChatHeaderAgentLogo agent={agent} />
          {agent.name}
        </span>
      ) : null}
      <SessionActionsMenu
        onRename={onRename}
        onShare={onShare}
        onArchive={onArchive}
      />

      <div className="flex-1" />

      <PresenceStack sessionId={sessionId} />

      <div className="flex items-center gap-0.5">
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Toggle side panel"
          onPress={onToggleRight}
        >
          <Icon
            icon={rightOpen ? "lucide:panel-right-close" : "lucide:panel-right"}
            className="h-4 w-4 text-muted"
          />
        </Button>
      </div>
    </div>
  )
})

function PresenceStack({ sessionId }: { sessionId?: string }) {
  const enabled = Boolean(sessionId)
  const sessionQuery = $api.useQuery(
    "get",
    "/v1/sessions/{id}",
    { params: { path: { id: sessionId ?? "" } } },
    { enabled, retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const membersQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/members",
    {},
    { enabled, retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const participants = sessionQuery.data?.participants
  const people = useMemo(() => {
    const membersByID = new Map(
      (membersQuery.data?.data ?? []).flatMap((member) =>
        member.user_id ? ([[member.user_id, member]] as const) : []
      )
    )
    return (participants ?? []).flatMap((participant) => {
      if (!participant.user_id) return []
      const member = membersByID.get(participant.user_id)
      const label = member ? memberLabel(member) : "Unknown member"
      return [{ id: participant.user_id, label, initials: initials(label) }]
    })
  }, [membersQuery.data?.data, participants])

  if (!sessionId || people.length === 0) return null

  const visible = people.slice(0, PRESENCE_MAX_AVATARS)
  const overflow = people.length - visible.length

  return (
    <div className="flex shrink-0 items-center px-1.5">
      <div className="flex items-center -space-x-1.5">
        {visible.map((person) => (
          <Tooltip key={person.id} delay={250} closeDelay={0}>
            <Tooltip.Trigger className="flex shrink-0 items-center">
              <Avatar size="sm" className="h-6 w-6 ring-2 ring-surface">
                <Avatar.Fallback className="text-[10px]">
                  {person.initials}
                </Avatar.Fallback>
              </Avatar>
            </Tooltip.Trigger>
            <Tooltip.Content placement="bottom" offset={8} className="text-xs">
              {person.label}
            </Tooltip.Content>
          </Tooltip>
        ))}
        {overflow > 0 ? (
          <span className="z-10 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-default text-[10px] font-medium text-muted ring-2 ring-surface">
            +{overflow}
          </span>
        ) : null}
      </div>
    </div>
  )
}

function memberLabel(member: OrgMember) {
  return member.name?.trim() || member.email?.trim() || "Unknown member"
}

function initials(label: string) {
  const parts = label.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return label.slice(0, 2).toUpperCase()
}
