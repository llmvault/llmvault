export interface UploadedDriveAsset {
  id: string
  asset_url: string
  key: string
  path: string
  filename: string
  content_type: string
  bytes: number
}

export type UploadedDriveImageAsset = UploadedDriveAsset

export interface ImageDescriptionResult {
  drive_asset_id: string
  asset_url: string
  filename: string
  content_type: string
  category: string
  confidence: number
  analysis: Record<string, unknown>
  rendered_description: string
}

export interface ImageAttachmentMetadata {
  drive_asset_id: string
  asset_url: string
  filename: string
  content_type: string
  bytes?: number
  category?: string
  confidence?: number
  rendered_description: string
  analysis?: Record<string, unknown>
}

export async function uploadDriveAsset({
  agentId,
  file,
  path = "uploads",
}: {
  agentId: string
  file: File
  path?: string
}): Promise<UploadedDriveAsset> {
  const form = new FormData()
  form.set("agent_id", agentId)
  form.set("path", path)
  form.set("file", file, file.name)

  const response = await fetch("/api/proxy/v1/assets/upload", {
    method: "POST",
    body: form,
  })
  if (!response.ok) {
    throw new Error(await responseError(response, "Drive upload failed"))
  }
  return (await response.json()) as UploadedDriveAsset
}

export async function uploadDriveImageAsset(options: {
  agentId: string
  file: File
  path?: string
}): Promise<UploadedDriveImageAsset> {
  return uploadDriveAsset(options)
}

export async function describeDriveImage(
  driveAssetId: string,
  sessionId?: string,
  detailLevel = "high"
): Promise<ImageDescriptionResult> {
  const response = await fetch("/api/proxy/v1/images/describe", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      drive_asset_id: driveAssetId,
      detail_level: detailLevel,
      session_id: sessionId,
    }),
  })
  if (!response.ok) {
    throw new Error(await responseError(response, "Image description failed"))
  }
  return (await response.json()) as ImageDescriptionResult
}

export function attachmentMetadataFromDescription(
  upload: UploadedDriveImageAsset,
  description: ImageDescriptionResult
): ImageAttachmentMetadata {
  return {
    drive_asset_id: description.drive_asset_id || upload.id,
    asset_url: description.asset_url || upload.asset_url,
    filename: description.filename || upload.filename,
    content_type: description.content_type || upload.content_type,
    bytes: upload.bytes,
    category: description.category,
    confidence: description.confidence,
    rendered_description: description.rendered_description,
    analysis: description.analysis,
  }
}

export function imageAttachmentMediaURL(
  attachment: Pick<ImageAttachmentMetadata, "asset_url">
) {
  return attachment.asset_url
}

export function imageAttachmentIDs(
  attachments: Pick<ImageAttachmentMetadata, "drive_asset_id">[]
) {
  return attachments.map((attachment) => attachment.drive_asset_id)
}

async function responseError(response: Response, fallback: string) {
  try {
    const data = (await response.json()) as {
      error?: unknown
      message?: unknown
    }
    if (typeof data.error === "string" && data.error.trim()) {
      return data.error
    }
    if (typeof data.message === "string" && data.message.trim()) {
      return data.message
    }
  } catch {
    // Ignore body parse failures and use the stable fallback below.
  }
  return fallback
}
