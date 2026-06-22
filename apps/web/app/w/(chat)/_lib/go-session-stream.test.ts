import { describe, expect, it } from "vitest"
import {
  goSessionStreamCursor,
  goSessionStreamURL,
  isRuntimeRepoChangeFrame,
  type GoSessionStreamFrame,
} from "@/app/w/(chat)/_lib/go-session-stream"

function frame(data: unknown): GoSessionStreamFrame {
  return {
    sessionId: "session_1",
    event: "token",
    id: "frame_1",
    data,
  }
}

describe("session stream", () => {
  it("builds the proxied Go SSE url", () => {
    const url = goSessionStreamURL("session_1", { mode: "none" })
    const parsed = new URL(url)
    expect(parsed.pathname).toBe("/api/proxy/v1/sessions/session_1/stream")
    expect(parsed.searchParams.get("replay")).toBeNull()
    expect(parsed.searchParams.get("after_seq")).toBeNull()
  })

  it("builds an after-seq url without replay=none", () => {
    const url = goSessionStreamURL("session_1", {
      mode: "after_seq",
      afterSeq: 42,
    })
    const parsed = new URL(url)
    expect(parsed.searchParams.get("replay")).toBeNull()
    expect(parsed.searchParams.get("after_seq")).toBe("42")
  })

  it("extracts a runtime cursor from streamed payloads", () => {
    expect(
      goSessionStreamCursor(
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
    expect(goSessionStreamCursor(frame("plain text"))).toBeNull()
    expect(goSessionStreamCursor(frame({ stream_id: "abc" }))).toBeNull()
  })

  it("detects runtime repo change batches", () => {
    expect(
      isRuntimeRepoChangeFrame({
        ...frame({
          repo_id: "repo_1",
          paths: ["README.md"],
        }),
        event: "repo.change_batch",
      })
    ).toBe(true)

    expect(
      isRuntimeRepoChangeFrame({
        ...frame({ id: "tool_result_1" }),
        event: "tool_result",
      })
    ).toBe(false)
  })
})
