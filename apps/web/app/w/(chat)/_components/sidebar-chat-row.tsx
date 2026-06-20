"use client"

import { useState } from "react"
import { Tooltip } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  useSessionRuntimeStatus,
  type SessionRuntimeStatus,
} from "@/app/w/(chat)/_stores/session-runtime-store"

export type SidebarSessionAgent = {
  name: string
  icon: string
  avatarURL?: string
}

export function ChatRow({
  sessionId,
  title,
  agent,
  meta,
  active,
  onIntent,
  onSelect,
}: {
  sessionId?: string
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
      <SessionRuntimeIndicator sessionId={sessionId} />
    </button>
  )
}

function SessionRuntimeIndicator({ sessionId }: { sessionId?: string }) {
  const status = useSessionRuntimeStatus(sessionId)
  const indicator = runtimeIndicator(status)
  if (!indicator) return null

  return (
    <Tooltip delay={250} closeDelay={0}>
      <Tooltip.Trigger className="flex h-4 w-4 shrink-0 items-center justify-center">
        <Icon
          icon={indicator.icon}
          className={`h-3.5 w-3.5 ${indicator.className}`}
        />
      </Tooltip.Trigger>
      <Tooltip.Content placement="right" offset={8} className="text-xs">
        {indicator.label}
      </Tooltip.Content>
    </Tooltip>
  )
}

function runtimeIndicator(status: SessionRuntimeStatus) {
  switch (status) {
    case "queued":
      return {
        icon: "lucide:loader-2",
        label: "Agent turn queued",
        className: "animate-spin text-muted",
      }
    case "streaming":
      return {
        icon: "lucide:loader-2",
        label: "Agent turn in progress",
        className: "animate-spin text-primary",
      }
    case "waiting_for_user":
      return {
        icon: "lucide:message-circle-question",
        label: "Waiting for your response",
        className: "text-warning",
      }
    case "stopped":
      return {
        icon: "lucide:square",
        label: "Last turn stopped",
        className: "text-muted",
      }
    case "failed":
      return {
        icon: "lucide:triangle-alert",
        label: "Last turn failed",
        className: "text-danger",
      }
    default:
      return null
  }
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
