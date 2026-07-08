import { describe, expect, it } from "vitest"
import { buildRegisterBody } from "@/hooks/use-password-auth"

describe("buildRegisterBody", () => {
  it("includes the required team_name, trimmed", () => {
    const body = buildRegisterBody(
      "Ada@Example.com",
      "s3cret",
      "  Acme Engineering  "
    )
    expect(body.team_name).toBe("Acme Engineering")
    expect(body.password).toBe("s3cret")
  })

  it("normalizes the email and derives a display name from it", () => {
    const body = buildRegisterBody("ada.lovelace@example.com", "pw", "Acme")
    expect(body.email).toBe("ada.lovelace@example.com")
    expect(body.name).toBe("Ada Lovelace")
  })

  it("passes an empty team_name through (server enforces the 400)", () => {
    // The server owns the required-team_name rule; we surface its 400 inline
    // rather than silently substituting a value here.
    expect(buildRegisterBody("a@b.com", "pw", "   ").team_name).toBe("")
  })
})
