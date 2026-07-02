"use client"

import { Avatar } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
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
  const icon = "icon" in agent && agent.icon ? agent.icon : "bot"

  if (avatarURL) {
    return (
      <Avatar size={size === "lg" ? "lg" : size} className="shrink-0">
        <Avatar.Image src={avatarURL} />
        <Avatar.Fallback>{agentInitials(name)}</Avatar.Fallback>
      </Avatar>
    )
  }

  return (
    <div
      className={cn(
        "bg-default flex shrink-0 items-center justify-center text-muted-foreground",
        dimension
      )}
    >
      <AppIcon icon={icon} className={iconSize} />
    </div>
  )
}
