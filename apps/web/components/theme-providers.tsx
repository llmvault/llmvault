"use client"

import { ThemeProvider } from "next-themes"
import { PresetProvider } from "@/lib/theme/preset-provider"

// App-wide theming: next-themes owns the light/dark/system mode (toggling
// the `.dark` class HeroUI reads), PresetProvider owns the named palette
// via a `data-theme-preset` attribute. Default mode is light so existing
// pages render unchanged until a user opts into dark/system.
export function ThemeProviders({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="light"
      enableSystem
      disableTransitionOnChange
    >
      <PresetProvider>{children}</PresetProvider>
    </ThemeProvider>
  )
}
