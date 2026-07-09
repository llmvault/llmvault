import type { UploadedDriveAsset } from "./image-attachments"

interface RecordedAudioFileOptions {
  filename?: string
  mimeType?: string
  timestamp?: number
}

export interface TranscribedSessionAudio {
  asset: UploadedDriveAsset
  text: string
}

export function recordedAudioFile(
  blob: Blob,
  options: RecordedAudioFileOptions = {}
) {
  const mimeType = options.mimeType || blob.type || "audio/webm"
  return new File(
    [blob],
    options.filename ??
      `voice-${options.timestamp ?? Date.now()}.${audioExtension(mimeType)}`,
    { type: mimeType }
  )
}

export function appendTranscriptToComposer(
  current: string,
  transcript: string
) {
  const next = transcript.trim()
  if (!next) return current

  const existing = current.trimEnd()
  return existing ? `${existing}\n${next}` : next
}

function audioExtension(mimeType: string) {
  const normalized = mimeType.toLowerCase().split(";")[0]?.trim()
  switch (normalized) {
    case "audio/m4a":
    case "audio/mp4":
    case "audio/x-m4a":
      return "m4a"
    case "audio/mp3":
    case "audio/mpeg":
      return "mp3"
    case "audio/ogg":
      return "ogg"
    case "audio/wav":
    case "audio/x-wav":
      return "wav"
    case "video/mp4":
      return "mp4"
    case "video/webm":
    case "audio/webm":
    default:
      return "webm"
  }
}
