"use client"

import { useId, useState } from "react"
import { Icon } from "@iconify/react"
import { AnimatePresence, motion } from "motion/react"
import {
  PlanStatusIcon,
  PlanStepRow,
} from "@/app/w/(chat)/_components/session-plan-step"
import type { SessionPlan } from "@/app/w/(chat)/_lib/session-plan"

const popoverTransition = {
  duration: 0.18,
  ease: [0.16, 1, 0.3, 1] as const,
}

export function SessionPlanCard({
  plan,
  turnActive,
}: {
  plan: SessionPlan
  turnActive: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const panelID = useId()
  const activeStep = plan.steps[plan.activeIndex] ?? plan.steps.at(-1)
  const activeLabel = activeStep?.text ?? "Plan updated"
  const activeStatus = displayStepStatus(plan, plan.activeIndex, turnActive)

  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-full z-30 pb-2">
      <div className="relative mx-auto w-full max-w-3xl px-4">
        <AnimatePresence initial={false}>
          {expanded ? (
            <motion.div
              key={plan.key}
              id={panelID}
              role="dialog"
              aria-label="Agent plan"
              initial={{ opacity: 0, y: 10, scale: 0.985 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: 8, scale: 0.985 }}
              transition={popoverTransition}
              className="bg-surface text-surface-foreground pointer-events-auto absolute inset-x-4 bottom-[calc(100%+0.5rem)] max-h-[min(26rem,calc(100vh-12rem))] overflow-y-auto rounded-2xl border border-border px-4 py-3 shadow-lg"
            >
              <div className="flex flex-col gap-3">
                {plan.steps.map((step, index) => (
                  <motion.div
                    key={`${index}:${step.text}`}
                    initial={{ opacity: 0, y: 4 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{
                      ...popoverTransition,
                      delay: Math.min(index * 0.015, 0.09),
                    }}
                  >
                    <PlanStepRow
                      step={step}
                      status={displayStepStatus(plan, index, turnActive)}
                    />
                  </motion.div>
                ))}
              </div>
            </motion.div>
          ) : null}
        </AnimatePresence>

        <motion.button
          type="button"
          aria-controls={panelID}
          aria-expanded={expanded}
          aria-haspopup="dialog"
          onClick={() => setExpanded((current) => !current)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              setExpanded(false)
            }
          }}
          animate={{ y: expanded ? -1 : 0 }}
          transition={popoverTransition}
          className="bg-surface text-surface-foreground hover:bg-surface-secondary pointer-events-auto flex min-h-12 w-full items-center gap-3 rounded-2xl border border-border px-4 py-3 text-left text-sm shadow-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
        >
          <PlanStatusIcon status={activeStatus} />
          <span className="shrink-0 font-medium text-foreground">
            Step {plan.activeIndex + 1} / {plan.steps.length}
          </span>
          <span className="min-w-0 flex-1 truncate text-muted-foreground">
            {activeLabel}
          </span>
          <motion.span
            aria-hidden="true"
            animate={{ rotate: expanded ? 180 : 0 }}
            transition={popoverTransition}
            className="flex h-4 w-4 shrink-0 items-center justify-center text-muted-foreground"
          >
            <Icon icon="lucide:chevron-up" className="h-4 w-4" />
          </motion.span>
        </motion.button>
      </div>
    </div>
  )
}

function displayStepStatus(
  plan: SessionPlan,
  index: number,
  turnActive: boolean
) {
  const step = plan.steps[index]
  if (!step) return "pending"
  if (step.status === "completed") return "completed"
  if (
    turnActive &&
    (step.status === "in_progress" || index === plan.activeIndex)
  ) {
    return "in_progress"
  }
  return "pending"
}
