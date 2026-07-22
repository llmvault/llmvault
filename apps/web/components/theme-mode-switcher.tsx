"use client"

import { useSyncExternalStore } from "react"
import { Button } from "@heroui/react"
import { useTheme } from "next-themes"
import { AppIcon } from "@/components/icon"
import { displayedThemeMode, type ThemeMode } from "@/lib/theme/mode"

const MODES: Array<{ id: ThemeMode; label: string; icon: string }> = [
  { id: "light", label: "Light", icon: "sun" },
  { id: "dark", label: "Dark", icon: "moon" },
  { id: "system", label: "System", icon: "monitor" },
]

const subscribeToHydration = () => () => {}

export function ThemeModeSwitcher() {
  const { theme, setTheme } = useTheme()
  const mounted = useSyncExternalStore(
    subscribeToHydration,
    () => true,
    () => false
  )
  const current = displayedThemeMode(theme, mounted)

  return (
    <div
      role="group"
      aria-label="Color mode"
      className="flex items-center rounded-sm bg-surface-secondary p-0.5"
    >
      {MODES.map((mode) => {
        const selected = current === mode.id
        return (
          <Button
            key={mode.id}
            type="button"
            size="sm"
            variant="ghost"
            isIconOnly
            aria-label={`Use ${mode.label.toLowerCase()} theme`}
            aria-pressed={selected}
            onPress={() => setTheme(mode.id)}
            className={`size-7 min-w-7 rounded-sm ${selected ? "bg-surface text-foreground shadow-xs" : "text-muted"}`}
          >
            <AppIcon icon={mode.icon} size={14} />
          </Button>
        )
      })}
    </div>
  )
}
