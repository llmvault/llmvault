"use client"

import { useCallback, useState } from "react"
import { toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"

async function openPaystackTransaction(
  accessCode: string,
  handlers: { onSuccess: () => void; onCancel: () => void }
) {
  const { default: PaystackPop } = await import("@paystack/inline-js")
  new PaystackPop().resumeTransaction(accessCode, handlers)
}

export function useCreditPurchase() {
  const queryClient = useQueryClient()
  const createPurchase = $api.useMutation("post", "/v1/billing/purchases")
  const verifyPurchase = $api.useMutation(
    "post",
    "/v1/billing/purchases/{id}/verify"
  )
  const [isPopupOpen, setIsPopupOpen] = useState(false)

  const refreshBilling = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.billingAccount() }),
      queryClient.invalidateQueries({ queryKey: queryKeys.billingPurchases() }),
      queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() }),
      queryClient.invalidateQueries({
        queryKey: queryKeys.billingPaymentMethods(),
      }),
    ])
  }, [queryClient])

  const verify = useCallback(
    (id: string, onComplete?: () => void) => {
      verifyPurchase.mutate(
        { params: { path: { id } } },
        {
          onSuccess: (verified) => {
            toast.success(
              `${(verified.credits ?? 0).toLocaleString()} credits added`
            )
            void refreshBilling()
            setIsPopupOpen(false)
            onComplete?.()
          },
          onError: (error) => {
            toast.danger(
              extractErrorMessage(
                error,
                "Payment has not been confirmed yet. Try again shortly."
              )
            )
            void refreshBilling()
            setIsPopupOpen(false)
          },
        }
      )
    },
    [refreshBilling, verifyPurchase]
  )

  const purchase = useCallback(
    ({
      currency,
      packID,
      subtotalMinor,
      paymentMethodID,
      savePaymentMethod,
      onComplete,
    }: {
      currency: "USD" | "NGN"
      packID?: string
      subtotalMinor?: number
      paymentMethodID?: string
      savePaymentMethod: boolean
      onComplete?: () => void
    }) => {
      createPurchase.mutate(
        {
          body: {
            currency,
            pack_id: packID,
            subtotal_minor: subtotalMinor,
            idempotency_key: crypto.randomUUID(),
            payment_method_id: paymentMethodID,
            save_payment_method: savePaymentMethod,
          },
        },
        {
          onSuccess: (created) => {
            if (!created.id) {
              toast.danger("Paystack purchase could not be started")
              return
            }
            const purchaseID = created.id
            if (!created.access_code) {
              verify(purchaseID, onComplete)
              return
            }
            setIsPopupOpen(true)
            void openPaystackTransaction(created.access_code, {
              onSuccess: () => verify(purchaseID, onComplete),
              onCancel: () => setIsPopupOpen(false),
            }).catch((error) => {
              toast.danger(
                extractErrorMessage(error, "Could not open Paystack")
              )
              setIsPopupOpen(false)
            })
          },
          onError: (error) => {
            toast.danger(
              extractErrorMessage(error, "Could not create credit purchase")
            )
          },
        }
      )
    },
    [createPurchase, verify]
  )

  return {
    purchase,
    verify,
    isPending:
      createPurchase.isPending || verifyPurchase.isPending || isPopupOpen,
  }
}
