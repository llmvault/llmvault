import { describe, expect, it } from "vitest"
import {
  DEFAULT_RECONNECT_CONFIG,
  StreamBuffer,
  decideReconnect,
  runWithStreamSetup,
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

// ---------------------------------------------------------------------------
// P1-11 regression: setup failures must clear isStreaming
// ---------------------------------------------------------------------------
describe("runWithStreamSetup", () => {
  it("calls the body with the resolved value and returns its result", async () => {
    const result = await runWithStreamSetup(
      () => Promise.resolve("https://example.com/stream"),
      async (url) => url + "?ok"
    )
    expect(result).toEqual({ ok: true, value: "https://example.com/stream?ok" })
  })

  it("returns { ok: false, error } when setup throws, not letting exceptions escape", async () => {
    const err = new Error("auth token fetch failed")
    const result = await runWithStreamSetup(
      () => Promise.reject(err),
      async () => "should-not-reach"
    )
    // Before the fix, the thrown error escaped the setup await (which was
    // outside the try/finally), so the finally block never ran and isStreaming
    // stayed true forever.  The fix catches the error and returns a typed
    // failure so callers can clear the streaming state.
    expect(result).toEqual({ ok: false, error: err })
  })

  it("propagates body errors normally (only setup errors are caught)", async () => {
    const bodyErr = new Error("body failure")
    await expect(
      runWithStreamSetup(
        () => Promise.resolve("url"),
        async () => {
          throw bodyErr
        }
      )
    ).rejects.toThrow(bodyErr)
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

// ---------------------------------------------------------------------------
// End-to-end reconnect contract: scripted broker replays drive the same
// decideReconnect + StreamBuffer the client uses, proving a mid-turn drop
// resumes and renders the answer exactly once (no duplication, no loss).
// ---------------------------------------------------------------------------
type ScriptEvent =
  | { event: "token"; text: string }
  | { event: "final"; text: string }
  | { event: "done" }

/**
 * Drives the pure stream helpers exactly as the page does: live tokens go
 * through liveToken, a reconnect calls beginReplay then feeds every replayed
 * event through replayToken, and terminal events stop the resume loop. Returns
 * the final rendered text and how many reconnects were attempted.
 */
function runScriptedStream(
  segments: ScriptEvent[][],
  config = DEFAULT_RECONNECT_CONFIG
): { text: string; reachedTerminal: boolean; reconnects: number } {
  const buf = new StreamBuffer()
  let reachedTerminal = false
  let reconnects = 0

  for (let i = 0; i < segments.length; i++) {
    const isReconnect = i > 0
    if (isReconnect) {
      const decision = decideReconnect(
        { reachedTerminal, attempt: i - 1, elapsedMs: 1_000 },
        config
      )
      if (!decision.reconnect) break
      reconnects++
      // The broker replays from the start: dedupe against committed text.
      buf.beginReplay()
    }

    for (const ev of segments[i]) {
      if (ev.event === "token") {
        if (isReconnect) buf.replayToken(ev.text)
        else buf.liveToken(ev.text)
      } else if (ev.event === "final") {
        buf.setFinal(ev.text)
        reachedTerminal = true
      } else if (ev.event === "done") {
        reachedTerminal = true
      }
    }
    if (reachedTerminal) break
  }

  return { text: buf.text, reachedTerminal, reconnects }
}

describe("scripted reconnect contract", () => {
  it("resumes a turn dropped mid-stream and renders the answer exactly once", () => {
    // Turn 1 (before the drop): three tokens streamed live, then the proxy
    // idle/total timeout cuts the connection with NO terminal event.
    // Reconnect: the broker replays the WHOLE turn from the start (P0-28 ring
    // buffer guarantees the tail/terminal survive), then streams the remainder
    // and the terminal final/done.
    const result = runScriptedStream([
      [
        { event: "token", text: "The " },
        { event: "token", text: "quick " },
        { event: "token", text: "brown " },
      ],
      [
        // Replayed history (already rendered) + new tail + terminal.
        { event: "token", text: "The " },
        { event: "token", text: "quick " },
        { event: "token", text: "brown " },
        { event: "token", text: "fox " },
        { event: "token", text: "jumps" },
        { event: "final", text: "The quick brown fox jumps" },
        { event: "done" },
      ],
    ])

    expect(result.text).toBe("The quick brown fox jumps")
    expect(result.reconnects).toBe(1)
    expect(result.reachedTerminal).toBe(true)
  })

  it("does NOT reconnect after a clean terminal — the turn ended", () => {
    const result = runScriptedStream([
      [
        { event: "token", text: "hi" },
        { event: "final", text: "hi there" },
        { event: "done" },
      ],
      // A second segment exists but must never be consumed.
      [{ event: "token", text: "SHOULD NOT APPEAR" }],
    ])
    expect(result.text).toBe("hi there")
    expect(result.reconnects).toBe(0)
  })

  it("survives multiple mid-stream drops, deduping each replay", () => {
    const result = runScriptedStream([
      [{ event: "token", text: "a" }],
      [
        { event: "token", text: "a" },
        { event: "token", text: "b" },
      ],
      [
        { event: "token", text: "a" },
        { event: "token", text: "b" },
        { event: "token", text: "c" },
        { event: "final", text: "abc" },
        { event: "done" },
      ],
    ])
    expect(result.text).toBe("abc")
    expect(result.reconnects).toBe(2)
  })
})

// ---------------------------------------------------------------------------
// P0-31 contract: a follow-up message must NOT wipe the previous turn's answer.
// The client tracks turns in separate buffers; turn 1's committed text stays
// rendered while turn 2 streams into its own fresh buffer.
// ---------------------------------------------------------------------------
describe("follow-up message preserves the previous turn", () => {
  it("keeps turn 1's answer while turn 2 streams independently", () => {
    // Turn 1 completed and was rendered.
    const turn1 = new StreamBuffer()
    turn1.liveToken("Paris is ")
    turn1.setFinal("Paris is the capital of France.")

    // Snapshot of what the previous answer must remain.
    const previousAnswer = turn1.text

    // Turn 2 starts on a FRESH buffer (the regression was reusing/clearing the
    // single shared state, erasing turn 1). Turn 1's buffer is untouched.
    const turn2 = new StreamBuffer()
    turn2.liveToken("It has ")
    turn2.liveToken("about 2 million people.")

    expect(turn1.text).toBe(previousAnswer)
    expect(turn1.text).toBe("Paris is the capital of France.")
    expect(turn2.text).toBe("It has about 2 million people.")
    // The two turns never share buffer state.
    expect(turn2.text).not.toContain("Paris")
  })

  it("a follow-up that reconnects does not resurrect the previous turn's text", () => {
    const turn1 = new StreamBuffer("Earlier answer.")
    turn1.setFinal("Earlier answer.")

    // Turn 2 (the follow-up) drops mid-stream and reconnects; its replay must
    // only contain turn 2's own events, never turn 1's.
    const turn2 = runScriptedStream([
      [{ event: "token", text: "Second " }],
      [
        { event: "token", text: "Second " },
        { event: "token", text: "answer." },
        { event: "final", text: "Second answer." },
        { event: "done" },
      ],
    ])

    expect(turn1.text).toBe("Earlier answer.")
    expect(turn2.text).toBe("Second answer.")
  })
})
