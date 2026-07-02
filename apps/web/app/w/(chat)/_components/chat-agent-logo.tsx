"use client"

import { useState } from "react"
import { AppIcon } from "@/components/icon"
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
  const [failed, setFailed] = useState(false)
  const avatarURL = agentAvatarURL(agent)
  const fallbackIcon = agentIcon(agent)
  const frameClassName = `${className} bg-default flex shrink-0 items-center justify-center overflow-hidden text-muted ring-1 ring-border/70`

  if (avatarURL && !failed) {
    return (
      <span className={frameClassName}>
        {/* eslint-disable-next-line @next/next/no-img-element -- agent avatars can come from arbitrary workspace-configured URLs */}
        <img
          src={avatarURL}
          alt=""
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      </span>
    )
  }

  return (
    <span className={frameClassName}>
      <AppIcon icon={fallbackIcon} className="h-3.5 w-3.5" />
    </span>
  )
}
