"use client"

import { useState } from "react"
import Link from "next/link"
import { Button, Popover, Slider, Switch } from "@heroui/react"
import { Icon } from "@iconify/react"
import { PatchDiff } from "@pierre/diffs/react"
import { useTheme } from "next-themes"
import { usePreset } from "@/lib/theme/preset-provider"
import { THEME_PRESETS } from "@/lib/theme/presets"

interface SettingsNavItem {
  id: string
  label: string
  icon: string
}

interface SettingsNavSection {
  label: string
  items: SettingsNavItem[]
}

const NAV_SECTIONS: SettingsNavSection[] = [
  {
    label: "Personal",
    items: [
      { id: "general", label: "General", icon: "lucide:settings" },
      { id: "profile", label: "Profile", icon: "lucide:circle-user" },
      { id: "appearance", label: "Appearance", icon: "lucide:sun" },
      { id: "configuration", label: "Configuration", icon: "lucide:sliders-horizontal" },
      { id: "personalization", label: "Personalization", icon: "lucide:smile" },
      { id: "shortcuts", label: "Keyboard shortcuts", icon: "lucide:keyboard" },
      { id: "billing", label: "Usage & billing", icon: "lucide:gauge" },
    ],
  },
  {
    label: "Integrations",
    items: [
      { id: "appshots", label: "Appshots", icon: "lucide:scan" },
      { id: "mcp", label: "MCP servers", icon: "lucide:paperclip" },
      { id: "browser", label: "Browser", icon: "lucide:app-window" },
      { id: "computer-use", label: "Computer use", icon: "lucide:mouse-pointer-click" },
    ],
  },
  {
    label: "Coding",
    items: [
      { id: "hooks", label: "Hooks", icon: "lucide:anchor" },
      { id: "connections", label: "Connections", icon: "lucide:globe" },
      { id: "git", label: "Git", icon: "lucide:git-branch" },
      { id: "environments", label: "Environments", icon: "lucide:monitor" },
      { id: "worktrees", label: "Worktrees", icon: "lucide:git-merge" },
    ],
  },
  {
    label: "Archived",
    items: [
      { id: "archived-chats", label: "Archived chats", icon: "lucide:archive" },
    ],
  },
]

export function SettingsPage() {
  const [activeSection, setActiveSection] = useState("general")
  const [query, setQuery] = useState("")

  const sections = NAV_SECTIONS.map((section) => ({
    ...section,
    items: section.items.filter((item) =>
      item.label.toLowerCase().includes(query.trim().toLowerCase())
    ),
  })).filter((section) => section.items.length > 0)

  const activeLabel =
    NAV_SECTIONS.flatMap((section) => section.items).find(
      (item) => item.id === activeSection
    )?.label ?? "General"

  return (
    <div className="fixed inset-0 flex overflow-hidden bg-background text-foreground">
      <div className="flex w-72 shrink-0 flex-col border-r border-border bg-surface">
        <div className="flex flex-col gap-3 px-4 pb-2 pt-5">
          <Link
            href="/w"
            className="flex items-center gap-2 text-sm text-muted transition-colors hover:text-foreground"
          >
            <Icon icon="lucide:arrow-left" className="h-4 w-4" />
            Back to app
          </Link>
          <div className="flex items-center gap-2 rounded-xl border border-border bg-field-background px-3 py-1.5">
            <Icon icon="lucide:search" className="h-4 w-4 shrink-0 text-muted" />
            <input
              type="text"
              value={query}
              placeholder="Search settings..."
              aria-label="Search settings"
              onChange={(event) => setQuery(event.target.value)}
              className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
            />
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
          {sections.map((section) => (
            <div key={section.label} className="flex flex-col gap-0.5 pt-4">
              <span className="px-3 pb-1 text-xs text-muted select-none">
                {section.label}
              </span>
              {section.items.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => setActiveSection(item.id)}
                  className={`flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors ${
                    item.id === activeSection ? "bg-default" : "hover:bg-default"
                  }`}
                >
                  <Icon icon={item.icon} className="h-4 w-4 shrink-0 text-muted" />
                  <span className="min-w-0 flex-1 truncate">{item.label}</span>
                </button>
              ))}
            </div>
          ))}
          {sections.length === 0 ? (
            <p className="px-3 pt-6 text-sm text-muted">No settings match</p>
          ) : null}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl px-6 py-12">
          {activeSection === "general" ? (
            <GeneralSection />
          ) : activeSection === "appearance" ? (
            <AppearanceSection />
          ) : (
            <PlaceholderSection label={activeLabel} />
          )}
        </div>
      </div>
    </div>
  )
}

function PlaceholderSection({ label }: { label: string }) {
  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold">{label}</h1>
      <div className="rounded-2xl border border-border bg-surface px-5 py-8 text-center">
        <p className="text-sm text-muted">
          This section isn&apos;t part of the static exploration yet — General
          shows the full pattern.
        </p>
      </div>
    </div>
  )
}

function GeneralSection() {
  const [workMode, setWorkMode] = useState<"coding" | "everyday">("coding")
  const [terminalLocation, setTerminalLocation] = useState<"Bottom" | "Right">("Bottom")
  const [codeReview, setCodeReview] = useState<"Inline" | "Detached">("Inline")

  return (
    <div className="flex flex-col gap-10">
      <h1 className="text-2xl font-semibold">General</h1>

      <section className="flex flex-col gap-3">
        <div>
          <h2 className="text-sm font-medium">Work mode</h2>
          <p className="text-sm text-muted">
            Choose how much technical detail Hivy shows
          </p>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <WorkModeCard
            icon="lucide:square-code"
            title="For coding"
            description="More technical responses and control"
            selected={workMode === "coding"}
            onSelect={() => setWorkMode("coding")}
          />
          <WorkModeCard
            icon="lucide:briefcase"
            title="For everyday work"
            description="Same power, less technical detail"
            selected={workMode === "everyday"}
            onSelect={() => setWorkMode("everyday")}
          />
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">Permissions</h2>
        <div className="rounded-2xl border border-border bg-surface">
          <ToggleRow
            title="Default permissions"
            description="By default, Hivy can read and edit files in its workspace. It can ask for additional access when needed"
            defaultSelected
          />
          <ToggleRow
            title="Auto-review"
            description="Hivy can read and edit files in its workspace. Hivy automatically reviews requests for additional access. Auto-review can make mistakes."
            learnMore
            defaultSelected
          />
          <ToggleRow
            title="Full access"
            description="When Hivy runs with full access, it can edit any file on your computer and run commands with network, without your approval. This significantly increases the risk of data loss, leaks, or unexpected behavior."
            learnMore
            defaultSelected
            last
          />
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">General</h2>
        <div className="rounded-2xl border border-border bg-surface">
          <SettingRow
            title="Default open destination"
            description="Where files and folders open by default"
          >
            <SettingSelect
              icon="vscode-icons:file-type-vscode"
              options={["VS Code", "Cursor", "Zed", "Finder"]}
            />
          </SettingRow>
          <SettingRow title="Language" description="Language for the app UI">
            <SettingSelect options={["Auto Detect", "English", "Deutsch", "Français"]} />
          </SettingRow>
          <SettingRow
            title="Show in menu bar"
            description="Keep Hivy in the macOS menu bar when the main window is closed"
          >
            <PlainSwitch defaultSelected />
          </SettingRow>
          <SettingRow
            title="Bottom panel"
            description="Show the bottom panel control in the app header"
          >
            <PlainSwitch defaultSelected />
          </SettingRow>
          <SettingRow
            title="Default terminal location"
            description="Choose where the terminal shortcut and environment actions open terminal tabs"
          >
            <Segmented
              options={["Bottom", "Right"]}
              value={terminalLocation}
              onChange={(value) => setTerminalLocation(value as "Bottom" | "Right")}
            />
          </SettingRow>
          <SettingRow
            title="Prevent sleep while running"
            description="Keep your computer awake while Hivy is running a chat"
          >
            <PlainSwitch />
          </SettingRow>
          <SettingRow
            title="Speed"
            description="Choose the inference tier used across chats, subagents, and compaction"
          >
            <SettingSelect options={["Standard", "Flex", "Priority"]} />
          </SettingRow>
          <SettingRow
            title="Code review"
            description="Start /review in the current chat when possible or launch a separate review chat"
          >
            <Segmented
              options={["Inline", "Detached"]}
              value={codeReview}
              onChange={(value) => setCodeReview(value as "Inline" | "Detached")}
            />
          </SettingRow>
          <SettingRow
            title="Import work from other AI apps"
            description="Bring over your setup, projects, and recent chats"
            last
          >
            <Button variant="tertiary" size="sm">
              Import
            </Button>
          </SettingRow>
        </div>
      </section>
    </div>
  )
}

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

function AppearanceSection() {
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
            options={{ theme: "pierre-light" }}
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
        codeFont='ui-monospace, "SFMono-Regular"'
        translucent={false}
        contrast={42}
      />

      <ThemeEditor
        title="Dark theme"
        accent="#339CFF"
        background="#181818"
        foreground="#FFFFFF"
        uiFont="-apple-system, BlinkMacSystemFont"
        codeFont='ui-monospace, "SFMono-Regular"'
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

function NumberField({
  value,
  onChange,
  unit,
}: {
  value: number
  onChange: (value: number) => void
  unit: string
}) {
  return (
    <div className="flex items-center gap-1.5">
      <input
        type="number"
        value={value}
        aria-label="Size"
        onChange={(event) => onChange(Number(event.target.value))}
        className="w-16 rounded-lg border border-border bg-field-background px-2 py-1 text-right text-sm outline-none focus:border-accent"
      />
      <span className="text-sm text-muted">{unit}</span>
    </div>
  )
}

function ControlledSwitch({
  selected,
  onChange,
}: {
  selected: boolean
  onChange: (selected: boolean) => void
}) {
  return (
    <Switch
      isSelected={selected}
      onChange={onChange}
      className="shrink-0"
    >
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
    </Switch>
  )
}

function IconSegmented({
  options,
  value,
  onChange,
}: {
  options: { id: string; label: string; icon: string }[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex items-center gap-1">
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          onClick={() => onChange(option.id)}
          className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-sm transition-colors ${
            option.id === value
              ? "bg-default font-medium"
              : "text-muted hover:text-foreground"
          }`}
        >
          <Icon icon={option.icon} className="h-4 w-4" />
          {option.label}
        </button>
      ))}
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

function WorkModeCard({
  icon,
  title,
  description,
  selected,
  onSelect,
}: {
  icon: string
  title: string
  description: string
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex items-center gap-3 rounded-2xl border px-4 py-4 text-left transition-colors ${
        selected ? "border-accent bg-surface" : "border-border bg-surface hover:bg-default"
      }`}
    >
      <Icon icon={icon} className="h-5 w-5 shrink-0 text-muted" />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-sm text-muted">{description}</span>
      </span>
      <span
        className={`flex h-4 w-4 shrink-0 items-center justify-center rounded-full border ${
          selected ? "border-accent" : "border-border"
        }`}
      >
        {selected ? <span className="h-2 w-2 rounded-full bg-accent" /> : null}
      </span>
    </button>
  )
}

function ToggleRow({
  title,
  description,
  learnMore,
  defaultSelected,
  last,
}: {
  title: string
  description: string
  learnMore?: boolean
  defaultSelected?: boolean
  last?: boolean
}) {
  return (
    <div
      className={`flex items-start gap-4 px-4 py-4 ${last ? "" : "border-b border-border"}`}
    >
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-sm leading-6 text-muted">
          {description}{" "}
          {learnMore ? (
            <a href="#" className="text-accent underline-offset-2 hover:underline">
              Learn more
            </a>
          ) : null}
        </span>
      </div>
      <PlainSwitch defaultSelected={defaultSelected} />
    </div>
  )
}

function PlainSwitch({ defaultSelected }: { defaultSelected?: boolean }) {
  return (
    <Switch defaultSelected={defaultSelected} className="shrink-0">
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
    </Switch>
  )
}

function SettingRow({
  title,
  description,
  children,
  last,
}: {
  title: string
  description: string
  children: React.ReactNode
  last?: boolean
}) {
  return (
    <div
      className={`flex items-center gap-4 px-4 py-3.5 ${last ? "" : "border-b border-border"}`}
    >
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-sm text-muted">{description}</span>
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

function SettingSelect({
  icon,
  options,
}: {
  icon?: string
  options: string[]
}) {
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState(options[0])

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`Select ${value}`}
        className="flex items-center gap-2 rounded-xl border border-border bg-field-background px-3 py-1.5 text-sm transition-colors hover:bg-default"
      >
        {icon ? <Icon icon={icon} className="h-4 w-4" /> : null}
        {value}
        <Icon icon="lucide:chevron-down" className="h-3.5 w-3.5 text-muted" />
      </Popover.Trigger>
      <Popover.Content className="w-48 rounded-2xl border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          {options.map((option) => (
            <button
              key={option}
              type="button"
              onClick={() => {
                setValue(option)
                setOpen(false)
              }}
              className="flex items-center gap-2 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
            >
              <span className="min-w-0 flex-1">{option}</span>
              {option === value ? (
                <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
              ) : null}
            </button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function Segmented({
  options,
  value,
  onChange,
}: {
  options: string[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex items-center gap-1">
      {options.map((option) => (
        <button
          key={option}
          type="button"
          onClick={() => onChange(option)}
          className={`rounded-lg px-3 py-1 text-sm transition-colors ${
            option === value
              ? "bg-default font-medium"
              : "text-muted hover:text-foreground"
          }`}
        >
          {option}
        </button>
      ))}
    </div>
  )
}
