"use client"

import type { ReactNode } from "react"
import { Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { SidebarAgentResponse } from "@/app/w/(chat)/_lib/sidebar-data"
import { AgentLogo } from "./chat-agent-logo"
import { ModelIcon, type DisplayModel } from "./model-display"

export function Picker({
  open,
  setOpen,
  label,
  icon,
  agent,
  model,
  value,
  suffix,
  width,
  children,
}: {
  open: boolean
  setOpen: (open: boolean) => void
  label: string
  icon?: string
  agent?: SidebarAgentResponse
  model?: DisplayModel
  value: string
  suffix?: string
  width: string
  children: ReactNode
}) {
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={label}
        className="hover:bg-default flex max-w-[240px] items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm transition-colors"
      >
        {agent ? (
          <AgentLogo agent={agent} className="h-4 w-4 rounded-[5px]" />
        ) : model ? (
          <ModelIcon model={model} />
        ) : (
          <AppIcon icon={icon ?? "circle"} className="h-4 w-4 text-muted" />
        )}
        <span className="min-w-0 truncate font-medium">{value}</span>
        {suffix ? <span className="shrink-0 text-muted">{suffix}</span> : null}
        <AppIcon
          icon="chevron-down"
          className="h-3.5 w-3.5 shrink-0 text-muted"
        />
      </Popover.Trigger>
      <Popover.Content
        className={`${width} rounded-2xl border border-border p-1.5`}
      >
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          {children}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

export function PickerButton({
  icon,
  agent,
  model,
  selected,
  onPress,
  description,
  children,
}: {
  icon?: string
  agent?: SidebarAgentResponse
  model?: DisplayModel
  selected?: boolean
  onPress: () => void
  description?: string
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onPress}
      className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
    >
      {agent ? (
        <AgentLogo agent={agent} className="h-5 w-5 rounded-md" />
      ) : model ? (
        <ModelIcon model={model} />
      ) : icon ? (
        <AppIcon icon={icon} className="h-4 w-4 text-muted" />
      ) : null}
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{children}</span>
        {description ? (
          <span className="text-xs text-muted">{description}</span>
        ) : null}
      </span>
      {selected ? <AppIcon icon="check" className="h-4 w-4" /> : null}
    </button>
  )
}
