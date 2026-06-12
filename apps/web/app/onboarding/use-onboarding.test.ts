import { describe, expect, it } from "vitest"
import {
  computeInitialSelectedChannelIds,
  needsConnectionForm,
  type Channel,
  type Integration,
} from "./use-onboarding"

function channel(partial: Partial<Channel>): Channel {
  return { ...partial }
}

describe("computeInitialSelectedChannelIds", () => {
  it("returns the SAME set reference when nothing is selectable (no re-render)", () => {
    // Regression for the onboarding infinite-update loop: while channels are
    // loading there is nothing to select, and the effect must not commit a new
    // Set identity each render. Returning the same reference keeps React from
    // scheduling another passive-effect pass.
    const current = new Set<string>()
    const result = computeInitialSelectedChannelIds([], current)
    expect(result).toBe(current)
  })

  it("returns the same reference when selectable channels exist but lack ids", () => {
    const current = new Set<string>()
    const result = computeInitialSelectedChannelIds(
      [channel({ is_private: false })],
      current
    )
    expect(result).toBe(current)
  })

  it("pre-selects all public channels with ids", () => {
    const current = new Set<string>()
    const result = computeInitialSelectedChannelIds(
      [
        channel({ id: "C1", is_private: false }),
        channel({ id: "C2", is_private: false }),
        channel({ id: "C3", is_private: true }),
        channel({ is_private: false }),
      ],
      current
    )
    expect([...result].sort()).toEqual(["C1", "C2"])
  })

  it("never overwrites an existing non-empty selection", () => {
    const current = new Set<string>(["C9"])
    const result = computeInitialSelectedChannelIds(
      [channel({ id: "C1", is_private: false })],
      current
    )
    expect(result).toBe(current)
    expect([...result]).toEqual(["C9"])
  })

  it("produces a stable selection across repeated calls (idempotent)", () => {
    let selection = new Set<string>()
    const channels = [channel({ id: "C1", is_private: false })]
    selection = computeInitialSelectedChannelIds(channels, selection)
    const first = selection
    // A second invocation (e.g. another render) must not churn the reference.
    selection = computeInitialSelectedChannelIds(channels, selection)
    expect(selection).toBe(first)
  })
})

describe("needsConnectionForm", () => {
  it("requires a form for API_KEY auth", () => {
    const integration = {
      nango_config: { auth_mode: "API_KEY" },
    } as unknown as Integration
    expect(needsConnectionForm(integration)).toBe(true)
  })

  it("does not require a form for plain OAuth without manual config fields", () => {
    const integration = {
      nango_config: { auth_mode: "OAUTH2" },
    } as unknown as Integration
    expect(needsConnectionForm(integration)).toBe(false)
  })
})
