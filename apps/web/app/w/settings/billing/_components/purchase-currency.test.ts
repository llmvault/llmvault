import { describe, expect, it } from "vitest"
import {
  filterByPurchaseCurrency,
  quoteCustomPurchase,
  resolveCompatiblePaymentMethodID,
} from "./purchase-currency"

describe("purchase currency options", () => {
  const methods = [
    { id: "usd-card", currency: "USD" },
    { id: "ngn-card", currency: "NGN" },
  ]

  it("only exposes saved cards authorized for the selected currency", () => {
    expect(filterByPurchaseCurrency(methods, "NGN")).toEqual([
      { id: "ngn-card", currency: "NGN" },
    ])
  })

  it("falls back to a compatible card when the previous selection uses another currency", () => {
    const ngnMethods = filterByPurchaseCurrency(methods, "NGN")

    expect(resolveCompatiblePaymentMethodID(ngnMethods, "usd-card")).toBe(
      "ngn-card"
    )
    expect(resolveCompatiblePaymentMethodID([], "usd-card")).toBe("new")
  })

  it("quotes custom USD and NGN deposits in minor units", () => {
    expect(quoteCustomPurchase("12.34", "USD", 1200, 145_000)).toEqual({
      subtotalMinor: 1_234,
      feeMinor: 149,
      totalMinor: 1_383,
      credits: 12_340,
    })
    expect(quoteCustomPurchase("7250", "NGN", 1200, 145_000)).toEqual({
      subtotalMinor: 725_000,
      feeMinor: 87_000,
      totalMinor: 812_000,
      credits: 5_000,
    })
  })

  it("rejects invalid custom deposit amounts", () => {
    expect(quoteCustomPurchase("0", "USD", 1200, 145_000)).toBeNull()
    expect(quoteCustomPurchase("1.001", "USD", 1200, 145_000)).toBeNull()
    expect(quoteCustomPurchase("0.01", "NGN", 1200, 145_000)).toBeNull()
  })
})
