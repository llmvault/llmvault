"use client"

import { ThemeProvider } from "next-themes"

// App-wide theming: next-themes owns light/dark/system mode. Command Center is
// the permanent brand palette and follows HeroUI's semantic token contract.
export function ThemeProviders({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
    >
      {children}
    </ThemeProvider>
  )
}
