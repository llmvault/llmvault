"use client"

import { AppIcon } from "@/components/icon"
import { AnimatePresence, motion } from "motion/react"
import { cn } from "@/lib/utils"
import {
  formatSessionCredits,
  formatSessionCostUSD,
  type SessionUsageSummary,
} from "@/app/w/(chat)/_lib/session-usage"

export function SessionSpendPill({ usage }: { usage?: SessionUsageSummary }) {
  const modelCredits = formatSessionCredits(usage?.modelCredits ?? 0)
  const modelCost = formatSessionCostUSD(usage?.modelCostUsd ?? 0)
  const sandboxCredits = formatSessionCredits(usage?.sandboxCredits ?? 0)
  const sandboxCost = formatSessionCostUSD(usage?.sandboxCostUsd ?? 0)

  return (
    <div
      aria-label={`Session spend. Model: ${modelCredits} credits, ${modelCost}. Sandbox: ${sandboxCredits} credits, ${sandboxCost}.`}
      className="flex h-8 shrink-0 items-center gap-2 rounded-full border border-transparent px-2 text-xs text-muted"
    >
      <SpendSegment
        icon="coins"
        label="Model"
        credits={modelCredits}
        cost={modelCost}
      />
      <span aria-hidden="true" className="h-3.5 w-px bg-border" />
      <SpendSegment
        icon="cpu"
        label="Sandbox"
        credits={sandboxCredits}
        cost={sandboxCost}
      />
    </div>
  )
}

function SpendSegment({
  icon,
  label,
  credits,
  cost,
}: {
  icon: "coins" | "cpu"
  label: string
  credits: string
  cost: string
}) {
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      <AppIcon icon={icon} className="h-3.5 w-3.5 shrink-0" />
      <span className="hidden font-medium text-foreground lg:inline">
        {label}
      </span>
      <AnimatedSpendNumber value={`${credits} cr`} />
      <AnimatedSpendNumber value={cost} className="hidden sm:inline-grid" />
    </span>
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
