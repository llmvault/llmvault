import { describe, expect, it } from "vitest"

import { AGENT_SANDBOX_SIZE_OPTIONS, sandboxSizeOptions } from "./_lib"

describe("agent sandbox pricing options", () => {
  it("makes every size available and prices allocated vCPU time", () => {
    expect(
      sandboxSizeOptions().map(({ key, isDisabled, price }) => ({
        key,
        isDisabled,
        price,
      }))
    ).toEqual([
      { key: "nano", isDisabled: false, price: "1 credit/active min" },
      { key: "small", isDisabled: false, price: "1 credit/active min" },
      { key: "medium", isDisabled: false, price: "2 credits/active min" },
      { key: "large", isDisabled: false, price: "4 credits/active min" },
    ])
  })

  it("does not expose xlarge at any tier", () => {
    expect(
      AGENT_SANDBOX_SIZE_OPTIONS.map(({ key }) => String(key))
    ).not.toContain("xlarge")
  })
})
