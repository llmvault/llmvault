"use client"

import { useCallback } from "react"
import type { paths } from "@/lib/api/schema"
import { $api } from "@/lib/api/hooks"
import { recordedAudioFile } from "@/app/w/(chat)/_lib/audio-transcriptions"

type TranscriptionUploadBody =
  paths["/v1/transcriptions"]["post"]["requestBody"]["content"]["multipart/form-data"]

interface TranscribeOrgAudioInput {
  blob: Blob
  filename?: string
  languageCode?: string
  mimeType?: string
}

// Org-scoped voice transcription. Mirrors `useSessionAudioTranscription` but
// posts to the session-independent `/v1/transcriptions` endpoint, so it can be
// used on settings surfaces (e.g. company context) that have no session.
export function useOrgAudioTranscription() {
  const { mutateAsync: transcribe, isPending } = $api.useMutation(
    "post",
    "/v1/transcriptions"
  )

  const mutateAsync = useCallback(
    async ({
      blob,
      filename,
      languageCode,
      mimeType,
    }: TranscribeOrgAudioInput): Promise<{ text: string }> => {
      const file = recordedAudioFile(blob, { filename, mimeType })
      const form = new FormData()
      form.set("file", file, file.name)
      if (languageCode) form.set("language_code", languageCode)

      const result = await transcribe({
        body: form as unknown as TranscriptionUploadBody,
      })
      return { text: result?.text ?? "" }
    },
    [transcribe]
  )

  return { mutateAsync, isPending }
}
