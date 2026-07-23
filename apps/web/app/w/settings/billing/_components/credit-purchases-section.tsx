"use client"

import { useState } from "react"
import { Button, Modal, Skeleton, Spinner, Switch, toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"
import { useIsOwner } from "@/lib/auth/use-role"
import { useCreditPurchase } from "@/hooks/use-credit-purchase"
import type { components } from "@/lib/api/schema"
import {
  filterByPurchaseCurrency,
  resolveCompatiblePaymentMethodID,
  type PurchaseCurrency,
} from "./purchase-currency"

type Currency = PurchaseCurrency
type PaymentMethod = components["schemas"]["billingPaymentMethodResponse"]

const CURRENCY_LABELS: Record<Currency, string> = {
  USD: "US dollars",
  NGN: "Nigerian naira",
}

function formatMoney(minor: number, currency: Currency): string {
  return new Intl.NumberFormat(currency === "NGN" ? "en-NG" : "en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: currency === "NGN" ? 0 : 2,
  }).format(minor / 100)
}

function paymentMethodLabel(method: PaymentMethod): string {
  const card = method.card_type?.trim() || "Card"
  const bank = method.bank?.trim()
  return `${bank ? `${bank} ` : ""}${card} ending in ${method.last4 ?? ""}`
}

export function CreditPurchasesSection({
  isOpen,
  onOpenChange,
}: {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}) {
  const isOwner = useIsOwner()
  const purchasesQuery = $api.useQuery("get", "/v1/billing/purchases", {
    params: { query: { limit: 10 } },
  })
  const { verify, isPending } = useCreditPurchase()

  return (
    <>
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
      </section>

      {isOwner ? (
        <BuyCreditsModal isOpen={isOpen} onOpenChange={onOpenChange} />
      ) : null}
    </>
  )
}

function BuyCreditsModal({
  isOpen,
  onOpenChange,
}: {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const accountQuery = $api.useQuery("get", "/v1/billing/account")
  const methodsQuery = $api.useQuery("get", "/v1/billing/payment-methods")
  const deleteMethod = $api.useMutation(
    "delete",
    "/v1/billing/payment-methods/{id}"
  )
  const { purchase, isPending } = useCreditPurchase()
  const [currency, setCurrency] = useState<Currency>("USD")
  const [packID, setPackID] = useState<string | null>(null)
  const [paymentMethodID, setPaymentMethodID] = useState<string | null>(null)
  const [saveCard, setSaveCard] = useState(true)

  const packs = filterByPurchaseCurrency(
    accountQuery.data?.packs ?? [],
    currency
  )
  const methods = filterByPurchaseCurrency(
    methodsQuery.data?.payment_methods ?? [],
    currency
  )
  const selectedPack = packs.find((pack) => pack.id === packID) ?? packs[0]
  const effectiveMethodID = resolveCompatiblePaymentMethodID(
    methods,
    paymentMethodID
  )

  const chooseCurrency = (choice: Currency) => {
    setCurrency(choice)
    setPackID(null)
    setPaymentMethodID(null)
  }

  const removeMethod = (id: string) => {
    deleteMethod.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          setPaymentMethodID("new")
          void queryClient.invalidateQueries({
            queryKey: queryKeys.billingPaymentMethods(),
          })
          toast.success("Saved card removed")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not remove card")),
      }
    )
  }

  const submit = () => {
    if (!selectedPack?.id) return
    const useNewCard = effectiveMethodID === "new"
    purchase({
      currency,
      packID: selectedPack.id,
      paymentMethodID: useNewCard ? undefined : effectiveMethodID,
      savePaymentMethod: useNewCard && saveCard,
      onComplete: () => onOpenChange(false),
    })
  }

  return (
    <Modal isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Backdrop>
        <Modal.Container placement="center">
          <Modal.Dialog className="w-full max-w-lg p-6">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>Buy credits</Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-5">
              {accountQuery.isLoading ? (
                <Skeleton className="h-72 rounded-xl" />
              ) : (
                <>
                  <div>
                    <div className="flex items-center justify-between gap-4">
                      <p className="text-sm font-medium">Payment currency</p>
                      <span className="text-xs text-muted">
                        Choose for this deposit
                      </span>
                    </div>
                    <div className="mt-3 grid grid-cols-2 gap-3">
                      {(["USD", "NGN"] as const).map((option) => (
                        <button
                          key={option}
                          type="button"
                          onClick={() => chooseCurrency(option)}
                          className={`rounded-xl border px-4 py-3 text-left transition-colors ${
                            currency === option
                              ? "border-foreground bg-default"
                              : "border-border hover:bg-default"
                          }`}
                        >
                          <span className="block text-sm font-medium">
                            {option}
                          </span>
                          <span className="text-xs text-muted">
                            {CURRENCY_LABELS[option]}
                          </span>
                        </button>
                      ))}
                    </div>
                  </div>

                  <div>
                    <div className="flex items-center justify-between gap-4">
                      <p className="text-sm font-medium">
                        Choose a credit pack
                      </p>
                      <span className="text-xs text-muted">{currency}</span>
                    </div>
                    <div className="mt-3 grid grid-cols-2 gap-3">
                      {packs.map((pack) => {
                        const selected = pack.id === selectedPack?.id
                        return (
                          <button
                            key={pack.id}
                            type="button"
                            onClick={() => setPackID(pack.id ?? null)}
                            className={`rounded-xl border p-3 text-left transition-colors ${
                              selected
                                ? "border-foreground bg-default"
                                : "border-border hover:bg-default"
                            }`}
                          >
                            <span className="block text-sm font-medium">
                              {formatMoney(pack.subtotal_minor ?? 0, currency)}
                            </span>
                            <span className="text-xs text-muted">
                              {(pack.credits ?? 0).toLocaleString()} credits
                            </span>
                          </button>
                        )
                      })}
                    </div>
                  </div>

                  <div>
                    <p className="text-sm font-medium">Payment method</p>
                    <div className="mt-3 flex flex-col gap-2">
                      {methods.map((method) => {
                        const methodID = method.id
                        return (
                          <div
                            key={methodID}
                            className={`flex items-center gap-3 rounded-xl border px-3 py-2.5 ${
                              effectiveMethodID === methodID
                                ? "border-foreground bg-default"
                                : "border-border"
                            }`}
                          >
                            <button
                              type="button"
                              onClick={() =>
                                setPaymentMethodID(methodID ?? "new")
                              }
                              className="min-w-0 flex-1 text-left"
                            >
                              <span className="block truncate text-sm font-medium">
                                {paymentMethodLabel(method)}
                              </span>
                              <span className="text-xs text-muted">
                                {method.currency} · Expires {method.exp_month}/
                                {method.exp_year}
                              </span>
                            </button>
                            {methodID ? (
                              <button
                                type="button"
                                onClick={() => removeMethod(methodID)}
                                className="text-xs text-muted hover:text-foreground"
                              >
                                Remove
                              </button>
                            ) : null}
                          </div>
                        )
                      })}
                      <button
                        type="button"
                        onClick={() => setPaymentMethodID("new")}
                        className={`rounded-xl border px-3 py-2.5 text-left text-sm font-medium transition-colors ${
                          effectiveMethodID === "new"
                            ? "border-foreground bg-default"
                            : "border-border hover:bg-default"
                        }`}
                      >
                        Pay with a new card
                      </button>
                    </div>
                  </div>

                  {effectiveMethodID === "new" ? (
                    <div className="flex items-center justify-between gap-4 rounded-xl bg-default p-3">
                      <div>
                        <p className="text-sm font-medium">
                          Save card for next time
                        </p>
                        <p className="text-xs text-muted">
                          Hivy stores only Paystack&apos;s encrypted reusable
                          token.
                        </p>
                      </div>
                      <Switch
                        aria-label="Save card for future purchases"
                        isSelected={saveCard}
                        onChange={setSaveCard}
                      >
                        <Switch.Control>
                          <Switch.Thumb />
                        </Switch.Control>
                      </Switch>
                    </div>
                  ) : null}

                  {selectedPack ? (
                    <div className="flex flex-col gap-2 border-t border-border pt-4 text-sm">
                      <div className="flex justify-between gap-4 text-muted">
                        <span>Credit pack</span>
                        <span>
                          {formatMoney(
                            selectedPack.subtotal_minor ?? 0,
                            currency
                          )}
                        </span>
                      </div>
                      <div className="flex justify-between gap-4 text-muted">
                        <span>
                          Deposit fee (
                          {(selectedPack.fee_basis_points ?? 0) / 100}%)
                        </span>
                        <span>
                          {formatMoney(selectedPack.fee_minor ?? 0, currency)}
                        </span>
                      </div>
                      <div className="flex justify-between gap-4 font-medium">
                        <span>Total charged</span>
                        <span>
                          {formatMoney(selectedPack.total_minor ?? 0, currency)}
                        </span>
                      </div>
                    </div>
                  ) : null}
                  <p className="text-xs leading-5 text-muted">
                    Paystack securely handles card details and authentication.
                    Credits are added only after Hivy verifies the exact amount,
                    currency, workspace, and purchase reference.
                  </p>
                </>
              )}
            </Modal.Body>
            <Modal.Footer>
              <Button variant="ghost" onPress={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                isDisabled={!selectedPack || isPending}
                onPress={submit}
              >
                {isPending ? <Spinner size="sm" /> : null}
                Pay{" "}
                {selectedPack
                  ? formatMoney(selectedPack.total_minor ?? 0, currency)
                  : ""}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
