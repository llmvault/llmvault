"use client"

import * as React from "react"

export type UploadAssetType = "avatar" | "org_logo" | "generic"

export interface UploadResult {
  /** Durable, CDN-served asset URL — store this. */
  publicUrl: string
  /** Storage key, useful for debugging. */
  key: string
  /** When the signed PUT URL expires. */
  expiresAt: string
  /** Max bytes allowed by policy at sign time. */
  maxSizeBytes: number
}

export interface UploadOptions {
  /** Required for org_logo, ignored otherwise. */
  orgId?: string
}

interface UseUploadResult {
  upload: (file: File, options?: UploadOptions) => Promise<UploadResult>
  isUploading: boolean
  error: Error | null
  reset: () => void
}

/**
 * Server-side upload to the public-assets bucket.
 *
 * POST the file as multipart form to /v1/uploads/upload. The backend signs
 * the upload and forwards it to S3/MinIO, returning the CDN asset URL.
 *
 * Files traverse the API server, which makes this work in environments where
 * the browser cannot reach the S3 endpoint directly (e.g. local Docker).
 */
export function useUpload(assetType: UploadAssetType): UseUploadResult {
  const [isUploading, setIsUploading] = React.useState(false)
  const [error, setError] = React.useState<Error | null>(null)

  const upload = React.useCallback(
    async (file: File, options: UploadOptions = {}): Promise<UploadResult> => {
      setIsUploading(true)
      setError(null)
      try {
        const form = new FormData()
        form.append("asset_type", assetType)
        form.append("file", file)
        if (options.orgId) form.append("org_id", options.orgId)

        const response = await fetch("/api/proxy/v1/uploads/upload", {
          method: "POST",
          body: form,
        })

        if (!response.ok) {
          const errBody = await response.json().catch(() => ({}))
          throw new Error(
            (errBody as Record<string, string>)?.error ??
              `Upload failed: ${response.status}`
          )
        }

        const result = (await response.json()) as {
          asset_url?: string
          public_url?: string
          key: string
          expires_at: string
          max_size_bytes: number
        }
        const publicUrl = result.asset_url ?? result.public_url ?? ""
        if (!publicUrl) {
          throw new Error("Upload did not return an asset URL")
        }

        return {
          publicUrl,
          key: result.key,
          expiresAt: result.expires_at,
          maxSizeBytes: result.max_size_bytes,
        }
      } catch (err) {
        const wrapped = err instanceof Error ? err : new Error(String(err))
        setError(wrapped)
        throw wrapped
      } finally {
        setIsUploading(false)
      }
    },
    [assetType]
  )

  const reset = React.useCallback(() => {
    setError(null)
  }, [])

  return { upload, isUploading, error, reset }
}
