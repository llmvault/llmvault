"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Modal, Spinner, Tooltip, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useIsOwner } from "@/lib/auth/use-role"
import type { components } from "@/lib/api/schema"

type SubscriptionResponse = components["schemas"]["subscriptionResponse"]

const SUBSCRIPTION_KEY = ["get", "/v1/billing/subscription"]

function formatPeriodEnd(value: string | undefined) {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return new Intl.DateTimeFormat("en-NG", {
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(date)
}

export function CancelPlanSection() {
  const queryClient = useQueryClient()
  const isOwner = useIsOwner()
  const subscriptionQuery = $api.useQuery("get", "/v1/billing/subscription")
  const cancelSubscription = $api.useMutation(
    "post",
    "/v1/billing/subscription/cancel"
  )
  const resumeSubscription = $api.useMutation(
    "post",
    "/v1/billing/subscription/resume"
  )
  const [confirmOpen, setConfirmOpen] = useState(false)

  const subscription = subscriptionQuery.data as
    | SubscriptionResponse
    | undefined

  // Only paid subscriptions (created through a payment provider) can be
  // canceled — the free plan has no provider subscription behind it.
  const hasPaidSubscription = Boolean(subscription?.provider)
  if (!hasPaidSubscription) return null

  const cancelsAtPeriodEnd = Boolean(subscription?.cancel_at_period_end)
  const periodEndLabel = formatPeriodEnd(subscription?.current_period_end)

  function handleCancel() {
    cancelSubscription.mutate(
      { body: { at_period_end: true } },
      {
        onSuccess: () => {
          toast.success("Your plan will cancel at the end of the period")
          queryClient.invalidateQueries({ queryKey: SUBSCRIPTION_KEY })
          setConfirmOpen(false)
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not cancel your plan")),
      }
    )
  }

  function handleResume() {
    resumeSubscription.mutate(
      {},
      {
        onSuccess: () => {
          toast.success("Your plan has been resumed")
          queryClient.invalidateQueries({ queryKey: SUBSCRIPTION_KEY })
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not resume your plan")),
      }
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-medium">Cancel plan</h2>
      <div className="bg-surface flex items-center gap-4 rounded-2xl border border-border px-4 py-4">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          {cancelsAtPeriodEnd ? (
            <>
              <span className="text-sm font-medium">
                {periodEndLabel
                  ? `Cancels on ${periodEndLabel}`
                  : "Cancels at the end of the current period"}
              </span>
              <span className="text-sm text-muted">
                Your plan stays active until then. Resume any time before that
                date to keep it.
              </span>
            </>
          ) : (
            <>
              <span className="text-sm font-medium">
                Cancel your subscription
              </span>
              <span className="text-sm text-muted">
                Your plan stays active until the end of the current billing
                period{periodEndLabel ? ` (${periodEndLabel})` : ""}.
              </span>
            </>
          )}
        </div>
        {cancelsAtPeriodEnd ? (
          <OwnerGatedButton
            isOwner={isOwner}
            hint="Only the workspace owner can resume the plan."
            isDisabled={resumeSubscription.isPending || !isOwner}
            onPress={handleResume}
          >
            {resumeSubscription.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
            Resume plan
          </OwnerGatedButton>
        ) : (
          <OwnerGatedButton
            isOwner={isOwner}
            hint="Only the workspace owner can cancel the plan."
            isDisabled={!isOwner}
            onPress={() => setConfirmOpen(true)}
          >
            Cancel plan
          </OwnerGatedButton>
        )}
      </div>

      <Modal
        isOpen={confirmOpen}
        onOpenChange={(next) => {
          if (!next) {
            if (!cancelSubscription.isPending) setConfirmOpen(false)
          } else {
            setConfirmOpen(true)
          }
        }}
      >
        <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
          <Modal.Container placement="center" size="md">
            <Modal.Dialog className="p-8">
              <Modal.CloseTrigger />

              <Modal.Header>
                <Modal.Icon className="bg-default size-12 text-foreground">
                  <AppIcon icon="circle-slash" className="h-6 w-6" />
                </Modal.Icon>
                <div className="flex flex-col gap-1">
                  <Modal.Heading>Cancel your plan?</Modal.Heading>
                  <p className="text-sm text-muted">
                    Your plan stays active until the end of the current billing
                    period{periodEndLabel ? ` on ${periodEndLabel}` : ""}.
                    After that you&apos;ll move to the free plan. You can
                    resume any time before then.
                  </p>
                </div>
              </Modal.Header>

              <Modal.Footer>
                <Button
                  variant="tertiary"
                  size="sm"
                  isDisabled={cancelSubscription.isPending}
                  onPress={() => setConfirmOpen(false)}
                >
                  Keep plan
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  isDisabled={cancelSubscription.isPending}
                  onPress={handleCancel}
                >
                  {cancelSubscription.isPending ? (
                    <Spinner color="current" size="sm" />
                  ) : null}
                  Cancel plan
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </section>
  )
}

function OwnerGatedButton({
  isOwner,
  hint,
  isDisabled,
  onPress,
  children,
}: {
  isOwner: boolean
  hint: string
  isDisabled: boolean
  onPress: () => void
  children: React.ReactNode
}) {
  const control = (
    <Button
      variant="tertiary"
      size="sm"
      isDisabled={isDisabled}
      onPress={onPress}
    >
      {children}
    </Button>
  )
  if (isOwner) return control
  return (
    <Tooltip delay={250} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">{control}</Tooltip.Trigger>
      <Tooltip.Content placement="left" offset={8} className="text-xs">
        {hint}
      </Tooltip.Content>
    </Tooltip>
  )
}
