"use client";

/**
 * Spike: bridges the app's OKLCH CSS-variable design tokens (defined in
 * app/hero.css, switched via next-themes `.dark` class + `[data-theme-preset]`)
 * onto Glide Data Grid's canvas Theme object.
 *
 * Canvas 2D accepts `oklch(...)` and `color-mix(...)` strings directly in all
 * modern browsers, so we pass the resolved token values through untouched.
 */

import { useEffect, useState } from "react";
import type { Theme } from "@glideapps/glide-data-grid";

function readToken(
  styles: CSSStyleDeclaration,
  token: string,
  fallback: string,
): string {
  const value = styles.getPropertyValue(token).trim();
  return value.length > 0 ? value : fallback;
}

/** Derive a translucent tint from a token without needing to parse OKLCH. */
function tint(color: string, percent: number): string {
  return `color-mix(in oklab, ${color} ${percent}%, transparent)`;
}

export function resolveGlideTheme(): Partial<Theme> {
  const styles = getComputedStyle(document.documentElement);

  const background = readToken(styles, "--background", "#ffffff");
  const surface = readToken(styles, "--surface", background);
  const foreground = readToken(styles, "--foreground", "#111111");
  const muted = readToken(styles, "--muted", foreground);
  const accent = readToken(styles, "--accent", "#4f5dff");
  const accentForeground = readToken(styles, "--accent-foreground", surface);
  const border = readToken(styles, "--border", muted);
  const fieldBackground = readToken(styles, "--field-background", surface);

  return {
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
    accentLight: tint(accent, 15),
    bgSearchResult: tint(accent, 25),
    linkColor: accent,

    // Bubbles / drilldown
    bgBubble: fieldBackground,
    bgBubbleSelected: accent,
    drilldownBorder: border,

    // Borders
    borderColor: border,
    horizontalBorderColor: border,
  };
}

/**
 * Resolves the Glide theme from the current design tokens and re-resolves
 * whenever the theme changes at runtime (next-themes toggling the `dark`
 * class, or the `data-theme-preset` attribute switching presets).
 *
 * Client-only: returns `undefined` until mounted (safe under SSR/prerender).
 */
export function useGlideTheme(): Partial<Theme> | undefined {
  const [theme, setTheme] = useState<Partial<Theme> | undefined>(undefined);

  useEffect(() => {
    setTheme(resolveGlideTheme());

    const observer = new MutationObserver(() => {
      setTheme(resolveGlideTheme());
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class", "data-theme-preset", "style"],
    });

    return () => observer.disconnect();
  }, []);

  return theme;
}
