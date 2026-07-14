import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

describe("DocsMediaPlaceholder", () => {
  it("renders actionable media guidance without a broken asset", () => {
    const html = renderToString(
      React.createElement(DocsMediaPlaceholder, {
        type: "image",
        title: "Agent catalog",
        description: "Capture the catalog with the category filter open.",
      })
    )

    expect(html).toContain("Image placeholder")
    expect(html).toContain("Agent catalog")
    expect(html).toContain("Capture the catalog")
    expect(html).not.toContain("<img")
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
