import { describe, expect, it } from "vitest"
import { teamGrantDiff } from "./_team-grants"

describe("teamGrantDiff", () => {
  it("returns only grants when teams are added", () => {
    expect(teamGrantDiff(["a"], ["a", "b", "c"])).toEqual({
      grant: ["b", "c"],
      revoke: [],
    })
  })

  it("returns only revokes when teams are removed", () => {
    expect(teamGrantDiff(["a", "b", "c"], ["a"])).toEqual({
      grant: [],
      revoke: ["b", "c"],
    })
  })

  it("returns both grants and revokes when selection changes both ways", () => {
    expect(teamGrantDiff(["a", "b"], ["b", "c"])).toEqual({
      grant: ["c"],
      revoke: ["a"],
    })
  })

  it("returns no changes when selection matches current", () => {
    expect(teamGrantDiff(["a", "b"], ["b", "a"])).toEqual({
      grant: [],
      revoke: [],
    })
  })

  it("dedupes repeated ids in either input", () => {
    expect(teamGrantDiff(["a", "a"], ["a", "b", "b"])).toEqual({
      grant: ["b"],
      revoke: [],
    })
    expect(teamGrantDiff(["a", "a", "b"], ["a"])).toEqual({
      grant: [],
      revoke: ["b"],
    })
  })

  it("handles empty inputs", () => {
    expect(teamGrantDiff([], [])).toEqual({ grant: [], revoke: [] })
    expect(teamGrantDiff([], ["a"])).toEqual({ grant: ["a"], revoke: [] })
    expect(teamGrantDiff(["a"], [])).toEqual({ grant: [], revoke: ["a"] })
  })
})
