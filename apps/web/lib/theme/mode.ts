export const THEME_MODES = ["light", "dark", "system"] as const

export type ThemeMode = (typeof THEME_MODES)[number]

export function themeMode(theme: string | undefined): ThemeMode {
  return THEME_MODES.includes(theme as ThemeMode)
    ? (theme as ThemeMode)
    : "system"
}

export function nextThemeMode(theme: string | undefined): ThemeMode {
  const current = themeMode(theme)
  const index = THEME_MODES.indexOf(current)
  return THEME_MODES[(index + 1) % THEME_MODES.length]
}

export function displayedThemeMode(
  theme: string | undefined,
  mounted: boolean
): ThemeMode {
  return mounted ? themeMode(theme) : "system"
}
