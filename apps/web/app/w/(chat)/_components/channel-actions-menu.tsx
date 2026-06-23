"use client"

import { memo, useCallback, useRef, useState, type ComponentProps } from "react"
import { Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { cn } from "@/lib/utils"

const CHANNEL_ACTIONS = [
  { id: "rename", label: "Rename channel", icon: "lucide:pencil" },
  { id: "details", label: "Channel details", icon: "lucide:info" },
] as const

export const ChannelActionsMenu = memo(function ChannelActionsMenu({
  ariaLabel = "Channel options",
  onDetails,
  onOpenChange,
  onRename,
  placement = "bottom start",
  triggerClassName,
}: {
  ariaLabel?: string
  onDetails?: () => void
  onOpenChange?: (open: boolean) => void
  onRename?: () => void
  placement?: ComponentProps<typeof Popover.Content>["placement"]
  triggerClassName?: string
}) {
  const [open, setOpen] = useState(false)
  const openRef = useRef(open)

  const updateOpen = useCallback(
    (nextOpen: boolean) => {
      if (openRef.current === nextOpen) return
      openRef.current = nextOpen
      setOpen(nextOpen)
      onOpenChange?.(nextOpen)
    },
    [onOpenChange]
  )

  function selectAction(actionID: (typeof CHANNEL_ACTIONS)[number]["id"]) {
    if (actionID === "rename") {
      onRename?.()
    } else if (actionID === "details") {
      onDetails?.()
    }
    updateOpen(false)
  }

  return (
    <Popover isOpen={open} onOpenChange={updateOpen}>
      <Popover.Trigger
        aria-label={ariaLabel}
        data-open={open ? "true" : undefined}
        className={cn(
          "hover:bg-surface data-[open=true]:bg-surface pointer-events-none mr-1 flex items-center rounded-md p-1 text-muted opacity-0 transition-[opacity,background-color,color] group-focus-within:pointer-events-auto group-focus-within:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100 focus-visible:pointer-events-auto focus-visible:opacity-100 data-[open=true]:pointer-events-auto data-[open=true]:opacity-100",
          triggerClassName
        )}
      >
        <Icon icon="lucide:ellipsis" className="h-3.5 w-3.5" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement={placement}
          offset={6}
          className="w-52 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {CHANNEL_ACTIONS.map((action) => {
              const disabled =
                (action.id === "rename" && !onRename) ||
                (action.id === "details" && !onDetails)
              return (
                <button
                  key={action.id}
                  type="button"
                  disabled={disabled}
                  onClick={() => selectAction(action.id)}
                  className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors disabled:pointer-events-none disabled:opacity-45"
                >
                  <Icon icon={action.icon} className="h-4 w-4 shrink-0" />
                  {action.label}
                </button>
              )
            })}
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
})
