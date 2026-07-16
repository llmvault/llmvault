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
    ])
  }, [queryClient])

  const verify = useCallback(
    (id: string) => {
      verifyPurchase.mutate(
        { params: { path: { id } } },
        {
          onSuccess: (verified) => {
            toast.success(
              `${(verified.credits ?? 0).toLocaleString()} credits added`
            )
            void refreshBilling()
            setIsPopupOpen(false)
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
    (subtotalMinor: number) => {
      const callbackURL =
        typeof window === "undefined" ? "" : window.location.href
      createPurchase.mutate(
        { body: { subtotal_minor: subtotalMinor, callback_url: callbackURL } },
        {
          onSuccess: (created) => {
            if (!created.id || !created.access_code) {
              toast.danger("Paystack checkout could not be started")
              return
            }
            const purchaseID = created.id
            setIsPopupOpen(true)
            void openPaystackTransaction(created.access_code, {
              onSuccess: () => verify(purchaseID),
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
