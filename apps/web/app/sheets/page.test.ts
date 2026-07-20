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
    expect(html).toContain("Added 3 account records")
    expect(html).toContain(
      "Give your first agent a database it can come back to."
    )
    expect(html).toContain("State between sessions")
    expect(html).not.toContain("Give every value a type.")
    expect(html).not.toContain("Find the exact rows before changing anything.")
    expect(html).not.toContain("The next session opens the same records.")
    expect(html).not.toContain(
      "Turn the Sheet into the interface the job needs."
    )
    expect(html).toContain("marketing-link-scope")
    expect(html).toContain('href="/auth/signup"')
    expect(html).not.toContain('href="/docs/sheets-and-apps/sheets"')
    expect(html).not.toContain("—")
  })

  it("publishes Sheets metadata", () => {
    expect(metadata.title).toContain("database agents can read and write")
    expect(metadata.description).toContain("people and agents")
  })
})
