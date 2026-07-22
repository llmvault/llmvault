import { describe, expect, it } from "vitest"

import { AGENT_SANDBOX_SIZE_OPTIONS, sandboxSizeOptionsForTier } from "./_lib"

describe("agent sandbox tier options", () => {
  it("keeps every size visible and disables sizes above the permanent org tier", () => {
    expect(sandboxSizeOptionsForTier(1)).toEqual([
      expect.objectContaining({ key: "nano", isDisabled: false }),
      expect.objectContaining({
        key: "small",
        isDisabled: true,
        disabledReason: "Tier 2 required",
      }),
      expect.objectContaining({
        key: "medium",
        isDisabled: true,
        disabledReason: "Tier 3 required",
      }),
      expect.objectContaining({
        key: "large",
        isDisabled: true,
        disabledReason: "Tier 4 required",
      }),
    ])
    expect(
      sandboxSizeOptionsForTier(4).map(({ key, isDisabled }) => ({
        key,
        isDisabled,
      }))
    ).toEqual([
      { key: "nano", isDisabled: false },
      { key: "small", isDisabled: false },
      { key: "medium", isDisabled: false },
      { key: "large", isDisabled: false },
    ])
  })

  it("treats missing tier data as tier 1", () => {
    expect(
      sandboxSizeOptionsForTier(undefined)
        .filter(({ isDisabled }) => !isDisabled)
        .map(({ key }) => key)
    ).toEqual(["nano"])
    expect(sandboxSizeOptionsForTier(undefined).map(({ key }) => key)).toEqual([
      "nano",
      "small",
      "medium",
      "large",
    ])
  })

  it("does not expose xlarge at any tier", () => {
    expect(
      AGENT_SANDBOX_SIZE_OPTIONS.map(({ key }) => String(key))
    ).not.toContain("xlarge")
  })
})
