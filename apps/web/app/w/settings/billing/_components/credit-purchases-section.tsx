"use client"

import { Skeleton } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { useIsOwner } from "@/lib/auth/use-role"
import { useCreditPurchase } from "@/hooks/use-credit-purchase"
import type { components } from "@/lib/api/schema"
import type { PurchaseCurrency } from "./purchase-currency"

type Currency = PurchaseCurrency
type Purchase = components["schemas"]["creditPurchaseResponse"]

function formatMoney(minor: number, currency: Currency): string {
  return new Intl.NumberFormat(currency === "NGN" ? "en-NG" : "en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: currency === "NGN" ? 0 : 2,
  }).format(minor / 100)
}

export function CreditPurchasesSection() {
  const isOwner = useIsOwner()
  const purchasesQuery = $api.useQuery("get", "/v1/billing/purchases", {
    params: { query: { limit: 10 } },
  })
  const { verify, isPending } = useCreditPurchase()

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">Recent purchases</h2>
        <p className="text-sm text-muted">
          Completed and pending Paystack credit purchases.
        </p>
      </div>
      {purchasesQuery.isLoading ? (
        <Skeleton className="h-20 rounded-xl" />
      ) : (purchasesQuery.data?.purchases?.length ?? 0) === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-4 py-5 text-center text-sm text-muted">
          No credit purchases yet.
        </div>
      ) : (
        <div className="divide-y divide-border overflow-hidden rounded-xl border border-border">
          {purchasesQuery.data?.purchases?.map((item: Purchase) => {
            const currency = (item.currency ?? "USD") as Currency
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
                    {formatMoney(item.total_minor ?? 0, currency)}
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
    </section>
  )
}
