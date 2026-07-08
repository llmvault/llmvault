import { describe, expect, it } from "vitest"
import {
  assignableRoles,
  canChangeRole,
  canDeleteOrg,
  canRemoveMember,
  canTransferOwnership,
  ownerCount,
} from "./member-actions"

describe("assignableRoles", () => {
  it("gives owners every tier up to and including owner", () => {
    expect(assignableRoles("owner")).toEqual(["owner", "admin", "member"])
  })

  it("gives admins admin and member, never owner", () => {
    expect(assignableRoles("admin")).toEqual(["admin", "member"])
  })

  it("gives non-admins nothing", () => {
    expect(assignableRoles("member")).toEqual([])
    expect(assignableRoles(null)).toEqual([])
    expect(assignableRoles(undefined)).toEqual([])
  })
})

describe("canChangeRole", () => {
  it("blocks changing your own role even as owner", () => {
    expect(
      canChangeRole({
        actorRole: "owner",
        targetRole: "member",
        isSelf: true,
        ownerCount: 1,
      })
    ).toBe(false)
  })

  it("blocks a non-admin actor", () => {
    expect(
      canChangeRole({
        actorRole: "member",
        targetRole: "member",
        isSelf: false,
        ownerCount: 1,
      })
    ).toBe(false)
  })

  it("blocks an admin from touching an owner's role", () => {
    expect(
      canChangeRole({
        actorRole: "admin",
        targetRole: "owner",
        isSelf: false,
        ownerCount: 2,
      })
    ).toBe(false)
  })

  it("allows an owner to demote another owner when there is more than one owner", () => {
    expect(
      canChangeRole({
        actorRole: "owner",
        targetRole: "owner",
        isSelf: false,
        ownerCount: 2,
      })
    ).toBe(true)
  })

  it("blocks an owner from demoting the last remaining owner", () => {
    expect(
      canChangeRole({
        actorRole: "owner",
        targetRole: "owner",
        isSelf: false,
        ownerCount: 1,
      })
    ).toBe(false)
  })

  it("allows an admin to change a member's role", () => {
    expect(
      canChangeRole({
        actorRole: "admin",
        targetRole: "member",
        isSelf: false,
        ownerCount: 1,
      })
    ).toBe(true)
  })
})

describe("canRemoveMember", () => {
  it("blocks removing yourself", () => {
    expect(
      canRemoveMember({
        actorRole: "owner",
        targetRole: "member",
        isSelf: true,
        ownerCount: 1,
      })
    ).toBe(false)
  })

  it("blocks a non-admin actor from removing anyone", () => {
    expect(
      canRemoveMember({
        actorRole: "member",
        targetRole: "member",
        isSelf: false,
        ownerCount: 1,
      })
    ).toBe(false)
  })

  it("blocks an admin from removing an owner", () => {
    expect(
      canRemoveMember({
        actorRole: "admin",
        targetRole: "owner",
        isSelf: false,
        ownerCount: 2,
      })
    ).toBe(false)
  })

  it("allows an owner to remove another owner when there is more than one owner", () => {
    expect(
      canRemoveMember({
        actorRole: "owner",
        targetRole: "owner",
        isSelf: false,
        ownerCount: 2,
      })
    ).toBe(true)
  })

  it("blocks removing the last remaining owner", () => {
    expect(
      canRemoveMember({
        actorRole: "owner",
        targetRole: "owner",
        isSelf: false,
        ownerCount: 1,
      })
    ).toBe(false)
  })

  it("allows an admin to remove a plain member", () => {
    expect(
      canRemoveMember({
        actorRole: "admin",
        targetRole: "member",
        isSelf: false,
        ownerCount: 1,
      })
    ).toBe(true)
  })
})

describe("canTransferOwnership", () => {
  it("allows an owner to transfer to someone else", () => {
    expect(canTransferOwnership({ actorRole: "owner", isSelf: false })).toBe(
      true
    )
  })

  it("blocks transferring to yourself", () => {
    expect(canTransferOwnership({ actorRole: "owner", isSelf: true })).toBe(
      false
    )
  })

  it("blocks non-owners from transferring ownership", () => {
    expect(canTransferOwnership({ actorRole: "admin", isSelf: false })).toBe(
      false
    )
  })
})

describe("canDeleteOrg", () => {
  it("allows only owners to delete the org", () => {
    expect(canDeleteOrg("owner")).toBe(true)
    expect(canDeleteOrg("admin")).toBe(false)
    expect(canDeleteOrg("member")).toBe(false)
    expect(canDeleteOrg(null)).toBe(false)
    expect(canDeleteOrg(undefined)).toBe(false)
  })
})

describe("ownerCount", () => {
  it("counts members with the owner role", () => {
    expect(
      ownerCount([
        { role: "owner" },
        { role: "admin" },
        { role: "owner" },
        { role: "member" },
      ])
    ).toBe(2)
  })

  it("returns zero when there are no owners", () => {
    expect(ownerCount([{ role: "admin" }, { role: "member" }])).toBe(0)
  })

  it("returns zero for an empty list", () => {
    expect(ownerCount([])).toBe(0)
  })
})
