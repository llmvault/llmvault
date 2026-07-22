import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { OAuthButtons } from "./shared"

describe("OAuthButtons", () => {
  it("renders the supported providers as branded buttons", () => {
    const html = renderToString(React.createElement(OAuthButtons))

    expect(html).toContain("Continue with Google")
    expect(html).toContain('aria-label="google"')
    expect(html).toContain("Continue with GitHub")
    expect(html).toContain('aria-label="github"')
    expect(html.match(/justify-center gap-3/g)).toHaveLength(2)
    expect(html.match(/-mt-2.5/g)).toHaveLength(2)
    expect(html).not.toContain("absolute left-4")
    expect(html).not.toContain("Continue with X")
    expect(html).not.toContain("/oauth/x")
  })
})
