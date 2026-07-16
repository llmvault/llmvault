"use client"

import { useMemo, useState } from "react"
import { Button, Input, Skeleton, Spinner, toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"
import { useIsOwner } from "@/lib/auth/use-role"
import { useCreditPurchase } from "@/hooks/use-credit-purchase"

type Currency = "USD" | "NGN"

const CURRENCY_LABELS: Record<Currency, string> = {
  USD: "US dollars",
  NGN: "Nigerian naira",
}

function amountToMinor(value: string): number | null {
  if (!/^\d+(\.\d{0,2})?$/.test(value.trim())) return null
  const amount = Number(value)
  if (!Number.isFinite(amount) || amount <= 0) return null
  return Math.round(amount * 100)
}

function formatMoney(minor: number, currency: Currency): string {
  return new Intl.NumberFormat(currency === "NGN" ? "en-NG" : "en-US", {
    style: "currency",
    currency,
  }).format(minor / 100)
}

export function CreditPurchasesSection() {
  const isOwner = useIsOwner()
  const queryClient = useQueryClient()
  const accountQuery = $api.useQuery("get", "/v1/billing/account")
  const purchasesQuery = $api.useQuery("get", "/v1/billing/purchases", {
    params: { query: { limit: 10 } },
  })
  const selectCurrency = $api.useMutation("put", "/v1/billing/account/currency")
  const { purchase, verify, isPending } = useCreditPurchase()
  const [currencyChoice, setCurrencyChoice] = useState<Currency>("USD")
  const [amount, setAmount] = useState("")

  const currency = accountQuery.data?.currency as Currency | undefined
  const subtotalMinor = amountToMinor(amount)
  const feeBasisPoints = accountQuery.data?.fee_basis_points ?? 1000
  const feeMinor = subtotalMinor
    ? Math.ceil((subtotalMinor * feeBasisPoints) / 10_000)
    : 0
  const totalMinor = (subtotalMinor ?? 0) + feeMinor
  const credits = useMemo(() => {
    if (!subtotalMinor || !currency) return 0
    if (currency === "USD") return subtotalMinor * 10
    const rate = accountQuery.data?.ngn_minor_per_usd ?? 0
    return rate > 0 ? Math.floor((subtotalMinor * 1000) / rate) : 0
  }, [accountQuery.data?.ngn_minor_per_usd, currency, subtotalMinor])

  const lockCurrency = () => {
    selectCurrency.mutate(
      { body: { currency: currencyChoice } },
      {
        onSuccess: () => {
          toast.success(`${CURRENCY_LABELS[currencyChoice]} selected`)
          void Promise.all([
            queryClient.invalidateQueries({
              queryKey: queryKeys.billingAccount(),
            }),
            queryClient.invalidateQueries({ queryKey: queryKeys.authMe() }),
          ])
          setAmount(currencyChoice === "NGN" ? "10000" : "10")
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not select billing currency")
          ),
      }
    )
  }

  return (
    <section id="credit-purchases" className="flex scroll-mt-8 flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">Buy credits</h2>
        <p className="text-sm text-muted">
          One-time purchases through Paystack. A {feeBasisPoints / 100}% deposit
          fee is added at checkout.
        </p>
      </div>

      {accountQuery.isLoading ? (
        <Skeleton className="h-48 rounded-2xl" />
      ) : !currency ? (
        <div className="flex flex-col gap-5 rounded-2xl border border-border bg-surface p-5">
          <div className="flex flex-col gap-1">
            <p className="text-sm font-medium">Choose your billing currency</p>
            <p className="text-sm text-muted">
              This choice is permanent for this workspace because deposits and
              credit reconciliation remain in one currency.
            </p>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {(["USD", "NGN"] as const).map((option) => (
              <button
                key={option}
                type="button"
                disabled={!isOwner}
                onClick={() => setCurrencyChoice(option)}
                className={`rounded-xl border px-4 py-3 text-left transition-colors ${
                  currencyChoice === option
                    ? "border-foreground bg-default"
                    : "border-border hover:bg-default"
                } disabled:cursor-not-allowed disabled:opacity-50`}
              >
                <span className="block text-sm font-medium">{option}</span>
                <span className="text-xs text-muted">
                  {CURRENCY_LABELS[option]}
                </span>
              </button>
            ))}
          </div>
          <Button
            variant="primary"
            isDisabled={!isOwner || selectCurrency.isPending}
            onPress={lockCurrency}
          >
            {selectCurrency.isPending ? <Spinner size="sm" /> : null}
            Lock in {currencyChoice}
          </Button>
          {!isOwner ? (
            <p className="text-xs text-muted">
              Only the workspace owner can select a currency.
            </p>
          ) : null}
        </div>
      ) : (
        <div className="flex flex-col gap-5 rounded-2xl border border-border bg-surface p-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium">Purchase amount</p>
              <p className="text-xs text-muted">
                Billing currency: {currency} · locked
              </p>
            </div>
            <span className="rounded-full bg-default px-2.5 py-1 text-xs font-medium">
              Paystack
            </span>
          </div>
          <Input
            aria-label={`Amount in ${currency}`}
            inputMode="decimal"
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            placeholder={currency === "USD" ? "10.00 USD" : "10000.00 NGN"}
          />
          <div className="flex flex-col gap-2 rounded-xl bg-default p-3 text-sm">
            <div className="flex justify-between gap-4 text-muted">
              <span>Credits</span>
              <span>{credits.toLocaleString()}</span>
            </div>
            <div className="flex justify-between gap-4 text-muted">
              <span>Deposit fee ({feeBasisPoints / 100}%)</span>
              <span>{formatMoney(feeMinor, currency)}</span>
            </div>
            <div className="flex justify-between gap-4 border-t border-border pt-2 font-medium">
              <span>Total</span>
              <span>{formatMoney(totalMinor, currency)}</span>
            </div>
          </div>
          <Button
            variant="primary"
            isDisabled={!isOwner || !subtotalMinor || credits <= 0 || isPending}
            onPress={() => subtotalMinor && purchase(subtotalMinor)}
          >
            {isPending ? <Spinner size="sm" /> : null}
            Continue to Paystack
          </Button>
          {!isOwner ? (
            <p className="text-xs text-muted">
              Only the workspace owner can buy credits.
            </p>
          ) : null}
        </div>
      )}

      <div className="mt-3 flex flex-col gap-2">
        <p className="text-sm font-medium">Recent purchases</p>
        {purchasesQuery.isLoading ? (
          <Skeleton className="h-20 rounded-xl" />
        ) : (purchasesQuery.data?.purchases?.length ?? 0) === 0 ? (
          <div className="rounded-xl border border-dashed border-border px-4 py-5 text-center text-sm text-muted">
            No credit purchases yet.
          </div>
        ) : (
          <div className="divide-y divide-border overflow-hidden rounded-xl border border-border">
            {purchasesQuery.data?.purchases?.map((item) => {
              const itemCurrency = (item.currency ?? "USD") as Currency
              const itemID = item.id
              return (
                <div
                  key={item.id}
                  className="flex items-center justify-between gap-4 px-4 py-3 text-sm"
                >
                  <div className="min-w-0">
                    <p className="font-medium">
                      {(item.credits ?? 0).toLocaleString()} credits
                    </p>
                    <p className="text-xs text-muted">
                      {item.created_at
                        ? new Date(item.created_at).toLocaleDateString()
                        : ""}
                    </p>
                  </div>
                  <div className="text-right">
                    <p className="font-medium">
                      {formatMoney(item.total_minor ?? 0, itemCurrency)}
                    </p>
                    {item.status === "pending" && itemID && isOwner ? (
                      <button
                        type="button"
                        disabled={isPending}
                        onClick={() => verify(itemID)}
                        className="text-xs font-medium text-foreground underline decoration-border underline-offset-2 disabled:opacity-50"
                      >
                        Verify payment
                      </button>
                    ) : (
                      <p className="text-xs text-muted capitalize">
                        {item.status}
                      </p>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </section>
  )
}
