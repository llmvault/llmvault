import { beforeEach, describe, expect, it } from "vitest"
import {
  assetPreviewAttachments,
  isAssetPreviewUrl,
} from "@/app/w/(chat)/_lib/asset-preview-links"
import { setPublicConfigForTests } from "@/lib/config/public-config"

beforeEach(() => setPublicConfigForTests())

const previewUrl =
  "https://api.usehivy.test/v1/assets/preview?path=pub%2Fe%2F11601c67-0860-44f8-9730-fd9820b3970b%2Fscreenshots%2Fexample-com.png"

describe("asset preview links", () => {
  it("detects Hivy asset preview URLs", () => {
    expect(isAssetPreviewUrl(previewUrl)).toBe(true)
    expect(isAssetPreviewUrl("https://example.com/v1/assets/preview")).toBe(
      false
    )
  })

  it("extracts unique image attachments from assistant text", () => {
    const attachments = assetPreviewAttachments(
      `Screenshot: ${previewUrl} and again ${previewUrl}.`,
      "assistant-1"
    )

    expect(attachments).toEqual([
      expect.objectContaining({
        id: expect.stringMatching(/^asset-preview:[a-z0-9]+:0:/),
        filename: "example-com.png",
        kind: "image",
        url: previewUrl,
      }),
    ])
  })

  it("strips markdown emphasis trailing a preview URL", () => {
    const attachments = assetPreviewAttachments(
      `The image is at **${previewUrl}**`,
      "assistant-2"
    )

    expect(attachments).toEqual([
      expect.objectContaining({
        filename: "example-com.png",
        url: previewUrl,
      }),
    ])
  })

  it("skips streamed preview URLs with incomplete percent escapes", () => {
    const firstEscape = previewUrl.indexOf("%2F")
    const partialUrls = [
      previewUrl.slice(0, firstEscape + 1),
      previewUrl.slice(0, firstEscape + 2),
    ]

    for (const partialUrl of partialUrls) {
      expect(() =>
        assetPreviewAttachments(`Screenshot: ${partialUrl}`, "streaming")
      ).not.toThrow()
      expect(
        assetPreviewAttachments(`Screenshot: ${partialUrl}`, "streaming")
      ).toEqual([])
    }
  })
})
