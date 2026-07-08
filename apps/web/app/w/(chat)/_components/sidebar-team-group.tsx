"use client"

import { useState, type ReactNode } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

export function SidebarTeamGroup({
  name,
  onAddChannel,
  defaultOpen = true,
  children,
}: {
  name: string
  onAddChannel: () => void
  defaultOpen?: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="flex flex-col">
      <div className="group flex items-center gap-1 pr-1">
        <button
          type="button"
          aria-expanded={open}
          aria-label={`${open ? "Collapse" : "Expand"} ${name}`}
          onClick={() => setOpen((value) => !value)}
          className="min-w-0 flex-1 truncate px-3 pt-2 pb-1 text-left text-xs text-muted uppercase select-none"
        >
          {name}
        </button>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label={`Create channel in ${name}`}
          onPress={onAddChannel}
          className="shrink-0 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
        >
          <AppIcon icon="plus" className="h-4 w-4 text-muted" />
        </Button>
      </div>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            key="channels"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={COLLAPSE_TRANSITION}
            className="overflow-hidden"
          >
            <div className="flex flex-col gap-0.5">{children}</div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}
