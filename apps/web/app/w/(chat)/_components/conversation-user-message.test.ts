import { describe, expect, it } from "vitest"
import { automatedMessageBadge } from "@/app/w/(chat)/_components/conversation-user-message"

describe("automatedMessageBadge", () => {
  it("returns null for regular user messages", () => {
    expect(automatedMessageBadge("web")).toBeNull()
    expect(automatedMessageBadge(undefined)).toBeNull()
    expect(automatedMessageBadge("external")).toBeNull()
  })

  it("labels scheduled automated messages", () => {
    expect(automatedMessageBadge("schedule")).toEqual({
      label: "Automated · Schedule",
      icon: "clock",
    })
  })

  it("labels system automated messages", () => {
    expect(automatedMessageBadge("system")).toEqual({
      label: "Automated · System",
      icon: "sparkles",
    })
  })

  it("labels github-provider trigger messages", () => {
    expect(automatedMessageBadge("trigger", "github-app")).toEqual({
      label: "Automated · GitHub",
      icon: "github",
    })
    expect(automatedMessageBadge("trigger", "github-app-code-reviews")?.label).toBe(
      "Automated · GitHub"
    )
  })

  it("prettifies known non-github providers", () => {
    expect(automatedMessageBadge("trigger", "slack")?.label).toBe(
      "Automated · Slack"
    )
    expect(automatedMessageBadge("trigger", "linear")?.label).toBe(
      "Automated · Linear"
    )
  })

  it("falls back to a bare Automated label for unknown providers", () => {
    expect(automatedMessageBadge("trigger", "acme")).toEqual({
      label: "Automated",
      icon: "sparkles",
    })
    expect(automatedMessageBadge("trigger")?.label).toBe("Automated")
  })
})
