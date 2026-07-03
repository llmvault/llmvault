"use client"

import { useCallback, useEffect, useRef } from "react"
import { $api } from "@/lib/api/hooks"
import {
  driveAssetUploadFormData,
  requireUploadedDriveAsset,
  type UploadedDriveAsset,
} from "@/app/w/(chat)/_lib/image-attachments"
import {
  createDraftBlobKey,
  deleteDraftAttachmentBlob,
  readDraftAttachmentBlob,
  selectSessionWorkspace,
  storeDraftAttachmentBlob,
  useSessionWorkspaceStore,
  type WorkspaceUploadItem,
} from "@/app/w/(chat)/_stores/session-workspace-store"

export type OrgDriveUploadStatus = "uploading" | "uploaded" | "error"

export interface OrgDriveUploadItem {
  id: string
  file?: File
  fileName: string
  contentType: string
  bytes: number
  lastModified: number
  blobKey?: string
  previewUrl: string
  status: OrgDriveUploadStatus
  asset?: UploadedDriveAsset
  error?: string
}

interface UseOrgDriveFileUploadsOptions {
  agentId: string
  // Workspace draft key; also the upload's destination session unless
  // sessionExists is false (pre-session drafts upload without a session).
  sessionId: string
  sessionExists?: boolean
  path?: string
}

export function useOrgDriveFileUploads({
  agentId,
  sessionId,
  sessionExists = true,
  path = "uploads",
}: UseOrgDriveFileUploadsOptions) {
  const uploads = useSessionWorkspaceStore(
    (state) => selectSessionWorkspace(state, sessionId).composer.uploads
  )
  const uploadsRef = useRef<WorkspaceUploadItem[]>([])
  const restoredUploadsRef = useRef<Set<string>>(new Set())
  const setComposerUploads = useSessionWorkspaceStore(
    (state) => state.setComposerUploads
  )
  const { mutateAsync: uploadDriveAsset } = $api.useMutation(
    "post",
    "/v1/assets/upload"
  )

  useEffect(() => {
    uploadsRef.current = uploads
  }, [uploads])

  const uploadFile = useCallback(
    async (id: string, file: File) => {
      try {
        const asset = requireUploadedDriveAsset(
          await uploadDriveAsset({
            body: driveAssetUploadFormData({
              agentId,
              sessionId: sessionExists ? sessionId : undefined,
              file,
              path,
            }),
          })
        )
        setComposerUploads(sessionId, (current) =>
          current.map((upload) =>
            upload.id === id
              ? { ...upload, status: "uploaded", asset, error: undefined }
              : upload
          )
        )
      } catch (error) {
        const message =
          error instanceof Error ? error.message : "File upload failed"
        setComposerUploads(sessionId, (current) =>
          current.map((upload) =>
            upload.id === id
              ? { ...upload, status: "error", error: message }
              : upload
          )
        )
      }
    },
    [agentId, path, sessionExists, sessionId, setComposerUploads, uploadDriveAsset]
  )

  useEffect(() => {
    for (const upload of uploads) {
      if (upload.previewUrl || restoredUploadsRef.current.has(upload.id)) {
        continue
      }
      restoredUploadsRef.current.add(upload.id)
      if (upload.asset?.asset_url) {
        setComposerUploads(sessionId, (current) =>
          current.map((entry) =>
            entry.id === upload.id
              ? { ...entry, previewUrl: upload.asset?.asset_url }
              : entry
          )
        )
        continue
      }
      if (!upload.blobKey) {
        setComposerUploads(sessionId, (current) =>
          current.map((entry) =>
            entry.id === upload.id
              ? { ...entry, status: "error", error: "Draft file is missing" }
              : entry
          )
        )
        continue
      }
      void readDraftAttachmentBlob(upload.blobKey)
        .then((blob) => {
          if (!blob) throw new Error("Draft file is missing")
          const file =
            blob instanceof File
              ? blob
              : new File([blob], upload.fileName, {
                  type: upload.contentType,
                  lastModified: upload.lastModified,
                })
          const previewUrl = URL.createObjectURL(file)
          setComposerUploads(sessionId, (current) =>
            current.map((entry) =>
              entry.id === upload.id
                ? {
                    ...entry,
                    file,
                    previewUrl,
                    status:
                      entry.status === "uploaded" ? "uploaded" : "uploading",
                  }
                : entry
            )
          )
          if (upload.status !== "uploaded") {
            void uploadFile(upload.id, file)
          }
        })
        .catch((error: unknown) => {
          const message =
            error instanceof Error ? error.message : "Draft file is missing"
          setComposerUploads(sessionId, (current) =>
            current.map((entry) =>
              entry.id === upload.id
                ? { ...entry, status: "error", error: message }
                : entry
            )
          )
        })
    }
  }, [sessionId, setComposerUploads, uploadFile, uploads])

  const addFiles = useCallback(
    (files: File[]) => {
      if (!files.length) return
      const next = files.map((file) => {
        const id = newUploadId()
        return {
          id,
          file,
          fileName: file.name,
          contentType: file.type,
          bytes: file.size,
          lastModified: file.lastModified,
          blobKey: createDraftBlobKey(sessionId, id),
          previewUrl: URL.createObjectURL(file),
          status: "uploading" as const,
        }
      })
      setComposerUploads(sessionId, (current) => [...current, ...next])
      for (const upload of next) {
        if (upload.blobKey) {
          void storeDraftAttachmentBlob(upload.blobKey, upload.file)
        }
        void uploadFile(upload.id, upload.file)
      }
    },
    [sessionId, setComposerUploads, uploadFile]
  )

  const retryUpload = useCallback(
    (id: string) => {
      const upload = uploadsRef.current.find((entry) => entry.id === id)
      if (!upload) return
      if (!upload.file) {
        setComposerUploads(sessionId, (current) =>
          current.map((entry) =>
            entry.id === id
              ? {
                  ...entry,
                  status: "error",
                  error: "Draft file is missing",
                }
              : entry
          )
        )
        return
      }
      setComposerUploads(sessionId, (current) =>
        current.map((entry) =>
          entry.id === id
            ? {
                ...entry,
                status: "uploading",
                asset: undefined,
                error: undefined,
              }
            : entry
        )
      )
      void uploadFile(id, upload.file)
    },
    [sessionId, setComposerUploads, uploadFile]
  )

  const removeUpload = useCallback(
    (id: string) => {
      setComposerUploads(sessionId, (current) =>
        current.filter((upload) => {
          if (upload.id === id) {
            if (upload.previewUrl?.startsWith("blob:")) {
              URL.revokeObjectURL(upload.previewUrl)
            }
            void deleteDraftAttachmentBlob(upload.blobKey)
            return false
          }
          return true
        })
      )
    },
    [sessionId, setComposerUploads]
  )

  const clearUploads = useCallback(() => {
    setComposerUploads(sessionId, (current) => {
      for (const upload of current) {
        if (upload.previewUrl?.startsWith("blob:")) {
          URL.revokeObjectURL(upload.previewUrl)
        }
        void deleteDraftAttachmentBlob(upload.blobKey)
      }
      return []
    })
  }, [sessionId, setComposerUploads])

  return {
    uploads: uploads.filter((upload): upload is OrgDriveUploadItem =>
      Boolean(upload.previewUrl)
    ),
    addFiles,
    retryUpload,
    removeUpload,
    clearUploads,
    hasPendingUploads: uploads.some((upload) => upload.status === "uploading"),
    hasFailedUploads: uploads.some((upload) => upload.status === "error"),
  }
}

function newUploadId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}
