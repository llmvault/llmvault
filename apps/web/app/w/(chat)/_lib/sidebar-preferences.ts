"use client"

const SIDEBAR_DEFAULT_WIDTH = 300
export const SIDEBAR_MIN_OPEN_WIDTH = 230
export const SIDEBAR_MAX_WIDTH = 420
export const SIDEBAR_COLLAPSED_THRESHOLD = 8
export const SIDEBAR_PREFERENCES_DEBOUNCE_MS = 200

const SIDEBAR_PREFERENCES_VERSION = 1
const SIDEBAR_PREFERENCES_STORAGE_PREFIX = "hivy-sidebar-preferences"
const ANONYMOUS_USER_ID = "anonymous"

export interface SidebarPreferences {
  open: boolean
  width: number
}

interface SidebarPreferencesScope {
  userId?: string | null
}

export type SidebarPreferencesStorage = Pick<Storage, "getItem" | "setItem">

export const DEFAULT_SIDEBAR_PREFERENCES: SidebarPreferences = {
  open: true,
  width: SIDEBAR_DEFAULT_WIDTH,
}

export function clampSidebarWidth(width: number): number {
  if (!Number.isFinite(width)) return SIDEBAR_DEFAULT_WIDTH
  return Math.min(
    SIDEBAR_MAX_WIDTH,
    Math.max(SIDEBAR_MIN_OPEN_WIDTH, Math.round(width))
  )
}

export function sidebarPreferencesStorageKey(
  scope: SidebarPreferencesScope
): string {
  const userId = scope.userId?.trim() || ANONYMOUS_USER_ID
  return `${SIDEBAR_PREFERENCES_STORAGE_PREFIX}:v${SIDEBAR_PREFERENCES_VERSION}:${encodeURIComponent(userId)}`
}

function normalizeSidebarPreferences(
  preferences: SidebarPreferences
): SidebarPreferences {
  return {
    open: preferences.open,
    width: clampSidebarWidth(preferences.width),
  }
}

export function parseStoredSidebarPreferences(
  stored: string | null
): SidebarPreferences | null {
  if (!stored) return null
  try {
    const value: unknown = JSON.parse(stored)
    if (!isRecord(value)) return null
    if (value.version !== SIDEBAR_PREFERENCES_VERSION) return null
    if (typeof value.open !== "boolean") return null
    if (typeof value.width !== "number" || !Number.isFinite(value.width)) {
      return null
    }
    return normalizeSidebarPreferences({
      open: value.open,
      width: value.width,
    })
  } catch {
    return null
  }
}

export function loadSidebarPreferences(
  scope: SidebarPreferencesScope,
  storage = browserStorage()
): SidebarPreferences {
  if (!storage) return DEFAULT_SIDEBAR_PREFERENCES
  try {
    return (
      parseStoredSidebarPreferences(
        storage.getItem(sidebarPreferencesStorageKey(scope))
      ) ?? DEFAULT_SIDEBAR_PREFERENCES
    )
  } catch {
    return DEFAULT_SIDEBAR_PREFERENCES
  }
}

export function saveSidebarPreferences(
  scope: SidebarPreferencesScope,
  preferences: SidebarPreferences,
  storage = browserStorage()
) {
  if (!storage) return
  const normalized = normalizeSidebarPreferences(preferences)
  try {
    storage.setItem(
      sidebarPreferencesStorageKey(scope),
      JSON.stringify({
        version: SIDEBAR_PREFERENCES_VERSION,
        open: normalized.open,
        width: normalized.width,
      })
    )
  } catch {
    // localStorage can be unavailable in private mode; the in-memory state still works.
  }
}

function browserStorage(): SidebarPreferencesStorage | null {
  if (typeof window === "undefined") return null
  return window.localStorage
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}
