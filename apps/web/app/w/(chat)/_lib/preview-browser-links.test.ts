import { describe, expect, it } from "vitest"
import {
  isPreviewBrowserURL,
  previewBrowserTargetFromURL,
  previewBrowserTargets,
} from "@/app/w/(chat)/_lib/preview-browser-links"

const previewUrl = "https://5173-um7j159u.preview.usehivy.com/"

describe("preview browser links", () => {
  it("extracts unique Hivy preview targets from assistant text", () => {
    const targets = previewBrowserTargets(
      `Open [the preview](${previewUrl}) and again ${previewUrl}.`
    )

    expect(targets).toEqual([
      {
        key: previewUrl,
        url: previewUrl,
        host: "5173-um7j159u.preview.usehivy.com",
        port: "5173",
      },
    ])
  })

  it("keeps separate preview paths distinct", () => {
    expect(
      previewBrowserTargets(
        `${previewUrl} https://5173-um7j159u.preview.usehivy.com/settings`
      ).map((target) => target.url)
    ).toEqual([
      previewUrl,
      "https://5173-um7j159u.preview.usehivy.com/settings",
    ])
  })

  it("rejects non-preview and non-HTTPS URLs", () => {
    expect(
      isPreviewBrowserURL("http://5173-um7j159u.preview.usehivy.com/")
    ).toBe(false)
    expect(isPreviewBrowserURL("https://preview.usehivy.com/")).toBe(false)
    expect(isPreviewBrowserURL("https://evilpreview.usehivy.com/")).toBe(false)
    expect(isPreviewBrowserURL("https://example.com/")).toBe(false)
  })

  it("returns one target for a direct URL", () => {
    expect(previewBrowserTargetFromURL(previewUrl)).toMatchObject({
      url: previewUrl,
      host: "5173-um7j159u.preview.usehivy.com",
    })
  })
})
