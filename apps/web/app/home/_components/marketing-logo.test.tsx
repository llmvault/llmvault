import React from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { MarketingLogo } from "./marketing-logo"

describe("MarketingLogo", () => {
  it("renders the themed Hivy wordmark", () => {
    const html = renderToStaticMarkup(
      React.createElement(MarketingLogo, { className: "h-10" })
    )

    expect(html).toContain('viewBox="15 46.8 99 77"')
    expect(html).toContain('fill="currentColor"')
    expect(html).toContain('aria-hidden="true"')
    expect(html.match(/<path/g)).toHaveLength(3)
  })
})
