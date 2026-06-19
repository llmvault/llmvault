"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import {
  uploadDriveAsset,
  type UploadedDriveAsset,
} from "@/app/w/(chat)/_lib/image-attachments"

export type OrgDriveUploadStatus = "uploading" | "uploaded" | "error"

export interface OrgDriveUploadItem {
  id: string
  file: File
  previewUrl: string
  status: OrgDriveUploadStatus
  asset?: UploadedDriveAsset
  error?: string
}

interface UseOrgDriveFileUploadsOptions {
  agentId: string
  path?: string
}

export function useOrgDriveFileUploads({
  agentId,
  path = "uploads",
}: UseOrgDriveFileUploadsOptions) {
  const [uploads, setUploads] = useState<OrgDriveUploadItem[]>([])
  const uploadsRef = useRef<OrgDriveUploadItem[]>([])

  useEffect(() => {
    uploadsRef.current = uploads
  }, [uploads])

  useEffect(() => {
    return () => {
      for (const upload of uploadsRef.current) {
        URL.revokeObjectURL(upload.previewUrl)
      }
    }
  }, [])

  const uploadFile = useCallback(
    async (id: string, file: File) => {
      try {
        const asset = await uploadDriveAsset({ agentId, file, path })
        setUploads((current) =>
          current.map((upload) =>
            upload.id === id
              ? { ...upload, status: "uploaded", asset, error: undefined }
              : upload
          )
        )
      } catch (error) {
        const message =
          error instanceof Error ? error.message : "File upload failed"
        setUploads((current) =>
          current.map((upload) =>
            upload.id === id
              ? { ...upload, status: "error", error: message }
              : upload
          )
        )
      }
    },
    [agentId, path]
  )

  const addFiles = useCallback(
    (files: File[]) => {
      if (!files.length) return
      const next = files.map((file) => ({
        id: newUploadId(),
        file,
        previewUrl: URL.createObjectURL(file),
        status: "uploading" as const,
      }))
      setUploads((current) => [...current, ...next])
      for (const upload of next) {
        void uploadFile(upload.id, upload.file)
      }
    },
    [uploadFile]
  )

  const retryUpload = useCallback(
    (id: string) => {
      const upload = uploadsRef.current.find((entry) => entry.id === id)
      if (!upload) return
      setUploads((current) =>
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
    [uploadFile]
  )

  const removeUpload = useCallback((id: string) => {
    setUploads((current) =>
      current.filter((upload) => {
        if (upload.id === id) {
          URL.revokeObjectURL(upload.previewUrl)
          return false
        }
        return true
      })
    )
  }, [])

  const clearUploads = useCallback(() => {
    setUploads((current) => {
      for (const upload of current) {
        URL.revokeObjectURL(upload.previewUrl)
      }
      return []
    })
  }, [])

  return {
    uploads,
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
