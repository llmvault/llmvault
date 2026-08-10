"use client"

import { useMemo, useState } from "react"
import { Slider } from "@heroui/react"
import { calculateDeposit } from "./pricing-model"

const depositMarkers = [0, 5, 10, 20, 50, 100, 200, 250, 500] as const
const minimumDepositIndex = 1

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
  const selectedStop = depositMarkers.indexOf(
    value as (typeof depositMarkers)[number]
  )

  return (
    <div className="py-6 pr-6 md:py-8 md:pr-8">
      <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
        Choose a balance
      </p>
      <h3 className="mt-3 text-2xl font-medium tracking-[-0.04em]">
        How much should your agents have to spend?
      </h3>
      <p className="mt-3 max-w-[52ch] text-sm leading-6 text-muted">
        Every dollar stays available for model usage and active sandbox time.
        Hivy’s 12% deposit fee appears separately.
      </p>

      <div className="mt-10">
        <Slider
          aria-label="Agent credit balance"
          minValue={0}
          maxValue={depositMarkers.length - 1}
          step={1}
          value={selectedStop}
          onChange={(nextValue) => {
            const nextIndex = Array.isArray(nextValue)
              ? nextValue[0]
              : nextValue
            const clampedIndex = Math.max(minimumDepositIndex, nextIndex)

            onChange(depositMarkers[clampedIndex])
          }}
        >
          {({ state }) => {
            const isDragging = state.isThumbDragging(0)

            return (
              <>
                <span
                  data-slot="label"
                  className="text-xs font-medium text-muted"
                >
                  Agent credit balance
                </span>
                <Slider.Output className="text-base font-medium">
                  {money(value)}
                </Slider.Output>
                <Slider.Track className="mt-2">
                  <Slider.Fill
                    className={
                      isDragging
                        ? "transition-none"
                        : "transition-[width] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none"
                    }
                  />
                  <Slider.Thumb
                    aria-valuetext={money(value)}
                    className={
                      isDragging
                        ? "transition-none"
                        : "transition-[left,background-color,transform,box-shadow] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none"
                    }
                  />
                </Slider.Track>
                <Slider.Marks className="relative col-span-2 mx-3 mt-2 h-14">
                  {depositMarkers.map((amount, index) => {
                    const markerPosition = {
                      left: `${(index / (depositMarkers.length - 1)) * 100}%`,
                      transform:
                        index === 0
                          ? undefined
                          : index === depositMarkers.length - 1
                            ? "translateX(-100%)"
                            : "translateX(-50%)",
                    }

                    return (
                      <span
                        key={amount}
                        className={`absolute top-0 flex flex-col gap-1.5 ${index === 0 ? "items-start" : index === depositMarkers.length - 1 ? "items-end" : "items-center"}`}
                        style={markerPosition}
                      >
                        {amount === 0 ? (
                          <span
                            data-slider-origin
                            aria-hidden="true"
                            className="text-[10px] whitespace-nowrap text-muted/70 sm:text-xs"
                          >
                            $0
                          </span>
                        ) : (
                          <button
                            type="button"
                            aria-label={`Select $${amount} deposit`}
                            onClick={() => onChange(amount)}
                            className={`cursor-pointer text-[10px] whitespace-nowrap text-muted transition-colors hover:text-foreground sm:text-xs ${value === amount ? "font-medium text-foreground" : ""}`}
                          >
                            ${amount}
                          </button>
                        )}
                      </span>
                    )
                  })}
                </Slider.Marks>
              </>
            )
          }}
        </Slider>
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
            Today’s receipt
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
          <span className="text-muted">Spendable balance</span>
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
          No subscription starts after payment.
        </p>
      </div>
    </div>
  )
}

export function PricingCalculator() {
  const [value, setValue] = useState(10)
  const estimate = useMemo(() => calculateDeposit(value), [value])

  return (
    <div className="grid overflow-hidden border-y border-border lg:grid-cols-[1fr_0.8fr]">
      <DepositForm value={value} onChange={setValue} />
      <DepositReceipt estimate={estimate} />
    </div>
  )
}
