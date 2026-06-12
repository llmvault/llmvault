// Theme presets are pure palettes: each one overrides the HeroUI design
// tokens (light + dark) in app/hero.css under a `[data-theme-preset]`
// selector. The id here must match the CSS block; "default" means no
// attribute (the base Hivy palette). `swatch` drives the "Aa" preview in
// the picker (accent text on a tint of the theme's light surface).
export interface ThemePreset {
  id: string
  label: string
  swatch: { accent: string; bg: string }
}

export const THEME_PRESETS: ThemePreset[] = [
  { id: "default", label: "Default", swatch: { accent: "#3b82f6", bg: "#eef2f7" } },
  { id: "catppuccin", label: "Catppuccin", swatch: { accent: "#1e66f5", bg: "#e6e9ef" } },
  { id: "dracula", label: "Dracula", swatch: { accent: "#7c4dff", bg: "#f0eefe" } },
  { id: "everforest", label: "Everforest", swatch: { accent: "#8da101", bg: "#f4f0d9" } },
  { id: "github", label: "GitHub", swatch: { accent: "#0969da", bg: "#eaeef2" } },
  { id: "gruvbox", label: "Gruvbox", swatch: { accent: "#af3a03", bg: "#f2e5bc" } },
  { id: "linear", label: "Linear", swatch: { accent: "#5e6ad2", bg: "#f7f8f8" } },
  { id: "nord", label: "Nord", swatch: { accent: "#5e81ac", bg: "#e5e9f0" } },
  { id: "one", label: "One", swatch: { accent: "#4078f2", bg: "#f0f0f0" } },
  { id: "rose-pine", label: "Rosé Pine", swatch: { accent: "#b4637a", bg: "#faf4ed" } },
  { id: "solarized", label: "Solarized", swatch: { accent: "#b58900", bg: "#fdf6e3" } },
  { id: "tokyo-night", label: "Tokyo Night", swatch: { accent: "#2e7de9", bg: "#e1e2e7" } },
  { id: "vercel", label: "Vercel", swatch: { accent: "#000000", bg: "#fafafa" } },
]

export const DEFAULT_PRESET = "default"
export const PRESET_STORAGE_KEY = "hivy-theme-preset"

export function isThemePreset(value: string | null | undefined): boolean {
  return THEME_PRESETS.some((preset) => preset.id === value)
}
