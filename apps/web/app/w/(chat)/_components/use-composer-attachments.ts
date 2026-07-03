"use client"

import { useCallback, useEffect, useMemo, useRef } from "react"
import { $api } from "@/lib/api/hooks"
import {
  useOrgDriveFileUploads,
  type OrgDriveUploadItem,
} from "@/hooks/use-org-drive-file-uploads"
import {
  deleteDraftAttachmentBlob,
  selectSessionWorkspace,
  useSessionWorkspaceStore,
  type WorkspaceUploadItem,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import {
  attachmentMetadataFromDescription,
  requireImageDescriptionResult,
  type ImageAttachmentMetadata,
} from "@/app/w/(chat)/_lib/image-attachments"
import type { ComposerImageAttachment } from "./composer-attachments"

export function useComposerAttachments({
  sessionId,
  agentId,
  sessionExists,
  isDisabled,
}: {
  sessionId: string
  agentId: string
  sessionExists: boolean
  isDisabled: boolean
}) {
  const workspace = useSessionWorkspaceStore((state) =>
    selectSessionWorkspace(state, sessionId)
  )
  const setAttachmentDescriptions = useSessionWorkspaceStore(
    (state) => state.setAttachmentDescriptions
  )
  const attachmentDescriptions = workspace.composer.attachmentDescriptions
  const describedUploadsRef = useRef<Set<string>>(new Set())
  const { mutateAsync: describeDriveImage } = $api.useMutation(
    "post",
    "/v1/images/describe"
  )
  const { uploads, addFiles, retryUpload, removeUpload } =
    useOrgDriveFileUploads({ agentId, sessionId, sessionExists })

  const attachments = useMemo(
    () =>
      uploads.map((upload): ComposerImageAttachment => {
        if (upload.status === "uploading") {
          return { upload, status: "uploading" }
        }
        if (upload.status === "error") {
          return {
            upload,
            status: "error",
            error: upload.error || "Image upload failed",
          }
        }
        if (!upload.asset?.content_type.startsWith("image/")) {
          return {
            upload,
            status: "error",
            error: "Uploaded file is not an image",
          }
        }

        const description = attachmentDescriptions[upload.id]
        if (description?.status === "ready") {
          return {
            upload,
            status: "ready",
            metadata: description.metadata,
          }
        }
        if (description?.status === "error") {
          return {
            upload,
            status: "error",
            error: description.error,
          }
        }
        return { upload, status: "describing" }
      }),
    [attachmentDescriptions, uploads]
  )
  const hasPendingAttachment = attachments.some(
    (attachment) =>
      attachment.status === "uploading" || attachment.status === "describing"
  )
  const hasFailedAttachment = attachments.some(
    (attachment) => attachment.status === "error"
  )
  const readyAttachments = attachments
    .map((attachment) => attachment.metadata)
    .filter((attachment): attachment is ImageAttachmentMetadata =>
      Boolean(attachment)
    )

  const describeUpload = useCallback(
    async (upload: OrgDriveUploadItem) => {
      if (!upload.asset) return
      setAttachmentDescriptions(sessionId, (current) => ({
        ...current,
        [upload.id]: { status: "describing" },
      }))
      try {
        const description = requireImageDescriptionResult(
          await describeDriveImage({
            body: {
              drive_asset_id: upload.asset.id,
              detail_level: "high",
              ...(sessionExists ? { session_id: sessionId } : {}),
            },
          })
        )
        const metadata = attachmentMetadataFromDescription(
          upload.asset,
          description
        )
        setAttachmentDescriptions(sessionId, (current) => ({
          ...current,
          [upload.id]: { status: "ready", metadata },
        }))
      } catch (error) {
        const message =
          error instanceof Error ? error.message : "Image processing failed"
        setAttachmentDescriptions(sessionId, (current) => ({
          ...current,
          [upload.id]: { status: "error", error: message },
        }))
      }
    },
    [describeDriveImage, sessionExists, sessionId, setAttachmentDescriptions]
  )

  useEffect(() => {
    for (const upload of uploads) {
      if (upload.status !== "uploaded" || !upload.asset) continue
      if (attachmentDescriptions[upload.id]?.status) continue
      if (describedUploadsRef.current.has(upload.id)) continue
      describedUploadsRef.current.add(upload.id)
      if (!upload.asset.content_type.startsWith("image/")) {
        window.queueMicrotask(() => {
          setAttachmentDescriptions(sessionId, (current) => ({
            ...current,
            [upload.id]: {
              status: "error",
              error: "Uploaded file is not an image",
            },
          }))
        })
        continue
      }
      window.queueMicrotask(() => {
        void describeUpload(upload)
      })
    }
  }, [
    attachmentDescriptions,
    describeUpload,
    sessionId,
    setAttachmentDescriptions,
    uploads,
  ])

  const retryAttachment = (attachment: ComposerImageAttachment) => {
    const id = attachment.upload.id
    setAttachmentDescriptions(sessionId, (current) => {
      const next = { ...current }
      delete next[id]
      return next
    })
    describedUploadsRef.current.delete(id)
    if (attachment.upload.status === "error" || !attachment.upload.asset) {
      retryUpload(id)
      return
    }
    describedUploadsRef.current.add(id)
    void describeUpload(attachment.upload)
  }

  const removeAttachment = (id: string) => {
    removeUpload(id)
    describedUploadsRef.current.delete(id)
    setAttachmentDescriptions(sessionId, (current) => {
      const next = { ...current }
      delete next[id]
      return next
    })
  }

  const onDropAccepted = useCallback(
    (files: File[]) => {
      if (isDisabled) return
      addFiles(files)
    },
    [addFiles, isDisabled]
  )

  return {
    attachments,
    attachmentDescriptions,
    setAttachmentDescriptions,
    describedUploadsRef,
    hasPendingAttachment,
    hasFailedAttachment,
    readyAttachments,
    retryAttachment,
    removeAttachment,
    onDropAccepted,
  }
}

export function discardDraftUploads(uploads: WorkspaceUploadItem[]) {
  for (const upload of uploads) {
    if (upload.previewUrl?.startsWith("blob:")) {
      URL.revokeObjectURL(upload.previewUrl)
    }
    void deleteDraftAttachmentBlob(upload.blobKey)
  }
}
