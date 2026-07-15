import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

describe("DocsMediaPlaceholder", () => {
  it("renders a theme-aware screenshot with an annotation", () => {
    const html = renderToString(
      React.createElement(DocsMediaPlaceholder, {
        type: "image",
        title: "A catalog agent’s team installation screen",
        description: "Capture the catalog with the category filter open.",
      })
    )

    expect(html).toContain("/docs/captures/agent-catalog-installation-light.png")
    expect(html).toContain("/docs/captures/agent-catalog-installation-dark.png")
    expect(html).toContain("A catalog agent’s team installation screen")
    expect(html).toContain("Live Hivy capture, available in light and dark themes")
    expect(html).toContain("<button")
    expect(html).toContain("Open A catalog agent’s team installation screen in a lightbox")
  })

  it("bleeds images but keeps videos inside the reading column", () => {
    const imageHtml = renderToString(
      React.createElement(DocsMediaPlaceholder, {
        type: "image",
        title: "Image",
        description: "Capture this state.",
      })
    )
    const videoHtml = renderToString(
      React.createElement(DocsMediaPlaceholder, {
        type: "video",
        title: "Video",
        description: "Record this flow.",
      })
    )

    expect(imageHtml).toContain("lg:w-[calc(100%+6rem)]")
    expect(videoHtml).not.toContain("lg:w-[calc(100%+6rem)]")
  })
})
