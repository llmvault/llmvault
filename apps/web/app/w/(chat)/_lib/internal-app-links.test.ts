import { describe, expect, it } from "vitest"
import {
  internalAppLinkFromURL,
  internalAppLinkTargets,
  isInternalAppURL,
} from "@/app/w/(chat)/_lib/internal-app-links"

const agentUrl =
  "https://usehivy.com/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73"
const agentHref = "/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73"

describe("internal app links", () => {
  it("strips our origin and titles the edit-agent link", () => {
    expect(internalAppLinkFromURL(agentUrl)).toEqual({
      key: agentHref,
      href: agentHref,
      label: "Edit agent details",
      icon: "lucide:bot",
    })
  })

  it("extracts unique targets from assistant text and dedupes", () => {
    const targets = internalAppLinkTargets(
      `Here it is [the agent](${agentUrl}) and again ${agentUrl}.`
    )
    expect(targets.map((t) => t.href)).toEqual([agentHref])
  })

  it("only cards curated agent-facing links, not every /w/ URL", () => {
    // A mix: the templated edit-agent link + non-templated app links.
    const targets = internalAppLinkTargets(
      `${agentUrl} https://usehivy.com/w/plugins/github https://usehivy.com/w/settings/general`
    )
    expect(targets.map((t) => t.href)).toEqual([agentHref])
  })

  it("matches only app hosts and only templated paths", () => {
    // Non-app host
    expect(isInternalAppURL(`https://example.com${agentHref}`)).toBe(false)
    // App host but no template for this path
    expect(isInternalAppURL("https://usehivy.com/w/plugins/github")).toBe(false)
    expect(isInternalAppURL("https://usehivy.com/pricing")).toBe(false)
    // App host + templated edit-agent path
    expect(isInternalAppURL(agentUrl)).toBe(true)
  })

  it("ignores non-hivy links entirely", () => {
    expect(
      internalAppLinkTargets("see https://github.com/foo/bar for details")
    ).toEqual([])
  })
})
