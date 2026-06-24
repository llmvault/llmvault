"use client"

import { useMutation } from "@tanstack/react-query"
import {
  recordedAudioFile,
  transcribeDriveAudioAsset,
  type TranscribedSessionAudio,
} from "@/app/w/(chat)/_lib/audio-transcriptions"
import { uploadDriveAsset } from "@/app/w/(chat)/_lib/image-attachments"

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
  return useMutation({
    mutationFn: async ({
      blob,
      filename,
      languageCode,
      mimeType,
      path = "uploads",
    }: TranscribeRecordedAudioInput): Promise<TranscribedSessionAudio> => {
      const file = recordedAudioFile(blob, { filename, mimeType })
      const asset = await uploadDriveAsset({ agentId, file, path })
      const text = await transcribeDriveAudioAsset({
        sessionId,
        driveAssetId: asset.id,
        languageCode,
      })
      return { asset, text }
    },
  })
}
