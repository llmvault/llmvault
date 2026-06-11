/**
 * Pure stream-lifecycle helpers for the web session SSE client.
 *
 * The browser streams a turn's tokens over an EventSource-style connection. The
 * connection can be cut mid-turn (e.g. an upstream proxy idle/total timeout)
 * even though the agent turn is still running and the broker still has the rest
 * of the answer available for replay. These helpers encode, as pure testable
 * functions, the two decisions that make resume correct:
 *
 *  1. whether (and how long to back off before) reconnecting to the same signed
 *     stream URL when the connection ends without a terminal event, and
 *  2. how to deduplicate the broker's replayed history so a reconnect appends
 *     only the genuinely-new suffix instead of doubling the rendered text.
 */

/** Terminal SSE events after which the turn is finished and no resume is needed. */
export type StreamTerminal = "done" | "final"

export interface ReconnectConfig {
  /** Maximum number of reconnect attempts before giving up. */
  maxAttempts: number
  /** Base backoff in milliseconds (attempt 1). */
  baseDelayMs: number
  /** Upper bound for a single backoff delay in milliseconds. */
  maxDelayMs: number
  /**
   * How long a turn is allowed to run, end to end, before we stop trying to
   * resume. Guards against reconnecting forever against a stuck server.
   */
  maxTurnDurationMs: number
}

export const DEFAULT_RECONNECT_CONFIG: ReconnectConfig = {
  maxAttempts: 6,
  baseDelayMs: 1_000,
  maxDelayMs: 15_000,
  // The Slack subscriber path tolerates ~610s turns; match that ceiling so the
  // web client resumes for the full plausible lifetime of a turn.
  maxTurnDurationMs: 610_000,
}

export interface ReconnectState {
  /** True once a `done`/`final` (or an unrecoverable error) has been seen. */
  reachedTerminal: boolean
  /** Number of reconnects already attempted for this turn. */
  attempt: number
  /** Wall-clock ms since the turn's stream first opened. */
  elapsedMs: number
}

export type ReconnectDecision =
  | { reconnect: false }
  | { reconnect: true; delayMs: number }

/**
 * Decide whether to reconnect after the SSE connection ended.
 *
 * We reconnect only while the turn is *plausibly still running*: the stream
 * ended without a terminal event, we still have attempts left, and the turn has
 * not exceeded its plausible lifetime. Backoff grows exponentially from
 * `baseDelayMs`, capped at `maxDelayMs`.
 */
export function decideReconnect(
  state: ReconnectState,
  config: ReconnectConfig = DEFAULT_RECONNECT_CONFIG
): ReconnectDecision {
  if (state.reachedTerminal) return { reconnect: false }
  if (state.attempt >= config.maxAttempts) return { reconnect: false }
  if (state.elapsedMs >= config.maxTurnDurationMs) return { reconnect: false }

  const delayMs = Math.min(
    config.maxDelayMs,
    config.baseDelayMs * 2 ** state.attempt
  )
  return { reconnect: true, delayMs }
}

/**
 * Tracks how much streamed text has already been committed to the UI so that a
 * reconnect — which replays the broker's buffered history from the start — can
 * skip everything it has already rendered and append only the new suffix.
 *
 * Usage:
 *  - Call {@link liveToken} for tokens from the *current* live connection.
 *  - On reconnect, call {@link beginReplay} once, then feed every replayed token
 *    through {@link replayToken}; each call returns the substring (if any) that
 *    is genuinely new and should be appended to the visible buffer.
 */
export class StreamBuffer {
  /** Full accumulated visible text. */
  private committed = ""
  /** During a replay, how many committed chars we have re-consumed so far. */
  private replayConsumed = 0
  private replaying = false

  constructor(initial = "") {
    this.committed = initial
  }

  get text(): string {
    return this.committed
  }

  get length(): number {
    return this.committed.length
  }

  /** Append a token from the current live connection. Returns the new total. */
  liveToken(token: string): string {
    this.replaying = false
    this.committed += token
    return this.committed
  }

  /**
   * Replace the buffer wholesale (used for a `final` event, which carries the
   * authoritative full text of the turn).
   */
  setFinal(text: string): string {
    this.replaying = false
    this.committed = text
    return this.committed
  }

  /** Mark the start of a replay after a reconnect. */
  beginReplay(): void {
    this.replaying = true
    this.replayConsumed = 0
  }

  /**
   * Feed a replayed token. Returns the genuinely-new suffix to append to the
   * visible buffer, or "" if this token was already rendered before the
   * disconnect. Handles the boundary token that is partially old / partially
   * new (e.g. broker re-chunking) by char offset against what we already had.
   */
  replayToken(token: string): string {
    if (!this.replaying) {
      // Not in replay mode — treat as a normal live token.
      this.liveToken(token)
      return token
    }

    const alreadyHave = this.committed.length
    const start = this.replayConsumed
    const end = start + token.length
    this.replayConsumed = end

    if (end <= alreadyHave) {
      // Entire token was already rendered before the disconnect.
      return ""
    }

    // The portion of this token beyond what we already had is new.
    const newPortionStart = Math.max(0, alreadyHave - start)
    const suffix = token.slice(newPortionStart)
    if (suffix) {
      this.replaying = false // caught up; remaining tokens are live again.
      this.committed += suffix
    }
    return suffix
  }
}
