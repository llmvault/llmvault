import { describe, expect, it } from "vitest"
import { isAdminRole, isOwnerRole } from "./use-role"

describe("isOwnerRole", () => {
  it("is true only for owner", () => {
    expect(isOwnerRole("owner")).toBe(true)
    expect(isOwnerRole("admin")).toBe(false)
    expect(isOwnerRole("member")).toBe(false)
  })

  it("is false for unknown, empty, null, or undefined roles", () => {
    expect(isOwnerRole("")).toBe(false)
    expect(isOwnerRole("OWNER")).toBe(false)
    expect(isOwnerRole(null)).toBe(false)
    expect(isOwnerRole(undefined)).toBe(false)
  })
})

describe("isAdminRole", () => {
  it("is true for owner and admin", () => {
    expect(isAdminRole("owner")).toBe(true)
    expect(isAdminRole("admin")).toBe(true)
  })

  it("is false for member", () => {
    expect(isAdminRole("member")).toBe(false)
  })

  it("is false for unknown, empty, null, or undefined roles", () => {
    expect(isAdminRole("")).toBe(false)
    expect(isAdminRole("Admin")).toBe(false)
    expect(isAdminRole(null)).toBe(false)
    expect(isAdminRole(undefined)).toBe(false)
  })
})
