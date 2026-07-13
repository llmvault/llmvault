import { describe, expect, it } from "vitest"
import { buildRegisterBody } from "@/hooks/use-password-auth"

describe("buildRegisterBody", () => {
  it("does not collect a team name during registration", () => {
    const body = buildRegisterBody("Ada@Example.com", "s3cret")
    expect(body).not.toHaveProperty("team_name")
    expect(body.password).toBe("s3cret")
  })

  it("normalizes the email and derives a display name from it", () => {
    const body = buildRegisterBody("ada.lovelace@example.com", "pw")
    expect(body.email).toBe("ada.lovelace@example.com")
    expect(body.name).toBe("Ada Lovelace")
  })
})
