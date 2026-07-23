"use client"

import { type FormEvent, useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { Button, Input, Skeleton, Switch, toast } from "@heroui/react"
import { Stripe } from "@thesvg/react"
import { useQueryClient } from "@tanstack/react-query"
import { AppIcon } from "@/components/icon"
import { useCreditPurchase } from "@/hooks/use-credit-purchase"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsOwner } from "@/lib/auth/use-role"
import type { components } from "@/lib/api/schema"
import { cn } from "@/lib/utils"
import {
  filterByPurchaseCurrency,
  quoteCustomPurchase,
  resolveCompatiblePaymentMethodID,
  type PurchaseCurrency,
} from "./_components/purchase-currency"
import {
  formatPurchaseMoney,
  PurchasePageSkeleton,
  PurchaseSummary,
} from "./_components/purchase-page-panels"

type Currency = PurchaseCurrency
type PaymentMethod = components["schemas"]["billingPaymentMethodResponse"]

const CUSTOM_AMOUNT_ID = "custom"
const CURRENCIES = ["USD", "NGN"] as const
const CURRENCY_LABELS: Record<Currency, string> = {
  USD: "US dollars",
  NGN: "Nigerian naira",
}

function paymentMethodLabel(method: PaymentMethod): string {
  const card = method.card_type?.trim() || "Card"
  const bank = method.bank?.trim()
  const ending = method.last4 ? ` ending in ${method.last4}` : ""
  return `${bank ? `${bank} ` : ""}${card}${ending}`
}

function SelectionDot({ selected }: { selected: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-4 shrink-0 items-center justify-center rounded-full border",
        selected ? "border-accent" : "border-border"
      )}
    >
      {selected ? <span className="size-2 rounded-full bg-accent" /> : null}
    </span>
  )
}

function PaystackMark({ className }: { className?: string }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 612 602"
      fill="none"
      className={className}
    >
      <path
        fill="#0BA4DB"
        d="M548.416 0H31.792C14.306 0 0 14.333 0 31.852v57.333c0 17.519 14.306 31.852 31.792 31.852h515.034c17.486 0 31.792-14.333 31.792-31.852V31.852C580.208 14.333 565.901 0 548.416 0ZM548.416 320.111H31.792C14.306 320.111 0 334.444 0 351.963v57.333c0 17.519 14.306 31.852 31.792 31.852h515.034c17.486 0 31.792-14.333 31.792-31.852v-57.333c1.59-17.519-12.717-31.852-30.202-31.852ZM322.691 480.963H31.792C14.306 480.963 0 495.296 0 512.815v57.333C0 587.667 14.306 602 31.792 602h290.899c17.486 0 31.792-14.333 31.792-31.852v-57.333c0-17.519-14.306-31.852-31.792-31.852ZM580.208 160.852H31.792C14.306 160.852 0 175.185 0 192.703v57.334c0 17.518 14.306 31.852 31.792 31.852h548.416c17.486 0 31.792-14.334 31.792-31.852v-57.334c0-17.518-14.306-31.851-31.792-31.851Z"
      />
    </svg>
  )
}

export default function PurchaseCreditsPageContent() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const isOwner = useIsOwner()
  const { isLoading: authLoading } = useAuth()
  const accountQuery = $api.useQuery(
    "get",
    "/v1/billing/account",
    {},
    { enabled: isOwner }
  )
  const methodsQuery = $api.useQuery(
    "get",
    "/v1/billing/payment-methods",
    {},
    { enabled: isOwner }
  )
  const deleteMethod = $api.useMutation(
    "delete",
    "/v1/billing/payment-methods/{id}"
  )
  const { purchase, isPending } = useCreditPurchase()
  const [currency, setCurrency] = useState<Currency>("USD")
  const [packID, setPackID] = useState<string | null>(null)
  const [customAmount, setCustomAmount] = useState("")
  const [paymentMethodID, setPaymentMethodID] = useState<string | null>(null)
  const [saveCard, setSaveCard] = useState(true)

  useEffect(() => {
    if (!authLoading && !isOwner) router.replace("/w/billing")
  }, [authLoading, isOwner, router])

  const packs = filterByPurchaseCurrency(
    accountQuery.data?.packs ?? [],
    currency
  )
  const methods = filterByPurchaseCurrency(
    methodsQuery.data?.payment_methods ?? [],
    currency
  )
  const isCustomAmount = packID === CUSTOM_AMOUNT_ID
  const selectedPack = isCustomAmount
    ? undefined
    : (packs.find((pack) => pack.id === packID) ?? packs[0])
  const customQuote = quoteCustomPurchase(
    customAmount,
    currency,
    accountQuery.data?.fee_basis_points ?? 0,
    accountQuery.data?.fx_minor_per_usd ?? 0
  )
  const selectedSubtotalMinor = isCustomAmount
    ? customQuote?.subtotalMinor
    : selectedPack?.subtotal_minor
  const selectedFeeMinor = isCustomAmount
    ? customQuote?.feeMinor
    : selectedPack?.fee_minor
  const selectedTotalMinor = isCustomAmount
    ? customQuote?.totalMinor
    : selectedPack?.total_minor
  const selectedCredits = isCustomAmount
    ? customQuote?.credits
    : selectedPack?.credits
  const selectedFeeBasisPoints =
    selectedPack?.fee_basis_points ?? accountQuery.data?.fee_basis_points ?? 0
  const effectiveMethodID = resolveCompatiblePaymentMethodID(
    methods,
    paymentMethodID
  )

  function chooseCurrency(choice: Currency) {
    setCurrency(choice)
    setPackID(null)
    setCustomAmount("")
    setPaymentMethodID(null)
  }

  function removeMethod(id: string) {
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

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (selectedTotalMinor === undefined) return

    const useNewCard = effectiveMethodID === "new"
    const onComplete = () => router.replace("/w/billing")

    if (isCustomAmount) {
      if (!customQuote) return
      purchase({
        currency,
        subtotalMinor: customQuote.subtotalMinor,
        paymentMethodID: useNewCard ? undefined : effectiveMethodID,
        savePaymentMethod: useNewCard && saveCard,
        onComplete,
      })
      return
    }

    if (!selectedPack?.id) return
    purchase({
      currency,
      packID: selectedPack.id,
      paymentMethodID: useNewCard ? undefined : effectiveMethodID,
      savePaymentMethod: useNewCard && saveCard,
      onComplete,
    })
  }

  if (!isOwner) return null

  return (
    <main className="mx-auto w-full max-w-5xl">
      <Button
        variant="ghost"
        size="sm"
        onPress={() => router.push("/w/billing")}
        className="-ml-2 text-muted"
      >
        <AppIcon icon="arrow-left" className="size-4" />
        Usage &amp; billing
      </Button>

      <div className="mt-5">
        <h1 className="text-2xl font-semibold tracking-tight">
          Purchase credits
        </h1>
        <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
          Add prepaid usage credits to this workspace. Review the exact fee and
          total before Paystack opens.
        </p>
      </div>

      {accountQuery.isLoading ? (
        <PurchasePageSkeleton />
      ) : accountQuery.isError ? (
        <div className="mt-8 rounded-2xl border border-border bg-surface px-6 py-12 text-center">
          <div className="mx-auto flex size-10 items-center justify-center rounded-full bg-default">
            <AppIcon icon="circle-alert" className="size-5 text-muted" />
          </div>
          <h2 className="mt-4 text-base font-semibold">
            Could not load purchase options
          </h2>
          <p className="mx-auto mt-2 max-w-sm text-sm text-muted">
            Your billing details could not be loaded. Try again before starting
            a payment.
          </p>
          <Button
            variant="tertiary"
            size="sm"
            className="mt-5"
            onPress={() => accountQuery.refetch()}
          >
            Try again
          </Button>
        </div>
      ) : (
        <form
          onSubmit={submit}
          className="mt-8 grid overflow-hidden rounded-2xl border border-border bg-surface shadow-sm lg:grid-cols-[minmax(0,1fr)_19rem]"
        >
          <div className="col-span-full flex min-h-12 flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-2 sm:px-7">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted">
              <span>Powered by</span>
              <span className="inline-flex items-center gap-1.5 font-semibold text-foreground">
                <PaystackMark className="size-4" />
                Paystack
              </span>
              <span aria-hidden="true" className="text-border">
                •
              </span>
              <span>a</span>
              <Stripe aria-label="Stripe" className="h-3.5 w-auto shrink-0" />
              <span>company</span>
            </div>

            <fieldset>
              <legend className="sr-only">Payment currency</legend>
              <div className="flex items-center rounded-lg bg-default p-0.5">
                {CURRENCIES.map((option) => {
                  const selected = currency === option
                  return (
                    <button
                      key={option}
                      type="button"
                      aria-label={`Pay in ${option}, ${CURRENCY_LABELS[option]}`}
                      aria-pressed={selected}
                      onClick={() => chooseCurrency(option)}
                      className={cn(
                        "focus-visible:ring-ring h-7 rounded-md px-3 text-xs font-semibold transition-colors duration-150 focus-visible:ring-2 focus-visible:outline-none",
                        selected
                          ? "bg-surface text-foreground shadow-xs"
                          : "text-muted hover:text-foreground"
                      )}
                    >
                      {option}
                    </button>
                  )
                })}
              </div>
            </fieldset>
          </div>

          <div className="min-w-0 px-5 py-5 sm:px-7 sm:py-6">
            <fieldset>
              <legend className="text-sm font-semibold">Purchase amount</legend>
              <p className="mt-1 text-sm text-muted">
                1,000 credits equals $1 of usage.
              </p>
              <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
                {packs.map((pack) => {
                  const selected = pack.id === selectedPack?.id
                  return (
                    <button
                      key={pack.id}
                      type="button"
                      aria-pressed={selected}
                      onClick={() => setPackID(pack.id ?? null)}
                      className={cn(
                        "focus-visible:ring-ring relative min-h-16 rounded-xl border p-3 pr-8 text-left transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none",
                        selected
                          ? "border-accent bg-default"
                          : "border-border hover:bg-default"
                      )}
                    >
                      <span>
                        <span className="block text-sm font-semibold whitespace-nowrap tabular-nums">
                          {formatPurchaseMoney(
                            pack.subtotal_minor ?? 0,
                            currency
                          )}
                        </span>
                        <span className="mt-0.5 block text-xs whitespace-nowrap text-muted">
                          {(pack.credits ?? 0).toLocaleString()} credits
                        </span>
                      </span>
                      <span className="absolute top-3 right-3">
                        <SelectionDot selected={selected} />
                      </span>
                    </button>
                  )
                })}
                <button
                  type="button"
                  aria-pressed={isCustomAmount}
                  onClick={() => setPackID(CUSTOM_AMOUNT_ID)}
                  className={cn(
                    "focus-visible:ring-ring relative min-h-16 rounded-xl border p-3 pr-8 text-left transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none",
                    isCustomAmount
                      ? "border-accent bg-default"
                      : "border-border hover:bg-default"
                  )}
                >
                  <span>
                    <span className="block text-sm font-semibold">Custom</span>
                    <span className="mt-0.5 block text-xs text-muted">
                      Any amount
                    </span>
                  </span>
                  <span className="absolute top-3 right-3">
                    <SelectionDot selected={isCustomAmount} />
                  </span>
                </button>
              </div>

              {isCustomAmount ? (
                <Input
                  id="custom-purchase-amount"
                  aria-label={`Custom amount in ${currency}`}
                  inputMode="decimal"
                  placeholder={`Amount in ${currency}`}
                  value={customAmount}
                  onChange={(event) => setCustomAmount(event.target.value)}
                  aria-invalid={
                    customAmount.length > 0 && !customQuote ? true : undefined
                  }
                  className="mt-3 w-full"
                />
              ) : null}
            </fieldset>

            <fieldset className="mt-6 border-t border-border pt-5">
              <legend className="text-sm font-semibold">Payment method</legend>

              <div className="mt-3 flex flex-col gap-2">
                {methodsQuery.isLoading ? (
                  <>
                    <Skeleton className="h-14 rounded-xl" />
                    <Skeleton className="h-14 rounded-xl" />
                  </>
                ) : (
                  <>
                    {methodsQuery.isError ? (
                      <p className="rounded-xl bg-default px-4 py-3 text-xs text-muted">
                        Saved cards are unavailable. You can still pay with a
                        new card.
                      </p>
                    ) : null}
                    {methods.map((method) => {
                      const methodID = method.id
                      const selected = effectiveMethodID === methodID
                      return (
                        <div
                          key={methodID}
                          className={cn(
                            "flex min-h-14 items-center gap-3 rounded-xl border px-3 transition-colors duration-150",
                            selected
                              ? "border-accent bg-default"
                              : "border-border"
                          )}
                        >
                          <button
                            type="button"
                            aria-pressed={selected}
                            onClick={() =>
                              setPaymentMethodID(methodID ?? "new")
                            }
                            className="focus-visible:ring-ring flex min-w-0 flex-1 items-center gap-3 py-2 text-left focus-visible:ring-2 focus-visible:outline-none"
                          >
                            <SelectionDot selected={selected} />
                            <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-surface">
                              <AppIcon
                                icon="credit-card"
                                className="size-4 text-muted"
                              />
                            </span>
                            <span className="min-w-0">
                              <span className="block truncate text-sm font-medium">
                                {paymentMethodLabel(method)}
                              </span>
                              <span className="block text-xs text-muted">
                                {method.currency} · Expires {method.exp_month}/
                                {method.exp_year}
                              </span>
                            </span>
                          </button>
                          {methodID ? (
                            <button
                              type="button"
                              disabled={deleteMethod.isPending}
                              onClick={() => removeMethod(methodID)}
                              className="text-xs font-medium text-muted transition-colors hover:text-foreground disabled:opacity-50"
                            >
                              Remove
                            </button>
                          ) : null}
                        </div>
                      )
                    })}
                    <div
                      className={cn(
                        "flex min-h-14 items-center rounded-xl border transition-colors duration-150",
                        effectiveMethodID === "new"
                          ? "border-accent bg-default"
                          : "border-border hover:bg-default"
                      )}
                    >
                      <button
                        type="button"
                        aria-pressed={effectiveMethodID === "new"}
                        onClick={() => setPaymentMethodID("new")}
                        className="focus-visible:ring-ring flex min-w-0 flex-1 items-center gap-3 px-3 py-2 text-left focus-visible:ring-2 focus-visible:outline-none"
                      >
                        <SelectionDot selected={effectiveMethodID === "new"} />
                        <span className="truncate text-sm font-medium">
                          New card
                        </span>
                      </button>
                      {effectiveMethodID === "new" ? (
                        <div className="flex shrink-0 items-center gap-2 px-3">
                          <span className="hidden text-xs text-muted sm:inline">
                            Save card
                          </span>
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
                    </div>
                  </>
                )}
              </div>
            </fieldset>
          </div>

          <PurchaseSummary
            balance={accountQuery.data?.balance ?? 0}
            currency={currency}
            credits={selectedCredits}
            feeBasisPoints={selectedFeeBasisPoints}
            feeMinor={selectedFeeMinor}
            subtotalMinor={selectedSubtotalMinor}
            totalMinor={selectedTotalMinor}
            isPending={isPending}
          />
        </form>
      )}
    </main>
  )
}
