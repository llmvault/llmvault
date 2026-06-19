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
  detailLevel = "high"
): Promise<ImageDescriptionResult> {
  const response = await fetch("/api/proxy/v1/images/describe", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      drive_asset_id: driveAssetId,
      detail_level: detailLevel,
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

export function composeMessageWithAttachments(
  text: string,
  attachments: ImageAttachmentMetadata[]
): string {
  const trimmed = text.trim()
  if (!attachments.length) return trimmed
  const tags = attachments.map(attachmentTag).join("\n")
  return trimmed ? `${trimmed}\n\n${tags}` : tags
}

export function attachmentTag(attachment: ImageAttachmentMetadata): string {
  return `<attachment name="${escapeXMLAttribute(attachment.filename)}" url="${escapeXMLAttribute(attachment.asset_url)}" mime_type="${escapeXMLAttribute(attachment.content_type)}">
<description>
${escapeXMLText(attachment.rendered_description)}
</description>
</attachment>`
}

export function imageAttachmentMediaURL(
  attachment: Pick<ImageAttachmentMetadata, "asset_url">
) {
  return attachment.asset_url
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

function escapeXMLAttribute(value: string) {
  return escapeXMLText(value).replaceAll('"', "&quot;")
}

function escapeXMLText(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
}
