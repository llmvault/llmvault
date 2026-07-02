"use client"

import { memo, useCallback, useRef, useState, type ComponentProps } from "react"
import { Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"

const SESSION_ACTIONS = [
  { id: "rename", label: "Rename chat", icon: "pencil" },
  { id: "share", label: "Share", icon: "share" },
  { id: "archive", label: "Archive", icon: "archive" },
] as const

export const SessionActionsMenu = memo(function SessionActionsMenu({
  ariaLabel = "Chat options",
  onOpenChange,
  onRename,
  onShare,
  onArchive,
  placement = "bottom start",
  triggerClassName,
}: {
  ariaLabel?: string
  onOpenChange?: (open: boolean) => void
  onRename?: () => void
  onShare?: () => void
  onArchive?: () => void
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

  function selectAction(actionID: (typeof SESSION_ACTIONS)[number]["id"]) {
    if (actionID === "rename") {
      onRename?.()
    } else if (actionID === "share") {
      onShare?.()
    } else if (actionID === "archive") {
      onArchive?.()
    }
    updateOpen(false)
  }

  return (
    <Popover isOpen={open} onOpenChange={updateOpen}>
      <Popover.Trigger
        aria-label={ariaLabel}
        data-open={open ? "true" : undefined}
        className={cn(
          "flex items-center rounded-lg p-1.5 text-muted transition-colors hover:bg-default",
          triggerClassName
        )}
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement={placement}
          offset={6}
          className="w-52 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {SESSION_ACTIONS.map((action) => {
              const disabled =
                (action.id === "rename" && !onRename) ||
                (action.id === "share" && !onShare) ||
                (action.id === "archive" && !onArchive)
              return (
                <button
                  key={action.id}
                  type="button"
                  disabled={disabled}
                  onClick={() => selectAction(action.id)}
                  className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default disabled:pointer-events-none disabled:opacity-45"
                >
                  <AppIcon icon={action.icon} className="h-4 w-4 shrink-0" />
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
