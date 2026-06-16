import { describe, expect, it } from "vitest"
import {
  directSessionStreamCursor,
  directSessionStreamURL,
  type DirectSessionStreamFrame,
} from "@/app/w/(chat)/_lib/direct-session-stream"

function frame(data: unknown): DirectSessionStreamFrame {
  return {
    sessionId: "session_1",
    event: "token",
    id: "frame_1",
    data,
  }
}

describe("direct session stream", () => {
  it("builds a live-only replay=none url", () => {
    const url = directSessionStreamURL(
      "https://preview.usehivy.com/sessions/session_1/stream",
      "stok_123",
      { mode: "none" }
    )
    const parsed = new URL(url)
    expect(parsed.searchParams.get("stream_token")).toBe("stok_123")
    expect(parsed.searchParams.get("replay")).toBe("none")
    expect(parsed.searchParams.get("after_seq")).toBeNull()
  })

  it("builds an after-seq url without replay=none", () => {
    const url = directSessionStreamURL(
      "https://preview.usehivy.com/sessions/session_1/stream?replay=none",
      "stok_123",
      { mode: "after_seq", afterSeq: 42 }
    )
    const parsed = new URL(url)
    expect(parsed.searchParams.get("stream_token")).toBe("stok_123")
    expect(parsed.searchParams.get("replay")).toBeNull()
    expect(parsed.searchParams.get("after_seq")).toBe("42")
  })

  it("extracts a runtime cursor from streamed payloads", () => {
    expect(
      directSessionStreamCursor(
        frame({
          stream_id: "session-stream-1",
          sequence: 7,
        })
      )
    ).toEqual({
      streamId: "session-stream-1",
      sequence: 7,
    })
  })

  it("ignores frames without a valid runtime cursor", () => {
    expect(directSessionStreamCursor(frame("plain text"))).toBeNull()
    expect(directSessionStreamCursor(frame({ stream_id: "abc" }))).toBeNull()
  })
})
