import { describe, expect, it } from "vitest"
import {
  attachmentMetadataFromDescription,
  imageAttachmentIDs,
  type ImageDescriptionResult,
  type UploadedDriveImageAsset,
} from "@/app/w/(chat)/_lib/image-attachments"

describe("image attachment helpers", () => {
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

  it("extracts only drive asset ids for message sends", () => {
    expect(
      imageAttachmentIDs([
        {
          drive_asset_id: "asset-1",
        },
      ])
    ).toEqual(["asset-1"])
  })
})
