import { beforeEach, describe, expect, it } from "vitest"
import {
  isPreviewBrowserURL,
  previewBrowserTargetFromURL,
  previewBrowserTargets,
} from "@/app/w/(chat)/_lib/preview-browser-links"
import { setPublicConfigForTests } from "@/lib/config/public-config"

beforeEach(() => setPublicConfigForTests())

const previewUrl = "https://5173-um7j159u.preview.usehivy.test/"

describe("preview browser links", () => {
  it("extracts unique Hivy preview targets from assistant text", () => {
    const targets = previewBrowserTargets(
      `Open [the preview](${previewUrl}) and again ${previewUrl}.`
    )

    expect(targets).toEqual([
      {
        key: previewUrl,
        url: previewUrl,
        host: "5173-um7j159u.preview.usehivy.test",
        port: "5173",
      },
    ])
  })

  it("keeps separate preview paths distinct", () => {
    expect(
      previewBrowserTargets(
        `${previewUrl} https://5173-um7j159u.preview.usehivy.test/settings`
      ).map((target) => target.url)
    ).toEqual([
      previewUrl,
      "https://5173-um7j159u.preview.usehivy.test/settings",
    ])
  })

  it("rejects non-preview and non-HTTPS URLs", () => {
    expect(
      isPreviewBrowserURL("http://5173-um7j159u.preview.usehivy.test/")
    ).toBe(false)
    expect(isPreviewBrowserURL("https://preview.usehivy.test/")).toBe(false)
    expect(isPreviewBrowserURL("https://evilpreview.usehivy.test/")).toBe(false)
    expect(isPreviewBrowserURL("https://example.com/")).toBe(false)
  })

  it("returns one target for a direct URL", () => {
    expect(previewBrowserTargetFromURL(previewUrl)).toMatchObject({
      url: previewUrl,
      host: "5173-um7j159u.preview.usehivy.test",
    })
  })

  it("captures the ?app= hint as appId", () => {
    const appUrl = `${previewUrl}?app=ea22cbda-3d72-4d17-8524-f0d07ac57b63`
    expect(previewBrowserTargetFromURL(appUrl)).toMatchObject({
      appId: "ea22cbda-3d72-4d17-8524-f0d07ac57b63",
    })
    // A plain preview URL has no app hint.
    expect(previewBrowserTargetFromURL(previewUrl)?.appId).toBeUndefined()
  })

  it("does not absorb trailing markdown emphasis into the ?app= hint", () => {
    const appId = "92cf1439-f307-4d57-b1b3-33bd9daba26e"
    const targets = previewBrowserTargets(
      `Preview: **${previewUrl}?app=${appId}**`
    )
    expect(targets.map((t) => t.appId)).toEqual([appId])
  })
})
