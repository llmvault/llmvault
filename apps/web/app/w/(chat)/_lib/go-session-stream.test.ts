import { fetchEventSource } from "@microsoft/fetch-event-source"
import { afterEach, describe, expect, it, vi, type Mock } from "vitest"

vi.mock("@microsoft/fetch-event-source", () => ({
  fetchEventSource: vi.fn(() => Promise.resolve()),
}))

import {
  goSessionStreamHTTPStatus,
  goSessionStreamCursor,
  goSessionStreamURL,
  isRuntimeRepoChangeFrame,
  subscribeToGoSessionStream,
  GoSessionStreamHTTPError,
  type GoSessionStreamFrame,
} from "@/app/w/(chat)/_lib/go-session-stream"

const fetchEventSourceMock = fetchEventSource as unknown as Mock

function frame(data: unknown): GoSessionStreamFrame {
  return {
    sessionId: "session_1",
    event: "token",
    id: "frame_1",
    data,
  }
}

describe("session stream", () => {
  afterEach(() => {
    fetchEventSourceMock.mockReset()
    fetchEventSourceMock.mockResolvedValue(undefined)
  })

  it("builds the API SSE proxy url", () => {
    const url = goSessionStreamURL("session_1", { mode: "none" })
    expect(url).toBe("/api/proxy/v1/sessions/session_1/stream?replay=none")
  })

  it("builds an after-seq url without replay=none", () => {
    const url = new URL(
      goSessionStreamURL("session_1", {
        mode: "after_seq",
        afterSeq: 42,
      }),
      "https://web.example.test"
    )
    expect(url.searchParams.get("replay")).toBeNull()
    expect(url.searchParams.get("after_seq")).toBe("42")
  })

  it("builds a from-turn url with optional follow", () => {
    const replay = new URL(
      goSessionStreamURL("session_1", {
        mode: "from_turn_id",
        turnId: "turn_123",
      }),
      "https://web.example.test"
    )
    const follow = new URL(
      goSessionStreamURL("session_1", {
        mode: "from_turn_id_follow",
        turnId: "turn_123",
      }),
      "https://web.example.test"
    )
    expect(replay.searchParams.get("from_turn_id")).toBe("turn_123")
    expect(replay.searchParams.get("follow")).toBeNull()
    expect(follow.searchParams.get("from_turn_id")).toBe("turn_123")
    expect(follow.searchParams.get("follow")).toBe("true")
  })

  it("uses browser authentication and never sends a runtime token", async () => {
    await subscribeToGoSessionStream({
      sessionId: "session_1",
      replay: { mode: "none" },
      signal: new AbortController().signal,
    })

    expect(fetchEventSourceMock).toHaveBeenCalledWith(
      "/api/proxy/v1/sessions/session_1/stream?replay=none",
      expect.objectContaining({
        credentials: "include",
        headers: { Accept: "text/event-stream" },
      })
    )
  })

  it("reports API HTTP status without retrying authorization locally", async () => {
    fetchEventSourceMock.mockImplementationOnce(async (_url, options) => {
      await options.onopen?.({
        ok: false,
        status: 403,
        headers: new Headers({ "content-type": "application/json" }),
        text: async () => JSON.stringify({ error: "session access denied" }),
      } as Response)
    })

    let caught: unknown
    try {
      await subscribeToGoSessionStream({
        sessionId: "session_1",
        signal: new AbortController().signal,
      })
    } catch (error) {
      caught = error
    }

    expect(caught).toBeInstanceOf(GoSessionStreamHTTPError)
    expect(goSessionStreamHTTPStatus(caught)).toBe(403)
    expect(caught).toMatchObject({
      message: "Session stream failed with HTTP 403: session access denied",
    })
  })

  it("passes raw runtime event payloads through", async () => {
    let received: GoSessionStreamFrame | undefined
    fetchEventSourceMock.mockImplementationOnce(async (_url, options) => {
      options.onmessage?.({
        event: "token",
        id: "",
        data: JSON.stringify({
          event_id: "event-token-1",
          text: "Hello",
        }),
      })
    })

    await subscribeToGoSessionStream({
      sessionId: "session_1",
      signal: new AbortController().signal,
      onEvent: (next) => {
        received = next
      },
    })

    expect(received).toEqual({
      sessionId: "session_1",
      event: "token",
      id: "event-token-1",
      data: {
        event_id: "event-token-1",
        text: "Hello",
      },
    })
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
        ...frame({ repo_id: "repo_1", paths: ["README.md"] }),
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
