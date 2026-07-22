import { readFileSync } from "node:fs"
import { describe, expect, it } from "vitest"

const brandCss = readFileSync(
  new URL("../../app/hero.css", import.meta.url),
  "utf8"
)

describe("Command Center brand theme", () => {
  it("is the only named brand palette", () => {
    expect(brandCss).toContain("Command Center")
    expect(brandCss).not.toContain("data-theme-preset")
  })

  it("keeps the permanent primary identical across modes", () => {
    expect(
      brandCss.match(/--command-center-primary:\s*#f05100;/g)
    ).toHaveLength(1)
    const darkBlock = brandCss.match(
      /\.dark,\s*\[data-theme="dark"\]\s*\{([\s\S]*?)\}/
    )?.[1]
    expect(darkBlock).toBeDefined()
    expect(darkBlock).not.toContain("--accent:")
    expect(darkBlock).not.toContain("--accent-foreground:")
  })

  it("keeps fields and selected tabs visible in dark mode", () => {
    expect(brandCss).toMatch(
      /\.dark,\s*\[data-theme="dark"\]\s*\{[\s\S]*?--field-background:\s*var\(--surface-secondary\)/
    )
    expect(brandCss).toMatch(
      /\.dark,\s*\[data-theme="dark"\]\s*\{[\s\S]*?--segment:\s*color-mix\(/
    )
  })
})
