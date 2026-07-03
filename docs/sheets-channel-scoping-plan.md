# Plan: Channel-scope sheets (visibility follows channel access)

## Goal

Make sheets belong to a **channel**, so that:

1. A sheet's visibility follows its channel's visibility. A user who cannot access
   a channel cannot see (or read/write) the sheets in it.
2. When an **agent** creates a sheet, the sheet is bound to the channel the agent
   is currently operating in.

**Locked decisions (from product):**

- **Channel-mandatory.** Every sheet must belong to exactly one channel
  (`channel_id NOT NULL`). No org-wide/channel-less sheets. Existing sheets are
  backfilled.
- **Strict channel-only visibility.** An agent (and a user) can only see/touch
  sheets in the channel they are in. No org-wide fallback set.

---

## 1. Current state (what exists today)

Sheets are **purely org-scoped** — there is no channel dimension anywhere.

- **Model** — `internal/model/sheet.go:9-26`. `Sheet` has `OrgID` (NOT NULL) plus
  provenance-only nullable columns `CreatedByAgentID`, `CreatedByUserID`,
  `SourceSessionID`. **No `channel_id`.** Child tables `SheetPage`, `SheetField`,
  `SheetView` (and `sheet_rows`, etc.) each carry their own `OrgID` and cascade
  off `sheet_id`/`page_id`.
- **Migration** — `internal/migrations/sql/000055_sheets.sql`. `sheets.org_id`
  FK to `orgs` (CASCADE); indexes `(org_id, slug)` and `(org_id, updated_at)`.
  No channel column. Latest migration on disk is `000059`; **next is `000060`.**
- **Read path is org-only.** `Service.ListSheets` filters
  `Where("org_id = ? AND archived_at IS NULL", orgID)` (`internal/sheets/service.go:157`).
  `sheet_list` MCP tool description literally says "List **this organization's**
  sheets" (`internal/sheets/mcptools_sheet.go:129`). The `sheets` package does
  **not** import `internal/access` and never references `Channel`/`ChannelMember`.
- **Agent create captures no channel.** `sheet_create` → `handleSheetCreate`
  (`mcptools_sheet.go:90`) → `svc.CreateSheet(..., token.OrgID, req, actor)`.
  The `Actor` struct (`service.go:48-52`) is `{AgentID, UserID, SessionID}` —
  **no `ChannelID`.** Sheets tools inject only `_hivy_session_id`, never
  `_hivy_actor_user_id`.

### The bridge that makes this cheap

`Session.ChannelID` is a **NOT NULL** FK to `channels` (`internal/model/session.go:23`,
DB `000009_sessions.sql:7` + FK `000013:190`). A session can never exist without a
channel — web, DM/personal, playground, cron, and trigger runs all bind one
(background runs are routed into a synthetic `system` channel;
see `000056_system_channel_backfill.sql` and `ensureTriggerSystemChannel`).

The sheets MCP tools **already inject `_hivy_session_id`** on every tool
(`mcptools_sheet.go:30`, `mcptools_rows.go:142`, `mcptools_manage.go:29`,
`mcptools_ops.go:21,127`). So the agent's current channel is always reliably
derivable as `session.ChannelID` — no new injected value is required for the
agent path.

### How channel access is enforced elsewhere (reuse target)

- Agent/MCP path: `internal/access/access.go` — `Actor.CanUseChannel` /
  `CanUseChannelID`. Resolves `_hivy_actor_user_id` → `Actor`; org managers pass;
  external or non-team channels are open to org members; team-scoped channels
  require active team membership.
- HTTP path: `internal/handler/channel_access.go:31` `canUseChannel(...)` and
  `SessionHandler.canUseChannel` (`internal/handler/sessions_auth.go:87`) — the
  same rule keyed off the request's `orgRole` + `userID`. The `access` package
  doc states these must not drift.

---

## 2. Target design

- `sheets.channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE`.
  Channel lives **only on the parent `sheets` row**; pages/fields/rows keep
  cascading off `sheet_id` and need no channel column (they inherit it).
- **Agent path** — the "current channel" is derived from the injected session:
  `channel_id := session.ChannelID`. `_hivy_session_id` becomes **required** for
  `sheet_create` (a channel-mandatory sheet cannot be created without it). Every
  read/write MCP tool resolves the session's channel and enforces
  `sheet.channel_id == session.channel_id` (strict channel-only).
- **Human/REST path** — sheets are listed and created **within a channel**. The
  list endpoint takes a `channel_id`; the caller's channel access is enforced via
  the existing `handler.canUseChannel` mirror. Per-sheet reads/writes re-check the
  sheet's channel against the caller.

---

## 3. Implementation

### Phase 0 — Migration `000060_sheets_channel_scope.sql`

No production data exists — the `sheets` table is empty — so **no backfill** is
needed. Add the column as NOT NULL directly:

1. `ALTER TABLE sheets ADD COLUMN channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE;`
2. Add index `idx_sheets_channel_updated_active ON sheets (channel_id, updated_at DESC) WHERE archived_at IS NULL;`
   (the hot path becomes "list sheets in a channel, newest first"). Keep the
   existing org indexes.
3. Down migration drops the column and index.

### Phase 1 — Model + service

- `internal/model/sheet.go`: add
  `ChannelID uuid.UUID` (NOT NULL) + `Channel *Channel` FK to `Sheet`.
- `internal/sheets/service.go`:
  - Add `ChannelID uuid.UUID` to `Actor` (§`service.go:48`).
  - `CreateSheet`: write `ChannelID: actor.ChannelID`; reject if
    `actor.ChannelID == uuid.Nil` with a clear error ("a sheet must be created
    within a channel"). Move the "max sheets" count to be **per channel or per
    org** — keep per-org for now unless product wants per-channel limits.
  - `ListSheets`: change signature to accept a `channelID uuid.UUID` and filter
    `Where("channel_id = ? AND archived_at IS NULL", channelID)`. Org scoping is
    implied by the channel (channel belongs to one org) but keep `org_id` in the
    WHERE as defense-in-depth.
  - `GetSheet` and every by-ID read/mutate (`UpdateSheet`, `ArchiveSheet`, page/
    row/view/import/operations methods): either (a) accept the caller's allowed
    `channelID` and add it to the WHERE so a cross-channel ID simply 404s, or
    (b) load the sheet, then have the caller authorize its `channel_id`. Prefer
    (a) for the agent path (cheap, no extra query) and (b) for REST where the
    user's access set is richer.

### Phase 2 — Agent/MCP enforcement (strict channel-only)

- New shared helper in `internal/sheets/mcptools.go`, e.g.
  `sheetToolChannel(ctx, token, rawSessionID) (uuid.UUID, error)`: parse the
  injected `_hivy_session_id`, load the session scoped to `token.OrgID`, return
  `session.ChannelID`. Empty/invalid session → error (no channel ⇒ deny). Fold
  this into `sheetToolActor` so `Actor.ChannelID` is always populated for agents.
- `sheet_create`: require `_hivy_session_id`; bind `actor.ChannelID`.
- **All other sheets tools** (`sheet_list`, `sheet_describe`, `sheet_manage`,
  `rows_query`, `rows_write`, `sheet_import_csv`, `sheet_operations`): resolve the
  current channel and pass it into the service so:
  - `sheet_list` lists only sheets in the current channel (also update the tool
    description — no longer "this organization's sheets").
  - by-ID tools enforce `sheet.channel_id == currentChannel`, returning a
    not-found/forbidden error otherwise. This is the core of strict isolation:
    an agent cannot reach a sheet in another channel even with its ID.
- Update `internal/sheets/mcptools_gating_test.go` and the isolation/contract
  tests; add a test proving an agent in channel A cannot read/write a sheet in
  channel B.

### Phase 3 — HTTP/REST + web UI

- `internal/handler/sheets.go` et al.:
  - `ListSheets` handler: require a `channel_id` (query param or route), verify
    the caller via `h.canUseChannel(...)` (reuse `channel_access.go`), pass it to
    `svc.ListSheets`.
  - `CreateSheet` handler: take `channel_id` from the request; authorize with
    `canUseChannel`; thread into the service actor.
  - `GetSheet`/`UpdateSheet`/`ArchiveSheet` + pages/rows/views/imports/stream
    (`sheets_pages.go`, `sheets_rows.go`, `sheets_views.go`, `sheets_imports.go`,
    `sheets_stream.go`): load the sheet, then authorize its `channel_id` with
    `canUseChannel` before proceeding. The SSE stream
    (`sheets_stream.go`) must reject subscribers who lack the sheet's channel.
- **Web UI** (`apps/web`): sheets need to be surfaced *within a channel* rather
  than a global org list. This is the largest product-facing change and should be
  scoped separately — confirm the desired UX (a "Sheets" tab per channel? a
  filtered global list?). The API is channel-first regardless.

### Phase 4 — Docs & tests

- Update `docs/sheets-plan.md`'s "Org-scoped / every query carries `org_id`"
  invariant to "channel-scoped; every query carries `channel_id`".
- Add cross-channel isolation tests at both the service and MCP layers.
- **OpenAPI: annotate, don't hand-edit.** `make openapi` runs
  `swag init` (godoc `@Router`/`@Summary`/`@Tags`/`@Param`/`@Success`
  annotations → `docs/swagger.json` → `openapi.json`). The sheets REST handlers
  today carry **no** swaggo annotations, which is why their paths were hand-added
  to the spec and wiped on every regeneration. Fix the root cause: add proper
  swaggo annotation blocks to the sheets handlers (`sheets.go`, `sheets_pages.go`,
  `sheets_rows.go`, `sheets_views.go`, `sheets_imports.go`, `sheets_stream.go`),
  including the new `channel_id` param on list/create. Then the `/v1/sheets`
  paths are emitted by `make openapi` and survive regeneration — no manual
  re-adding.

---

## 4. Resolved decisions (2026-07-03)

1. **Backfill** — none. The `sheets` table is empty, so `channel_id` is added
   NOT NULL directly (no migration backfill step).
2. **Moving a sheet between channels** — **not a feature.** `channel_id` is set at
   creation and does not change; no move endpoint.
3. **Channel deletion** — `ON DELETE CASCADE` is correct: deleting a channel
   deletes its sheets.
4. **Web UX** — surfacing sheets per channel is a non-trivial front-end change,
   deferred and tracked separately. The backend/API becomes channel-first now.
5. **Limits** — `MaxSheetsPerOrg` stays org-wide for now; no per-channel ceiling.
