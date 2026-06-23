"use client"

import { useState } from "react"
import { Tooltip } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  useSessionRuntimeStatus,
  type SessionRuntimeStatus,
} from "@/app/w/(chat)/_stores/session-runtime-store"
import { cn } from "@/lib/utils"
import { SessionActionsMenu } from "./session-actions-menu"

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
  onRename,
  onShare,
}: {
  sessionId?: string
  title: string
  agent: SidebarSessionAgent
  meta?: string
  active?: boolean
  onIntent?: () => void
  onSelect: () => void
  onRename?: () => void
  onShare?: () => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)

  return (
    <div
      onMouseEnter={onIntent}
      className={cn(
        "group flex items-center gap-1 rounded-lg pr-1 text-sm transition-colors",
        active ? "bg-default" : "hover:bg-default"
      )}
    >
      <button
        type="button"
        onFocus={onIntent}
        onPointerDown={onIntent}
        onClick={onSelect}
        className="flex min-w-0 flex-1 items-center gap-2 py-1.5 pl-9 text-left"
      >
        <span className="flex min-w-0 flex-1 items-center gap-1">
          <SessionAgentAvatar agent={agent} />
          <span className="min-w-0 flex-1 truncate">{title}</span>
        </span>
      </button>
      <SessionRuntimeIndicator sessionId={sessionId} />
      <span className="flex h-7 w-12 shrink-0 items-center justify-end">
        {meta ? (
          <span
            className={cn(
              "px-1 text-xs text-muted",
              menuOpen
                ? "hidden"
                : "group-focus-within:hidden group-hover:hidden"
            )}
          >
            {meta}
          </span>
        ) : null}
        <SessionActionsMenu
          ariaLabel="Chat options"
          onOpenChange={setMenuOpen}
          onRename={onRename}
          onShare={onShare}
          placement="bottom end"
          triggerClassName="rounded-md p-1 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto data-[open=true]:opacity-100 data-[open=true]:pointer-events-auto hover:bg-surface"
        />
      </span>
    </div>
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
