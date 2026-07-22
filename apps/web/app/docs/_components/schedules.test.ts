import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { Schedules } from "./schedules"

describe("Schedules", () => {
  it("documents cadence, timezone handling, run review, and lifecycle", () => {
    const html = renderToString(React.createElement(Schedules))

    expect(html).toContain("Create a schedule around the result")
    expect(html).toContain("Custom interval")
    expect(html).toContain("Custom cron")
    expect(html).toContain("browser&#x27;s local timezone")
    expect(html).toContain("View session")
    expect(html).toContain("Pause before you delete")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("Image placeholder")
    expect(html).not.toContain("/docs/captures/")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
