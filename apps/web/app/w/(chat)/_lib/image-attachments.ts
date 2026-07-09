import type { components, paths } from "@/lib/api/schema"

type DriveAssetUploadBody =
  paths["/v1/assets/upload"]["post"]["requestBody"]["content"]["multipart/form-data"]
type UploadedDriveAssetResponse = components["schemas"]["streamAssetResponse"]
type ImageDescriptionResponse = components["schemas"]["imageDescribeResponse"]

export type UploadedDriveAsset = Required<UploadedDriveAssetResponse>

export type UploadedDriveImageAsset = UploadedDriveAsset

export type ImageDescriptionResult = Required<ImageDescriptionResponse>

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

export function driveAssetUploadFormData({
  agentId,
  sessionId,
  file,
  path = "uploads",
}: {
  agentId: string
  // Omit for pre-session uploads (new-chat drafts); the asset is stored
  // without a sandbox and referenced when the session is created.
  sessionId?: string
  file: File
  path?: string
}): DriveAssetUploadBody {
  const form = new FormData()
  form.set("agent_id", agentId)
  if (sessionId) form.set("session_id", sessionId)
  form.set("path", path)
  form.set("file", file, file.name)
  return form as unknown as DriveAssetUploadBody
}

export function requireUploadedDriveAsset(
  asset: UploadedDriveAssetResponse | undefined
): UploadedDriveAsset {
  if (
    !asset?.id ||
    !asset.asset_url ||
    !asset.key ||
    asset.path === undefined ||
    !asset.filename ||
    !asset.content_type ||
    asset.bytes === undefined
  ) {
    throw new Error("Drive upload response was incomplete")
  }
  return asset as UploadedDriveAsset
}

export function requireImageDescriptionResult(
  description: ImageDescriptionResponse | undefined
): ImageDescriptionResult {
  if (
    !description?.drive_asset_id ||
    !description.asset_url ||
    !description.filename ||
    !description.content_type ||
    !description.category ||
    description.confidence === undefined ||
    !description.analysis ||
    !description.rendered_description
  ) {
    throw new Error("Image description response was incomplete")
  }
  return description as ImageDescriptionResult
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

function imageAttachmentMediaURL(
  attachment: Pick<ImageAttachmentMetadata, "asset_url">
) {
  return attachment.asset_url
}

export function imageAttachmentIDs(
  attachments: Pick<ImageAttachmentMetadata, "drive_asset_id">[]
) {
  return attachments.map((attachment) => attachment.drive_asset_id)
}
