"use client"

import { useTheme } from "next-themes"
import { IconSegmented, SettingRow } from "../_components/controls"

const THEME_MODES = [
  { id: "light", label: "Light", icon: "sun" },
  { id: "dark", label: "Dark", icon: "moon" },
  { id: "system", label: "System", icon: "monitor" },
]

export default function AppearanceSettingsPage() {
  const { theme, setTheme } = useTheme()

  return (
    <div className="flex flex-col gap-10">
      <h1 className="text-2xl font-semibold">Appearance</h1>

      <section className="rounded-2xl border border-border bg-surface">
        <SettingRow
          title="Theme"
          description="Use light, dark, or match your system"
          last
        >
          <IconSegmented
            options={THEME_MODES}
            value={theme ?? "light"}
            onChange={setTheme}
          />
        </SettingRow>
      </section>
    </div>
  )
}
