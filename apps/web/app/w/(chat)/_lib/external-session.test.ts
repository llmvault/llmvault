import { describe, expect, it } from "vitest"
import {
  externalProviderLabel,
  externalSessionContinuation,
} from "@/app/w/(chat)/_lib/external-session"

describe("external session continuation", () => {
  it("builds a Slack thread link from the source resource key", () => {
    const continuation = externalSessionContinuation({
      source: "external",
      sourceResourceKey:
        "slack:33c95935-9db7-4b14-aaab-681902ba5522:T0B57ABFZD5:C0B5KCLRSF7:1782596616.040029",
    })

    expect(continuation).toEqual({
      provider: "slack",
      providerLabel: "Slack",
      url: "https://app.slack.com/client/T0B57ABFZD5/C0B5KCLRSF7/p1782596616040029",
    })
  })

  it("does not return a continuation for web sessions", () => {
    expect(
      externalSessionContinuation({
        source: "web",
        sourceResourceKey: "slack:conn:T:C:123.456",
      })
    ).toBeNull()
  })

  it("labels future providers without requiring URL support", () => {
    expect(externalProviderLabel("microsoft-teams")).toBe("Microsoft Teams")
  })
})
