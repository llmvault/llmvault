import { describe, expect, it } from "vitest"

import { AGENT_SANDBOX_SIZE_OPTIONS, sandboxSizeOptionsForTier } from "./_lib"

describe("agent sandbox tier options", () => {
  it("unlocks one additional size per permanent org tier", () => {
    expect(sandboxSizeOptionsForTier(1).map(({ key }) => key)).toEqual(["nano"])
    expect(sandboxSizeOptionsForTier(2).map(({ key }) => key)).toEqual([
      "nano",
      "small",
    ])
    expect(sandboxSizeOptionsForTier(4).map(({ key }) => key)).toEqual([
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
