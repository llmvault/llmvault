import { describe, expect, it } from "vitest"
import { nextSidebarTheme, sidebarThemeMode } from "./sidebar-theme"

describe("sidebar theme toggle", () => {
  it("cycles through light, dark, and system modes", () => {
    expect(nextSidebarTheme("light")).toBe("dark")
    expect(nextSidebarTheme("dark")).toBe("system")
    expect(nextSidebarTheme("system")).toBe("light")
  })

  it("uses system when the stored theme is unavailable", () => {
    expect(sidebarThemeMode(undefined)).toBe("system")
  })
})
