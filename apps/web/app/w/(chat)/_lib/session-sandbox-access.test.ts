import { afterEach, describe, expect, it, vi, type Mock } from "vitest"

vi.mock("@/lib/api/client", () => ({
  api: {
    POST: vi.fn(),
  },
}))

import { api } from "@/lib/api/client"
import {
  clearSessionSandboxAccess,
  getCachedSessionSandboxAccess,
  getSessionSandboxAccess,
  isSessionSandboxAccessFresh,
} from "@/app/w/(chat)/_lib/session-sandbox-access"

const postMock = api.POST as unknown as Mock

describe("session sandbox access cache", () => {
  afterEach(() => {
    clearSessionSandboxAccess()
    postMock.mockReset()
    vi.useRealTimers()
  })

  it("dedupes access requests and reuses fresh cached access", async () => {
    postMock.mockResolvedValueOnce({
      data: sandboxAccess({ sessionId: "session-a" }),
      error: undefined,
    })

    const [first, second] = await Promise.all([
      getSessionSandboxAccess("session-a"),
      getSessionSandboxAccess("session-a"),
    ])
    const third = await getSessionSandboxAccess("session-a")

    expect(first).toBe(second)
    expect(third).toBe(first)
    expect(first.scopes).toContain("stream:read")
    expect(postMock).toHaveBeenCalledTimes(1)
    expect(postMock.mock.calls[0]?.[0]).toBe("/v1/sessions/{id}/sandbox-access")
  })

  it("refreshes access when the cached token is inside the expiry buffer", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-06-20T10:00:00Z"))
    postMock
      .mockResolvedValueOnce({
        data: sandboxAccess({
          sessionId: "session-a",
          token: "token-1",
          expiresAt: "2026-06-20T10:10:00Z",
        }),
        error: undefined,
      })
      .mockResolvedValueOnce({
        data: sandboxAccess({
          sessionId: "session-a",
          token: "token-2",
          expiresAt: "2026-06-20T11:00:00Z",
        }),
        error: undefined,
      })

    const first = await getSessionSandboxAccess("session-a")
    vi.setSystemTime(new Date("2026-06-20T10:06:00Z"))
    const second = await getSessionSandboxAccess("session-a")

    expect(first.token).toBe("token-1")
    expect(second.token).toBe("token-2")
    expect(postMock).toHaveBeenCalledTimes(2)
  })

  it("ignores cached access when the expected sandbox changes", async () => {
    postMock
      .mockResolvedValueOnce({
        data: sandboxAccess({
          sessionId: "session-a",
          sandboxId: "sandbox-old",
        }),
        error: undefined,
      })
      .mockResolvedValueOnce({
        data: sandboxAccess({
          sessionId: "session-a",
          sandboxId: "sandbox-new",
        }),
        error: undefined,
      })

    await getSessionSandboxAccess("session-a", {
      expectedSandboxId: "sandbox-old",
    })
    expect(
      getCachedSessionSandboxAccess("session-a", {
        expectedSandboxId: "sandbox-new",
      })
    ).toBeNull()
    const fresh = await getSessionSandboxAccess("session-a", {
      expectedSandboxId: "sandbox-new",
    })

    expect(fresh.sandbox_id).toBe("sandbox-new")
    expect(postMock).toHaveBeenCalledTimes(2)
  })

  it("treats near-expiry access as stale", () => {
    expect(
      isSessionSandboxAccessFresh(
        { expires_at: "2026-06-20T10:04:59Z" },
        Date.parse("2026-06-20T10:00:00Z")
      )
    ).toBe(false)
  })
})

function sandboxAccess({
  sessionId,
  sandboxId = "sandbox-a",
  token = "token",
  expiresAt = new Date(Date.now() + 60 * 60 * 1000).toISOString(),
}: {
  sessionId: string
  sandboxId?: string
  token?: string
  expiresAt?: string
}) {
  return {
    session_id: sessionId,
    sandbox_id: sandboxId,
    sandbox_base_url: "https://sandbox.example.test",
    token,
    expires_at: expiresAt,
    scopes: ["repo:read", "stream:read"],
  }
}
