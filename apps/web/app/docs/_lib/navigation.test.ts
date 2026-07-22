import { readdirSync, readFileSync } from "node:fs"
import { fileURLToPath } from "node:url"
import { describe, expect, it } from "vitest"
import {
  DOC_PAGES,
  getAdjacentDocPages,
  getDocPage,
  searchDocPages,
} from "./navigation"

describe("documentation navigation", () => {
  it("starts with the Hivy welcome page", () => {
    expect(DOC_PAGES[0]).toMatchObject({
      slug: "welcome-to-hivy",
      title: "Welcome to Hivy",
    })
    expect(getDocPage("what-is-hivy")).toBeUndefined()
  })

  it("links the guided setup to the first agent guide", () => {
    expect(DOC_PAGES[1]).toMatchObject({
      slug: "set-up-your-workspace",
      title: "Set up your workspace",
    })
    expect(DOC_PAGES[2]).toMatchObject({
      slug: "run-your-first-agent",
      title: "Run your first agent",
    })
    expect(getDocPage("your-first-agent-run")).toBeUndefined()
  })

  it("resolves every public documentation slug", () => {
    expect(DOC_PAGES).toHaveLength(28)
    for (const page of DOC_PAGES) {
      expect(getDocPage(page.slug)).toEqual(page)
    }
  })

  it("provides previous and next pages in navigation order", () => {
    const adjacent = getAdjacentDocPages("automations/schedules")
    expect(adjacent.previous?.slug).toBe("automations/event-triggers")
    expect(adjacent.next?.slug).toBe("automations/http-webhooks")
  })

  it("searches titles, descriptions, and section names", () => {
    expect(searchDocPages("Automations")).toHaveLength(5)
    expect(searchDocPages("Knowledge and memory")).toHaveLength(3)
    expect(searchDocPages("   ")).toEqual([])
  })

  it("keeps internal links on registered pages and old captures out of source", () => {
    const componentsDir = fileURLToPath(
      new URL("../_components", import.meta.url)
    )
    const sources = readdirSync(componentsDir)
      .filter((name) => name.endsWith(".tsx"))
      .map((name) => readFileSync(componentsDir + "/" + name, "utf8"))

    for (const source of sources) {
      expect(source).not.toContain("/docs/captures/")
      for (const match of source.matchAll(/href="\/docs\/([^"]+)"/g)) {
        expect(getDocPage(match[1])).toBeDefined()
      }
    }
  })
})
