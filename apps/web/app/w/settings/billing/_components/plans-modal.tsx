import { useMemo, useState } from "react"
import { Button, Modal, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { usePaystackPop } from "@/hooks/use-paystack-pop"
import type { components } from "@/lib/api/schema"
import {
  CreditSlider,
  DetailRow,
  PlanCard,
  type CreditStep,
  type PlanAction,
} from "./plans-modal-components"

type Plan = components["schemas"]["planDTO"]
type SubscriptionResponse = components["schemas"]["subscriptionResponse"]
type PreviewChangeResponse = components["schemas"]["previewChangeResponse"]

function planPrice(plan: Plan | undefined | null) {
  return plan?.price_cents ?? 0
}

function formatMoney(minor: number | undefined | null, currency?: string) {
  if (minor == null) return "—"
  const cur = (currency ?? "NGN").toUpperCase()
  return new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: cur,
    maximumFractionDigits: 0,
  }).format(minor / 100)
}

function formatNumber(value: number | undefined | null) {
  if (value == null) return "—"
  return value.toLocaleString("en-NG")
}

function formatDate(value: string | undefined | null) {
  if (!value) return "—"
  return new Intl.DateTimeFormat("en-NG", {
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(new Date(value))
}

function formatCreditLabel(credits: number) {
  if (credits >= 1000) {
    const thousands = credits / 1000
    return `${Number.isInteger(thousands) ? thousands : thousands.toFixed(1)}K`
  }
  return credits.toLocaleString("en-NG")
}

function planActionFor(
  plan: Plan,
  currentSlug: string,
  currentPrice: number
): PlanAction {
  if (plan.slug === currentSlug) {
    return { label: "Current plan", variant: "tertiary", disabled: true, kind: "current" }
  }
  if (planPrice(plan) > currentPrice) {
    return { label: "Upgrade", variant: "primary", disabled: false, kind: "upgrade" }
  }
  return { label: "Downgrade", variant: "outline", disabled: false, kind: "downgrade" }
}

export function PlansModal({
  open,
  onOpenChange,
  plans,
  currentPlanSlug,
  subscription,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  plans: Plan[]
  currentPlanSlug: string
  subscription: SubscriptionResponse | undefined
}) {
  const queryClient = useQueryClient()
  const previewChange = $api.useMutation("post", "/v1/billing/subscription/preview-change")
  const initUpgrade = $api.useMutation("post", "/v1/billing/subscription/init-upgrade")
  const applyChange = $api.useMutation("post", "/v1/billing/subscription/apply-change")

  const [sliderValue, setSliderValue] = useState<number | null>(null)
  const [pendingPlan, setPendingPlan] = useState<Plan | null>(null)
  const [pendingPreview, setPendingPreview] = useState<PreviewChangeResponse | null>(null)
  const [paidReference, setPaidReference] = useState<string | null>(null)
  const [busySlug, setBusySlug] = useState<string | null>(null)

  const refreshBilling = () => {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/billing/subscription"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/usage"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/dashboard"] })
  }

  const paystack = usePaystackPop({
    onSubscribed: () => {
      refreshBilling()
      onOpenChange(false)
    },
  })

  const freePlan = plans.find((plan) => plan.slug === "free")
  const businessSteps = useMemo<CreditStep[]>(
    () =>
      plans
        .filter(
          (plan) =>
            plan.slug?.startsWith("business-") && (plan.monthly_credits ?? 0) > 0
        )
        .sort((a, b) => (a.monthly_credits ?? 0) - (b.monthly_credits ?? 0))
        .map((plan) => ({
          plan,
          credits: plan.monthly_credits ?? 0,
          priceMinor: plan.price_cents ?? 0,
          label: formatCreditLabel(plan.monthly_credits ?? 0),
        })),
    [plans]
  )

  const currentPlan = plans.find((plan) => plan.slug === currentPlanSlug)
  const currentPrice = planPrice(currentPlan)
  const hasPaidSubscription = subscription?.status === "active" && currentPrice > 0

  const defaultIndex = useMemo(() => {
    if (businessSteps.length === 0) return 0
    const currentIdx = businessSteps.findIndex(
      (step) => step.plan.slug === currentPlanSlug
    )
    if (currentIdx >= 0) return currentIdx
    const nextIdx = businessSteps.findIndex((step) => step.priceMinor > currentPrice)
    return nextIdx >= 0 ? nextIdx : 0
  }, [businessSteps, currentPlanSlug, currentPrice])

  const selectedIndex = Math.min(
    sliderValue ?? defaultIndex,
    Math.max(0, businessSteps.length - 1)
  )
  const selectedTier = businessSteps[selectedIndex]

  const anyPending =
    paystack.isPending ||
    previewChange.isPending ||
    initUpgrade.isPending ||
    applyChange.isPending ||
    busySlug !== null

  function resetConfirm() {
    setPendingPlan(null)
    setPendingPreview(null)
    setPaidReference(null)
    setBusySlug(null)
  }

  async function handleChoose(plan: Plan | undefined) {
    if (!plan?.slug || anyPending) return

    // Brand-new subscription (no active paid plan yet): straight to checkout.
    // paystack.pendingSlug tracks the in-flight plan and self-clears on
    // success or popup cancel, so we don't touch busySlug here.
    if (!hasPaidSubscription) {
      if (planPrice(plan) <= 0) return
      paystack.subscribe(plan)
      return
    }

    // Existing subscriber: preview the prorated change, then confirm.
    setBusySlug(plan.slug)
    try {
      const preview = await previewChange.mutateAsync({ body: { plan_slug: plan.slug } })
      setPendingPlan(plan)
      setPendingPreview(preview)
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not preview this plan change"))
    } finally {
      // Preview is done; the confirm dialog now waits on the user, so release
      // the busy lock (otherwise the confirm button stays disabled forever).
      setBusySlug(null)
    }
  }

  async function applyQuote(reference: string | undefined) {
    if (!pendingPreview?.quote_id) {
      toast.danger("Plan change quote is missing")
      return
    }
    try {
      const response = await applyChange.mutateAsync({
        body: { quote_id: pendingPreview.quote_id, paystack_reference: reference },
      })
      const isDowngrade = pendingPreview.kind === "downgrade"
      toast.success(
        isDowngrade
          ? `Scheduled change to ${pendingPlan?.name ?? response.plan_slug ?? "the selected plan"}`
          : `Switched to ${pendingPlan?.name ?? response.plan_slug ?? "the selected plan"}`
      )
      resetConfirm()
      refreshBilling()
      onOpenChange(false)
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not apply this plan change"))
      setBusySlug(null)
    }
  }

  async function handleConfirm() {
    if (!pendingPreview?.quote_id || applyChange.isPending || initUpgrade.isPending) return

    // Downgrades / no-cost changes apply immediately without payment.
    if (!pendingPreview.requires_payment_now) {
      await applyQuote(undefined)
      return
    }
    if (paidReference) {
      await applyQuote(paidReference)
      return
    }

    // Lock the action through init-upgrade → Paystack popup → apply-change.
    setBusySlug(pendingPlan?.slug ?? "pending")
    try {
      const upgrade = await initUpgrade.mutateAsync({
        body: { quote_id: pendingPreview.quote_id },
      })
      if (!upgrade.access_code) {
        toast.danger("Provider did not return an access code")
        setBusySlug(null)
        return
      }
      paystack.openPopup(
        upgrade.access_code,
        (reference) => {
          setPaidReference(reference)
          void applyQuote(reference)
        },
        () => setBusySlug(null)
      )
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not start the upgrade payment"))
      setBusySlug(null)
    }
  }

  const confirmIsUpgrade = pendingPreview?.kind !== "downgrade"
  const confirmActionLabel = pendingPreview?.requires_payment_now
    ? "Pay and upgrade"
    : confirmIsUpgrade
      ? "Confirm upgrade"
      : "Schedule change"

  const fromPlan = plans.find((plan) => plan.slug === pendingPreview?.from_plan_slug)
  const fromName = fromPlan?.name ?? pendingPreview?.from_plan_slug ?? "Current plan"
  const toName = pendingPlan?.name ?? pendingPreview?.to_plan_slug ?? "New plan"

  const confirmRows: { label: string; value: string; emphasis?: boolean }[] = [
    { label: "Current plan", value: fromName },
    { label: "New plan", value: toName },
    ...(confirmIsUpgrade
      ? [
          {
            label: "Due today",
            value: formatMoney(pendingPreview?.amount_minor, pendingPreview?.currency),
            emphasis: true,
          },
          ...(pendingPreview?.credit_grant_minor
            ? [
                {
                  label: "Credits added",
                  value: `+${formatNumber(pendingPreview.credit_grant_minor)}`,
                },
              ]
            : []),
          { label: "Takes effect", value: "Immediately" },
          { label: "Quote expires", value: formatDate(pendingPreview?.expires_at) },
        ]
      : [
          {
            label: "New monthly price",
            value: formatMoney(pendingPlan?.price_cents, pendingPlan?.currency),
          },
          { label: "Takes effect", value: formatDate(pendingPreview?.effective_at) },
        ]),
  ]

  function cardState(plan: Plan | undefined) {
    return {
      busy: Boolean(plan?.slug) &&
        (busySlug === plan?.slug || paystack.pendingSlug === plan?.slug),
      disabled:
        anyPending && busySlug !== plan?.slug && paystack.pendingSlug !== plan?.slug,
    }
  }

  return (
    <>
      <Modal isOpen={open} onOpenChange={onOpenChange}>
        <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
          <Modal.Container size="full" className="overflow-y-auto">
            <Modal.Dialog className="flex min-h-full w-full flex-col bg-background outline-none">
              <header className="sticky top-0 z-10 flex items-center justify-end bg-background/95 px-6 py-4 backdrop-blur">
                <button
                  type="button"
                  aria-label="Close"
                  onClick={() => onOpenChange(false)}
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted transition-colors hover:bg-default hover:text-foreground"
                >
                  <Icon icon="lucide:x" className="h-5 w-5" />
                </button>
              </header>

              <div className="mx-auto w-full max-w-4xl flex-1 px-6 py-12">
                <div className="mx-auto max-w-2xl text-center">
                  <h1 className="text-3xl font-semibold tracking-tight">
                    Pricing for AI work that actually runs
                  </h1>
                  <p className="mx-auto mt-3 max-w-xl text-sm text-muted">
                    Start free, then choose the Business credit checkpoint that
                    matches how much work your team wants Hivy to handle.
                  </p>
                </div>

                {businessSteps.length === 0 ? (
                  <div className="mt-12 rounded-2xl border border-border bg-surface px-5 py-16 text-center text-sm text-muted">
                    Pricing plans are temporarily unavailable.
                  </div>
                ) : (
                  <>
                    <div className="mx-auto mt-12 max-w-xl">
                      <CreditSlider
                        steps={businessSteps}
                        value={selectedIndex}
                        onChange={setSliderValue}
                      />
                    </div>

                    <div className="mx-auto mt-12 grid max-w-4xl grid-cols-1 gap-5 md:grid-cols-2">
                      <PlanCard
                        accentLabel="Free"
                        price="Free"
                        description={`Try Hivy with ${formatNumber(
                          freePlan?.welcome_credits ?? 0
                        )} credits.`}
                        features={freePlan?.features ?? []}
                        action={
                          freePlan
                            ? planActionFor(freePlan, currentPlanSlug, currentPrice)
                            : { label: "Unavailable", variant: "tertiary", disabled: true, kind: "current" }
                        }
                        {...cardState(freePlan)}
                        onChoose={() => void handleChoose(freePlan)}
                      />
                      {selectedTier ? (
                        <PlanCard
                          accentLabel={`${selectedTier.label} checkpoint`}
                          badgeLabel="Business"
                          price={formatMoney(
                            selectedTier.priceMinor,
                            selectedTier.plan.currency
                          )}
                          period="/mo"
                          description={`Includes ${formatNumber(
                            selectedTier.credits
                          )} monthly credits.`}
                          features={selectedTier.plan.features ?? []}
                          action={planActionFor(
                            selectedTier.plan,
                            currentPlanSlug,
                            currentPrice
                          )}
                          {...cardState(selectedTier.plan)}
                          onChoose={() => void handleChoose(selectedTier.plan)}
                          highlighted
                        />
                      ) : null}
                    </div>
                  </>
                )}
              </div>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      <Modal
        isOpen={pendingPreview !== null}
        onOpenChange={(next) => {
          if (!next && !applyChange.isPending) resetConfirm()
        }}
      >
        <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
          <Modal.Container placement="center" size="md">
            <Modal.Dialog className="p-8">
              <Modal.CloseTrigger />

              <Modal.Header>
                <Modal.Icon className="size-12 bg-default text-foreground">
                  <Icon icon="lucide:credit-card" className="h-6 w-6" />
                </Modal.Icon>
                <div className="flex flex-col gap-1">
                  <Modal.Heading>
                    {confirmIsUpgrade ? "Confirm upgrade" : "Confirm downgrade"}
                  </Modal.Heading>
                  <p className="text-sm text-muted">
                    {confirmIsUpgrade
                      ? "Review the prorated charge before continuing."
                      : "This change is scheduled for the end of your current billing period."}
                  </p>
                </div>
              </Modal.Header>

              <Modal.Body>
                <div className="overflow-hidden rounded-2xl border border-border bg-surface">
                  {confirmRows.map((row, index) => (
                    <DetailRow
                      key={row.label}
                      label={row.label}
                      value={row.value}
                      emphasis={row.emphasis}
                      last={index === confirmRows.length - 1}
                    />
                  ))}
                </div>
              </Modal.Body>

              <Modal.Footer>
                <Button
                  variant="tertiary"
                  size="sm"
                  isDisabled={applyChange.isPending || initUpgrade.isPending}
                  onPress={resetConfirm}
                >
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  isDisabled={anyPending}
                  onPress={() => void handleConfirm()}
                >
                  {applyChange.isPending || initUpgrade.isPending ? (
                    <Spinner color="current" size="sm" />
                  ) : null}
                  {confirmActionLabel}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </>
  )
}
