import { Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import type {
  SessionPlanStep,
  SessionPlanStepStatus,
} from "@/app/w/(chat)/_lib/session-plan"

export function PlanStepRow({
  step,
  status,
}: {
  step: SessionPlanStep
  status: SessionPlanStepStatus
}) {
  const muted = status === "completed"

  return (
    <div className="flex min-w-0 items-start gap-3 text-sm">
      <PlanStatusIcon status={status} />
      <span
        className={cn(
          "min-w-0 flex-1 wrap-break-word",
          muted ? "text-muted-foreground" : "text-foreground"
        )}
      >
        {step.text}
      </span>
    </div>
  )
}

export function PlanStatusIcon({ status }: { status: SessionPlanStepStatus }) {
  if (status === "completed") {
    return (
      <AppIcon
        icon="circle-check"
        className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground"
      />
    )
  }

  if (status === "in_progress") {
    return (
      <span className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center">
        <Spinner color="current" size="sm" className="text-primary" />
      </span>
    )
  }

  return (
    <span
      className="mt-0.5 h-4 w-4 shrink-0 rounded-full border-2 border-muted-foreground"
      aria-hidden="true"
    />
  )
}
