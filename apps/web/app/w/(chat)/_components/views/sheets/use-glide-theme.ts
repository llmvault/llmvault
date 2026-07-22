"use client"

/**
 * Bridges the app's OKLCH CSS-variable design tokens (defined in
 * app/hero.css and switched via next-themes' `.dark` class)
 * onto Glide Data Grid's canvas Theme object.
 *
 * Glide renders cells on a 2D canvas and runs every theme color through its own
 * color pipeline, which does not understand `oklch(...)` / `color-mix(...)`.
 * Unparseable colors collapse to the canvas default (black), which is why the
 * default (OKLCH) theme rendered black cells in light mode. So we resolve each
 * token to a plain `rgba(...)` string first — via a scratch canvas, whose color
 * parser *does* understand these functions — before handing it to Glide.
 */

import { useEffect, useState } from "react"
import type { Theme } from "@glideapps/glide-data-grid"

function readToken(
  styles: CSSStyleDeclaration,
  token: string,
  fallback: string
): string {
  const value = styles.getPropertyValue(token).trim()
  return value.length > 0 ? value : fallback
}

let scratchCtx: CanvasRenderingContext2D | null = null

/**
 * Resolve any CSS color string (`oklch(...)`, `color-mix(...)`, hex, …) to a
 * plain `rgba(r, g, b, a)` value the Glide canvas can paint. Uses a 1x1 scratch
 * canvas in `copy` mode so the painted pixel is exactly the resolved color.
 * Falls back to the input string if resolution isn't possible.
 */
function toCanvasColor(value: string): string {
  if (typeof document === "undefined") return value
  if (!scratchCtx) {
    const canvas = document.createElement("canvas")
    canvas.width = 1
    canvas.height = 1
    scratchCtx = canvas.getContext("2d", { willReadFrequently: true })
  }
  if (!scratchCtx) return value
  try {
    scratchCtx.globalCompositeOperation = "copy"
    scratchCtx.fillStyle = value
    scratchCtx.fillRect(0, 0, 1, 1)
    const [r, g, b, a] = scratchCtx.getImageData(0, 0, 1, 1).data
    return `rgba(${r}, ${g}, ${b}, ${(a / 255).toFixed(3)})`
  } catch {
    return value
  }
}

/** Override the alpha of a resolved `rgb(a)(...)` color. */
function withAlpha(color: string, alpha: number): string {
  const match = color.match(/rgba?\(([^)]+)\)/)
  if (!match) return color
  const [r, g, b] = match[1].split(",").map((part) => part.trim())
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function resolveGlideTheme(): Partial<Theme> {
  const styles = getComputedStyle(document.documentElement)
  const read = (token: string, fallback: string) =>
    toCanvasColor(readToken(styles, token, fallback))

  const background = read("--background", "#ffffff")
  const surface = read("--surface", background)
  const foreground = read("--foreground", "#111111")
  const muted = read("--muted", foreground)
  const accent = read("--accent", "#4f5dff")
  const accentForeground = read("--accent-foreground", surface)
  const border = read("--border", muted)
  const fieldBackground = read("--field-background", surface)
  // The app font (next/font exposes it as this CSS variable on <html>). Glide
  // paints on a canvas, so it needs the family passed explicitly.
  const fontFamily = readToken(styles, "--font-bricolage-sans", "").trim()

  return {
    ...(fontFamily
      ? { fontFamily: `${fontFamily}, ui-sans-serif, system-ui, sans-serif` }
      : {}),

    // Cells
    bgCell: surface,
    bgCellMedium: fieldBackground,
    textDark: foreground,
    textMedium: muted,
    textLight: muted,
    textBubble: foreground,

    // Header
    bgHeader: background,
    bgHeaderHasFocus: fieldBackground,
    bgHeaderHovered: fieldBackground,
    textHeader: muted,
    textHeaderSelected: foreground,
    bgIconHeader: muted,
    fgIconHeader: background,

    // Accent / selection
    accentColor: accent,
    accentFg: accentForeground,
    accentLight: withAlpha(accent, 0.15),
    bgSearchResult: withAlpha(accent, 0.25),
    linkColor: accent,

    // Bubbles / drilldown
    bgBubble: fieldBackground,
    bgBubbleSelected: accent,
    drilldownBorder: border,

    // Borders
    borderColor: border,
    horizontalBorderColor: border,
  }
}

/**
 * Resolves the Glide theme from the current design tokens and re-resolves
 * whenever next-themes toggles the `dark` class at runtime.
 *
 * Client-only: returns `undefined` until mounted (safe under SSR/prerender).
 */
export function useGlideTheme(): Partial<Theme> | undefined {
  const [theme, setTheme] = useState<Partial<Theme> | undefined>(() =>
    typeof document === "undefined" ? undefined : resolveGlideTheme()
  )

  useEffect(() => {
    const observer = new MutationObserver(() => {
      setTheme(resolveGlideTheme())
    })
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class", "style"],
    })

    // Canvas can only paint a font once it has loaded; re-resolve when the app
    // webfont is ready so the grid re-renders with it instead of a fallback.
    document.fonts?.ready.then(() => setTheme(resolveGlideTheme()))

    return () => observer.disconnect()
  }, [])

  return theme
}
