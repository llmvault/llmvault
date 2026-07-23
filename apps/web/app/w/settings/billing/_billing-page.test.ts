import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  onBuyCredits: undefined as (() => void) | undefined,
  push: vi.fn(),
  replace: vi.fn(),
}))

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mocks.push,
    replace: mocks.replace,
  }),
}))

vi.mock("@/lib/auth/auth-context", () => ({
  useAuth: () => ({ isLoading: false }),
}))

vi.mock("@/lib/auth/use-role", () => ({
  useIsAdmin: () => true,
}))

vi.mock("./_components/credits-balance-section", () => ({
  CreditsBalanceSection: ({ onBuyCredits }: { onBuyCredits: () => void }) => {
    mocks.onBuyCredits = onBuyCredits
    return React.createElement("button", null, "Buy credits")
  },
}))

vi.mock("./_components/credits-usage-section", () => ({
  CreditsUsageSection: () => React.createElement("section", null, "Usage"),
}))

vi.mock("./_components/credit-purchases-section", () => ({
  CreditPurchasesSection: () =>
    React.createElement("section", null, "Recent purchases"),
}))

import BillingPageContent from "./_billing-page"

describe("BillingPageContent", () => {
  beforeEach(() => {
    mocks.onBuyCredits = undefined
    mocks.push.mockReset()
    mocks.replace.mockReset()
  })

  it("takes Buy credits to the dedicated purchase page", () => {
    const html = renderToString(React.createElement(BillingPageContent))

    expect(html).toContain("Buy credits")
    expect(mocks.onBuyCredits).toBeTypeOf("function")

    mocks.onBuyCredits?.()

    expect(mocks.push).toHaveBeenCalledWith("/w/billing/purchase")
  })
})
