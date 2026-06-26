import { afterEach, describe, expect, it, vi } from "vitest"
import type { QueryClient } from "@tanstack/react-query"
import {
  clearSessionSandboxAccess,
  getCachedSessionSandboxAccess,
  getSessionSandboxAccess,
  isSessionSandboxAccessFresh,
} from "@/app/w/(chat)/_lib/session-sandbox-access"

describe("session sandbox access cache", () => {
  afterEach(() => {
    clearSessionSandboxAccess()
    vi.useRealTimers()
  })

  it("dedupes access requests and reuses fresh cached access", async () => {
    const queryClient = testQueryClient()
    queryClient.fetchQueryMock.mockResolvedValueOnce(
      sandboxAccess({ sessionId: "session-a" })
    )

    const [first, second] = await Promise.all([
      getSessionSandboxAccess("session-a", queryClient.client),
      getSessionSandboxAccess("session-a", queryClient.client),
    ])
    const third = await getSessionSandboxAccess("session-a", queryClient.client)

    expect(first).toBe(second)
    expect(third).toBe(first)
    expect(first.scopes).toContain("stream:read")
    expect(queryClient.fetchQueryMock).toHaveBeenCalledTimes(1)
  })

  it("refreshes access when the cached token is inside the expiry buffer", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-06-20T10:00:00Z"))
    const queryClient = testQueryClient()
    queryClient.fetchQueryMock
      .mockResolvedValueOnce(
        sandboxAccess({
          sessionId: "session-a",
          token: "token-1",
          expiresAt: "2026-06-20T10:10:00Z",
        })
      )
      .mockResolvedValueOnce(
        sandboxAccess({
          sessionId: "session-a",
          token: "token-2",
          expiresAt: "2026-06-20T11:00:00Z",
        })
      )

    const first = await getSessionSandboxAccess("session-a", queryClient.client)
    vi.setSystemTime(new Date("2026-06-20T10:06:00Z"))
    const second = await getSessionSandboxAccess(
      "session-a",
      queryClient.client
    )

    expect(first.token).toBe("token-1")
    expect(second.token).toBe("token-2")
    expect(queryClient.fetchQueryMock).toHaveBeenCalledTimes(2)
  })

  it("ignores cached access when the expected sandbox changes", async () => {
    const queryClient = testQueryClient()
    queryClient.fetchQueryMock
      .mockResolvedValueOnce(
        sandboxAccess({
          sessionId: "session-a",
          sandboxId: "sandbox-old",
        })
      )
      .mockResolvedValueOnce(
        sandboxAccess({
          sessionId: "session-a",
          sandboxId: "sandbox-new",
        })
      )

    await getSessionSandboxAccess("session-a", queryClient.client, {
      expectedSandboxId: "sandbox-old",
    })
    expect(
      getCachedSessionSandboxAccess("session-a", {
        expectedSandboxId: "sandbox-new",
      })
    ).toBeNull()
    const fresh = await getSessionSandboxAccess(
      "session-a",
      queryClient.client,
      {
        expectedSandboxId: "sandbox-new",
      }
    )

    expect(fresh.sandbox_id).toBe("sandbox-new")
    expect(queryClient.fetchQueryMock).toHaveBeenCalledTimes(2)
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

function testQueryClient() {
  const fetchQueryMock = vi.fn()
  return {
    client: { fetchQuery: fetchQueryMock } as unknown as QueryClient,
    fetchQueryMock,
  }
}

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
