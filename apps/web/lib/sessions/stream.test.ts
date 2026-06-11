import { describe, expect, it } from "vitest"
import {
  DEFAULT_RECONNECT_CONFIG,
  StreamBuffer,
  decideReconnect,
  type ReconnectConfig,
} from "./stream"

const config: ReconnectConfig = {
  maxAttempts: 4,
  baseDelayMs: 1_000,
  maxDelayMs: 8_000,
  maxTurnDurationMs: 60_000,
}

describe("decideReconnect", () => {
  it("does NOT reconnect once a terminal event was seen", () => {
    expect(
      decideReconnect(
        { reachedTerminal: true, attempt: 0, elapsedMs: 0 },
        config
      )
    ).toEqual({ reconnect: false })
  })

  it("reconnects with exponential backoff while the turn is plausibly running", () => {
    // This is the regression: before the fix, onerror rethrew and a clean
    // server close ended the stream forever. Now an unterminated stream resumes.
    expect(
      decideReconnect(
        { reachedTerminal: false, attempt: 0, elapsedMs: 1_000 },
        config
      )
    ).toEqual({ reconnect: true, delayMs: 1_000 })
    expect(
      decideReconnect(
        { reachedTerminal: false, attempt: 1, elapsedMs: 1_000 },
        config
      )
    ).toEqual({ reconnect: true, delayMs: 2_000 })
    expect(
      decideReconnect(
        { reachedTerminal: false, attempt: 2, elapsedMs: 1_000 },
        config
      )
    ).toEqual({ reconnect: true, delayMs: 4_000 })
  })

  it("caps the backoff delay at maxDelayMs", () => {
    const decision = decideReconnect(
      { reachedTerminal: false, attempt: 3, elapsedMs: 1_000 },
      config
    )
    // 1000 * 2^3 = 8000, capped at 8000.
    expect(decision).toEqual({ reconnect: true, delayMs: 8_000 })
  })

  it("stops after maxAttempts is reached", () => {
    expect(
      decideReconnect(
        { reachedTerminal: false, attempt: config.maxAttempts, elapsedMs: 1 },
        config
      )
    ).toEqual({ reconnect: false })
  })

  it("stops once the turn exceeds its plausible lifetime", () => {
    expect(
      decideReconnect(
        { reachedTerminal: false, attempt: 0, elapsedMs: 60_000 },
        config
      )
    ).toEqual({ reconnect: false })
  })

  it("uses sane defaults", () => {
    expect(DEFAULT_RECONNECT_CONFIG.maxTurnDurationMs).toBeGreaterThanOrEqual(
      600_000
    )
    expect(DEFAULT_RECONNECT_CONFIG.maxAttempts).toBeGreaterThan(0)
  })
})

describe("StreamBuffer", () => {
  it("accumulates live tokens", () => {
    const buf = new StreamBuffer()
    buf.liveToken("Hello")
    buf.liveToken(", ")
    expect(buf.liveToken("world")).toBe("Hello, world")
    expect(buf.text).toBe("Hello, world")
    expect(buf.length).toBe("Hello, world".length)
  })

  it("dedupes a full replay so reconnect appends only the new suffix", () => {
    // Before the disconnect we rendered "The quick brown ".
    const buf = new StreamBuffer("The quick brown ")
    buf.beginReplay()
    // Broker replays the WHOLE turn from the start, then sends the new tail.
    expect(buf.replayToken("The ")).toBe("") // already had it
    expect(buf.replayToken("quick ")).toBe("") // already had it
    expect(buf.replayToken("brown ")).toBe("") // already had it
    expect(buf.replayToken("fox ")).toBe("fox ") // new
    expect(buf.replayToken("jumps")).toBe("jumps") // new
    expect(buf.text).toBe("The quick brown fox jumps")
  })

  it("handles a boundary token that is partially old and partially new", () => {
    // We had 6 chars; the broker re-chunks so a replayed token straddles the
    // boundary ("ck brow" overlaps the last "ck " we had and adds "brow").
    const buf = new StreamBuffer("The qu")
    buf.beginReplay()
    expect(buf.replayToken("The ")).toBe("") // fully old (4 <= 6)
    // "quick" -> consumed 4..9, we had 6, so "ick" is new.
    expect(buf.replayToken("quick")).toBe("ick")
    expect(buf.text).toBe("The quick")
    // After catching up, subsequent tokens are appended live.
    expect(buf.replayToken(" fox")).toBe(" fox")
    expect(buf.text).toBe("The quick fox")
  })

  it("a final event replaces the buffer wholesale", () => {
    const buf = new StreamBuffer("partial answer")
    expect(buf.setFinal("the complete final answer")).toBe(
      "the complete final answer"
    )
    expect(buf.text).toBe("the complete final answer")
  })

  it("replayToken outside replay mode behaves as a live token", () => {
    const buf = new StreamBuffer("abc")
    expect(buf.replayToken("def")).toBe("def")
    expect(buf.text).toBe("abcdef")
  })

  it("a fresh buffer replaying everything appends it all (no prior text)", () => {
    const buf = new StreamBuffer()
    buf.beginReplay()
    expect(buf.replayToken("one ")).toBe("one ")
    expect(buf.replayToken("two")).toBe("two")
    expect(buf.text).toBe("one two")
  })
})
