import { beforeEach, describe, expect, it, vi } from "vitest"
import {
  appendTranscriptToComposer,
  recordedAudioFile,
  transcribeDriveAudioAsset,
} from "@/app/w/(chat)/_lib/audio-transcriptions"
import { api } from "@/lib/api/client"

vi.mock("@/lib/api/client", () => ({
  api: {
    POST: vi.fn(),
  },
}))

describe("audio transcription helpers", () => {
  beforeEach(() => {
    vi.mocked(api.POST).mockReset()
  })

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

  it("transcribes an uploaded drive asset through the session endpoint", async () => {
    vi.mocked(api.POST).mockResolvedValue({
      data: { text: "Summarize the launch plan." },
      response: new Response(),
    })

    await expect(
      transcribeDriveAudioAsset({
        sessionId: "session-1",
        driveAssetId: "asset-1",
        languageCode: "en",
      })
    ).resolves.toBe("Summarize the launch plan.")

    expect(api.POST).toHaveBeenCalledWith("/v1/sessions/{id}/transcriptions", {
      params: { path: { id: "session-1" } },
      body: {
        drive_asset_id: "asset-1",
        language_code: "en",
      },
    })
  })
})
