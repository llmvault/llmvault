import { afterEach, beforeEach, describe, expect, it } from "vitest"
import {
  internalAppLinkFromURL,
  internalAppLinkTargets,
  isInternalAppURL,
} from "@/app/w/(chat)/_lib/internal-app-links"

const agentUrl =
  "https://usehivy.test/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73"
const agentHref = "/w/settings/agents/edit/f34232d7-a756-4407-9eac-d48d7ec27f73"

// Internal app links are now matched purely by same-origin (window.location.origin),
// not a hardcoded host allowlist. Simulate the app being served from usehivy.test so
// the hosted-style URLs above resolve as same-origin.
const originalWindow = (globalThis as { window?: unknown }).window

beforeEach(() => {
  ;(globalThis as { window?: unknown }).window = {
    location: { origin: "https://usehivy.test" },
  }
})

afterEach(() => {
  if (originalWindow === undefined) {
    delete (globalThis as { window?: unknown }).window
  } else {
    ;(globalThis as { window?: unknown }).window = originalWindow
  }
})

describe("internal app links", () => {
  it("strips our origin and titles the edit-agent link", () => {
    expect(internalAppLinkFromURL(agentUrl)).toEqual({
      key: agentHref,
      href: agentHref,
      label: "Edit agent details",
      icon: "bot",
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
      `${agentUrl} https://usehivy.test/w/plugins/github https://usehivy.test/w/settings/general`
    )
    expect(targets.map((t) => t.href)).toEqual([agentHref])
  })

  it("matches only same-origin URLs and only templated paths", () => {
    // Different origin than the app
    expect(isInternalAppURL(`https://example.com${agentHref}`)).toBe(false)
    // Same origin but no template for this path
    expect(isInternalAppURL("https://usehivy.test/w/plugins/github")).toBe(false)
    expect(isInternalAppURL("https://usehivy.test/pricing")).toBe(false)
    // Same origin + templated edit-agent path
    expect(isInternalAppURL(agentUrl)).toBe(true)
  })

  it("ignores non-hivy links entirely", () => {
    expect(
      internalAppLinkTargets("see https://github.com/foo/bar for details")
    ).toEqual([])
  })
})
