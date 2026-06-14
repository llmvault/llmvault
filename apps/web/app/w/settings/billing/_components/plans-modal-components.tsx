import { Button, Slider, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Plan = components["schemas"]["planDTO"]

export type CreditStep = {
  plan: Plan
  credits: number
  priceMinor: number
  label: string
}

export interface PlanAction {
  label: string
  variant: "primary" | "secondary" | "tertiary" | "outline"
  disabled: boolean
  kind: "current" | "upgrade" | "downgrade"
}

export function CreditSlider({
  steps,
  value,
  onChange,
}: {
  steps: CreditStep[]
  value: number
  onChange: (value: number) => void
}) {
  return (
    <div className="w-full">
      <div className="mb-4 flex items-center justify-between">
        <span className="text-xs font-medium text-muted">
          Business credits per month
        </span>
        <span className="text-lg font-medium text-foreground">
          {steps[value]?.credits.toLocaleString("en-NG") ?? "0"}
        </span>
      </div>
      <Slider
        aria-label="Business credits per month"
        value={value}
        onChange={(next) => onChange(Array.isArray(next) ? next[0] : next)}
        minValue={0}
        maxValue={Math.max(0, steps.length - 1)}
        step={1}
        className="w-full"
      >
        <Slider.Track>
          <Slider.Fill />
          <Slider.Thumb />
        </Slider.Track>
      </Slider>
      <div className="mt-3 flex justify-between">
        {steps.map((step, index) => (
          <button
            key={step.label}
            type="button"
            onClick={() => onChange(index)}
            className={cn(
              "text-xs font-medium transition-colors",
              index === value ? "text-foreground" : "text-muted hover:text-foreground"
            )}
          >
            {step.label}
          </button>
        ))}
      </div>
    </div>
  )
}

export function DetailRow({
  label,
  value,
  emphasis,
  last,
}: {
  label: string
  value: string
  emphasis?: boolean
  last?: boolean
}) {
  return (
    <div
      className={cn(
        "flex items-center justify-between gap-4 px-4 py-3",
        last ? "" : "border-b border-border"
      )}
    >
      <span className="text-sm text-muted">{label}</span>
      <span
        className={cn(
          "tabular-nums",
          emphasis
            ? "text-base font-semibold text-primary"
            : "text-sm font-medium text-foreground"
        )}
      >
        {value}
      </span>
    </div>
  )
}

export function PlanCard({
  accentLabel,
  badgeLabel,
  price,
  period,
  description,
  features,
  action,
  busy,
  disabled,
  onChoose,
  highlighted,
}: {
  accentLabel: string
  badgeLabel?: string
  price: string
  period?: string
  description: string
  features: string[]
  action: PlanAction
  busy: boolean
  disabled: boolean
  onChoose: () => void
  highlighted?: boolean
}) {
  return (
    <div
      className={cn(
        "relative flex flex-col rounded-2xl border bg-surface p-6 sm:p-8",
        highlighted ? "border-primary/30" : "border-border"
      )}
    >
      {badgeLabel ? (
        <div className="absolute -top-3 left-1/2 -translate-x-1/2">
          <div className="rounded-full bg-primary px-3 py-1 text-xs font-semibold text-primary-foreground">
            {badgeLabel}
          </div>
        </div>
      ) : null}

      <div className="mb-6">
        <div
          className={cn(
            "mb-2 inline-flex rounded-full px-3 py-1 text-xs font-medium",
            highlighted ? "bg-primary/10 text-primary" : "bg-default text-muted"
          )}
        >
          {accentLabel}
        </div>
        <div className="mt-4 flex items-baseline gap-1">
          <span className="text-4xl font-semibold tracking-tight">{price}</span>
          {period ? <span className="text-sm text-muted">{period}</span> : null}
        </div>
        <p className="mt-2 text-sm text-muted">{description}</p>
      </div>

      <Button
        variant={action.variant}
        size="lg"
        fullWidth
        isDisabled={action.disabled || disabled || busy}
        onPress={onChoose}
      >
        {busy ? <Spinner color="current" size="sm" /> : null}
        {action.label}
      </Button>

      {features.length > 0 ? (
        <>
          <div className="my-6 h-px bg-border" />
          <ul className="flex flex-col gap-3">
            {features.map((feature) => (
              <li key={feature} className="flex items-center gap-2.5 text-sm text-muted">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
                  <Icon icon="lucide:check" className="h-3 w-3" />
                </span>
                <span>{feature}</span>
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  )
}
