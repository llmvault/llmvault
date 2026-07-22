import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

describe("DocsMediaPlaceholder", () => {
  it("renders a capture brief without an image asset", () => {
    const html = renderToString(
      React.createElement(DocsMediaPlaceholder, {
        type: "image",
        title: "A catalog agent’s team installation screen",
        description: "Capture the catalog with the category filter open.",
      })
    )

    expect(html).toContain("A catalog agent’s team installation screen")
    expect(html).toContain("Capture the catalog with the category filter open.")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/")
    expect(html).not.toContain("<img")
    expect(html).not.toContain("<button")
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
