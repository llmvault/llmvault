import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { RunFirstAgent } from "./run-first-agent"

describe("RunFirstAgent", () => {
  it("guides a first session without referencing screenshot assets", () => {
    const html = renderToString(React.createElement(RunFirstAgent))

    expect(html).toContain("Choose the agent for the job")
    expect(html).toContain("DeepSeek V4 Flash")
    expect(html).toContain("Ricky - App builder")
    expect(html).toContain("Sheets are channel-scoped databases")
    expect(html).toContain(
      "/docs/captures/model-picker-deepseek-reasoning-light.png"
    )
    expect(html).toContain("Record a 45-second clip at 100% zoom")
    expect(html).not.toContain("run-first-agent-model-picker-light.png")
    expect(html).not.toContain("run-first-agent-sheet-result-light.png")
    expect(html).not.toContain("run-first-agent-app-result-light.png")
  })
})
