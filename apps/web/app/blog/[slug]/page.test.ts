import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import BlogArticlePage, { generateMetadata, generateStaticParams } from "./page"

describe("blog article page", () => {
  it("renders a complete article", async () => {
    const page = await BlogArticlePage({
      params: Promise.resolve({ slug: "right-model-right-job" }),
    })
    const html = renderToString(React.createElement(React.Fragment, null, page))

    expect(html).toContain("Back to blog")
    expect(html).toContain(
      "The right model for every agent is rarely the biggest one"
    )
    expect(html).toContain("The short version")
    expect(html).toContain("Bigger models hide bad job design")
    expect(html).toContain("Give each agent a budget")
    expect(html).toContain("Start for free")
    expect(html).toContain("marketing-link-scope")
  })

  it("generates metadata and routes for every sample post", async () => {
    const metadata = await generateMetadata({
      params: Promise.resolve({ slug: "right-model-right-job" }),
    })

    expect(metadata.title).toBe(
      "The right model for every agent is rarely the biggest one"
    )
    expect(generateStaticParams()).toHaveLength(10)
  })
})
