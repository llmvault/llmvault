import { Button, Skeleton, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { PurchaseCurrency } from "./purchase-currency"

export function formatPurchaseMoney(
  minor: number,
  currency: PurchaseCurrency
): string {
  return new Intl.NumberFormat(currency === "NGN" ? "en-NG" : "en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: currency === "NGN" ? 0 : 2,
  }).format(minor / 100)
}

export function PurchaseSummary({
  balance,
  currency,
  credits,
  feeBasisPoints,
  feeMinor,
  subtotalMinor,
  totalMinor,
  isPending,
}: {
  balance: number
  currency: PurchaseCurrency
  credits: number | undefined
  feeBasisPoints: number
  feeMinor: number | undefined
  subtotalMinor: number | undefined
  totalMinor: number | undefined
  isPending: boolean
}) {
  return (
    <aside className="flex flex-col border-t border-border bg-surface-secondary px-5 py-5 sm:px-7 sm:py-6 lg:border-t-0 lg:border-l">
      <div className="lg:sticky lg:top-8">
        <div className="flex items-baseline justify-between gap-4">
          <p className="text-xs font-semibold tracking-wide text-muted uppercase">
            Order summary
          </p>
          <p className="text-sm font-semibold tabular-nums">
            {(credits ?? 0).toLocaleString()}{" "}
            <span className="font-normal text-muted">credits</span>
          </p>
        </div>

        <dl className="mt-5 space-y-2.5 border-t border-border pt-4 text-sm">
          <div className="flex justify-between gap-4 text-muted">
            <dt>Credit deposit</dt>
            <dd className="tabular-nums">
              {subtotalMinor === undefined
                ? "Select an amount"
                : formatPurchaseMoney(subtotalMinor, currency)}
            </dd>
          </div>
          <div className="flex justify-between gap-4 text-muted">
            <dt>Deposit fee ({feeBasisPoints / 100}%)</dt>
            <dd className="tabular-nums">
              {feeMinor === undefined
                ? "Not selected"
                : formatPurchaseMoney(feeMinor, currency)}
            </dd>
          </div>
          <div className="flex justify-between gap-4 border-t border-border pt-3 font-semibold">
            <dt>Total charged</dt>
            <dd className="tabular-nums">
              {totalMinor === undefined
                ? "Not selected"
                : formatPurchaseMoney(totalMinor, currency)}
            </dd>
          </div>
        </dl>

        {credits !== undefined ? (
          <p className="mt-3 text-xs leading-5 text-muted">
            New workspace balance:{" "}
            <span className="font-medium text-foreground">
              {(balance + credits).toLocaleString()} credits
            </span>
          </p>
        ) : null}

        <Button
          type="submit"
          variant="primary"
          className="mt-5 w-full"
          isDisabled={totalMinor === undefined || isPending}
        >
          {isPending ? <Spinner size="sm" /> : null}
          {isPending
            ? "Confirming payment"
            : totalMinor === undefined
              ? "Choose an amount"
              : `Pay ${formatPurchaseMoney(totalMinor, currency)}`}
        </Button>

        <div className="mt-4 flex gap-3 border-t border-border pt-4">
          <AppIcon
            icon="shield-check"
            strokeWidth={2}
            className="size-5 shrink-0 text-success"
          />
          <p className="text-xs leading-5 text-muted">
            Your card details are encrypted and handled by Paystack, a Stripe
            company certified to PCI DSS Level 1 v4.0, ISO 27001:2022, and ISO
            27701:2019.
          </p>
        </div>
      </div>
    </aside>
  )
}

export function PurchasePageSkeleton() {
  return (
    <div className="mt-8 grid overflow-hidden rounded-2xl border border-border bg-surface lg:grid-cols-[minmax(0,1fr)_19rem]">
      <div className="col-span-full flex h-12 items-center justify-between border-b border-border px-5 sm:px-9">
        <Skeleton className="h-4 w-56 rounded" />
        <Skeleton className="h-8 w-24 rounded-lg" />
      </div>
      <div className="space-y-6 px-5 py-5 sm:px-9 sm:py-6">
        <div>
          <Skeleton className="h-4 w-28 rounded" />
          <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
            <Skeleton className="h-16 rounded-xl" />
            <Skeleton className="h-16 rounded-xl" />
            <Skeleton className="h-16 rounded-xl" />
            <Skeleton className="h-16 rounded-xl" />
            <Skeleton className="h-16 rounded-xl" />
            <Skeleton className="h-16 rounded-xl" />
          </div>
        </div>
        <div className="border-t border-border pt-5">
          <Skeleton className="h-4 w-36 rounded" />
          <Skeleton className="mt-3 h-14 rounded-xl" />
        </div>
      </div>
      <div className="border-t border-border bg-surface-secondary px-7 py-6 lg:border-t-0 lg:border-l">
        <div className="flex justify-between">
          <Skeleton className="h-3 w-24 rounded" />
          <Skeleton className="h-4 w-24 rounded" />
        </div>
        <Skeleton className="mt-5 h-28 rounded-xl" />
        <Skeleton className="mt-5 h-10 rounded-lg" />
      </div>
    </div>
  )
}
