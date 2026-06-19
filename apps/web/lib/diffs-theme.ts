"use client"

import { registerCustomCSSVariableTheme } from "@pierre/diffs"
import type { CSSProperties } from "react"

type DiffsCSSProperties = CSSProperties &
  Record<`--${string}`, string | number>

type DiffsOptions = {
  unsafeCSS?: string
  [key: string]: unknown
}

const HIVY_DIFF_THEME_NAME = "hivy-hero"
const HIVY_DIFF_THEME = {
  dark: HIVY_DIFF_THEME_NAME,
  light: HIVY_DIFF_THEME_NAME,
} as const

let hivyDiffThemeRegistered = false

export const HIVY_DIFF_STYLE: DiffsCSSProperties = {
  fontFamily: "var(--font-mono)",
  "--diffs-font-family": "var(--font-mono)",
  "--diffs-font-size": "12px",
  "--diffs-line-height": "20px",
}

export function hivyDiffOptions<const TOptions extends DiffsOptions>(
  options: TOptions
) {
  ensureHivyDiffTheme()
  const { unsafeCSS, ...rest } = options
  return {
    ...rest,
    theme: HIVY_DIFF_THEME,
    themeType: "system" as const,
    diffIndicators: "classic" as const,
    unsafeCSS: [HIVY_DIFF_UNSAFE_CSS, unsafeCSS].filter(Boolean).join("\n"),
  }
}

function ensureHivyDiffTheme() {
  if (hivyDiffThemeRegistered) return
  registerCustomCSSVariableTheme(
    HIVY_DIFF_THEME_NAME,
    {
      foreground: "var(--surface-foreground)",
      background: "var(--surface)",
      "token-comment": "var(--muted)",
      "token-constant": "var(--warning)",
      "token-deleted": "var(--danger)",
      "token-function": "var(--accent)",
      "token-inserted": "var(--success)",
      "token-keyword": "var(--accent)",
      "token-link": "var(--accent)",
      "token-parameter": "var(--surface-foreground)",
      "token-punctuation": "var(--muted)",
      "token-string": "var(--success)",
      "token-string-expression": "var(--warning)",
      "token-changed": "var(--warning)",
      "ansi-black": "var(--surface-secondary)",
      "ansi-red": "var(--danger)",
      "ansi-green": "var(--success)",
      "ansi-yellow": "var(--warning)",
      "ansi-blue": "var(--accent)",
      "ansi-magenta": "var(--accent)",
      "ansi-cyan": "var(--success)",
      "ansi-white": "var(--surface-foreground)",
      "ansi-bright-black": "var(--muted)",
      "ansi-bright-red": "var(--danger)",
      "ansi-bright-green": "var(--success)",
      "ansi-bright-yellow": "var(--warning)",
      "ansi-bright-blue": "var(--accent)",
      "ansi-bright-magenta": "var(--accent)",
      "ansi-bright-cyan": "var(--success)",
      "ansi-bright-white": "var(--foreground)",
    },
    true
  )
  hivyDiffThemeRegistered = true
}

const HIVY_DIFF_UNSAFE_CSS = `
  :host {
    display: block;
    min-width: 0;
    overflow: hidden;
    color: var(--surface-foreground);
    background: var(--surface);
    --diffs-foreground: var(--surface-foreground);
    --diffs-background: var(--surface);
    --diffs-font-family: var(--font-mono);
    --diffs-header-font-family: var(--font-sans);
    --diffs-gap-inline: 12px;
    --diffs-gap-block: 8px;
    --diffs-gap-style: 1px solid var(--border);
    --diffs-tab-size: 2;
    --diffs-bg-buffer-override: var(--surface-secondary);
    --diffs-bg-context-override: var(--surface);
    --diffs-bg-hover-override: var(--surface-secondary);
    --diffs-bg-separator-override: var(--surface-secondary);
    --diffs-fg-number-override: var(--muted);
    --diffs-fg-number-addition-override: var(--success);
    --diffs-fg-number-deletion-override: var(--danger);
    --diffs-fg-conflict-marker-override: var(--muted);
    --diffs-addition-color-override: var(--success);
    --diffs-deletion-color-override: var(--danger);
    --diffs-modified-color-override: var(--accent);
    --diffs-bg-addition-override:
      color-mix(in oklch, var(--success) 11%, var(--surface));
    --diffs-bg-addition-number-override:
      color-mix(in oklch, var(--success) 15%, var(--surface-secondary));
    --diffs-bg-addition-hover-override:
      color-mix(in oklch, var(--success) 17%, var(--surface));
    --diffs-bg-addition-emphasis-override:
      color-mix(in oklch, var(--success) 22%, transparent);
    --diffs-bg-deletion-override:
      color-mix(in oklch, var(--danger) 10%, var(--surface));
    --diffs-bg-deletion-number-override:
      color-mix(in oklch, var(--danger) 14%, var(--surface-secondary));
    --diffs-bg-deletion-hover-override:
      color-mix(in oklch, var(--danger) 16%, var(--surface));
    --diffs-bg-deletion-emphasis-override:
      color-mix(in oklch, var(--danger) 20%, transparent);
    --diffs-selection-color-override: var(--accent);
    --diffs-bg-selection-override:
      color-mix(in oklch, var(--accent) 14%, var(--surface));
    --diffs-bg-selection-number-override:
      color-mix(in oklch, var(--accent) 18%, var(--surface-secondary));
    --diffs-bg-selection-background-override:
      color-mix(in oklch, var(--accent) 18%, var(--surface));
    --diffs-bg-selection-number-background-override:
      color-mix(in oklch, var(--accent) 22%, var(--surface-secondary));
  }

  pre,
  code,
  [data-error-wrapper] {
    background: var(--surface);
    color: var(--surface-foreground);
  }

  [data-code] {
    background: var(--surface);
  }

  [data-code]::-webkit-scrollbar-thumb {
    background-color: var(--scrollbar);
    border-color: transparent;
  }

  [data-diffs-header='default'] {
    min-height: 36px;
    padding-inline: 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    color: var(--surface-foreground);
    font-size: 12px;
    font-weight: 500;
    line-height: 18px;
  }

  [data-header-content],
  [data-title],
  [data-metadata] {
    min-width: 0;
  }

  [data-title],
  [data-prev-name] {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  [data-metadata],
  [data-deletions-count],
  [data-additions-count] {
    color: var(--muted);
    font-size: 11px;
    font-weight: 500;
  }

  [data-gutter] {
    background: var(--surface-secondary);
  }

  [data-gutter] [data-gutter-buffer],
  [data-gutter] [data-column-number] {
    border-right: 1px solid var(--border);
  }

  [data-column-number] {
    min-width: 5ch;
    padding-inline: 1ch;
    background: var(--surface-secondary);
    color: var(--muted);
  }

  [data-line] {
    color: var(--surface-foreground);
  }

  [data-line],
  [data-column-number],
  [data-no-newline] {
    transition:
      background-color 140ms ease,
      color 140ms ease;
  }

  [data-diff-type='split'][data-overflow='scroll'] [data-additions] {
    border-left: 1px solid var(--border);
  }

  [data-diff-type='split'][data-overflow='scroll'] [data-deletions] {
    border-right: 0;
  }

  [data-separator='line-info'],
  [data-separator='line-info-basic'],
  [data-separator='metadata'],
  [data-separator='simple'] {
    background: var(--surface-secondary);
    color: var(--muted);
  }

  [data-separator='line-info'],
  [data-separator='line-info-basic'],
  [data-separator='metadata'] {
    height: 28px;
    border-block: 1px solid var(--border);
  }

  [data-separator-wrapper] {
    background: var(--surface-secondary);
    color: var(--muted);
    font-size: 11px;
    font-weight: 500;
  }

  [data-diff-span] {
    border-radius: 4px;
    padding-inline: 1px;
  }

  [data-no-newline] {
    color: var(--muted);
  }

  [data-error-wrapper] {
    background: var(--surface);
    color: var(--surface-foreground);
  }

  [data-error-message] {
    color: var(--danger);
  }

  [data-error-stack] {
    color: var(--muted);
  }
`
