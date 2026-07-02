"use client"

import { AppIcon } from "@/components/icon"
import { AnimatePresence, motion } from "motion/react"
import { cn } from "@/lib/utils"
import {
  formatSessionCostUSD,
  type SessionUsageSummary,
} from "@/app/w/(chat)/_lib/session-usage"

export function SessionSpendPill({ usage }: { usage?: SessionUsageSummary }) {
  const costUsd = usage?.costUsd ?? 0
  const credits = usage?.credits ?? 0
  const formattedCredits = credits.toLocaleString("en-NG")
  const formattedCost = formatSessionCostUSD(costUsd)

  return (
    <div
      aria-label={`Session spend: ${formattedCredits} credits, ${formattedCost}`}
      className="flex h-8 shrink-0 items-center gap-1.5 rounded-full border border-transparent px-2 text-xs text-muted"
    >
      <AppIcon icon="coins" className="h-3.5 w-3.5" />
      <AnimatedSpendNumber value={formattedCredits} />
      <AnimatedSpendNumber
        value={formattedCost}
        className="hidden sm:inline-grid"
      />
    </div>
  )
}

function AnimatedSpendNumber({
  value,
  className,
}: {
  value: string
  className?: string
}) {
  return (
    <span
      className={cn(
        "relative inline-grid h-4 overflow-hidden tabular-nums",
        className
      )}
    >
      <span className="invisible col-start-1 row-start-1 whitespace-nowrap">
        {value}
      </span>
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={value}
          initial={{ y: -10, opacity: 0 }}
          animate={{ y: 0, opacity: 1 }}
          exit={{ y: 10, opacity: 0 }}
          transition={{ duration: 0.16, ease: "easeOut" }}
          className="col-start-1 row-start-1 whitespace-nowrap"
        >
          {value}
        </motion.span>
      </AnimatePresence>
    </span>
  )
}
