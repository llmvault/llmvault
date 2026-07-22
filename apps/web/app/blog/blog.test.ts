import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import BlogPage from "./page"

describe("blog page", () => {
  it("renders the production blog index", () => {
    const html = renderToString(React.createElement(BlogPage))

    expect(html).toContain(">Blog</h1>")
    expect(html).toContain(
      "Practical notes on building, operating, and improving AI agents for real teams."
    )
    expect(html).toContain("The right model for every agent")
    expect(html).toContain("Latest posts")
    expect(html).toContain("Put the ideas to work")
    expect(html).toContain("marketing-link-scope")
    expect(html).not.toContain("Choose a blog direction")
    expect(html).not.toContain("/blog/variant-")
    expect(html).toContain('href="/blog/right-model-right-job"')
  })

  it("uses HeroUI tabs for filtering by topic", () => {
    const html = renderToString(React.createElement(BlogPage))

    expect(html).toContain('aria-label="Browse Hivy blog posts by category"')
    expect(html).toContain('data-slot="tabs"')
    expect(html).toContain("All posts")
    expect(html).toContain("Agents")
    expect(html).not.toContain("Workflows")
  })
})
