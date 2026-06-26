"use client"

import { useCallback } from "react"
import { $api } from "@/lib/api/hooks"
import {
  recordedAudioFile,
  type TranscribedSessionAudio,
} from "@/app/w/(chat)/_lib/audio-transcriptions"
import {
  driveAssetUploadFormData,
  requireUploadedDriveAsset,
} from "@/app/w/(chat)/_lib/image-attachments"

interface UseSessionAudioTranscriptionOptions {
  agentId: string
  sessionId: string
}

export interface TranscribeRecordedAudioInput {
  blob: Blob
  filename?: string
  languageCode?: string
  mimeType?: string
  path?: string
}

export function useSessionAudioTranscription({
  agentId,
  sessionId,
}: UseSessionAudioTranscriptionOptions) {
  const { mutateAsync: uploadDriveAsset, isPending: isUploading } =
    $api.useMutation("post", "/v1/assets/upload")
  const { mutateAsync: transcribeAudio, isPending: isTranscribing } =
    $api.useMutation("post", "/v1/sessions/{id}/transcriptions")

  const mutateAsync = useCallback(
    async ({
      blob,
      filename,
      languageCode,
      mimeType,
      path = "uploads",
    }: TranscribeRecordedAudioInput): Promise<TranscribedSessionAudio> => {
      const file = recordedAudioFile(blob, { filename, mimeType })
      const asset = requireUploadedDriveAsset(
        await uploadDriveAsset({
          body: driveAssetUploadFormData({ agentId, file, path }),
        })
      )
      const transcription = await transcribeAudio({
        params: { path: { id: sessionId } },
        body: {
          drive_asset_id: asset.id,
          language_code: languageCode,
        },
      })
      return { asset, text: transcription?.text ?? "" }
    },
    [agentId, sessionId, transcribeAudio, uploadDriveAsset]
  )

  return {
    mutateAsync,
    isPending: isUploading || isTranscribing,
  }
}
