"use client"

import { useCallback, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import {
  CreditSlider,
  PricingPlanCard,
  formatCreditLabel,
  formatNGN,
} from "@/components/pricing/credit-pricing"
import { usePaystackPop } from "@/hooks/use-paystack-pop"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import type { components } from "@/lib/api/schema"

type Plan = components["schemas"]["planDTO"]
type PreviewChangeResponse = components["schemas"]["previewChangeResponse"]
type SubscriptionResponse = components["schemas"]["subscriptionResponse"]

type BusinessCreditStep = {
  plan: Plan
  credits: number
  priceMinor: number
  label: string
}

function formatDate(value: string | undefined | null) {
  if (!value) return "—"
  return new Intl.DateTimeFormat("en-NG", {
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(new Date(value))
}

function formatNumber(value: number | undefined | null) {
  if (value == null) return "—"
  return value.toLocaleString("en-NG")
}

function formatMoney(minor: number | undefined | null, currency?: string) {
  if (minor == null) return "—"
  if ((currency ?? "NGN").toUpperCase() === "NGN") return formatNGN(minor)
  return new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: currency ?? "NGN",
    maximumFractionDigits: 0,
  }).format(minor / 100)
}

function planPrice(plan: Plan | undefined | null) {
  return plan?.price_cents ?? 0
}

function isPaidPlan(plan: Plan | undefined | null) {
  return planPrice(plan) > 0
}

function subscriptionIsPaidActive(
  subscription: SubscriptionResponse | undefined,
  currentPlan: Plan | undefined,
) {
  return subscription?.status === "active" && isPaidPlan(currentPlan)
}

function SummaryCard({
  label,
  value,
  detail,
}: {
  label: string
  value: string
  detail?: string
}) {
  return (
    <Card className="p-5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className="mt-2 font-heading text-2xl font-medium tracking-[-0.02em] text-foreground">
        {value}
      </p>
      {detail ? (
        <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
      ) : null}
    </Card>
  )
}

export default function CreditsPage() {
  const { activeOrg, plans, isLoading: authLoading } = useAuth()
  const queryClient = useQueryClient()
  const subscriptionQuery = $api.useQuery("get", "/v1/billing/subscription")
  const previewChange = $api.useMutation(
    "post",
    "/v1/billing/subscription/preview-change",
  )
  const initUpgrade = $api.useMutation(
    "post",
    "/v1/billing/subscription/init-upgrade",
  )
  const applyChange = $api.useMutation(
    "post",
    "/v1/billing/subscription/apply-change",
  )
  const [selectedPlanSlug, setSelectedPlanSlug] = useState<string | null>(null)
  const [pendingPlan, setPendingPlan] = useState<Plan | null>(null)
  const [pendingPreview, setPendingPreview] =
    useState<PreviewChangeResponse | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [upgradePaymentPending, setUpgradePaymentPending] = useState(false)
  const [paidUpgradeReference, setPaidUpgradeReference] = useState<
    string | null
  >(null)

  const refreshBilling = useCallback(() => {
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/billing/subscription"],
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/usage"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/dashboard"] })
  }, [queryClient])

  const paystack = usePaystackPop({
    onSubscribed: () => refreshBilling(),
  })

  const subscription = subscriptionQuery.data as
    | SubscriptionResponse
    | undefined
  const currentPlanSlug =
    subscription?.plan_slug ?? activeOrg?.plan?.slug ?? "free"
  const currentPlan =
    plans.find((plan) => plan.slug === currentPlanSlug) ??
    activeOrg?.plan ??
    plans.find((plan) => plan.slug === "free")

  const businessSteps = useMemo<BusinessCreditStep[]>(
    () =>
      plans
        .filter(
          (plan) =>
            plan.slug?.startsWith("business-") &&
            (plan.monthly_credits ?? 0) > 0,
        )
        .sort((a, b) => (a.monthly_credits ?? 0) - (b.monthly_credits ?? 0))
        .map((plan) => ({
          plan,
          credits: plan.monthly_credits ?? 0,
          priceMinor: plan.price_cents ?? 0,
          label: formatCreditLabel(plan.monthly_credits ?? 0),
        })),
    [plans],
  )

  const defaultSliderValue = useMemo(() => {
    if (businessSteps.length === 0) return 0
    const nextIndex = businessSteps.findIndex(
      (step) => step.priceMinor > planPrice(currentPlan),
    )
    if (nextIndex >= 0) return nextIndex
    return Math.max(
      0,
      businessSteps.findIndex((step) => step.plan.slug === currentPlanSlug),
    )
  }, [businessSteps, currentPlan, currentPlanSlug])
  const explicitSliderValue = selectedPlanSlug
    ? businessSteps.findIndex((step) => step.plan.slug === selectedPlanSlug)
    : -1
  const selectedSliderValue =
    explicitSliderValue >= 0 ? explicitSliderValue : defaultSliderValue
  const selectedTier = businessSteps[selectedSliderValue]
  const selectedPlan = selectedTier?.plan
  const selectedIsCurrent = selectedPlan?.slug === currentPlanSlug
  const selectedIsUpgrade =
    selectedPlan != null && planPrice(selectedPlan) > planPrice(currentPlan)
  const hasPaidSubscription = subscriptionIsPaidActive(
    subscription,
    currentPlan,
  )
  const pricingActionPending =
    paystack.isPending ||
    previewChange.isPending ||
    initUpgrade.isPending ||
    applyChange.isPending ||
    upgradePaymentPending
  const isLoading = authLoading || subscriptionQuery.isLoading

  function resetUpgradeDialog() {
    setPendingPlan(null)
    setPendingPreview(null)
    setConfirmOpen(false)
    setUpgradePaymentPending(false)
    setPaidUpgradeReference(null)
  }

  async function handleUpgrade(plan: Plan | undefined) {
    if (!plan?.slug) {
      toast.error("Plan is missing a slug")
      return
    }
    if (!selectedIsUpgrade || pricingActionPending) return

    if (!hasPaidSubscription) {
      paystack.subscribe(plan)
      return
    }

    try {
      const preview = await previewChange.mutateAsync({
        body: { plan_slug: plan.slug },
      })
      setPendingPlan(plan)
      setPendingPreview(preview)
      setConfirmOpen(true)
    } catch (error) {
      toast.error(
        extractErrorMessage(error, "Could not preview plan upgrade"),
      )
    }
  }

  async function applyQuote(reference: string | undefined) {
    if (!pendingPreview?.quote_id) {
      toast.error("Upgrade quote is missing")
      return
    }
    try {
      const response = await applyChange.mutateAsync({
        body: {
          quote_id: pendingPreview.quote_id,
          paystack_reference: reference,
        },
      })
      toast.success(
        `Upgraded to ${
          pendingPlan?.name ?? response.plan_slug ?? "the selected plan"
        }`,
      )
      resetUpgradeDialog()
      refreshBilling()
    } catch (error) {
      toast.error(extractErrorMessage(error, "Could not apply plan upgrade"))
      setUpgradePaymentPending(false)
    }
  }

  async function handleConfirmUpgrade() {
    if (!pendingPreview?.quote_id || pricingActionPending) return

    if (!pendingPreview.requires_payment_now) {
      await applyQuote(undefined)
      return
    }

    if (paidUpgradeReference) {
      await applyQuote(paidUpgradeReference)
      return
    }

    setUpgradePaymentPending(true)
    try {
      const upgrade = await initUpgrade.mutateAsync({
        body: { quote_id: pendingPreview.quote_id },
      })
      if (!upgrade.access_code) {
        toast.error("Provider did not return an access code")
        setUpgradePaymentPending(false)
        return
      }
      paystack.openPopup(
        upgrade.access_code,
        (reference) => {
          setPaidUpgradeReference(reference)
          void applyQuote(reference)
        },
        () => {
          setUpgradePaymentPending(false)
        },
      )
    } catch (error) {
      toast.error(extractErrorMessage(error, "Could not start upgrade payment"))
      setUpgradePaymentPending(false)
    }
  }

  const actionLabel = selectedIsCurrent
    ? "Current plan"
    : selectedIsUpgrade
      ? "Upgrade"
      : "Not available"
  const confirmActionLabel =
    pendingPreview?.requires_payment_now && !paidUpgradeReference
      ? "Pay and upgrade"
      : "Confirm upgrade"

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-8">
      <div>
        <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground">
          Credits
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">
          Manage your workspace plan, credit balance, and monthly AI usage
          checkpoint.
        </p>
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-6">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            {Array.from({ length: 3 }).map((_, index) => (
              <Skeleton key={index} className="h-28 rounded-md" />
            ))}
          </div>
          <Skeleton className="h-96 rounded-md" />
        </div>
      ) : (
        <>
          <section className="flex flex-col gap-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="font-heading text-xl font-medium text-foreground">
                  Current plan
                </h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  {activeOrg?.name ?? "Workspace"} is currently on{" "}
                  {currentPlan?.name ?? currentPlanSlug}.
                </p>
              </div>
              <Badge variant="outline" className="capitalize">
                {subscription?.status ?? "free"}
              </Badge>
            </div>

            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <SummaryCard
                label="Plan"
                value={currentPlan?.name ?? currentPlanSlug}
                detail={`${formatNumber(
                  currentPlan?.monthly_credits ?? currentPlan?.welcome_credits,
                )} included credits`}
              />
              <SummaryCard
                label="Credit balance"
                value={formatNumber(
                  subscription?.credits_balance ?? activeOrg?.credits,
                )}
                detail="Available for AI usage"
              />
              <SummaryCard
                label="Renews"
                value={formatDate(subscription?.current_period_end)}
                detail={
                  subscription?.pending_plan_slug
                    ? `Pending ${subscription.pending_plan_slug} on ${formatDate(
                        subscription.pending_change_at,
                      )}`
                    : subscription?.payment_channel
                      ? `Paid by ${subscription.payment_channel}`
                      : "No paid renewal scheduled"
                }
              />
            </div>
          </section>

          <section className="flex flex-col gap-6">
            <div>
              <h2 className="font-heading text-xl font-medium text-foreground">
                Upgrade plan
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Pick a higher Business checkpoint. Downgrades and lateral plan
                changes are handled separately.
              </p>
            </div>

            {selectedTier ? (
              <>
                <CreditSlider
                  steps={businessSteps}
                  value={selectedSliderValue}
                  onChange={(value) => {
                    setSelectedPlanSlug(businessSteps[value]?.plan.slug ?? null)
                  }}
                />
                <div className="mx-auto w-full max-w-xl">
                  <PricingPlanCard
                    accentLabel={`${selectedTier.label} checkpoint`}
                    badgeLabel="Business"
                    price={formatNGN(selectedTier.priceMinor)}
                    description={`Includes ${selectedTier.credits.toLocaleString(
                      "en-NG",
                    )} monthly credits.`}
                    features={selectedPlan?.features ?? []}
                    actionLabel={actionLabel}
                    onAction={() => void handleUpgrade(selectedPlan)}
                    disabled={!selectedIsUpgrade || pricingActionPending}
                    loading={pricingActionPending}
                    highlighted
                  />
                </div>
              </>
            ) : (
              <Card className="p-8 text-center text-sm text-muted-foreground">
                Pricing plans are temporarily unavailable.
              </Card>
            )}
          </section>
        </>
      )}

      <Dialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!open) resetUpgradeDialog()
          else setConfirmOpen(open)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirm upgrade</DialogTitle>
            <DialogDescription>
              Review the prorated charge before opening Paystack checkout.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-3 rounded-md border border-border bg-secondary p-4 text-sm">
            <div className="flex items-center justify-between gap-4">
              <span className="text-muted-foreground">Plan</span>
              <span className="font-medium text-foreground">
                {pendingPlan?.name ?? pendingPreview?.to_plan_slug ?? "—"}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-muted-foreground">Pay today</span>
              <span className="font-medium text-foreground">
                {formatMoney(
                  pendingPreview?.amount_minor,
                  pendingPreview?.currency,
                )}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-muted-foreground">Credits granted</span>
              <span className="font-medium text-foreground">
                {formatNumber(pendingPreview?.credit_grant_minor)}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-muted-foreground">Effective</span>
              <span className="font-medium text-foreground">
                {formatDate(pendingPreview?.effective_at)}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-muted-foreground">Quote expires</span>
              <span className="font-medium text-foreground">
                {formatDate(pendingPreview?.expires_at)}
              </span>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={resetUpgradeDialog}
              disabled={pricingActionPending}
            >
              Cancel
            </Button>
            <Button
              onClick={() => void handleConfirmUpgrade()}
              loading={pricingActionPending}
            >
              {confirmActionLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
