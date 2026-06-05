"use client"

import type { ReactNode } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Tick02Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Slider } from "@/components/ui/slider"
import { cn } from "@/lib/utils"

export type CreditStep = {
  credits: number
  priceMinor: number
  label: string
}

export function formatNGN(minor: number) {
  return new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    maximumFractionDigits: 0,
  }).format(minor / 100)
}

export function formatCreditLabel(credits: number) {
  if (credits >= 1000) {
    const thousands = credits / 1000
    return `${Number.isInteger(thousands) ? thousands : thousands.toFixed(1)}K`
  }
  return credits.toLocaleString("en-NG")
}

export function FeatureItem({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-center gap-2.5">
      <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
        <HugeiconsIcon icon={Tick02Icon} className="size-3" />
      </div>
      <span className="text-sm text-muted-foreground">{children}</span>
    </div>
  )
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
    <div className="mx-auto w-full max-w-xl">
      <div className="mb-4 flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">
          Business credits per month
        </span>
        <span className="font-heading text-lg font-medium text-foreground">
          {steps[value]?.credits.toLocaleString() ?? "0"}
        </span>
      </div>
      <Slider
        min={0}
        max={Math.max(0, steps.length - 1)}
        step={1}
        value={[value]}
        onValueChange={(v) => {
          const arr = Array.isArray(v) ? v : [v]
          if (typeof arr[0] === "number") onChange(arr[0])
        }}
      />
      <div className="mt-3 flex justify-between">
        {steps.map((step, i) => (
          <button
            key={step.label}
            type="button"
            onClick={() => onChange(i)}
            className={cn(
              "text-xs font-medium transition-colors",
              i === value
                ? "text-foreground"
                : "text-muted-foreground hover:text-foreground"
            )}
          >
            {step.label}
          </button>
        ))}
      </div>
    </div>
  )
}

export function PricingPlanCard({
  accentLabel,
  badgeLabel,
  price,
  period = "/month",
  description,
  features,
  actionLabel,
  actionVariant = "default",
  onAction,
  disabled,
  loading,
  highlighted,
  className,
}: {
  accentLabel: string
  badgeLabel?: string
  price: string
  period?: string
  description: string
  features: string[]
  actionLabel: string
  actionVariant?: "default" | "secondary" | "outline"
  onAction?: () => void
  disabled?: boolean
  loading?: boolean
  highlighted?: boolean
  className?: string
}) {
  return (
    <div
      className={cn(
        "relative flex flex-col rounded-lg border bg-secondary p-6 sm:p-8",
        highlighted ? "border-primary/20" : "border-border",
        className
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
            highlighted
              ? "bg-primary/10 text-primary"
              : "bg-muted text-muted-foreground"
          )}
        >
          {accentLabel}
        </div>
        <div className="mt-4 flex items-baseline gap-1">
          <span className="font-heading text-4xl font-medium text-foreground">
            {price}
          </span>
          {period ? (
            <span className="text-sm text-muted-foreground">{period}</span>
          ) : null}
        </div>
        <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      </div>

      <Button
        variant={actionVariant}
        size="lg"
        className="w-full"
        onClick={onAction}
        disabled={disabled}
        loading={loading}
      >
        {actionLabel}
      </Button>

      {features.length > 0 ? (
        <>
          <div className="my-6 h-px bg-border" />
          <div className="flex flex-col gap-3">
            {features.map((feature) => (
              <FeatureItem key={feature}>{feature}</FeatureItem>
            ))}
          </div>
        </>
      ) : null}
    </div>
  )
}
