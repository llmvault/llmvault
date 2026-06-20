"use client"

import { useState } from "react"
import { Icon } from "@iconify/react"
import type { ChatHeaderAgent } from "./chat-header-types"

export function ChatHeaderAgentLogo({ agent }: { agent: ChatHeaderAgent }) {
  const [failed, setFailed] = useState(false)
  if (agent.avatarURL && !failed) {
    return (
      <span className="bg-default flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden rounded-[5px] ring-1 ring-border/70">
        {/* eslint-disable-next-line @next/next/no-img-element -- agent avatars can come from arbitrary workspace-configured URLs */}
        <img
          src={agent.avatarURL}
          alt=""
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      </span>
    )
  }

  return <Icon icon={agent.icon} className="h-3.5 w-3.5 shrink-0" />
}
