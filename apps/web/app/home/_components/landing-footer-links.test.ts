import { existsSync } from "node:fs"
import { describe, expect, it } from "vitest"
import { footerGroups } from "./landing-footer-links"

describe("marketing footer links", () => {
  it("only lists current product and resource pages", () => {
    expect(footerGroups.map((group) => group.title)).toEqual([
      "Product",
      "Resources",
      "Legal",
    ])

    for (const group of footerGroups) {
      for (const link of group.links) {
        expect(link.href).not.toBe("#")
      }
    }

    for (const group of footerGroups.filter(({ title }) => title !== "Legal")) {
      for (const link of group.links) {
        const route = new URL(
          `../../${link.href.slice(1)}/page.tsx`,
          import.meta.url
        )
        expect(
          existsSync(route),
          `${link.label} should have a public route`
        ).toBe(true)
      }
    }
  })
})
