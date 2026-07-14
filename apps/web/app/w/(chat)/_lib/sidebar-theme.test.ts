import { describe, expect, it } from "vitest"
import { displayedThemeMode, nextThemeMode, themeMode } from "@/lib/theme/mode"

describe("sidebar theme toggle", () => {
  it("cycles through light, dark, and system modes", () => {
    expect(nextThemeMode("light")).toBe("dark")
    expect(nextThemeMode("dark")).toBe("system")
    expect(nextThemeMode("system")).toBe("light")
  })

  it("uses system when the stored theme is unavailable", () => {
    expect(themeMode(undefined)).toBe("system")
  })

  it("keeps the server and first client render on system mode", () => {
    const initial = displayedThemeMode("dark", false)
    expect(initial).toBe("system")
    expect(nextThemeMode(initial)).toBe("light")
    expect(displayedThemeMode("dark", true)).toBe("dark")
  })
})
