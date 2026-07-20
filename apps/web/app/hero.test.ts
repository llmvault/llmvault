import { readFileSync } from "node:fs"
import { describe, expect, it } from "vitest"

describe("marketing link decoration", () => {
  it("suppresses HeroUI decoration for every interactive link state", () => {
    const css = readFileSync(new URL("./hero.css", import.meta.url), "utf8")

    expect(css).toContain(".marketing-link-scope .link:hover")
    expect(css).toContain('.marketing-link-scope .link[data-hovered="true"]')
    expect(css).toContain(".marketing-link-scope .link:active")
    expect(css).toContain('.marketing-link-scope .link[data-pressed="true"]')
    expect(css).toMatch(
      /\.marketing-link-scope \.link[\s\S]*?text-decoration: none;/
    )
  })
})
