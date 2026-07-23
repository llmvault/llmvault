import { describe, expect, it } from "vitest"
import {
  filterByPurchaseCurrency,
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
})
