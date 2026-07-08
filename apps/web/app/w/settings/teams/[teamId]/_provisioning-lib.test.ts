import { describe, expect, it } from "vitest"
import { enabledIdSet, isProvisioned, nextEnabledSet } from "./_provisioning-lib"

describe("enabledIdSet", () => {
  it("collects truthy ids from a list of rows", () => {
    const set = enabledIdSet([{ id: "a" }, { id: "b" }, { id: "a" }])
    expect(set).toEqual(new Set(["a", "b"]))
  })

  it("skips rows with undefined or empty ids", () => {
    const set = enabledIdSet([{ id: undefined }, { id: "" }, { id: "c" }])
    expect(set).toEqual(new Set(["c"]))
  })

  it("returns an empty set when given undefined", () => {
    expect(enabledIdSet(undefined)).toEqual(new Set())
  })

  it("returns an empty set for an empty list", () => {
    expect(enabledIdSet([])).toEqual(new Set())
  })
})

describe("isProvisioned", () => {
  it("returns true when the id is in the enabled set", () => {
    expect(isProvisioned("a", new Set(["a", "b"]))).toBe(true)
  })

  it("returns false when the id is not in the enabled set", () => {
    expect(isProvisioned("z", new Set(["a", "b"]))).toBe(false)
  })

  it("returns false when the id is undefined", () => {
    expect(isProvisioned(undefined, new Set(["a"]))).toBe(false)
  })
})

describe("nextEnabledSet", () => {
  it("adds the id when turning on", () => {
    const current = new Set(["a"])
    const next = nextEnabledSet(current, "b", true)
    expect(next).toEqual(new Set(["a", "b"]))
  })

  it("removes the id when turning off", () => {
    const current = new Set(["a", "b"])
    const next = nextEnabledSet(current, "b", false)
    expect(next).toEqual(new Set(["a"]))
  })

  it("does not mutate the input set", () => {
    const current = new Set(["a"])
    nextEnabledSet(current, "b", true)
    expect(current).toEqual(new Set(["a"]))
  })

  it("is a no-op when adding an id that is already present", () => {
    const current = new Set(["a"])
    const next = nextEnabledSet(current, "a", true)
    expect(next).toEqual(new Set(["a"]))
    expect(next).not.toBe(current)
  })

  it("is a no-op when removing an id that is not present", () => {
    const current = new Set(["a"])
    const next = nextEnabledSet(current, "z", false)
    expect(next).toEqual(new Set(["a"]))
  })
})
