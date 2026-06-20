"use client"

import { useState } from "react"
import { Tooltip } from "@heroui/react"
import { Icon } from "@iconify/react"

export type SidebarSessionAgent = {
  name: string
  icon: string
  avatarURL?: string
}

export function ChatRow({
  title,
  agent,
  meta,
  active,
  onIntent,
  onSelect,
}: {
  title: string
  agent: SidebarSessionAgent
  meta?: string
  active?: boolean
  onIntent?: () => void
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onFocus={onIntent}
      onMouseEnter={onIntent}
      onPointerDown={onIntent}
      onClick={onSelect}
      className={`flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9 text-left text-sm transition-colors ${
        active ? "bg-default" : "hover:bg-default"
      }`}
    >
      <span className="flex min-w-0 flex-1 items-center gap-1">
        <SessionAgentAvatar agent={agent} />
        <span className="min-w-0 flex-1 truncate">{title}</span>
      </span>
      {meta ? (
        <span className="shrink-0 text-xs text-muted">{meta}</span>
      ) : null}
    </button>
  )
}

function SessionAgentAvatar({ agent }: { agent: SidebarSessionAgent }) {
  const [failed, setFailed] = useState(false)

  return (
    <Tooltip delay={250} closeDelay={0}>
      <Tooltip.Trigger className="flex h-3 w-3 shrink-0 items-center justify-center">
        <span className="bg-default flex h-3 w-3 items-center justify-center overflow-hidden rounded-full text-muted ring-1 ring-border/70">
          {agent.avatarURL && !failed ? (
            <>
              {/* eslint-disable-next-line @next/next/no-img-element -- agent avatars can come from arbitrary workspace-configured URLs */}
              <img
                src={agent.avatarURL}
                alt=""
                className="h-full w-full object-cover"
                onError={() => setFailed(true)}
              />
            </>
          ) : (
            <Icon icon={agent.icon} className="h-2 w-2" />
          )}
        </span>
      </Tooltip.Trigger>
      <Tooltip.Content placement="right" offset={8} className="text-xs">
        {agent.name}
      </Tooltip.Content>
    </Tooltip>
  )
}
