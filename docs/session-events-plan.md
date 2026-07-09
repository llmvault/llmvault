# Session Notices: backend→frontend push over Redis

**Status: PLANNED 2026-07-09 — not built.**

Goal: when a session is open, the frontend holds one subscription to a Go-API
stream; backend components publish typed events to Redis and the stream
forwards them. First consumers: canvas artifact syncs (auto-refresh + auto-open
the Canvas panel) and org credit changes (no polling/refetch loops). The bus is
generic — any future "something changed, react now" signal rides the same
channel.

## What already exists (verified 2026-07-09)

The design is mostly assembly, not construction:

- `internal/runtimestream` is a complete Redis-backed session event bus:
  per-session pub/sub channel `runtime_session:{id}:live`
  (`LiveChannel`, `event.go:151`), `LiveMessage{Kind, SessionID, ...}` envelope
  (`event.go:48`), publishers on both the api (WS ingress `Store.Append`) and
  worker (projector `PublishCommitted`) processes.
- `internal/handler/sessions_stream.go` is a full SSE handler over that channel
  (replay + live tail + keepalive) but is **not mounted to any route**. The
  frontend's live transcript instead streams **directly from the sandbox**
  (`apps/web/app/w/(chat)/_lib/go-session-stream.ts` →
  `{sandbox_base_url}/sessions/{id}/stream`).
- Parallel per-session SSE connections are already the norm: `name-updates`
  (SSE through the Next `/api/proxy`, cookie auth,
  `sessions_name_updates.go:33`) and sheet-live
  (`use-sheet-live.ts`, minted token, direct to Go API).
- `internal/sheets/events.go` is the reference service publisher
  (`EventPublisher` interface + `RedisEventPublisher`, echo-suppression via
  `WithMutationID`).
- `internal/cache/invalidation.go:118` is the reference long-lived subscriber
  loop with reconnect/backoff.
- Artifact sync (`POST /internal/agents/{agentID}/canvas/artifacts/sync` →
  `canvasartifact.Service.SyncArtifactForAgent`) already receives an optional
  `SessionID` from the runtime and persists `OrgID`/`CanvasProjectID`/
  `SourceSessionID` — everything needed to target the notification.
- Credit debits happen in the **worker** (asynq batch,
  `internal/tasks/billing_batch_debits.go:15`), so cross-process fan-out via
  Redis is mandatory, not optional.
- In-session cost is **already live** (sandbox stream `model_usage` frames →
  `usageBySessionId` in the session runtime store). The gap is **org-level
  credits** (billing settings, dashboard), which are plain REST with no
  polling today.

## Architecture

```
canvas sync handler (api)  ──┐
billing batch task (worker) ─┤  PUBLISH LiveMessage{Kind:"notice"}
future publishers ───────────┘        │
                     session-scoped: runtime_session:{sessionID}:live
                     org-scoped:     org_notice:{orgID}          (new)
                                      │
              GET /v1/sessions/{id}/notices   (api, SSE, new slim handler)
              subscribes BOTH channels, forwards Kind=="notice" only
                                      │
              frontend session-notices client (one per open session)
              switch(notice.type) → invalidate queries / open panel / patch stores
```

### Decisions (with alternatives considered)

1. **New slim SSE endpoint `GET /v1/sessions/{id}/notices`**, not mounting the
   existing full `SessionHandler.Stream`. The full handler replays and forwards
   runtime frames the frontend already gets from the sandbox connection —
   mounting it would double-deliver every token. The notices handler reuses the
   same store/channel and `authorizeSession`, forwards only `Kind=="notice"`.
   The unmounted `Stream` handler stays as-is: it is the future path for moving
   the transcript off the sandbox connection entirely, and this plan must not
   preclude that (same channel, same envelope — notices would ride along free).
2. **Notices are hints, not state.** Fire-and-forget pub/sub; no durable notice
   log, no replay. Missed-event mitigation: on every notices (re)connect the
   frontend runs the same invalidations it would run on receipt (cheap; React
   Query dedupes). This matches the sheets-live model.
3. **Org-scoped events get their own channel** (`org_notice:{orgID}`) rather
   than fanning out to every session of the org at publish time. The per-session
   SSE handler subscribes to both channels in one `Subscribe` call, so the
   frontend still holds a single connection.
4. **Transport: SSE through the Next `/api/proxy` with cookie auth**, like
   name-updates — no minted token needed (`authorizeSession` on the Go side).
   Sheets' minted-token/direct-connection approach exists for its own reasons;
   notices are low-volume and fine through the proxy.
5. **Auto-open policy**: `artifact.synced` opens the Canvas panel only if the
   `design` view is not already among the session's open panel tabs, and always
   targets the synced artifact for selection. Mirrors the existing
   `openBrowserURL`/`openSubagentRun` auto-open precedents.

### Event envelope

`LiveMessage` gains `Kind: "notice"` and a `Notice` field:

```go
type Notice struct {
    Type        string          `json:"type"`         // "artifact.synced", "credits.updated", ...
    OrgID       uuid.UUID       `json:"org_id"`
    SessionID   *uuid.UUID      `json:"session_id,omitempty"`
    Data        json.RawMessage `json:"data"`
    PublishedAt time.Time       `json:"published_at"`
}
```

SSE frame: event name `session.notice`, data = the `Notice` JSON. New types are
added by defining a `Type` constant + payload struct next to the publisher; the
handler and transport never change.

**v1 event catalog:**

| type | scope | payload | published from |
|---|---|---|---|
| `artifact.synced` | session | `artifact_id, project_id, slug, name, artifact_type, created (bool)` | api: canvasartifact service after successful sync, only when `SessionID != nil` |
| `credits.updated` | org | `balance` (post-debit) | worker: billing batch after ledger write; later also subscription renewals |

## Workstreams

### WS1 — Bus: notice kind + org channel (`internal/runtimestream`)
- Add `Kind:"notice"`, the `Notice` struct, `OrgNoticeChannel(orgID)` naming.
- `Store.PublishNotice(ctx, sessionID, notice)` → session live channel;
  `Store.PublishOrgNotice(ctx, orgID, notice)` → org channel. Plain `PUBLISH`
  (no XADD — notices are not durable runtime events and must not enter the
  projector's stream).
- Unit tests beside existing store tests (miniredis pattern already in package).

### WS2 — SSE endpoint (`internal/handler`)
- New `SessionHandler.Notices`: `authorizeSession` → load session (gives
  OrgID) → `Subscribe(LiveChannel(id), OrgNoticeChannel(orgID))` → forward
  `Kind=="notice"` as `session.notice`, 15s keepalive, close on ctx done.
  Structure copied from `sessions_stream.go` minus replay/poll.
- Mount `GET /v1/sessions/{id}/notices` in `serve_routes_session.go`.
- Regenerate the OpenAPI spec if the endpoint is declared there (SSE endpoints
  may be documented-only; follow how `name-updates` is specced — the frontend
  client is hand-written either way, per the streaming exemption).

### WS3 — Artifact publisher (`internal/canvasartifact`)
- `Service.WithPublisher(p)` mirroring `sheets.Service` — interface, not a
  concrete Redis dependency, so tests stub it.
- In `SyncArtifactForAgent`, after commit: if `req.SessionID != nil`, publish
  `artifact.synced` with the payload above (`created` from the service's
  create-vs-update knowledge). Sync must never fail because publish failed:
  log and continue.
- Wire the Redis publisher in bootstrap where the canvas service is built.
- Note: the new `canvas artifact watch` auto-sync makes this chatty (one sync
  per quiescent edit burst). That cadence (~1s debounced) is acceptable for
  invalidation; no server-side throttle in v1.

### WS4 — Credits publisher (worker, `internal/tasks`)
- After the batch debit transaction commits in `billing_batch_debits.go`
  (per-org), publish `credits.updated` with the post-debit balance on
  `OrgNoticeChannel(orgID)`. Needs the runtimestream store (or a slim publisher
  iface) injected into the task deps — worker already constructs the store for
  the projector.
- Skip publish when the batch made no change for that org.

### WS5 — Frontend client + dispatch (`apps/web/app/w/(chat)`)
- `_lib/session-notices.ts`: `fetchEventSource` to
  `/api/proxy/v1/sessions/{id}/notices`, `credentials:"include"`, reconnect
  with backoff (copy the shape of `session-name-updates.ts` +
  `use-sheet-live.ts`'s reconnect loop).
- Lifecycle: managed next to `ensureSessionStream` in
  `session-stream-manager.ts` (second controller map keyed by session), armed
  from the same `session-view.tsx` effect. On (re)connect: run the catch-up
  invalidations (decision 2).
- Dispatch `switch(notice.type)` (mirror `use-sheet-live.ts`'s
  `handleSheetEvent`):
  - `artifact.synced` → invalidate `["get","/v1/canvas/projects"]`,
    `["get","/v1/canvas/artifacts"]`, and the specific
    `["get","/v1/canvas/artifacts/{artifactID}"]`; add these builders to
    `lib/api/query-keys.ts` (none exist today). Then auto-open per decision 5.
  - `credits.updated` → invalidate `queryKeys.dashboard()`,
    `queryKeys.usage()`, `queryKeys.billingSubscription()` (surgical
    `setQueryData` on the balance later if invalidation proves noisy).

### WS6 — Panel auto-open + artifact targeting (`apps/web`)
- New `_stores/panel-artifact-target-store.ts` copying
  `panel-app-target-store.ts`: `openArtifact(sessionId, artifactId)`.
- Wire `DesignView` (`views/design.tsx`) to consume the target: replace the
  local `selectedArtifactId` `useState` with target-store-aware selection
  (target wins once, then user selection).
- Notice handler calls
  `useSessionWorkspaceStore.getState().openPanelView(sessionId, "design")`
  (only when `design` ∉ `openViews`) + `openArtifact(...)`.

### WS7 — Tests + verification
- Go: notices handler test (subscribe, publish both channels, assert SSE
  frames + keepalive + filter of non-notice kinds); canvasartifact publish
  test (stub publisher, assert payload + no-session skip); billing publish
  test.
- Frontend: dispatch unit tests for the two notice types (query invalidation +
  panel-store calls), following existing sheets-live-events test shape if one
  exists.
- E2E happy path: sync an artifact via the internal endpoint with a session id
  and assert the SSE frame arrives on `/v1/sessions/{id}/notices`.

## Order & sizing

WS1 → WS2 (backend spine, small) → WS3 + WS4 in parallel (publishers, small)
→ WS5 → WS6 (frontend, medium) → WS7 throughout. No migrations, no new
services, no runtime-image changes — everything deploys with api/worker/web.

## Risks / open items

- **Multiplexed Subscribe correctness**: one `Subscribe(chA, chB)` per open
  session per api instance; go-redis handles this fine but keep an eye on
  connection counts if sessions-with-notices grows (pool is 500; each
  Subscribe holds a dedicated conn — same cost profile as sheets-live today).
- **Notice loss is by design** (decision 2); if a future event type needs
  guaranteed delivery, it should become a durable runtime event through the
  existing XADD/projector path instead of a notice.
- **Auto-open UX**: opening the panel mid-typing could be jarring; v1 follows
  the browser/subagent precedent, revisit with feedback.
- The unmounted `SessionHandler.Stream` remains the long-term path to serve
  the transcript from the api instead of the sandbox; notices deliberately
  share its channel + envelope so that migration subsumes them for free.
