"use client"

import { AgentAvatar as AgentAvatarBase } from "@/components/agent-avatar"
import {
  agentAvatarURL,
  agentInitials,
  agentName,
  type CatalogAgent,
  type InstalledAgent,
} from "./_lib"

export function AgentAvatar({
  agent,
  size,
}: {
  agent: CatalogAgent | InstalledAgent
  size: "sm" | "md" | "lg"
}) {
  const avatarURL = agentAvatarURL(agent)
  const name = agentName(agent)
  const dimension =
    size === "sm"
      ? "h-6 w-6 rounded-md"
      : size === "md"
        ? "h-9 w-9 rounded-lg"
        : "h-12 w-12 rounded-xl"
  const iconSize =
    size === "sm"
      ? "h-3.5 w-3.5"
      : size === "md"
        ? "h-[18px] w-[18px]"
        : "h-6 w-6"
  const textSize =
    size === "sm" ? "text-[10px]" : size === "md" ? "text-xs" : "text-sm"
  const icon = "icon" in agent && agent.icon ? agent.icon : "bot"

  return (
    <AgentAvatarBase
      avatarURL={avatarURL}
      icon={icon}
      className={dimension}
      iconClassName={iconSize}
      imageFallback={
        <span className={`font-medium ${textSize}`}>{agentInitials(name)}</span>
      }
    />
  )
}
