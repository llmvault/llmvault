import { describe, expect, it } from "vitest"
import {
  labelInitials,
  normalizedWorkspaceName,
  userPrimaryLabel,
  userSecondaryLabel,
} from "./workspace-switcher"

describe("workspace switcher identity", () => {
  it("shows a user's name and email without duplicating an email-only identity", () => {
    const namedUser = { name: "Ada Lovelace", email: "ada@example.com" }
    const emailOnlyUser = { email: "operator@example.com" }

    expect(userPrimaryLabel(namedUser)).toBe("Ada Lovelace")
    expect(userSecondaryLabel(namedUser)).toBe("ada@example.com")
    expect(userPrimaryLabel(emailOnlyUser)).toBe("operator@example.com")
    expect(userSecondaryLabel(emailOnlyUser)).toBeNull()
  })

  it("creates compact initials for user and workspace avatars", () => {
    expect(labelInitials("Ada Lovelace")).toBe("AL")
    expect(labelInitials("Hivy")).toBe("HI")
  })

  it("normalizes a new workspace name before submission", () => {
    expect(normalizedWorkspaceName("  Acme Operations  ")).toBe(
      "Acme Operations"
    )
    expect(normalizedWorkspaceName("   ")).toBeNull()
  })
})
