"use client"

import { useRouter } from "next/navigation"
import { Button, Spinner } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import type { components } from "@/lib/api/schema"
import { formatCreditsWithUsage } from "./credit-value"

type Plan = components["schemas"]["planDTO"]
type SubscriptionResponse = components["schemas"]["subscriptionResponse"]

function formatMoney(minor: number | undefined | null, currency?: string) {
  if (minor == null) return "—"
  const cur = (currency ?? "USD").toUpperCase()
  return new Intl.NumberFormat(cur === "USD" ? "en-US" : "en-NG", {
    style: "currency",
    currency: cur,
    maximumFractionDigits: 0,
  }).format(minor / 100)
}

export function YourPlanSection() {
  const router = useRouter()
  const { plans, activeOrg, isLoading: authLoading } = useAuth()
  const subscriptionQuery = $api.useQuery("get", "/v1/billing/subscription")

  const subscription = subscriptionQuery.data as
    | SubscriptionResponse
    | undefined
  const currentPlanSlug =
    subscription?.plan_slug ?? activeOrg?.plan?.slug ?? "free"
  const currentPlan: Plan | undefined =
    plans.find((plan) => plan.slug === currentPlanSlug) ??
    activeOrg?.plan ??
    plans.find((plan) => plan.slug === "free")

  const isLoading = authLoading || subscriptionQuery.isLoading
  const isPaid = (currentPlan?.price_cents ?? 0) > 0
  const includedCredits =
    (currentPlan?.monthly_credits ?? 0) > 0
      ? (currentPlan?.monthly_credits ?? 0)
      : (currentPlan?.welcome_credits ?? 0)
  const includedCreditsLabel = isPaid
    ? "monthly credits included"
    : "welcome credits included"
  const priceLabel = formatMoney(
    currentPlan?.price_cents,
    currentPlan?.currency
  )

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-medium">Your plan</h2>
      <div className="bg-surface flex items-center gap-4 rounded-2xl border border-border px-4 py-4">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          {isLoading ? (
            <>
              <span className="bg-default h-4 w-32 animate-pulse rounded" />
              <span className="bg-default mt-1 h-3.5 w-48 animate-pulse rounded" />
            </>
          ) : (
            <>
              <span className="text-sm font-medium">
                {currentPlan?.name ?? currentPlanSlug}
                {isPaid ? (
                  <span className="text-muted"> ({priceLabel}/mo)</span>
                ) : null}
              </span>
              <span className="text-sm text-muted">
                {formatCreditsWithUsage(includedCredits)} ·{" "}
                {includedCreditsLabel}
              </span>
            </>
          )}
        </div>
        <Button
          variant="tertiary"
          size="sm"
          isDisabled={isLoading}
          onPress={() => router.push("/w/billing/plans")}
        >
          {isLoading ? <Spinner color="current" size="sm" /> : null}
          View plans
        </Button>
      </div>
    </section>
  )
}
