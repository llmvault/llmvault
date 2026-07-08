"use client"

import type { ReactNode } from "react"
import { IconChevronDown, Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
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
  variant = "inline",
  disabled = false,
  avatarOnly = false,
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
  /**
   * "inline" renders the compact composer-style pill (default). "field" renders
   * a full-width bordered control that matches the HeroUI Select used elsewhere
   * in forms.
   */
  variant?: "inline" | "field"
  disabled?: boolean
  avatarOnly?: boolean
  children: ReactNode
}) {
  const isField = variant === "field"
  const leading = agent ? (
    <AgentLogo agent={agent} className="h-4 w-4 rounded-[5px]" />
  ) : model ? (
    <ModelIcon model={model} />
  ) : (
    <AppIcon icon={icon ?? "circle"} className="h-4 w-4 text-muted" />
  )
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      {isField ? (
        // Match the HeroUI <Select> control exactly by reusing its own slot
        // classes (`select__trigger`/`select__indicator`) and chevron icon, so
        // the border, shadow, background, and indicator are identical.
        <Popover.Trigger
          aria-label={label}
          className="select__trigger select__trigger--full-width"
        >
          <span className="flex min-w-0 flex-1 items-center gap-2 text-start">
            {leading}
            <span className="min-w-0 truncate">{value}</span>
            {suffix ? (
              <span className="shrink-0 text-field-placeholder">{suffix}</span>
            ) : null}
          </span>
          <IconChevronDown
            className="select__indicator"
            data-slot="select-default-indicator"
            data-open={open ? "true" : undefined}
          />
        </Popover.Trigger>
      ) : (
        <Popover.Trigger
          aria-label={label}
          aria-disabled={disabled || undefined}
          className={cn(
            "flex max-w-[240px] items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm transition-colors",
            disabled
              ? "cursor-not-allowed opacity-70"
              : "hover:bg-default"
          )}
        >
          {leading}
          {avatarOnly ? null : (
            <>
              <span className="min-w-0 truncate font-medium">{value}</span>
              {suffix ? (
                <span className="shrink-0 text-muted">{suffix}</span>
              ) : null}
            </>
          )}
          {disabled || avatarOnly ? null : (
            <AppIcon
              icon="chevron-down"
              className="h-3.5 w-3.5 shrink-0 text-muted"
            />
          )}
        </Popover.Trigger>
      )}
      <Popover.Content
        placement="bottom start"
        className={`${isField ? "" : `${width} `}rounded-2xl border border-border p-1.5`}
        style={isField ? { width: "var(--trigger-width)" } : undefined}
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
      className="hover:bg-default flex shrink-0 items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
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
          <span className="truncate text-xs text-muted">{description}</span>
        ) : null}
      </span>
      {selected ? <AppIcon icon="check" className="h-4 w-4" /> : null}
    </button>
  )
}
