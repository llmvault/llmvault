import type { GitStatusEntry } from "@pierre/trees"
import type { UploadedDriveAsset } from "@/app/w/(chat)/_lib/image-attachments"

export type WorkspaceUploadStatus = "uploading" | "uploaded" | "error"

export interface WorkspaceUploadItem {
  id: string
  fileName: string
  contentType: string
  bytes: number
  lastModified: number
  blobKey?: string
  file?: File
  previewUrl?: string
  status: WorkspaceUploadStatus
  asset?: UploadedDriveAsset
  error?: string
}

export interface WorkspaceRepoTreeCache {
  directoryPaths: string[]
  failedDirectories: Record<string, string>
  gitStatus: GitStatusEntry[]
  loadedDirectories: string[]
  loadingDirectories: string[]
  paths: string[]
}
