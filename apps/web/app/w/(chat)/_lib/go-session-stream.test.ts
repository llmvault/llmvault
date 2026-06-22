import { fetchEventSource } from "@microsoft/fetch-event-source"
import { afterEach, describe, expect, it, vi, type Mock } from "vitest"

vi.mock("@microsoft/fetch-event-source", () => ({
  fetchEventSource: vi.fn(() => Promise.resolve()),
}))

import {
  goSessionStreamHeaders,
  goSessionStreamHTTPStatus,
  goSessionStreamCursor,
  goSessionStreamURL,
  isRuntimeRepoChangeFrame,
  subscribeToGoSessionStream,
  GoSessionStreamHTTPError,
  type GoSessionStreamFrame,
} from "@/app/w/(chat)/_lib/go-session-stream"
import type { SessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"

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

  it("builds the direct sandbox runtime SSE url", () => {
    const url = goSessionStreamURL("session_1", sandboxAccess(), {
      mode: "none",
    })
    const parsed = new URL(url)
    expect(parsed.origin).toBe("https://sandbox.example.test")
    expect(parsed.pathname).toBe("/sessions/session_1/stream")
    expect(parsed.searchParams.get("replay")).toBe("none")
    expect(parsed.searchParams.get("after_seq")).toBeNull()
    expect(url).not.toContain("/api/proxy")
  })

  it("builds an after-seq url without replay=none", () => {
    const url = goSessionStreamURL("session_1", sandboxAccess(), {
      mode: "after_seq",
      afterSeq: 42,
    })
    const parsed = new URL(url)
    expect(parsed.searchParams.get("replay")).toBeNull()
    expect(parsed.searchParams.get("after_seq")).toBe("42")
  })

  it("builds a from-turn url without replay or after_seq", () => {
    const url = goSessionStreamURL("session_1", sandboxAccess(), {
      mode: "from_turn_id",
      turnId: "turn_123",
    })
    const parsed = new URL(url)
    expect(parsed.searchParams.get("from_turn_id")).toBe("turn_123")
    expect(parsed.searchParams.get("replay")).toBeNull()
    expect(parsed.searchParams.get("after_seq")).toBeNull()
  })

  it("authenticates direct runtime reads with the sandbox token", async () => {
    await subscribeToGoSessionStream({
      sessionId: "session_1",
      access: sandboxAccess({ token: " sandbox-token " }),
      replay: { mode: "none" },
      signal: new AbortController().signal,
    })

    expect(fetchEventSourceMock).toHaveBeenCalledWith(
      "https://sandbox.example.test/sessions/session_1/stream?replay=none",
      expect.objectContaining({
        credentials: "omit",
        headers: {
          Accept: "text/event-stream",
          Authorization: "Bearer sandbox-token",
        },
      })
    )
  })

  it("reports direct runtime HTTP status for auth refresh decisions", async () => {
    fetchEventSourceMock.mockImplementationOnce(async (_url, options) => {
      await options.onopen?.({
        ok: false,
        status: 403,
        headers: new Headers({ "content-type": "application/json" }),
        text: async () => JSON.stringify({ error: "missing stream scope" }),
      } as Response)
    })

    let caught: unknown
    try {
      await subscribeToGoSessionStream({
        sessionId: "session_1",
        access: sandboxAccess(),
        signal: new AbortController().signal,
      })
    } catch (error) {
      caught = error
    }

    expect(caught).toBeInstanceOf(GoSessionStreamHTTPError)
    expect(goSessionStreamHTTPStatus(caught)).toBe(403)
    expect(caught).toMatchObject({
      message: "Session stream failed with HTTP 403: missing stream scope",
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
      access: sandboxAccess(),
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

  it("builds sandbox auth headers", () => {
    expect(goSessionStreamHeaders(sandboxAccess({ token: " token " }))).toEqual(
      {
        Accept: "text/event-stream",
        Authorization: "Bearer token",
      }
    )
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

function sandboxAccess(
  overrides: Partial<SessionSandboxAccess> = {}
): SessionSandboxAccess {
  return {
    session_id: "session_1",
    sandbox_id: "sandbox_1",
    sandbox_base_url: "https://sandbox.example.test/",
    token: "sandbox-token",
    expires_at: "2026-06-20T12:00:00Z",
    scopes: ["repo:read", "stream:read"],
    ...overrides,
  }
}
