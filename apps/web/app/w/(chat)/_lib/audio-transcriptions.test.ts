import { describe, expect, it } from "vitest"
import {
  appendTranscriptToComposer,
  recordedAudioFile,
} from "@/app/w/(chat)/_lib/audio-transcriptions"

describe("audio transcription helpers", () => {
  it("creates a stable recorded audio file from a blob", () => {
    const blob = new Blob(["audio"], { type: "audio/webm" })

    const file = recordedAudioFile(blob, { timestamp: 123 })

    expect(file.name).toBe("voice-123.webm")
    expect(file.type).toBe("audio/webm")
    expect(file.size).toBe(blob.size)
  })

  it("appends transcript text to the composer draft", () => {
    expect(appendTranscriptToComposer("", "  Hello there.  ")).toBe(
      "Hello there."
    )
    expect(appendTranscriptToComposer("Existing prompt  ", "New line")).toBe(
      "Existing prompt\nNew line"
    )
  })
})
