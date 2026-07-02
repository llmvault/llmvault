import { describe, expect, it } from "vitest"
import {
  BROWSER_BASH_COMMAND_PATTERNS,
  detectBrowserBashCommand,
} from "@/app/w/(chat)/_lib/browser-bash-commands"

describe("detectBrowserBashCommand", () => {
  it("covers the common browser command surface", () => {
    expect(BROWSER_BASH_COMMAND_PATTERNS.length).toBeGreaterThanOrEqual(20)
  })

  it("detects browser commands nested inside shell scripts", () => {
    expect(
      detectBrowserBashCommand(`
        if [[ -n "$URL" ]]; then
          browser --session docs open "$URL"
        fi
      `)
    ).toMatchObject({
      id: "open",
      icon: "chrome",
      actionIcon: "external-link",
      label: "Opened Chrome browser",
      runningLabel: "Opening Chrome browser",
    })
  })

  it("labels screenshot and snapshot commands distinctly", () => {
    expect(
      detectBrowserBashCommand("browser screenshot --full /tmp/page.png")
    ).toMatchObject({
      id: "screenshot",
      label: "Took a screenshot",
      actionIcon: "camera",
    })

    expect(detectBrowserBashCommand("browser snapshot -i")).toMatchObject({
      id: "snapshot",
      label: "Took a page snapshot",
      actionIcon: "scan-search",
    })
  })

  it("detects browser install commands", () => {
    expect(detectBrowserBashCommand("browser install")).toMatchObject({
      id: "install",
      label: "Installed Chrome browser",
      runningLabel: "Installing Chrome browser",
      actionIcon: "download",
    })
  })

  it("uses semantic locator actions for find commands", () => {
    expect(
      detectBrowserBashCommand('browser find text "Sign In" click --exact')
    ).toMatchObject({
      id: "semantic_locator_action",
      label: "Clicked matching text",
      runningLabel: "Clicking matching text",
    })

    expect(
      detectBrowserBashCommand('browser find label "Email" fill "foo@test.com"')
    ).toMatchObject({
      id: "semantic_locator_action",
      label: "Filled matching label",
      runningLabel: "Filling matching label",
      actionIcon: "text-cursor-input",
    })
  })

  it("detects piped JavaScript evaluation", () => {
    expect(
      detectBrowserBashCommand("cat script.js | browser eval --stdin")
    ).toMatchObject({
      id: "eval",
      label: "Ran JavaScript in the page",
      actionIcon: "braces",
    })
  })

  it("ignores non-browser commands", () => {
    expect(detectBrowserBashCommand("echo browserless")).toBeUndefined()
  })
})
