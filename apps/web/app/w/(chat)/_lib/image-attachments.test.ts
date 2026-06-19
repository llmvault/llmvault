import { describe, expect, it } from "vitest"
import {
  attachmentMetadataFromDescription,
  composeMessageWithAttachments,
  type ImageDescriptionResult,
  type UploadedDriveImageAsset,
} from "@/app/w/(chat)/_lib/image-attachments"

describe("image attachment helpers", () => {
  it("composes attachment tags into the outgoing message", () => {
    const message = composeMessageWithAttachments("Please match this UI", [
      {
        drive_asset_id: "asset-1",
        asset_url: "https://api.test/v1/assets/preview?path=a&b=c",
        filename: `screenshot "one".png`,
        content_type: "image/png",
        rendered_description: "Primary category: Product UI\nColor: <blue>",
      },
    ])

    expect(message).toContain("Please match this UI")
    expect(message).toContain("<attachment")
    expect(message).toContain('mime_type="image/png"')
    expect(message).toContain("screenshot &quot;one&quot;.png")
    expect(message).toContain("a&amp;b=c")
    expect(message).toContain("Color: &lt;blue&gt;")
  })

  it("keeps structured metadata from upload and description responses", () => {
    const upload: UploadedDriveImageAsset = {
      id: "asset-1",
      asset_url: "https://api.test/v1/assets/preview?path=asset",
      key: "pub/e/a/uploads/asset.png",
      path: "uploads",
      filename: "asset.png",
      content_type: "image/png",
      bytes: 42,
    }
    const description: ImageDescriptionResult = {
      drive_asset_id: "asset-1",
      asset_url: upload.asset_url,
      filename: upload.filename,
      content_type: upload.content_type,
      category: "product_ui",
      confidence: 0.94,
      analysis: { summary: "A settings screen" },
      rendered_description: "Primary category: Product UI",
    }

    expect(attachmentMetadataFromDescription(upload, description)).toEqual({
      drive_asset_id: "asset-1",
      asset_url: upload.asset_url,
      filename: "asset.png",
      content_type: "image/png",
      bytes: 42,
      category: "product_ui",
      confidence: 0.94,
      analysis: { summary: "A settings screen" },
      rendered_description: "Primary category: Product UI",
    })
  })
})
