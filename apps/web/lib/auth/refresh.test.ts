import { describe, expect, it } from "vitest"
import {
  RefreshCoordinator,
  hashToken,
  type RefreshFetcher,
  type RefreshOutcome,
} from "./refresh"
import type { SessionData } from "./session"

function session(token: string): SessionData {
  return {
    access_token: `access-for-${token}`,
    refresh_token: `next-for-${token}`,
    expires_at: Date.now() + 900_000,
  }
}

/** A fetcher that resolves after a tick so concurrent calls actually overlap. */
function deferredFetcher(): {
  fetcher: RefreshFetcher
  calls: string[]
} {
  const calls: string[] = []
  const fetcher: RefreshFetcher = (refreshToken) => {
    calls.push(refreshToken)
    return new Promise<RefreshOutcome>((resolve) => {
      setTimeout(
        () => resolve({ session: session(refreshToken), definitivelyRejected: false }),
        5
      )
    })
  }
  return { fetcher, calls }
}

describe("RefreshCoordinator", () => {
  it("dedupes concurrent refreshes for the SAME token into one fetch", async () => {
    const coord = new RefreshCoordinator()
    const { fetcher, calls } = deferredFetcher()

    const results = await Promise.all([
      coord.refresh("token-A", fetcher),
      coord.refresh("token-A", fetcher),
      coord.refresh("token-A", fetcher),
    ])

    // Only one underlying refresh ran...
    expect(calls).toEqual(["token-A"])
    // ...and every caller got that token's session.
    for (const r of results) {
      expect(r.session?.access_token).toBe("access-for-token-A")
    }
  })

  it("NEVER hands one token's session to a caller refreshing a different token", async () => {
    const coord = new RefreshCoordinator()
    const { fetcher, calls } = deferredFetcher()

    // Concurrent refreshes for DIFFERENT tokens. The previous (buggy) global
    // single-flight would have handed whoever-was-first's session to everyone.
    const [a, b] = await Promise.all([
      coord.refresh("user-A-token", fetcher),
      coord.refresh("user-B-token", fetcher),
    ])

    expect(a.session?.access_token).toBe("access-for-user-A-token")
    expect(b.session?.access_token).toBe("access-for-user-B-token")
    expect(a.session?.access_token).not.toBe(b.session?.access_token)
    // Both distinct tokens triggered their own fetch.
    expect(calls.sort()).toEqual(["user-A-token", "user-B-token"])
  })

  it("interleaves many distinct + duplicate tokens without cross-talk", async () => {
    const coord = new RefreshCoordinator()
    const { fetcher, calls } = deferredFetcher()

    const tokens = ["t1", "t2", "t1", "t3", "t2", "t1", "t4"]
    const results = await Promise.all(tokens.map((t) => coord.refresh(t, fetcher)))

    results.forEach((r, i) => {
      expect(r.session?.access_token).toBe(`access-for-${tokens[i]}`)
    })
    // One fetch per distinct token, regardless of how many duplicates raced.
    expect(calls.sort()).toEqual(["t1", "t2", "t3", "t4"])
  })

  it("clears the in-flight entry after completion so the next refresh re-fetches", async () => {
    const coord = new RefreshCoordinator()
    const { fetcher, calls } = deferredFetcher()

    await coord.refresh("token-X", fetcher)
    expect(coord.size).toBe(0)

    await coord.refresh("token-X", fetcher)
    // Sequential (non-overlapping) refreshes each run their own fetch.
    expect(calls).toEqual(["token-X", "token-X"])
  })

  it("clears the in-flight entry on failure so a transient failure isn't sticky", async () => {
    const coord = new RefreshCoordinator()
    let attempt = 0
    const fetcher: RefreshFetcher = async () => {
      attempt++
      if (attempt === 1) {
        return { session: null, definitivelyRejected: false }
      }
      return { session: session("token-Y"), definitivelyRejected: false }
    }

    const first = await coord.refresh("token-Y", fetcher)
    expect(first.session).toBeNull()
    expect(coord.size).toBe(0)

    const second = await coord.refresh("token-Y", fetcher)
    expect(second.session?.access_token).toBe("access-for-token-Y")
  })

  it("propagates definitivelyRejected through the coordinator", async () => {
    const coord = new RefreshCoordinator()
    const fetcher: RefreshFetcher = async () => ({
      session: null,
      definitivelyRejected: true,
    })

    const out = await coord.refresh("dead-token", fetcher)
    expect(out.session).toBeNull()
    expect(out.definitivelyRejected).toBe(true)
  })
})

describe("hashToken", () => {
  it("produces a stable 64-char hex sha-256 digest", async () => {
    const h = await hashToken("hello")
    expect(h).toMatch(/^[0-9a-f]{64}$/)
    // SHA-256("hello")
    expect(h).toBe(
      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
    )
  })

  it("maps different tokens to different keys", async () => {
    const a = await hashToken("token-A")
    const b = await hashToken("token-B")
    expect(a).not.toBe(b)
  })
})
