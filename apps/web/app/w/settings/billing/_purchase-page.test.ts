import React, { type ReactNode } from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  invalidateQueries: vi.fn(),
  mutate: vi.fn(),
  purchase: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  useQuery: vi.fn(),
}))

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mocks.push,
    replace: mocks.replace,
  }),
}))

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: mocks.invalidateQueries }),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: {
    useMutation: () => ({ isPending: false, mutate: mocks.mutate }),
    useQuery: mocks.useQuery,
  },
}))

vi.mock("@/lib/auth/auth-context", () => ({
  useAuth: () => ({ isLoading: false }),
}))

vi.mock("@/lib/auth/use-role", () => ({
  useIsOwner: () => true,
}))

vi.mock("@/hooks/use-credit-purchase", () => ({
  useCreditPurchase: () => ({
    isPending: false,
    purchase: mocks.purchase,
  }),
}))

vi.mock("@/components/icon", () => ({
  AppIcon: ({ icon }: { icon: string }) =>
    React.createElement("span", { "data-icon": icon }),
}))

vi.mock("@heroui/react", () => {
  const Button = ({
    children,
    isDisabled,
    ...props
  }: {
    children: ReactNode
    isDisabled?: boolean
  }) =>
    React.createElement("button", { ...props, disabled: isDisabled }, children)
  const Input = (props: Record<string, unknown>) =>
    React.createElement("input", props)
  const Skeleton = (props: Record<string, unknown>) =>
    React.createElement("div", props)
  const Spinner = () => React.createElement("span", null, "Loading")
  const Switch = Object.assign(
    ({ children }: { children: ReactNode }) =>
      React.createElement("span", null, children),
    {
      Control: ({ children }: { children: ReactNode }) =>
        React.createElement("span", null, children),
      Thumb: () => React.createElement("span"),
    }
  )

  return {
    Button,
    Input,
    Skeleton,
    Spinner,
    Switch,
    toast: {
      danger: vi.fn(),
      success: vi.fn(),
    },
  }
})

import PurchaseCreditsPageContent from "./_purchase-page"

describe("PurchaseCreditsPageContent", () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.replace.mockReset()
    mocks.useQuery.mockReset()
    mocks.useQuery.mockImplementation((_method: string, path: string) => {
      if (path === "/v1/billing/account") {
        return {
          data: {
            balance: 1_200,
            fee_basis_points: 1_200,
            fx_minor_per_usd: 145_000,
            packs: [
              {
                id: "usd-5",
                currency: "USD",
                credits: 5_000,
                subtotal_minor: 500,
                fee_minor: 60,
                total_minor: 560,
                fee_basis_points: 1_200,
              },
            ],
          },
          isError: false,
          isLoading: false,
          refetch: vi.fn(),
        }
      }
      if (path === "/v1/billing/payment-methods") {
        return {
          data: { payment_methods: [] },
          isError: false,
          isLoading: false,
        }
      }
      return { data: undefined, isError: false, isLoading: false }
    })
  })

  it("renders the complete purchase form as a page instead of a Hivy modal", () => {
    const html = renderToString(
      React.createElement(PurchaseCreditsPageContent)
    ).replaceAll("<!-- -->", "")

    expect(mocks.useQuery.mock.calls).toContainEqual([
      "get",
      "/v1/billing/account",
      {},
      { enabled: true },
    ])
    expect(html).toContain("Purchase credits")
    expect(html).toContain("Powered by")
    expect(html).toContain("Paystack")
    expect(html).toContain('aria-label="Stripe"')
    expect(html).toContain("company")
    expect(html).toContain("Payment currency")
    expect(html).toContain("Purchase amount")
    expect(html).toContain("5,000 credits")
    expect(html).toContain("Payment method")
    expect(html).toContain("New card")
    expect(html).not.toContain("Enter details in Paystack")
    expect(html).not.toContain('data-icon="plus"')
    expect(html).toContain("Order summary")
    expect(html).toContain("Pay $5.60")
    expect(html).toContain('data-icon="shield-check"')
    expect(html).toContain("PCI DSS Level 1 v4.0")
    expect(html).toContain("ISO 27001:2022")
    expect(html).toContain("ISO 27701:2019")
    expect(html).not.toContain('role="dialog"')
  })
})
