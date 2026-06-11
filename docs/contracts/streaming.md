# Streaming contract: Rust broker → Go SSE proxy → web client

This document specifies the end-to-end streaming contract for a web session turn,
derived from the code as it currently stands (post-P0). It is the reference for
the contract tests in:

- Rust: `sandboxes/runtime/crates/api/src/http_gateway.rs` (`#[cfg(test)] mod tests`)
- Go: `internal/employeeruntime/client_test.go`, `internal/handler/employees_session_stream_test.go`
- Web: `apps/web/lib/sessions/stream.test.ts`

The three layers are:

1. **Rust broker** — `HttpStreamBroker` in `crates/api/src/http_gateway.rs`. Owns
   per-stream broadcast channels + a bounded replay history. Streamed over SSE by
   `stream_response`.
2. **Go SSE proxy** — `EmployeeHandler.StreamSession` in
   `internal/handler/employees_session_messages.go`, using the runtime client
   `internal/employeeruntime/client.go` + `client_stream.go`. Authenticates the
   browser, then byte-forwards the runtime SSE body.
3. **Web client** — `startSessionStream` in `apps/web/app/w/sessions/page.tsx`,
   built on the pure helpers in `apps/web/lib/sessions/stream.ts`.

---

## 1. Event model

Each broker stream carries a sequence of `HttpStreamEvent { event: String, payload: Value }`
serialized as SSE frames (`event: <name>\ndata: <json>\n\n`).

### Event types (producer: Rust runtime/agent)

| event             | meaning                                              | terminal |
|-------------------|------------------------------------------------------|----------|
| `turn_started`    | a turn began                                         | no       |
| `thinking`        | reasoning delta (stripped from the visible answer)   | no       |
| `token`           | assistant text delta — appended to the visible answer| no       |
| `tool_call`       | a tool invocation started                            | no       |
| `tool_result`     | a tool invocation completed                          | no       |
| `model_usage`     | per-request token usage (billing/observability)      | no       |
| `subagent_*`      | delegate lifecycle (`subagent_errored`, …)           | no       |
| `session_waiting` | this stream's message was queued/merged behind an in-flight turn, or the turn is blocked on delegates/background processes; the stream stays open | no |
| `resync`          | proxy-synthesized after a slow-consumer `Lagged` gap; followed by replayed history (§3)               | no       |
| `final`           | the authoritative full text of the turn              | **yes**  |
| `done`            | the run is complete; consumers stop                  | **yes**  |
| `error`           | the turn failed (`max_turns_exhausted`, model errors)| **yes**  |

`final` carries the complete answer text and is authoritative: a client that has
been accumulating `token`s **replaces** its buffer with `final.text`
(`StreamBuffer.setFinal`). `done` is the run-level terminator.

### Ordering guarantees

- `turn_started` precedes the body of a turn.
- `token`/`thinking`/`tool_*`/`model_usage` interleave during the turn.
- `final` (if emitted) precedes `done`.
- `done` is always last on a completed stream. `error` is terminal and the client
  treats it as end-of-turn (no resume).

The web client stops the resume loop on `done`, `final`-then-`done`, or `error`.
`session_waiting` is explicitly **non-terminal**: it tells the client "stay open,
output is coming on this stream" (see §6).

---

## 2. Replay history & ring buffer (P0-28)

`StreamState.history` is a `VecDeque` bounded to `HISTORY_CAPACITY = 512`. On
`publish`, when full, the **oldest** event is evicted (`pop_front`) and the new
event is pushed at the back. This is the critical correctness property: a
token-heavy turn (> 512 events) must still leave the terminal `final`/`done` at
the tail replayable. A first-512 buffer (the old bug) would evict the terminals
and a reconnecting/late subscriber would hang on `recv()` forever.

`subscribe(stream_id)` returns `(history_snapshot, broadcast::Receiver)`:

- the full current history (oldest-retained → newest), then
- a live receiver for everything published afterward.

`stream_response` first yields every history event, then loops on `recv()`,
breaking on `done`.

### Subscriber timing semantics

- **Late subscriber, turn still running:** replays history tail, then continues
  live. Tests: `reconnect_mid_stream_replays_tail_then_continues_live`.
- **Subscriber after `done`:** history ends with `done` (and contains `final`);
  the subscriber stops from the replayed terminal alone — it must **not** wait on
  the live channel, which has no further events (the broadcast sender is still
  held by the broker so it is not `Closed`). Test:
  `subscriber_after_done_replays_terminal_events_and_does_not_hang`.
- **Late subscriber after history overflow:** the oldest tokens are gone but the
  newest tokens + `final` + `done` are retained. Test:
  `late_subscriber_after_history_overflow_still_sees_terminal_events`.

### Per-event sequence numbers

Every published event is tagged with a monotonically increasing per-stream
sequence number (`SeqEvent { seq, inner }`). The sequence is internal — it never
appears on the SSE wire — but it lets a subscriber that fell behind resync
precisely against the replay history (see §3). History therefore stores
`SeqEvent`s; `subscribe` returns `(Vec<SeqEvent>, broadcast::Receiver<SeqEvent>)`,
`history_after(stream_id, seq)` returns the buffered events with `seq` strictly
greater than the argument, and `history_snapshot` returns the whole retained tail.

### State eviction after `done` + grace (P2-1)

When the terminal `done` is published, the stream's `done_at` is stamped. The
per-stream state (and the `session_streams`/`active_session_streams` entries that
still point at it) is retained for `STREAM_EVICTION_GRACE = 120s` so a late
subscriber can still replay the terminals, then reclaimed. Reclamation is
opportunistic — performed under the `streams` lock during `create_stream` and
`publish` (one sweep per turn / per published event), with no background task. A
session that has re-registered onto a newer stream keeps its newer mapping; only
mappings still pointing at the evicted stream are dropped. Lock order is always
`streams` → `session_streams` → `active_session_streams`. Tests:
`finished_stream_state_is_evicted_after_grace_on_next_activity`,
`reregistered_session_keeps_newer_mapping_when_old_stream_evicted`.

---

## 3. Slow-consumer / Lagged resync (P2-2)

The broadcast channel capacity is 256 (`broadcast::channel(256)`). A subscriber
that falls more than 256 events behind receives `RecvError::Lagged(n)`.
`stream_response` (via the testable `replay_stream` core) handles this by
**resyncing from the replay history** rather than silently skipping the gap:

1. It tracks `last_seq`, the highest sequence number delivered so far.
2. On `Lagged(skipped)` it emits a synthetic **`resync`** SSE frame
   (`event: resync`, payload `{ stream_id, skipped, from_seq }`) so the client
   knows a gap occurred.
3. It then replays every buffered history event with `seq > last_seq`
   (`history_after`), in order, advancing `last_seq` — exactly the events the
   broadcast dropped, with no duplication of the already-delivered prefix.
4. If the replayed range contains `done`, the stream terminates as usual.

The only events that cannot be recovered are ones already evicted from the ring
history (a consumer more than `HISTORY_CAPACITY` events behind); the newest
`HISTORY_CAPACITY` events, including the terminals, are always replayable.

`Closed` still ends the stream. Tests:
`lagged_subscriber_resyncs_skipped_range_from_history` (the test the contract
agent deliberately skipped), `non_lagged_stream_emits_no_resync_frame`.

The `resync` frame is non-terminal; web clients should treat it as a hint to
rely on their existing replay-dedupe (§7) for any frames that follow.

---

## 4. Auth (stream tokens)

The browser **never** receives a raw bearer access token. SSE is always routed
through the web's own cookie-auth proxy (`/api/proxy/...`), which authenticates
the browser via the httpOnly `__session` cookie and injects the user's bearer
server-side (P2-27). Three credentials are in play across the hops:

- **Browser → Next.js proxy:** the httpOnly `__session` cookie. The web client
  resolves the backend-relative stream path through `proxiedStreamURL`
  (`apps/web/lib/sessions/stream.ts`), which prefixes it with `/api/proxy` and
  **rejects absolute URLs** (`http(s)://…` / protocol-relative `//…`) so a
  credential is never attached to a cross-origin request from client JS. The
  fetch uses `credentials: "include"` with only an `Accept: text/event-stream`
  header — no `Authorization` header is set client-side. There is no
  `/api/auth/stream-token` endpoint: the previous design that handed the bearer
  to client JS has been removed.
- **Proxy → Go backend (`StreamSession`):** the user's JWT bearer (added by the
  proxy from the session) **plus** an HMAC-signed **web stream token**, minted by
  `signedWebStreamURL` / `signWebStreamToken` (`employees_session_stream_token.go`).
  Payload is `sessionID|streamID|expiresAt`, signed with `compileDeps.SigningKey`
  (HMAC-SHA256), base64url-encoded. TTL is `webStreamTokenLifetime = 1h`.
  The token is extracted by `extractWebStreamToken`:
  1. **Preferred:** `X-Stream-Token: <token>` request header — the token stays
     out of access logs and Sentry breadcrumbs.
  2. **Fallback (backward compat):** `?token=<token>` query parameter — accepted
     for clients that cannot set headers (e.g. `EventSource` without custom
     headers); the `?token=` value is scrubbed from Sentry events by the
     `beforeSend` hook.
  `verifyWebStreamToken` checks the session/stream binding, expiry, and a
  constant-time HMAC compare. A token for stream A cannot be replayed against
  stream B or session B. `StreamSession` requires both the org context (from the
  bearer) and a valid stream token.
- **Go backend → Rust runtime:** the per-sandbox **runtime bearer secret**
  (decrypted from `EncryptedRuntimeSecret`), sent as `Authorization: Bearer …` by
  `openStream`. The browser never sees this.

The Next.js proxy and the Go backend together form the trust boundary: the proxy
authenticates the browser by cookie and adds the bearer; the backend verifies the
web stream token, then opens the runtime stream with the runtime secret and
byte-forwards the body.

Tests:
- Web (`stream.test.ts`, `proxiedStreamURL` suite): a backend-relative signed
  path is prefixed with `/api/proxy`; an already-proxied path is unchanged;
  absolute `http(s)://` and protocol-relative `//` URLs are rejected.
- Go (`employees_session_messages_test.go`): `StreamSession` rejects a missing /
  forged / cross-stream / expired web stream token.

---

## 5. Go SSE proxy: no 2-minute kill (P0-29)

`employeeruntime.Client` has **two** HTTP clients:

- `http` — general requests, `Timeout: 2m` (Go's `Client.Timeout` covers the
  *entire* body read).
- `stream` — built by `newStreamingHTTPClient()` with **`Timeout: 0`** and only
  transport-level bounds (`DialContext` 10s, `TLSHandshakeTimeout` 10s,
  `ResponseHeaderTimeout` 30s, `DisableCompression`). Liveness during the body is
  enforced by the **request context** and the broker's 15s keep-alive frames.

`openStream` (used by `StreamHTTP`) uses `streamClient()`. This is the fix for the
old bug where the SSE proxy reused the 2m-timeout client and cut every turn longer
than ~2 minutes mid-answer.

The proxy forwards the body with `copySSEStream` (`employees_session_messages.go`),
which reads line-by-line and `Flush`es at each SSE event boundary (blank line), so
tokens reach the browser progressively and are never buffered for the whole turn.
A clean EOF returns `nil`; a mid-body read error propagates as an error (ending the
request scope), after the already-received bytes have been flushed.

Tests:
- `TestClientStreamHTTPNotBoundByGeneralTimeout` — a 50ms general timeout, a
  server that streams slowly for ~300ms; the stream completes in full (fails
  without the dedicated client).
- `TestClientStreamHTTPContextCancelStopsBody` — context cancel unblocks the read
  (untimed ≠ uncancellable).
- `TestClientStreamHTTPDisconnectMidBody` — server closes mid-stream; the partial
  body is delivered with no synthesized terminal.
- `TestCopySSEStreamForwardsSlowChunksProgressively` — progressive per-event flush
  for a slowly-streamed body.
- `TestCopySSEStreamPropagatesMidBodyDropError` — a transport drop propagates as
  an error after the pre-drop events were flushed; no terminal `done` is fabricated.

---

## 6. Queued / follow-up message semantics (P1-30, P0-31)

When a follow-up message arrives for a session whose turn is still in flight,
`SessionCoordinator.submit_or_queue` returns `Submission::Queued`. The runtime
(`handle_inbound`) then:

- emits `session_waiting` (reason `merged_into_active_turn`) on **both** the
  follow-up's request and response streams, so the client's freshly opened stream
  stays open instead of receiving a bare `done` (the old bug, which showed
  nothing), and
- merges the queued inbound into the active turn (`merge_queued_inbound`); the
  merged turn's terminal output is bridged back so queued streams see it.

`session_waiting` is also emitted while a parent turn is blocked on delegates
(reason `delegated_tasks`) or background processes (`background_processes`).

### Web client: previous turn is preserved (P0-31)

Each turn streams into its **own** `StreamBuffer`; a follow-up never reuses or
clears turn 1's committed text. On stream completion the client invalidates the
`employee-session-events` (and `employee-sessions`) React Query caches
(`refetchPersistedSession`) so the previous turn's answer is refetched from the
durable store rather than being lost when the live state for the new turn starts.

Tests (`stream.test.ts`):
- `follow-up message preserves the previous turn` — turn 1's buffer is untouched
  while turn 2 streams independently; a reconnecting follow-up never resurrects
  turn 1's text.

---

## 7. Reconnect / resume (P0-30) — web client

`startSessionStream` drives a reconnect loop. A single SSE connection ending
**without** a terminal event (e.g. an upstream proxy idle/total timeout, a network
blip) is treated as "the turn is plausibly still running" and resumed against the
**same signed stream URL**:

- `decideReconnect(state, config)` (pure, in `stream.ts`) decides whether to
  reconnect and the backoff delay. It reconnects only while:
  `!reachedTerminal && attempt < maxAttempts && elapsedMs < maxTurnDurationMs`.
  Backoff is exponential from `baseDelayMs`, capped at `maxDelayMs`. Defaults:
  `maxAttempts 6`, `baseDelayMs 1s`, `maxDelayMs 15s`,
  `maxTurnDurationMs 610s` (matching the Slack subscriber ceiling).
- On reconnect, the broker replays history from the start. `StreamBuffer`
  deduplicates: `beginReplay()` then `replayToken(t)` returns only the
  genuinely-new suffix (handling a boundary token that is partially
  already-rendered / partially new). `liveToken` appends verbatim once caught up.
  `final` replaces the buffer wholesale.

The loop stops on `done`, `final`-then-`done`, `error`, controller abort, or when
`decideReconnect` says stop.

Tests (`stream.test.ts`):
- `decideReconnect` suite — terminal, backoff, caps, max attempts, max duration.
- `StreamBuffer` suite — live append, full-replay dedupe, boundary token,
  `setFinal`, fresh buffer.
- `scripted reconnect contract` — end-to-end: a turn dropped mid-stream resumes
  and renders the answer exactly once; multiple drops each dedupe; a clean
  terminal never reconnects.
- `runWithStreamSetup` — setup (token/URL) failures are caught so `isStreaming`
  is cleared instead of the composer sticking (P1-11).

---

## 8. End-to-end happy path (web session turn)

1. `POST /v1/employees/{id}/sessions/messages` → `PostHTTPMessage` →
   broker `inject_message` creates a stream, registers the session, returns
   `stream_id` + signed `stream_url`.
2. Browser opens `GET …/streams/{streamID}?token=…` via the Go proxy.
3. Proxy verifies the web stream token, opens the runtime SSE with the bearer
   (untimed stream client), forwards frames flushing per event boundary.
4. Runtime publishes `turn_started`, `token`*, optional `tool_*`/`model_usage`,
   then `final`, then `done`. Every published event is also appended to the
   bounded replay history.
5. Browser accumulates `token`s, replaces on `final`, stops on `done`. On any
   non-terminal disconnect it reconnects, replays history, dedupes, and continues.
6. On completion the browser refetches persisted session events so the answer is
   durable and a subsequent follow-up turn does not erase it.
