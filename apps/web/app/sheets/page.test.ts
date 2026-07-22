import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import SheetsPage, { metadata } from "./page"

describe("SheetsPage", () => {
  it("renders a focused Sheets product journey", () => {
    const html = renderToString(React.createElement(SheetsPage))

    expect(html).toContain(
      "Give Hivy agents a database they can read and write."
    )
    expect(html).toContain(
      "A shared Hivy Sheet with account records, placeholder"
    )
    expect(html).toContain("Give agents records they can act on.")
    expect(html).toContain("Every session starts from the latest records.")
    expect(html).toContain("Renewal review")
    expect(html).toContain('aria-label="Search rows"')
    expect(html).toContain('aria-label="Filter rows"')
    expect(html).toContain('aria-label="Sort rows"')
    expect(html).toContain('aria-label="Saved views"')
    expect(html).toContain('aria-label="Sheet pages"')
    expect(html).toContain('aria-label="Change status for Northline Foods"')
    expect(html).toContain('aria-label="Refresh records"')
    expect(html).toContain('aria-live="polite"')
    expect(html).toContain('data-testid="sheet-agent-update-grid"')
    expect(html).toContain("lg:grid-cols-[1fr_2fr]")
    expect(html).toContain("Added 3 account records")
    expect(html).toContain(
      "Give your first agent a database it can come back to."
    )
    expect(html).toContain("Watch a 2min demo")
    expect(html).not.toContain(
      "max-w-[1300px] flex-col items-center justify-center border-t border-border text-center"
    )
    expect(html).toContain("State between sessions")
    expect(html).not.toContain("Give every value a type.")
    expect(html).not.toContain("Find the exact rows before changing anything.")
    expect(html).not.toContain("The next session opens the same records.")
    expect(html).not.toContain(
      "Turn the Sheet into the interface the job needs."
    )
    expect(html).toContain("marketing-link-scope")
    expect(html.match(/>Start for free</g)).toHaveLength(3)
    expect(html).not.toContain("Create free workspace")
    expect(html).toContain('href="/auth/signup"')
    expect(html).not.toContain('href="/docs/sheets-and-apps/sheets"')
    expect(html).not.toContain("—")
  })

  it("publishes Sheets metadata", () => {
    expect(metadata.title).toContain("database agents can read and write")
    expect(metadata.description).toContain("people and agents")
  })
})
