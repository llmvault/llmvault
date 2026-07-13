export const SIDEBAR_THEME_MODES = ["light", "dark", "system"] as const

export type SidebarThemeMode = (typeof SIDEBAR_THEME_MODES)[number]

export function sidebarThemeMode(theme: string | undefined): SidebarThemeMode {
  return SIDEBAR_THEME_MODES.includes(theme as SidebarThemeMode)
    ? (theme as SidebarThemeMode)
    : "system"
}

export function nextSidebarTheme(theme: string | undefined): SidebarThemeMode {
  const current = sidebarThemeMode(theme)
  const index = SIDEBAR_THEME_MODES.indexOf(current)
  return SIDEBAR_THEME_MODES[(index + 1) % SIDEBAR_THEME_MODES.length]
}
