"use client"

import { useState } from "react"
import { Popover, Slider } from "@heroui/react"
import { Icon } from "@iconify/react"
import { PatchDiff } from "@pierre/diffs/react"
import { useTheme } from "next-themes"
import { usePreset } from "@/lib/theme/preset-provider"
import { THEME_PRESETS } from "@/lib/theme/presets"
import { HIVY_DIFF_STYLE, hivyDiffOptions } from "@/lib/diffs-theme"
import {
  ControlledSwitch,
  IconSegmented,
  NumberField,
  PlainSwitch,
  Segmented,
  SettingRow,
} from "../_components/controls"

const THEME_MODES = [
  { id: "light", label: "Light", icon: "lucide:sun" },
  { id: "dark", label: "Dark", icon: "lucide:moon" },
  { id: "system", label: "System", icon: "lucide:monitor" },
]

const THEME_PREVIEW_PATCH = [
  "diff --git a/theme.ts b/theme.ts",
  "index 1111111..2222222 100644",
  "--- a/theme.ts",
  "+++ b/theme.ts",
  "@@ -1,5 +1,5 @@",
  " const themePreview: ThemeConfig = {",
  '-  surface: "sidebar",',
  '-  accent: "#2563eb",',
  "-  contrast: 42,",
  '+  surface: "sidebar-elevated",',
  '+  accent: "#0ea5e9",',
  "+  contrast: 68,",
  " };",
  "",
].join("\n")

const THEME_PREVIEW_DIFF_OPTIONS = hivyDiffOptions({
  diffStyle: "unified",
  overflow: "scroll",
})

export default function AppearanceSettingsPage() {
  const { theme, setTheme } = useTheme()
  const [pointerCursors, setPointerCursors] = useState(false)
  const [reduceMotion, setReduceMotion] = useState("System")
  const [uiFontSize, setUiFontSize] = useState(14)
  const [codeFontSize, setCodeFontSize] = useState(12)
  const [diffMarkers, setDiffMarkers] = useState("Color")

  return (
    <div className="flex flex-col gap-10">
      <h1 className="text-2xl font-semibold">Appearance</h1>

      <section className="overflow-hidden rounded-2xl border border-border bg-surface">
        <div className="flex items-center gap-4 border-b border-border px-4 py-3.5">
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="text-sm font-medium">Theme</span>
            <span className="text-sm text-muted">
              Use light, dark, or match your system
            </span>
          </div>
          <IconSegmented
            options={THEME_MODES}
            value={theme ?? "light"}
            onChange={setTheme}
          />
        </div>
        <div className="p-3">
          <PatchDiff
            patch={THEME_PREVIEW_PATCH}
            options={THEME_PREVIEW_DIFF_OPTIONS}
            style={HIVY_DIFF_STYLE}
            disableWorkerPool
          />
        </div>
      </section>

      <ThemeEditor
        title="Light theme"
        accent="#2563EB"
        background="#FFFFFF"
        foreground="#0A0A0A"
        uiFont="-apple-system, BlinkMacSystemFont"
        codeFont="Geist Mono"
        translucent={false}
        contrast={42}
      />

      <ThemeEditor
        title="Dark theme"
        accent="#339CFF"
        background="#181818"
        foreground="#FFFFFF"
        uiFont="-apple-system, BlinkMacSystemFont"
        codeFont="Geist Mono"
        translucent
        contrast={60}
      />

      <section className="rounded-2xl border border-border bg-surface">
        <SettingRow
          title="Use pointer cursors"
          description="Change the cursor to a pointer when hovering over interactive elements"
        >
          <ControlledSwitch
            selected={pointerCursors}
            onChange={setPointerCursors}
          />
        </SettingRow>
        <SettingRow
          title="Reduce motion"
          description="Reduce animations or match your system"
        >
          <Segmented
            options={["System", "On", "Off"]}
            value={reduceMotion}
            onChange={setReduceMotion}
          />
        </SettingRow>
        <SettingRow
          title="UI font size"
          description="Adjust the base size used for the Hivy UI"
        >
          <NumberField value={uiFontSize} onChange={setUiFontSize} unit="px" />
        </SettingRow>
        <SettingRow
          title="Code font size"
          description="Adjust the base size used for code across chats and diffs"
        >
          <NumberField
            value={codeFontSize}
            onChange={setCodeFontSize}
            unit="px"
          />
        </SettingRow>
        <SettingRow
          title="Diff markers"
          description="How added and removed lines are marked across chats and diffs"
          last
        >
          <Segmented
            options={["Color", "+/-"]}
            value={diffMarkers}
            onChange={setDiffMarkers}
          />
        </SettingRow>
      </section>
    </div>
  )
}

function ThemeEditor({
  title,
  accent,
  background,
  foreground,
  uiFont,
  codeFont,
  translucent,
  contrast,
}: {
  title: string
  accent: string
  background: string
  foreground: string
  uiFont: string
  codeFont: string
  translucent: boolean
  contrast: number
}) {
  const [contrastValue, setContrastValue] = useState(contrast)

  return (
    <section className="rounded-2xl border border-border bg-surface">
      <div className="flex items-center gap-3 border-b border-border px-4 py-3">
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {title}
        </span>
        <button
          type="button"
          className="text-sm text-muted transition-colors hover:text-foreground"
        >
          Import
        </button>
        <button
          type="button"
          className="text-sm text-muted transition-colors hover:text-foreground"
        >
          Copy theme
        </button>
        <ThemeSelect />
      </div>

      <ColorRow label="Accent" color={accent} />
      <ColorRow label="Background" color={background} />
      <ColorRow label="Foreground" color={foreground} />
      <FontRow label="UI font" value={uiFont} />
      <FontRow label="Code font" value={codeFont} />
      <div className="flex items-center gap-4 border-b border-border px-4 py-3">
        <span className="min-w-0 flex-1 text-sm">Translucent sidebar</span>
        <PlainSwitch defaultSelected={translucent} />
      </div>
      <div className="flex items-center gap-4 px-4 py-3">
        <span className="min-w-0 flex-1 text-sm">Contrast</span>
        <Slider
          aria-label="Contrast"
          value={contrastValue}
          onChange={(value) =>
            setContrastValue(Array.isArray(value) ? value[0] : value)
          }
          minValue={0}
          maxValue={100}
          className="w-40"
        >
          <Slider.Track>
            <Slider.Fill />
            <Slider.Thumb />
          </Slider.Track>
        </Slider>
        <span className="w-7 text-right text-sm tabular-nums">
          {contrastValue}
        </span>
      </div>
    </section>
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
        <Icon icon="lucide:chevron-down" className="h-3.5 w-3.5 text-muted" />
      </Popover.Trigger>
      <Popover.Content className="w-56 rounded-2xl border border-border p-1.5">
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
                <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
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

function ColorRow({ label, color }: { label: string; color: string }) {
  return (
    <div className="flex items-center gap-4 border-b border-border px-4 py-3">
      <span className="min-w-0 flex-1 text-sm">{label}</span>
      <button
        type="button"
        aria-label={`${label} color ${color}`}
        className="flex items-center gap-2 rounded-lg px-3 py-1 font-mono text-xs ring-1 ring-inset ring-black/10"
        style={{ backgroundColor: color, color: contrastText(color) }}
      >
        {color.toUpperCase()}
      </button>
    </div>
  )
}

function FontRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center gap-4 border-b border-border px-4 py-3">
      <span className="min-w-0 flex-1 text-sm">{label}</span>
      <div className="max-w-48 truncate rounded-lg border border-border bg-field-background px-3 py-1.5 text-sm text-muted">
        {value}
      </div>
    </div>
  )
}

// Picks readable text for a color swatch from its hex luminance.
function contrastText(hex: string) {
  const value = hex.replace("#", "")
  if (value.length !== 6) return "#000000"
  const r = parseInt(value.slice(0, 2), 16)
  const g = parseInt(value.slice(2, 4), 16)
  const b = parseInt(value.slice(4, 6), 16)
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255
  return luminance > 0.6 ? "#0A0A0A" : "#FFFFFF"
}
