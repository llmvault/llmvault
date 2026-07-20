import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import DrivePage from "./page"

describe("DrivePage", () => {
  it("renders a focused Drive landing page", () => {
    const html = renderToString(React.createElement(DrivePage))

    expect(html).toContain("One file store for every Hivy agent.")
    expect(html).toContain("Keep the work beyond the sandbox.")
    expect(html).toContain("Agent Drive with searchable files, placeholder")
    expect(html).toContain("Keep every file an agent needs in one place.")
    expect(html).toContain(
      "Agents find the exact file, use it, and keep the result."
    )
    expect(html).toContain("Agent outputs saved beside them")
    expect(html).toContain("Give your first agent a file it can keep using.")
    expect(html).toContain('aria-label="Choose which files to show"')
    expect(html).toContain("accounts-at-risk.csv")
    expect(html).toContain("drive_search")
    expect(html).toContain("drive_download")
    expect(html).not.toContain("Put the evidence beside the request.")
    expect(html).not.toContain("When the answer is a file, keep the file.")
    expect(html).not.toContain("An image arrives with more than a filename.")
    expect(html).not.toContain("Move the file when its job changes.")
    expect(html).toContain('href="/auth/signup"')
    expect(html).toContain('href="#file-flow"')
    expect(html).toContain("marketing-link-scope")
    expect(html).toContain("Create free workspace")
    expect(html).not.toContain("Read the Drive guide")
  })
})
