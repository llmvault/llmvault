import type { QueryClient } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi, type Mock } from "vitest"

vi.mock("@/app/w/(chat)/_lib/session-sandbox-access", () => ({
  getSessionSandboxAccess: vi.fn(),
}))

vi.mock("@/app/w/(chat)/_lib/go-session-stream", async (importOriginal) => {
  const actual =
    await importOriginal<
      typeof import("@/app/w/(chat)/_lib/go-session-stream")
    >()
  return {
    ...actual,
    subscribeToGoSessionStream: vi.fn(() => Promise.resolve()),
  }
})

import {
  ensureSessionStream,
  resumeSessionConnectionsForOrg,
  stopAllSessionStreams,
  suspendSessionConnectionsForOrg,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import { useSessionRuntimeStore } from "@/app/w/(chat)/_stores/session-runtime-store"
import { getSessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"
import {
  GoSessionStreamHTTPError,
  subscribeToGoSessionStream,
  type GoSessionStreamFrame,
} from "@/app/w/(chat)/_lib/go-session-stream"
import type { SessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"

const getSessionSandboxAccessMock = getSessionSandboxAccess as unknown as Mock
const subscribeToGoSessionStreamMock =
  subscribeToGoSessionStream as unknown as Mock

describe("session stream manager", () => {
  afterEach(() => {
    stopAllSessionStreams()
    vi.useRealTimers()
    vi.clearAllMocks()
    useSessionRuntimeStore.setState({
      statusBySessionId: {},
      liveEventsBySessionId: {},
      subagentRunsBySessionId: {},
      cursorBySessionId: {},
      reconnectAttemptsBySessionId: {},
    })
  })

  it("opens the chat stream with sandbox access instead of a backend stream", async () => {
    markTurnActive()
    const access = sandboxAccess({ token: "token-1" })
    getSessionSandboxAccessMock.mockResolvedValueOnce(access)
    subscribeToGoSessionStreamMock.mockResolvedValueOnce(undefined)

    const queryClient = testQueryClient()
    ensureSessionStream("session-1", { queryClient })
    await flushAsync()

    expect(getSessionSandboxAccessMock).toHaveBeenCalledWith(
      "session-1",
      queryClient,
      {
        force: undefined,
      }
    )
    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: "session-1",
        access,
        replay: { mode: "all" },
      })
    )
  })

  it("refreshes sandbox access once after a direct stream auth failure", async () => {
    vi.useFakeTimers()
    markTurnActive()
    const firstAccess = sandboxAccess({ token: "expired-token" })
    const refreshedAccess = sandboxAccess({ token: "fresh-token" })
    getSessionSandboxAccessMock
      .mockResolvedValueOnce(firstAccess)
      .mockResolvedValueOnce(refreshedAccess)
    subscribeToGoSessionStreamMock
      .mockRejectedValueOnce(new GoSessionStreamHTTPError(401, "expired"))
      .mockResolvedValueOnce(undefined)

    const queryClient = testQueryClient()
    ensureSessionStream("session-1", { queryClient })
    await flushAsync()
    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(400)
    await flushAsync()

    expect(getSessionSandboxAccessMock).toHaveBeenNthCalledWith(
      2,
      "session-1",
      queryClient,
      {
        force: true,
      }
    )
    expect(subscribeToGoSessionStreamMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        access: refreshedAccess,
      })
    )
  })

  it("keeps one stream open when the loaded replay mode changes", async () => {
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValue(sandboxAccess())
    let firstSignal: AbortSignal | undefined
    subscribeToGoSessionStreamMock.mockImplementationOnce(({ signal }) => {
      firstSignal = signal
      return new Promise<void>((resolve) => {
        signal.addEventListener("abort", () => resolve(), { once: true })
      })
    })

    const queryClient = testQueryClient()
    ensureSessionStream("session-1", {
      queryClient,
      replay: { mode: "none" },
    })
    await flushAsync()

    ensureSessionStream("session-1", {
      queryClient,
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()

    expect(firstSignal?.aborted).toBe(false)
    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledTimes(1)
    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ replay: { mode: "none" } })
    )
  })

  it("suspends an active workspace stream and resumes from its cursor", async () => {
    getSessionSandboxAccessMock.mockResolvedValue(sandboxAccess())
    const signals: AbortSignal[] = []
    subscribeToGoSessionStreamMock.mockImplementation(({ signal }) => {
      signals.push(signal)
      return new Promise<void>((resolve) => {
        signal.addEventListener("abort", () => resolve(), { once: true })
      })
    })
    useSessionRuntimeStore.setState({
      statusBySessionId: {
        "session-1": {
          status: "streaming",
          updatedAt: Date.now(),
        },
      },
      cursorBySessionId: {
        "session-1": {
          streamId: "stream-1",
          sequence: 42,
        },
      },
    })

    const queryClient = testQueryClient()
    ensureSessionStream("session-1", {
      queryClient,
      orgId: "org-a",
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()

    suspendSessionConnectionsForOrg("org-a")
    expect(signals[0]?.aborted).toBe(true)

    resumeSessionConnectionsForOrg("org-a", queryClient)
    await flushAsync()

    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledTimes(2)
    expect(subscribeToGoSessionStreamMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        replay: { mode: "after_seq", afterSeq: 42 },
      })
    )
  })

  it("preserves from_turn_id across an early reconnect before a cursor exists", async () => {
    vi.useFakeTimers()
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValue(sandboxAccess())
    subscribeToGoSessionStreamMock
      .mockRejectedValueOnce(new Error("network closed"))
      .mockResolvedValueOnce(undefined)

    ensureSessionStream("session-1", {
      queryClient: testQueryClient(),
      replay: { mode: "from_turn_id", turnId: "turn-1" },
    })
    await flushAsync()
    await vi.advanceTimersByTimeAsync(400)
    await flushAsync()

    expect(subscribeToGoSessionStreamMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        replay: { mode: "from_turn_id", turnId: "turn-1" },
      })
    )
  })

  it("preserves durable from_turn_id follow across an early reconnect before a cursor exists", async () => {
    vi.useFakeTimers()
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValue(sandboxAccess())
    subscribeToGoSessionStreamMock
      .mockRejectedValueOnce(new Error("network closed"))
      .mockResolvedValueOnce(undefined)

    ensureSessionStream("session-1", {
      queryClient: testQueryClient(),
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()
    await vi.advanceTimersByTimeAsync(400)
    await flushAsync()

    expect(subscribeToGoSessionStreamMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
      })
    )
  })

  it("reconnects a durable stream that closes without an error", async () => {
    vi.useFakeTimers()
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValue(sandboxAccess())
    subscribeToGoSessionStreamMock
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce(undefined)

    ensureSessionStream("session-1", {
      queryClient: testQueryClient(),
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()
    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(400)
    await flushAsync()

    expect(subscribeToGoSessionStreamMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
      })
    )
  })

  it("stops the direct stream as soon as the turn becomes idle", async () => {
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValueOnce(sandboxAccess())
    let signal: AbortSignal | undefined
    subscribeToGoSessionStreamMock.mockImplementationOnce(
      async ({ onEvent, signal: streamSignal }) => {
        signal = streamSignal
        onEvent?.(
          frame("turn_completed", {
            turn_id: "turn-1",
          })
        )
        await new Promise<void>((resolve) => {
          streamSignal.addEventListener("abort", () => resolve(), {
            once: true,
          })
        })
      }
    )

    ensureSessionStream("session-1", {
      queryClient: testQueryClient(),
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()

    expect(signal?.aborted).toBe(true)
    expect(
      useSessionRuntimeStore.getState().statusBySessionId["session-1"]
    ).toMatchObject({
      status: "idle",
      lastOutcome: "completed",
    })
  })

  it("stops the direct stream as soon as the turn fails", async () => {
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValueOnce(sandboxAccess())
    let signal: AbortSignal | undefined
    subscribeToGoSessionStreamMock.mockImplementationOnce(
      async ({ onEvent, signal: streamSignal }) => {
        signal = streamSignal
        onEvent?.(
          frame("turn_failed", {
            turn_id: "turn-1",
            error: "runtime failed",
          })
        )
        await new Promise<void>((resolve) => {
          streamSignal.addEventListener("abort", () => resolve(), {
            once: true,
          })
        })
      }
    )

    ensureSessionStream("session-1", {
      queryClient: testQueryClient(),
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()

    expect(signal?.aborted).toBe(true)
    expect(
      useSessionRuntimeStore.getState().statusBySessionId["session-1"]
    ).toMatchObject({
      status: "failed",
      lastOutcome: "failed",
    })
  })

  it("finishes the session when a final frame arrives before turn_completed", async () => {
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValueOnce(sandboxAccess())
    subscribeToGoSessionStreamMock.mockImplementationOnce(
      async ({ onEvent }) => {
        onEvent?.(
          frame("final", {
            event_id: "final-1",
            turn_id: "turn-1",
            text: "Done.",
          })
        )
      }
    )

    ensureSessionStream("session-1", {
      queryClient: testQueryClient(),
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()

    expect(
      useSessionRuntimeStore.getState().statusBySessionId["session-1"]
    ).toMatchObject({
      status: "idle",
      lastOutcome: "completed",
    })
    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledTimes(1)
  })

  it("does not open a direct stream while the session is idle", async () => {
    useSessionRuntimeStore.setState({
      statusBySessionId: {
        "session-1": {
          status: "idle",
          updatedAt: Date.now(),
        },
      },
    })

    ensureSessionStream("session-1", {
      queryClient: testQueryClient(),
      replay: { mode: "none" },
    })
    await flushAsync()

    expect(getSessionSandboxAccessMock).not.toHaveBeenCalled()
    expect(subscribeToGoSessionStreamMock).not.toHaveBeenCalled()
  })

  it("opens a new direct stream when the next agent turn starts", async () => {
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValue(sandboxAccess())
    subscribeToGoSessionStreamMock
      .mockImplementationOnce(async ({ onEvent }) => {
        onEvent?.(frame("turn_completed", { turn_id: "turn-1" }))
      })
      .mockResolvedValueOnce(undefined)

    const queryClient = testQueryClient()
    ensureSessionStream("session-1", {
      queryClient,
      replay: { mode: "from_turn_id_follow", turnId: "turn-1" },
    })
    await flushAsync()

    expect(subscribeToGoSessionStreamMock).toHaveBeenCalledTimes(1)
    markTurnActive()
    ensureSessionStream("session-1", {
      queryClient,
      replay: { mode: "from_turn_id_follow", turnId: "turn-2" },
    })
    await flushAsync()

    expect(subscribeToGoSessionStreamMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        replay: { mode: "from_turn_id_follow", turnId: "turn-2" },
      })
    )
  })

  it("reconnects with a full replay after resync_required", async () => {
    vi.useFakeTimers()
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValue(sandboxAccess())
    subscribeToGoSessionStreamMock
      .mockImplementationOnce(async ({ onEvent }) => {
        onEvent?.(frame("resync_required", { reason: "projection_gap" }))
      })
      .mockResolvedValueOnce(undefined)

    const queryClient = testQueryClient()
    ensureSessionStream("session-1", { queryClient })
    await flushAsync()
    await vi.advanceTimersByTimeAsync(400)
    await flushAsync()

    expect(queryClient.invalidateQueries).toHaveBeenCalled()
    expect(subscribeToGoSessionStreamMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        replay: { mode: "all" },
      })
    )
  })

  it("keeps repo-change invalidation on direct runtime frames", async () => {
    markTurnActive()
    getSessionSandboxAccessMock.mockResolvedValueOnce(sandboxAccess())
    subscribeToGoSessionStreamMock.mockImplementationOnce(
      async ({ onEvent }) => {
        onEvent?.(
          frame("repo.change_batch", {
            repo_id: "repo-1",
            paths: ["README.md"],
          })
        )
      }
    )

    const queryClient = testQueryClient()
    ensureSessionStream("session-1", { queryClient })
    await flushAsync()

    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["sandbox-runtime-review-diffs", "session-1"],
    })
  })
})

function markTurnActive() {
  useSessionRuntimeStore.setState({
    statusBySessionId: {
      "session-1": {
        status: "streaming",
        updatedAt: Date.now(),
      },
    },
  })
}

function sandboxAccess(
  overrides: Partial<SessionSandboxAccess> = {}
): SessionSandboxAccess {
  return {
    session_id: "session-1",
    sandbox_id: "sandbox-1",
    sandbox_base_url: "https://sandbox.example.test",
    token: "token",
    expires_at: "2026-06-20T12:00:00Z",
    scopes: ["repo:read", "stream:read"],
    ...overrides,
  }
}

function frame(
  event: string,
  data: Record<string, unknown>
): GoSessionStreamFrame {
  return {
    sessionId: "session-1",
    event,
    id: `${event}-1`,
    data,
  }
}

function testQueryClient() {
  return {
    invalidateQueries: vi.fn(() => Promise.resolve()),
  } as unknown as QueryClient & { invalidateQueries: Mock }
}

async function flushAsync() {
  await Promise.resolve()
  await Promise.resolve()
}
