"use client"

import { useState } from "react"
import { Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useTheme } from "next-themes"
import { usePreset } from "@/lib/theme/preset-provider"
import { THEME_PRESETS } from "@/lib/theme/presets"
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
        >
          <IconSegmented
            options={THEME_MODES}
            value={theme ?? "light"}
            onChange={setTheme}
          />
        </SettingRow>
        <SettingRow
          title="Theme preset"
          description="Pick the color palette used across Hivy"
          last
        >
          <ThemeSelect />
        </SettingRow>
      </section>
    </div>
  )
}

function ThemeSelect() {
  const [open, setOpen] = useState(false)
  const { preset, setPreset } = usePreset()
  const current =
    THEME_PRESETS.find((entry) => entry.id === preset) ?? THEME_PRESETS[0]

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`Theme preset: ${current.label}`}
        className="flex items-center gap-2 rounded-xl border border-border bg-field-background px-3 py-1.5 text-sm transition-colors hover:bg-default"
      >
        <AaSwatch swatch={current.swatch} />
        {current.label}
        <AppIcon icon="chevron-down" className="h-3.5 w-3.5 text-muted" />
      </Popover.Trigger>
      <Popover.Content className="w-56 border border-border p-1.5">
        <Popover.Dialog className="flex max-h-72 w-full flex-col gap-0.5 overflow-y-auto p-0">
          {THEME_PRESETS.map((option) => (
            <button
              key={option.id}
              type="button"
              onClick={() => {
                setPreset(option.id)
                setOpen(false)
              }}
              className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
            >
              <AaSwatch swatch={option.swatch} />
              <span className="min-w-0 flex-1">{option.label}</span>
              {option.id === preset ? (
                <AppIcon icon="check" className="h-4 w-4 shrink-0" />
              ) : null}
            </button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function AaSwatch({ swatch }: { swatch: { accent: string; bg: string } }) {
  return (
    <span
      className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md text-[10px] font-semibold ring-1 ring-inset ring-black/10"
      style={{ backgroundColor: swatch.bg, color: swatch.accent }}
    >
      Aa
    </span>
  )
}
