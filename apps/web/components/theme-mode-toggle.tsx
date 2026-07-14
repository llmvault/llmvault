"use client"

import { useSyncExternalStore } from "react"
import { useTheme } from "next-themes"
import { AppIcon } from "@/components/icon"
import {
  displayedThemeMode,
  nextThemeMode,
  type ThemeMode,
} from "@/lib/theme/mode"

const THEME_ICONS: Record<ThemeMode, string> = {
  light: "sun",
  dark: "moon",
  system: "monitor",
}

const subscribeToHydration = () => () => {}

export function ThemeModeToggle() {
  const { theme, setTheme } = useTheme()
  const mounted = useSyncExternalStore(
    subscribeToHydration,
    () => true,
    () => false
  )

  const current = displayedThemeMode(theme, mounted)
  const next = nextThemeMode(current)
  const currentLabel = capitalize(current)
  const nextLabel = capitalize(next)

  return (
    <button
      type="button"
      aria-label={`Theme: ${currentLabel}. Switch to ${nextLabel}`}
      title={`Theme: ${currentLabel}`}
      onClick={() => setTheme(next)}
      className="focus-visible:ring-ring flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors hover:bg-default focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
    >
      <AppIcon icon={THEME_ICONS[current]} className="h-4 w-4 text-muted" />
    </button>
  )
}

function capitalize(value: string) {
  return value[0].toUpperCase() + value.slice(1)
}
