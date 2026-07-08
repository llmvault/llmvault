"use client"

import { AgentAvatar } from "@/components/agent-avatar"
import {
  agentAvatarURL,
  agentIcon,
  type SidebarAgentResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"

export function AgentLogo({
  agent,
  className,
}: {
  agent: SidebarAgentResponse
  className: string
}) {
  return (
    <AgentAvatar
      avatarURL={agentAvatarURL(agent)}
      icon={agentIcon(agent)}
      className={`${className} ring-1 ring-border/70`}
      iconClassName="h-3.5 w-3.5"
    />
  )
}
