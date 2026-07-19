"use client"

import { useMemo, useState } from "react"
import { Button, Input } from "@heroui/react"
import { calculateDeposit } from "./pricing-model"

export type CalculatorMode = "plain" | "unlimited" | "receipt" | "manifesto"

const depositPresets = [10, 25, 50, 100] as const

function money(value: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value)
}

function integer(value: number) {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 }).format(
    value
  )
}

function DepositForm({
  value,
  onChange,
}: {
  value: number
  onChange: (value: number) => void
}) {
  return (
    <div className="p-6 md:p-8">
      <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
        Choose a deposit
      </p>
      <h3 className="mt-3 text-2xl font-medium tracking-[-0.04em]">
        How much credit value do you want?
      </h3>
      <p className="mt-3 max-w-[52ch] text-sm leading-6 text-muted">
        This balance pays for agent costs at the underlying model or provider
        price. Hivy adds no markup when your agents use it.
      </p>

      <label className="mt-8 block">
        <span className="text-xs font-medium text-muted">Credit value</span>
        <div className="mt-2 flex items-center gap-3">
          <span className="text-lg text-muted">$</span>
          <Input
            aria-label="Credit value"
            type="number"
            min="0"
            step="1"
            value={String(value)}
            onChange={(event) => onChange(Number(event.target.value))}
            className="max-w-[260px]"
          />
        </div>
      </label>

      <div className="mt-5 flex flex-wrap gap-2" aria-label="Deposit presets">
        {depositPresets.map((amount) => (
          <Button
            key={amount}
            size="sm"
            variant={value === amount ? "secondary" : "ghost"}
            onPress={() => onChange(amount)}
          >
            ${amount}
          </Button>
        ))}
      </div>
    </div>
  )
}

function DepositReceipt({
  estimate,
}: {
  estimate: ReturnType<typeof calculateDeposit>
}) {
  return (
    <div className="flex min-h-full flex-col bg-surface-secondary p-6 md:p-8">
      <div className="flex items-start justify-between gap-6">
        <div>
          <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
            Deposit receipt
          </p>
          <p className="mt-3 text-4xl font-medium tracking-[-0.055em]">
            {money(estimate.checkoutTotal)}
          </p>
        </div>
        <span className="rounded-sm border border-border bg-background px-3 py-2 text-xs font-medium">
          12% once
        </span>
      </div>

      <div className="mt-8 divide-y divide-border border-y border-border">
        <div className="flex items-center justify-between gap-6 py-4 text-sm">
          <span className="text-muted">Credits added</span>
          <span>{integer(estimate.creditsAdded)}</span>
        </div>
        <div className="flex items-center justify-between gap-6 py-4 text-sm">
          <span className="text-muted">Credit value</span>
          <span>{money(estimate.creditValue)}</span>
        </div>
        <div className="flex items-center justify-between gap-6 py-4 text-sm">
          <span className="text-muted">Hivy deposit fee (12%)</span>
          <span>{money(estimate.depositFee)}</span>
        </div>
      </div>

      <div className="mt-auto flex items-end justify-between gap-6 pt-8">
        <div>
          <p className="text-xs text-muted">Pay today</p>
          <p className="mt-1 text-2xl font-medium tracking-[-0.04em]">
            {money(estimate.checkoutTotal)}
          </p>
        </div>
        <p className="max-w-[24ch] text-right text-xs leading-5 text-muted">
          No subscription follows this payment.
        </p>
      </div>
    </div>
  )
}

export function PricingCalculator({ mode }: { mode: CalculatorMode }) {
  const [value, setValue] = useState(100)
  const estimate = useMemo(() => calculateDeposit(value), [value])

  const shellClass =
    mode === "manifesto"
      ? "grid overflow-hidden border-y border-border lg:grid-cols-[1fr_0.8fr]"
      : mode === "receipt"
        ? "grid overflow-hidden rounded-sm border border-border bg-surface lg:grid-cols-[0.8fr_1fr]"
        : mode === "unlimited"
          ? "grid overflow-hidden rounded-sm border border-border bg-background lg:grid-cols-[1fr_0.9fr]"
          : "grid overflow-hidden rounded-sm border border-border bg-surface lg:grid-cols-[1.05fr_0.95fr]"

  return (
    <div className={shellClass}>
      <DepositForm value={value} onChange={setValue} />
      <DepositReceipt estimate={estimate} />
    </div>
  )
}
