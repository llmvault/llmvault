"use client"

import { useState, type ReactNode } from "react"
import { AnimatePresence, motion } from "motion/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

export function SidebarTeamGroup({
  name,
  defaultOpen = true,
  children,
}: {
  name: string
  defaultOpen?: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="flex flex-col">
      <button
        type="button"
        aria-expanded={open}
        aria-label={`${open ? "Collapse" : "Expand"} ${name}`}
        onClick={() => setOpen((value) => !value)}
        className="group flex items-center gap-1 px-3 pt-2 pb-1 text-left select-none"
      >
        <AppIcon
          icon="chevron-right"
          className={cn(
            "h-3 w-3 shrink-0 text-muted transition-transform",
            open && "rotate-90"
          )}
        />
        <span className="min-w-0 flex-1 truncate text-xs font-medium tracking-wide text-muted uppercase">
          {name}
        </span>
      </button>

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
