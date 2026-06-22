import { describe, expect, it } from "vitest"
import {
  DEFAULT_SIDEBAR_PREFERENCES,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_OPEN_WIDTH,
  clampSidebarWidth,
  loadSidebarPreferences,
  parseStoredSidebarPreferences,
  saveSidebarPreferences,
  sidebarPreferencesStorageKey,
  type SidebarPreferencesStorage,
} from "@/app/w/(chat)/_lib/sidebar-preferences"

function memoryStorage(): SidebarPreferencesStorage & {
  values: Map<string, string>
} {
  const values = new Map<string, string>()
  return {
    values,
    getItem(key) {
      return values.get(key) ?? null
    },
    setItem(key, value) {
      values.set(key, value)
    },
  }
}

describe("sidebar preferences", () => {
  it("clamps persisted widths to the sidebar range", () => {
    expect(clampSidebarWidth(10)).toBe(SIDEBAR_MIN_OPEN_WIDTH)
    expect(clampSidebarWidth(999)).toBe(SIDEBAR_MAX_WIDTH)
    expect(clampSidebarWidth(321.6)).toBe(322)
  })

  it("parses stored preferences", () => {
    expect(
      parseStoredSidebarPreferences(
        JSON.stringify({ version: 1, open: false, width: 360 })
      )
    ).toEqual({ open: false, width: 360 })
  })

  it("ignores missing or invalid stored preferences", () => {
    expect(parseStoredSidebarPreferences(null)).toBeNull()
    expect(parseStoredSidebarPreferences("{")).toBeNull()
    expect(
      parseStoredSidebarPreferences(
        JSON.stringify({ version: 1, open: false, width: "360" })
      )
    ).toBeNull()
    expect(
      parseStoredSidebarPreferences(
        JSON.stringify({ version: 2, open: false, width: 360 })
      )
    ).toBeNull()
  })

  it("loads and saves preferences per user", () => {
    const storage = memoryStorage()

    saveSidebarPreferences(
      { userId: "user:1" },
      { open: false, width: 999 },
      storage
    )

    expect(loadSidebarPreferences({ userId: "user:1" }, storage)).toEqual({
      open: false,
      width: SIDEBAR_MAX_WIDTH,
    })
    expect(loadSidebarPreferences({ userId: "user:2" }, storage)).toEqual(
      DEFAULT_SIDEBAR_PREFERENCES
    )
    expect(
      storage.values.has(sidebarPreferencesStorageKey({ userId: "user:1" }))
    ).toBe(true)
  })
})
