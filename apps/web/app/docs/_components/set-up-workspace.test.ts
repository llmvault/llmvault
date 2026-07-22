import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { SetUpWorkspace } from "./set-up-workspace"

describe("SetUpWorkspace", () => {
  it("matches the guided first-run flow", () => {
    const html = renderToString(React.createElement(SetUpWorkspace))

    expect(html).toContain("Finish setup in three steps")
    expect(html).toContain("Create your first team")
    expect(html).toContain("Connect one account to continue")
    expect(html).toContain("Start your first chat")
    expect(html).toContain("1,000 credits")
    expect(html).toContain("/docs/run-your-first-agent")
    expect(html).not.toContain("<img")
  })
})
