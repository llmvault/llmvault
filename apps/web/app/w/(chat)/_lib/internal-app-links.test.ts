import { describe, expect, it } from "vitest"
import {
  internalAppLinkFromURL,
  internalAppLinkTargets,
  isInternalAppURL,
} from "@/app/w/(chat)/_lib/internal-app-links"

const agentUrl =
  "https://usehivy.com/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73"

describe("internal app links", () => {
  it("strips our origin and returns a client-side href + label", () => {
    expect(internalAppLinkFromURL(agentUrl)).toEqual({
      key: "/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73",
      href: "/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73",
      subtitle: "/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73",
      label: "Agent",
      icon: "lucide:bot",
    })
  })

  it("extracts unique targets from assistant text and dedupes", () => {
    const targets = internalAppLinkTargets(
      `Here it is [the agent](${agentUrl}) and again ${agentUrl}.`
    )
    expect(targets.map((t) => t.href)).toEqual([
      "/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73",
    ])
  })

  it("keeps distinct internal paths separate and labels them", () => {
    const targets = internalAppLinkTargets(
      "https://usehivy.com/w/plugins/github and https://www.usehivy.com/w/automations/triggers/new"
    )
    expect(targets.map((t) => ({ href: t.href, label: t.label }))).toEqual([
      { href: "/w/plugins/github", label: "Plugin" },
      { href: "/w/automations/triggers/new", label: "Automation" },
    ])
  })

  it("only matches app hosts and only /w/ paths", () => {
    // Non-app host
    expect(isInternalAppURL("https://example.com/w/plugins")).toBe(false)
    // App host but marketing/home path (not /w/)
    expect(isInternalAppURL("https://usehivy.com/pricing")).toBe(false)
    expect(isInternalAppURL("https://usehivy.com/")).toBe(false)
    // App host + /w/ path
    expect(isInternalAppURL("https://usehivy.com/w/plugins/github")).toBe(true)
  })

  it("ignores non-hivy links entirely", () => {
    expect(
      internalAppLinkTargets("see https://github.com/foo/bar for details")
    ).toEqual([])
  })
})
