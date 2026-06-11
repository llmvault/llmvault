import type { SessionData } from "@/lib/auth/session"

/**
 * Single-flight coordination for refresh-token rotation.
 *
 * Background: the backend treats each refresh token as single-use and rotates
 * it on every refresh. If the web tier fires multiple concurrent refreshes for
 * the SAME token (the proxy + the stream-token route + parallel requests), the
 * naive approach burns the single-use token more than once.
 *
 * A previous implementation deduped with a single process-global promise that
 * IGNORED which token was being refreshed. Under concurrent load, user B would
 * await user A's in-flight refresh and receive A's freshly rotated session —
 * a silent cross-account takeover. We MUST key the in-flight map by the token
 * being refreshed so a promise started for one token is never handed to a
 * caller presenting a different token.
 */

/**
 * Outcome of a refresh attempt.
 *
 * `session` is the rotated pair on success, or null on any failure.
 * `definitivelyRejected` is true ONLY when the backend authoritatively rejected
 * the refresh token (HTTP 401 — dead even after the backend grace window). It
 * is false for transient failures (network errors, 5xx) so callers don't
 * force-logout users on a blip or a lost concurrent-rotation race.
 */
export type RefreshOutcome = {
  session: SessionData | null
  definitivelyRejected: boolean
}

export type RefreshFetcher = (refreshToken: string) => Promise<RefreshOutcome>

/**
 * SHA-256 hex digest of a string, used as the single-flight map key so the raw
 * refresh token never becomes a map key (and so two different tokens can never
 * collide onto the same in-flight promise).
 */
export async function hashToken(token: string): Promise<string> {
  const data = new TextEncoder().encode(token)
  const digest = await crypto.subtle.digest("SHA-256", data)
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("")
}

/**
 * Coordinates concurrent refreshes so that:
 *   - at most one refresh is in flight per distinct refresh token, and
 *   - a caller presenting token X can only ever receive the result of a
 *     refresh started for token X (never another token's session).
 */
export class RefreshCoordinator {
  private inFlight = new Map<string, Promise<RefreshOutcome>>()

  /**
   * Returns the in-flight refresh for `refreshToken` if one exists, otherwise
   * starts one with `fetcher`. The entry is keyed by sha256(refreshToken) and
   * removed in `finally`, so a failed/expired refresh doesn't poison future
   * attempts for the same token.
   */
  async refresh(
    refreshToken: string,
    fetcher: RefreshFetcher
  ): Promise<RefreshOutcome> {
    const key = await hashToken(refreshToken)

    const existing = this.inFlight.get(key)
    if (existing) return existing

    const promise = fetcher(refreshToken).finally(() => {
      this.inFlight.delete(key)
    })
    this.inFlight.set(key, promise)
    return promise
  }

  /** Number of distinct refreshes currently in flight (test/diagnostics). */
  get size(): number {
    return this.inFlight.size
  }
}

/**
 * Process-wide coordinator shared by the proxy and the stream-token route so
 * a refresh triggered by one is reused by the other (within the same Node
 * process) instead of double-spending the single-use token.
 */
export const refreshCoordinator = new RefreshCoordinator()
