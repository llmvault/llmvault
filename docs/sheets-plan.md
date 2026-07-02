# Sheets — Implementation Plan

Org-scoped, agent- and user-created **sheets**: Notion-database ease, mini-Google-Sheets power. Agents (lead finding, research, scraping) spin up sheets and store structured results; users browse, edit, filter, and import/export CSV from the web app.

Naming: the feature/container is a **sheet**; the tabs inside it are **pages** (each page has its own fields + rows). This avoids the collision with the existing external-database machinery (`database_connections`, `database_proxy`, postgres/mysql plugins).

Storage decision (locked): **metadata + JSONB-rows model**. Fixed Postgres tables shared by all orgs. No dynamic DDL per tenant, no EAV, not a `canvas_artifacts` type (canvas is file/S3-shaped; this is structured control-plane data).

---

## 0. Data model

Hierarchy: **sheet → pages (tabs) → fields (columns) + rows**. Views are saved lenses per page.

```sql
-- 000055_sheets.sql
sheets (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  slug          text NOT NULL,
  name          text NOT NULL,
  description   text NOT NULL DEFAULT '',
  icon          text NOT NULL DEFAULT '',
  created_by_agent_id uuid,
  created_by_user_id  uuid,
  source_session_id   uuid,
  archived_at   timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ON sheets (org_id, slug) WHERE archived_at IS NULL;

sheet_pages (
  id uuid PK, sheet_id uuid NOT NULL, org_id uuid NOT NULL,
  name text NOT NULL, position double precision NOT NULL,
  display_field_id text,   -- which field labels this page's rows in relation chips; defaults to first text field
  archived_at timestamptz, created_at, updated_at
);
-- unique (sheet_id, name) WHERE archived_at IS NULL

sheet_fields (
  id           text PRIMARY KEY,          -- 'fld_' + 10 rand base36, generated in Go
  page_id      uuid NOT NULL, org_id uuid NOT NULL,
  name         text NOT NULL,
  type         text NOT NULL,             -- see field types below
  options      jsonb NOT NULL DEFAULT '{}',  -- select choices, number format, etc.
  position     double precision NOT NULL,
  archived_at  timestamptz, created_at, updated_at
);
-- unique (page_id, name) WHERE archived_at IS NULL; index (page_id)

sheet_rows (
  id            uuid PK DEFAULT gen_random_uuid(),
  page_id       uuid NOT NULL, org_id uuid NOT NULL,
  data          jsonb NOT NULL DEFAULT '{}',   -- { "fld_x": value } — keyed by FIELD ID, never name
  position      double precision NOT NULL,     -- fractional index for manual ordering
  import_job_id uuid,                          -- provenance; enables rollback of failed imports
  created_by_agent_id uuid, created_by_user_id uuid,
  archived_at   timestamptz, created_at, updated_at
);
-- index (page_id, position) WHERE archived_at IS NULL
-- index (page_id, created_at, id)              -- keyset pagination
-- GIN (data jsonb_path_ops)                    -- equality/containment filters
-- index (import_job_id) WHERE import_job_id IS NOT NULL

sheet_views (
  id uuid PK, page_id uuid NOT NULL, org_id uuid NOT NULL,
  name text NOT NULL, type text NOT NULL DEFAULT 'grid',  -- grid|kanban|gallery|calendar (grid only in v1)
  config jsonb NOT NULL DEFAULT '{}',  -- filters, sorts, hidden/ordered fields, column widths, group_by
  position double precision NOT NULL,
  archived_at timestamptz, created_at, updated_at
);

sheet_import_jobs (
  id uuid PK, org_id uuid NOT NULL, page_id uuid NOT NULL,
  object_key text NOT NULL,
  status text NOT NULL DEFAULT 'pending',   -- pending|running|completed|failed
  total_rows bigint NOT NULL DEFAULT 0,     -- 0 until counted; may stay 0 for streaming estimate
  processed_rows bigint NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  options jsonb NOT NULL DEFAULT '{}',      -- has_header, delimiter, field_mapping | create_fields
  created_by_agent_id uuid, created_by_user_id uuid,
  created_at, updated_at
);

sheet_operations (
  id            uuid PK DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL, page_id uuid NOT NULL,
  type          text NOT NULL,   -- rows_insert|rows_update|rows_delete|csv_import|field_change
  row_count     int NOT NULL DEFAULT 0,
  inverse       jsonb NOT NULL DEFAULT '{}',  -- what undo needs (see below)
  actor_agent_id uuid, actor_user_id uuid, source_session_id uuid,
  reverted_at   timestamptz, reverted_by_agent_id uuid, reverted_by_user_id uuid,
  created_at    timestamptz NOT NULL DEFAULT now()
);
-- index (page_id, created_at DESC)
```

**Operation log + revert (undo log, NOT a versioning system).** Every mutation (one `rows_write` call, one import job, one field change) writes one `sheet_operations` row capturing its inverse:
- `rows_insert` → inverse = `{row_ids}`; undo archives them.
- `rows_update` → inverse = `{patches: {row_id: {fld_x: old_value}}}` — **only the changed keys' prior values**. Batches are capped at ≤100 rows/call, so the before-image stays small by construction. Row `position` changes (drag-reorder) are intentionally NOT captured in the inverse — undo restores cell values only, never row order.
- `rows_delete` → deletes are `archived_at` soft-deletes, so inverse = `{row_ids}`; undo un-archives (data was never lost).
- `csv_import` → inverse = `{import_job_id}`; undo archives `WHERE import_job_id = ?`.
- `field_change` → inverse = the prior field definition (archive/restore for field deletes; rows untouched since cells key by field ID).

Revert = apply the inverse in a transaction, mark `reverted_at` (revert is itself not re-undoable in v1 — keep it single-level). The log doubles as the **audit trail**: which agent/user changed what, when, from which session.

Retention: **count-capped, pruned on write** — inside the same transaction that records a new operation, delete this page's operations beyond the newest `operationRetentionPerPage = 25` (named constant). 25, not 5: agents write in ≤100-row batches, so one bulk task can emit ~20 operations in under a minute — the window must cover a whole agent work session, and op rows are only a few KB each. No periodic job needed. Pruning only discards undo inverses, never data: deletes remain recoverable via `archived_at` and imports via `import_job_id` regardless of the log window; update before-images are the one thing that expires.

To be explicit about scope: this is an **undo log** (Ctrl+Z depth per page), not a versioning system — no page snapshots, no browsable history, no whole-sheet rollback. Each entry can reverse exactly one operation.

Conventions honored: uuid PKs via `gen_random_uuid()`, `org_id` CASCADE on every table, `archived_at` + partial indexes, jsonb via `model.JSON`/`model.RawJSON` (free NUL-byte sanitization), GORM model structs in `internal/model/` **and** hand-written goose migration — both required.

**Field types (v1):** `text`, `long_text`, `number`, `checkbox`, `select`, `multi_select`, `date`, `url`, `email`, `phone`, `attachment`, `relation`.
**Phase 2 (deferred):** `formula` (when built: materialized into `data` on write so filtering stays layer-1 SQL; interim answer — agents compute derived values into plain columns on request).

**`attachment`** — cell value = array of S3 object keys under a new `AssetPolicy` type `sheet_attachment` (prefix `pub/o/{orgID}/sheets/attachments/`, ≤25 MB/file). Uploads go through the existing sign→PUT flow with `sign(sheet_attachment)`. Decision (2026-07-02): cell acceptance and the download-url endpoint share one predicate (`sheets.ValidAttachmentKey`) — any org-owned key under `pub/o/{orgID}/` with no `..` traversal is accepted, and agent drive objects (`pub/e/{agentID}/…`) are not attachable, so agents must reference org-owned keys rather than their drive uploads. Downloads via presigned GET URLs. Lifecycle v1: objects kept until row hard-delete (org cascade) — no reaping on archive. No query-compiler involvement beyond `is_empty`/`is_not_empty`.

**`relation`** — cell value = array of row UUIDs; the target page lives in field `options.target_page_id` (same org, validated on every write — cross-org or cross-page-mismatch links are rejected in the service layer; this is KI-07-class surface and gets the same test rigor as the query compiler). To render chips, `sheet_pages` gains `display_field_id text` (nullable → defaults to the page's first text field): reads can hydrate linked row IDs into `{id, label}` pairs via one batched lookup. Archived target rows render as broken chips (no cascade into referrers). Filtering v1: `contains` (links to row X), `is_empty`, `is_not_empty` — plain jsonb containment. **Filtering *through* relations ("where linked company's status = qualified") is explicitly deferred** — it's a subquery feature in the compiler that can land independently later.

**Load-bearing rules:**
- Cells keyed by **field ID** → column rename is metadata-only, rows never touched.
- Validation/coercion of cell values happens in the Go service layer per field type.
- All querying happens in Postgres (jsonb predicates + casts). Never load a page into app memory.
- Every single query carries `AND org_id = ?` (KI-07 — the #1 recurring vuln class).

**Query compiler** (`internal/sheets/query.go`) — the heart of the feature:
- Input: filter AST `{"and":[{"field":"fld_x","op":"eq","value":...},{"or":[...]}],"search":"acme"}` + sorts + cursor.
- Ops per type (whitelisted): `eq neq contains not_contains starts_with gt gte lt lte is_empty is_not_empty in`.
- Compiles to one parameterized SQL statement. **Injection-proof by construction:** every `field` is resolved against the page's loaded field defs (unknown ID → error), ops come from a whitelist, values are bind params, casts chosen by field type (`(data->>'fld_x')::numeric`, `::timestamptz`).
- `search` compiles to `data::text ILIKE '%…%'` in v1; phase 2 adds a write-maintained `search_text` column + `pg_trgm` GIN index (extension already enabled).
- Keyset pagination on `(position, id)` or `(created_at, id)`; limit clamped (100 MCP / 200 REST).
- Hot-field escape hatch (later, if ever needed): expression index on `(data->>'fld_x')` — ordinary index DDL, still no dynamic tables.

---

## 1. MCP tools plan

New package `internal/sheets/mcptools.go`, modeled on `internal/agents/mcptools.go` (newest template) + `internal/mcpserver/cron_tool.go` (action-dispatch pattern).

**Eight tools** (balance between capability and prompt bloat):

| Tool | Purpose |
|---|---|
| `sheet_create` | name, description, optional pages `[{name, fields:[{name,type,options}]}]`. Returns sheet + page ids + field ids. |
| `sheet_list` | org's sheets with page names + row counts. Limit-clamped. |
| `sheet_describe` | full structure of one sheet: pages, fields (id/name/type/options), row counts. The schema-discovery call agents make before any write/query. |
| `sheet_manage` | `action` enum: `rename_sheet\|archive_sheet\|create_page\|rename_page\|archive_page\|add_field\|update_field\|archive_field` (cron-style dispatch keeps tool count down). |
| `rows_query` | page_id + filter AST + sorts + `search` + cursor + `resolve_relations` (hydrate linked row IDs to `{id,label}`). Returns `{rows:[{id,data}], fields_legend, next_cursor}`. Limit clamp 100. |
| `rows_write` | `action`: `insert\|update\|delete`; batch arrays (≤100 rows/call). Insert takes `[{data}]`, update `[{id, data}]` (partial merge), delete `[ids]`. |
| `sheet_import_csv` | `action: start` (drive asset key + options → job id) \| `status` (`job_id` → status/processed/total). |
| `sheet_operations` | `action`: `list` (recent operations on a page, with type/row_count/actor) \| `revert` (undo one operation by id). Backed by the operation log in §0. |

Implementation conventions (all confirmed in-repo):
- Factory `NewToolsFunc(db *gorm.DB, enq enqueue.TaskEnqueuer) func(*mcp.Server, *model.Token)`; guard `agentProxyToken(token)` + `tokenAgentID`; scope everything by `token.OrgID`.
- `InputSchema` as raw `map[string]any` JSON Schema with `additionalProperties:false`, enums, maxItems.
- Errors via `toolError()` returned as `(result, nil)` with `IsError:true`; success via `toolJSON()` — JSON in one `TextContent`.
- Hidden `_hivy_session_id` arg (runtime-injected, same as cron/memory) → recorded as `source_session_id` on created sheets/rows.
- Per-call `context.WithTimeout` (20s; import enqueue 5s).
- **Plugin-gated registration.** The `sheets` plugin is manually installed per agent (no auto-install). `NewToolsFunc` checks `agent_plugin_installs` for the `sheets` plugin slug and registers nothing if absent — mirroring the `agentBuilderEnabled` conditional-gating precedent in `internal/agents/mcptools.go`. Installing the plugin grants both the skill (existing behavior) and the tools (this check). Per-agent `McpToolFilter` still applies on top.
- **Install-timing caveat:** the MCP server is cached per token JTI (`ServerCache` in `mcphandler.go`), so installing the plugin mid-session doesn't add tools to an already-running session — they appear on the next session/token. Verify during Phase 3 and reflect in install-UI copy ("takes effect on the agent's next session") rather than fighting the cache.

Wiring (three known edit sites):
1. `internal/mcpserver/builder.go` — `SheetToolsFunc` typedef, new `BuildServer` param, `if addSheetTools != nil { … }` (+ `builder_test.go`).
2. `internal/handler/mcphandler.go` — field + `SetSheetTools` setter + pass into `BuildServer`.
3. `cmd/server/serve.go` (~line 145-161) — `mcpHandler.SetSheetTools(sheets.NewToolsFunc(db, enqueuer))`.

Tests mirroring `internal/agents/tools_test.go` and `internal/memory/mcptools_test.go`.

**Payload contract (locked by the shipped SKILL.md — tool schemas MUST match):** `rows_write` wraps batches as `rows` (insert/update) and `ids` (delete); sorts are `sorts: [{field, direction}]`; `search` is a top-level `rows_query` param (sibling of `filter`, not inside the AST); `sheet_manage` takes flat args (`page_id`, `field_id`, `name`, `type`, `options`); `sheet_operations` revert takes `operation_id`. Any deviation requires editing `global/plugins/sheets/skills/sheets/SKILL.md` in the same change.

---

## 2. CSV import / upload plan

Two entry paths converging on one Asynq worker.

**User path (web app):**
1. Dropzone (`react-dropzone`, already used in composer — `accept: {"text/csv":[".csv"]}`).
2. `POST /v1/uploads/sign` with a new `AssetPolicy` type `sheet_import` (key prefix `pub/o/{orgID}/sheetimports/`, max 50 MB) → browser `PUT`s to S3 directly (flow already exists: `apps/web/app/w/settings/general/page.tsx`). Local-dev fallback: existing server-side proxy `POST /v1/uploads/upload`.
3. `POST /v1/sheets/{sheetID}/pages/{pageID}/imports {object_key, options}` → creates `sheet_import_jobs` row, enqueues task.

**Agent path (sandbox):**
1. Agent writes CSV in sandbox, uploads via the existing drive endpoint `PUT /internal/agents/{agentID}/sandboxes/{sandboxID}/drive/*` (`agent_uploads_stream.go` — streams to S3, no size cap, arbitrary content types already allowed).
2. Calls `sheet_import_csv` MCP tool with the drive path/asset → same job row + task.
3. Skill rule of thumb: < ~500 rows → just `rows_write` in batches; larger → CSV upload + import.

**Worker** — `internal/tasks/sheet_csv_import.go`, copying the established anatomy (`memory_embed.go` skeleton + `billing_batch_process.go` chunking):
- `TypeSheetCSVImport = "sheet:csv_import"`, payload `{JobID uuid}` (IDs only), `init()` + `RegisterTaskBuilder`, queue `bulk`, `MaxRetry(3)`, `Timeout(10m)`.
- `WorkerDeps` gains one field: `Storage storage.Reader` (S3 read — the only missing dep).
- Handler: load job (gone/stale → `return nil`, no retry); status guard for idempotency; stream S3 object → stdlib `encoding/csv` `Reader` row-by-row (no full-file buffering; no new Go deps).
- Header/type handling per `options`: map to existing fields, or create fields with type inference over the first 1,000 rows (number/date/checkbox/email/url detection, fallback `text`).
- Batches of 500–1000 → `CreateInBatches` in a transaction → update `processed_rows` per chunk.
- Rows stamped with `import_job_id`; on hard failure, rollback = `DELETE WHERE import_job_id = ?` and mark job `failed` — retries start clean.
- Cell values through `model.JSON` (NUL sanitization free) + the same field-type coercion as `rows_write`.

**Progress to frontend:** the worker publishes `import_progress` events per chunk on the sheet's SSE channel (§2c); `GET /v1/sheets/imports/{jobID}` remains as the poll fallback and for post-hoc status.

**Export:** synchronous streaming `GET /v1/sheets/{sheetID}/pages/{pageID}/export.csv` — cursor-walk rows, `csv.Writer` straight to the response. Fine even at 100k rows; no job needed.

---

## 2b. REST API — full user editing surface

Everything a user can do in the grid maps to a `/v1` endpoint (chi routes in `cmd/server/serve_routes_v1.go`, org auth middleware, `X-Org-ID` scoping). REST handlers and MCP tools are thin shells over the **same `internal/sheets` service functions** — one code path for validation, field-type coercion, org filters, and operation capture. User mutations record `actor_user_id` in `sheet_operations`, so manual edits are undoable and audited exactly like agent writes.

**Sheets**
| Method + path | Purpose |
|---|---|
| `GET /v1/sheets` | list org sheets (cursor, `?search=` on name) |
| `POST /v1/sheets` | create (name, description, icon, optional pages+fields inline) |
| `GET /v1/sheets/{sheetID}` | full structure: pages, fields per page, row counts (the UI's bootstrap call) |
| `PATCH /v1/sheets/{sheetID}` | rename / description / icon |
| `DELETE /v1/sheets/{sheetID}` | archive |

**Pages (tabs)**
| `POST /v1/sheets/{sheetID}/pages` | add page |
| `PATCH …/pages/{pageID}` | rename, `position` (drag-reorder tabs) |
| `DELETE …/pages/{pageID}` | archive |

**Fields (columns)**
| `POST …/pages/{pageID}/fields` | add column (name, type, options, position) |
| `PATCH …/fields/{fieldID}` | rename, options, `position` (drag-reorder), type change (service re-coerces lazily) |
| `DELETE …/fields/{fieldID}` | archive column (rows untouched — cells key by field ID) |

**Rows + cells**
| `POST …/pages/{pageID}/rows/query` | filter AST + sorts + `search` + cursor + `resolve_relations` (POST body; limit clamp 200). The relation row-picker UI is this same endpoint with `search` against the target page — no extra endpoint |
| `POST …/pages/{pageID}/rows` | insert batch `[{data, position?}]` |
| `PATCH …/pages/{pageID}/rows` | update batch `[{id, data}]` — **partial merge per field key; a single cell edit is a batch of one row with one key**. Row drag-reorder = same endpoint with `position` |
| `DELETE …/pages/{pageID}/rows` | archive batch `{ids}` |

Concurrency: last-write-wins **per field key** (PATCH merges only the keys sent), so a user editing the "status" cell and an agent updating "score" on the same row don't clobber each other. No row locking in v1.

**Views**
| `POST …/pages/{pageID}/views` / `PATCH …/views/{viewID}` / `DELETE …/views/{viewID}` | saved lenses; `config` carries filters/sorts/hidden fields/column widths |

**Attachments**
| `POST /v1/uploads/sign` (type `sheet_attachment`) | existing sign flow, new asset policy |
| `POST …/pages/{pageID}/attachments/download-url` | presigned GET for object keys in a cell (server re-checks org ownership of the keys) |

**Imports / export**
| `POST …/pages/{pageID}/imports` | start CSV import (`object_key`, options) |
| `GET /v1/sheets/imports/{jobID}` | poll progress |
| `GET …/pages/{pageID}/export.csv` | streaming export |

**Undo (operations)**
| `GET …/pages/{pageID}/operations` | recent operations (type, row_count, actor, timestamp) — powers the UI undo affordance + activity trail |
| `POST …/operations/{operationID}/revert` | apply the inverse |

All endpoints go into `docs/openapi.json` → `pnpm generate`, so the frontend gets the typed `$api` client for every one of these.

---

## 2c. Realtime sync — SSE, direct to the API

Goal: user watches the grid while an agent populates it; cells appear/change live. Also: two users viewing the same sheet see each other's edits, and import progress streams instead of polling.

**Transport: SSE, not WebSocket.** The flow is one-directional (writes go up via REST/MCP; only change notifications come down), and the whole SSE stack already exists in-repo: Redis pub/sub, the `sessions_stream.go` endpoint pattern (flush controller, 15s keepalive), `@microsoft/fetch-event-source` on the client with retry/reconnect. A WebSocket would add the backend's first websocket dependency for zero gained capability.

**Publisher — in the service layer, so every write path emits.** After each committed mutation, `internal/sheets` publishes a compact JSON event to Redis channel `sheet:{sheetID}`. Because REST handlers, MCP tools, the CSV import worker, and revert all go through the same service functions, agent edits, user edits, imports, and undos all emit automatically. Events:
- `rows_changed {page_id, action: insert|update|delete, row_ids, patches?, actor, mutation_id}` — `patches` (the changed field keys/values) included when small, so the client can patch its cache without a refetch
- `fields_changed` / `pages_changed {page_id|field_id, action}` — schema edits → client refetches structure
- `import_progress {job_id, processed_rows, total_rows, status}` — replaces v1 polling
- `operation_reverted {operation_id, page_id}`

**Endpoint:** `GET /v1/sheets/{sheetID}/live` — SSE handler modeled on `sessions_stream.go`, subscribing to the Redis channel.

**Auth: direct connection, bypassing the Next proxy** (mirroring the sandbox session stream, which already connects direct with a bearer token, and the canvas preview-token precedent):
1. Client calls `POST /v1/sheets/{sheetID}/live-token` (through the normal proxy) → short-lived scoped JWT (`sheets:read`, ~10 min, minted like `canvasPreviewToken` in `canvas_artifacts_helpers.go`).
2. Client opens `fetchEventSource("{API_BASE}/v1/sheets/{id}/live", Authorization: Bearer <jwt>, openWhenHidden: true)` straight against the Go API; re-mints on expiry/reconnect. Requires `NEXT_PUBLIC_HIVY_API_URL` (or equivalent) exposed to the browser + CORS allowance for this one endpoint. Avoids buffering and long-held connections through a Next route handler.

**Delivery semantics: events are hints, REST is truth.** No durable event store (unlike session events) — the client applies `rows_changed` patches to its TanStack Query cache optimistically and heals any missed events by refetching the visible window on SSE reconnect and window focus. Self-echo suppression: mutations carry a client-generated `mutation_id`; the client ignores events echoing its own writes (applying them anyway is idempotent, so this is polish, not correctness).

---

## 2d. Limits (production guardrails)

Service-layer constants, enforced on every write path (REST, MCP, import) — checked, not silent-truncated; violations return typed errors:
- cell value ≤ 16 KB; row `data` ≤ 128 KB (also guards jsonb bloat and event payload size)
- ≤ 200 fields per page, ≤ 100 pages per sheet, ≤ 500 select options per field
- ≤ 10 attachments per cell (≤ 25 MB per file); ≤ 100 relation links per cell
- rows per page: soft cap 200,000 (import fails fast with a clear error beyond it; revisit with partitioning if real usage approaches)
- `rows_write` ≤ 100 rows/call (MCP), REST batch ≤ 500; `rows_query` limit clamp 100 (MCP) / 200 (REST)
- CSV upload ≤ 50 MB
- sheets per org: soft cap 1,000 (constant)

---

## 3. Plugin + skill plan

**Plugin** — `global/plugins/sheets/plugin.json`:
```json
{
  "version": 1,
  "slug": "sheets",
  "name": "Sheets",
  "description": "Create and manage structured sheets — store leads, research, and any tabular work output.",
  "category": "Data & Analytics",
  "icon": "lucide:table",
  "capabilities": ["Read", "Write"],
  "plugin_version": "1",
  "enabled": true,
  "examples": ["Build me a lead sheet of 50 SaaS founders", "Import this CSV into a new sheet", "Which leads have status = qualified?"]
}
```
**No `auto_install`** — the plugin is installed manually for specific agents via the normal install path (agent update / `create_agent` `plugin_slugs`). Installing it grants the skill (via `agent_plugin_installs`, existing machinery) *and* the MCP tools (via the gating check in §1). No `required_connections` — this is first-party storage, not an external DB connector. Sync is automatic: files land in `global/plugins/`, boot-time `syncGlobalPlugins` upserts `plugins`/`skills` rows (no CLI step).

**Skill** — `global/plugins/sheets/skills/sheets/{skill.json, SKILL.md}`:

`skill.json`: `name: "Sheets"`, `description` = the trigger ("Use when work produces structured/tabular data the user should keep, browse, or edit — leads, research results, inventories, comparisons — or when the user asks for a sheet, database, table, tracker, or spreadsheet."), `human_description`, `category: "Data & Analytics"`, `root: "./SKILL.md"`, `tags: ["sheets","spreadsheet","database","csv","leads"]`, `files: []`.

`SKILL.md` structure (canvas skill = structural template; postgres skill = tone template for the rules):
1. Frontmatter (`name`, `description`) + when-to-use intro. Explicit: **prefer a sheet over markdown tables / loose CSV files** for anything the user will revisit.
2. **Concepts** — sheet → pages → fields → rows; field IDs vs display names ("`data` payloads are keyed by field ID like `fld_8k2mx1q9`, never by name — call `sheet_describe` to get the legend"); views belong to users, don't touch.
3. **Core Workflow** — numbered tool-call loop with exact JSON payloads:
   1. `sheet_list` → reuse an existing sheet if one fits; never create duplicates.
   2. `sheet_create` with properly typed fields (show a leads example: text/url/email/select/number).
   3. `rows_write` inserts in batches of ≤100.
   4. `rows_query` with 3–4 filter AST examples (good/bad pairs), `search` usage, cursor paging.
   5. Pages: when to add a page (same project, different entity) vs a new sheet (different project).
4. **CSV import** — the <500-rows rule; drive-upload + `sheet_import_csv` walkthrough for large files; how to check job status.
5. **Field types reference** — table of the 12 types with `options` examples and coercion behavior; relation section with a worked example (link deals to competitor rows, `resolve_relations` in queries); attachment section (upload to drive first, then reference in the cell).
6. **Mistakes & recovery** — `sheet_operations` list/revert: how to undo a bad batch write or import.
7. **Rules** (postgres-skill style, hard bullets): always `sheet_describe` before writing; field IDs not names; batch writes; never stuff blobs/HTML into cells; query fresh before bulk updates (users edit rows too); archive don't delete unless asked.
8. **Final Response Checklist** — state sheet name, page, rows added/updated, and that the user can open it in the Sheets panel.

Delivery is already built: `skills_list` surfaces it, `skill_view` materializes it to `.skills/sheets` in the sandbox.

---

## 4. Code implementation plan (phased)

**Phase 1 — schema + core service** (foundation, no UI/tools yet)
- `internal/migrations/sql/000055_sheets.sql` (§0) + models `internal/model/sheet.go` (Go types `Sheet`, `SheetPage`, `SheetField`, `SheetRow`, `SheetView`, `SheetImportJob`, `SheetOperation` — no clash with the external-connector `DatabaseConnection` models).
- `internal/sheets/` package: `service.go` (CRUD, slug allocation à la `canvasartifact.normalizeSlug`), `fields.go` (type registry, validation, coercion), `fieldid.go` (`fld_` generator), `query.go` (filter-AST → SQL compiler), `operations.go` (operation capture + count-capped prune inside each mutation's transaction, plus revert).
- Relation integrity lives here too: target-page/org validation on write, batched `{id,label}` hydration, `display_field_id` resolution. Attachment value-shape validation (org-owned object keys only).
- **Unit-test the query compiler, coercion, and relation validation hardest** — they're the security and correctness core (cross-org relation links are a KI-07-class vuln).

**Phase 2 — REST API + realtime (frontend surface)**
- The complete endpoint surface in §2b — sheets/pages/fields/rows(+cells)/views/imports/export/operations. Handlers split across `internal/handler/sheets.go`, `sheets_rows.go`, `sheets_imports.go` (respect the 300-line ceiling / `file-length-allowlist`).
- Handlers are thin: parse/authz → call the same `internal/sheets` service functions the MCP tools use → shape response.
- Realtime (§2c): Redis publisher in the service layer, `GET /v1/sheets/{sheetID}/live` SSE endpoint, `POST …/live-token` mint, CORS allowance for the direct connection.
- Limits (§2d) enforced in the service layer with typed errors.
- Update `docs/openapi.json` → `pnpm generate` for the typed `$api` client.

**Phase 3 — MCP tools** (§1: package, 8 tools, plugin-install gating, 3 wiring sites, tests).

**Phase 4 — CSV import/export** (§2: asset policy, import job endpoint + task, export stream).

**Phase 5 — plugin + skill** (§3: two JSON files + SKILL.md; sync is automatic).

**Phase 6 — frontend** (§5 for library choice)
- New right-panel view, 4 known registration points: `PanelViewID` in `right-panel.tsx`, `WorkspacePanelViewID` in `_stores/session-workspace-store.ts`, `case "sheets"` in `right-panel-active-view.tsx`, new `views/sheets.tsx`.
- Data layer mirrors `canvas-artifacts.ts` (raw `requestJSON` + TanStack Query) or `$api.useQuery` once OpenAPI regenerated.
- Grid components under `_components/views/sheets/` — Glide `DataEditor` wrapper + `useGlideTheme` token-bridge hook (CSS vars → Glide theme object, re-resolved on dark-mode/preset change), `cells/` module (custom renderers: select chips, checkbox, url/email, attachment thumbnails, relation chips), HeroUI editors in Glide's DOM overlay portals — including the attachment upload editor (dropzone → sign → PUT) and the relation row-picker (search via `rows/query` on the target page) — toolbar chrome in plain HeroUI/Tailwind (page tabs, view switcher, filter builder), CSV dropzone + import-progress bar, undo affordance backed by operations list/revert.
- React 19 compat (verified 2026-07-02, low risk): the React 19 upgrade was merged upstream June 2025 (glide-data-grid PR #1036, closing issue #1021) and is published in the `6.0.4-alpha` line (`alpha24` declares `19.x` peers); stable `6.0.3` still declares `≤18.x`, which hard-fails npm but only warns under pnpm (this repo's PM). No runtime bugs with React 19 are reported in the tracker — only install-time peer resolution. Use `6.0.4-alpha24` (or `6.0.4` stable if released by Phase 6). Glide's own peer deps (`lodash`, `marked`, `react-responsive-carousel`) come along; lazy-load the grid view chunk.
- Live updates via the sheet SSE channel (§2c): `useSheetLiveStream(sheetID)` hook — mint live-token, `fetchEventSource` direct to the API, apply `rows_changed` patches into the TanStack Query page cache, refetch structure on `fields_changed`/`pages_changed`, drive the import progress bar from `import_progress`, reconcile with a visible-window refetch on reconnect/focus.
- Later (post-v1): standalone org-level `/w/sheets` page outside the chat panel.

**Phase 7 — hardening**
- E2E mirroring `e2e/agent_sessions_canvas_artifacts_e2e_test.go` (agent creates sheet → inserts → queries → reverts an op → user reads via /v1).
- Load test: 100k-row page — query latency, GIN index behavior, import throughput.
- Org-scoping audit of every new query (KI-07 class).

Dependency order: 1 → 2 → {3,4,5 in parallel} → 6 → 7. Phases 3–5 are independent of each other once 1–2 land.

---

## 5. Frontend grid library

**Recommendation: Glide Data Grid (MIT, canvas-rendered) — chosen because the target is a Google-Sheets-grade editing experience, not just a Notion-grade one.**

The decisive requirement is spreadsheet *interaction*: multi-cell range selection, Excel/Sheets-compatible copy/paste, fill handle, type-to-overwrite keyboard navigation, smooth 100k-row scrolling. Glide ships all of it out of the box. A headless grid (TanStack Table) gives zero help there — selection models, clipboard TSV parsing, focus management across virtualized (unmounting) cells, and drag-while-scroll are 4–8 weeks of edge-case-heavy interaction engineering to build by hand. Buying the hard part and bridging the theme is the better trade.

How it fits the codebase:
- **License:** MIT, fully open source, no paid tier withheld (unlike AG Grid).
- **Editors are DOM, not canvas.** Glide's cell editing overlays are React portals — HeroUI inputs, selects, and date pickers work inside them natively. Only the *display* layer is canvas.
- **Theme bridge for the OKLCH token system:** one hook resolves the CSS variables (`--surface`, `--border`, `--foreground`, `--muted`, `--accent`, `--field-*`…) via `getComputedStyle` and maps them onto Glide's ~20-key theme object (`bgCell`, `borderColor`, `headerBg`, `accentColor`…), re-resolving when `next-themes` dark mode or the `data-theme-preset` changes. All 13 presets keep working; modern canvas accepts `oklch()` strings directly.
- **Custom cell renderers** (select chips, checkbox, url/email, rating) are hand-drawn via Glide's custom-cell API — the real remaining cost, but bounded (days, not weeks) and done once in a `cells/` module.
- **Server-side data model fits perfectly:** Glide pulls cells through a `getCellContent(row, col)` callback, which pairs naturally with windowed `rows/query` pages cached in TanStack Query — the client never holds a full page's data.
- Heavy client libs are already the norm in this app (Monaco, CodeMirror, react-pdf, vidstack); a canvas grid raises no new build constraints. `papaparse` (already installed) still handles client-side CSV preview before upload.

Trade-offs accepted, eyes open:
- The grid's chrome (toolbar, page tabs, filter builder, popovers) is normal HeroUI/Tailwind DOM — only the cell matrix is canvas, so most of the UI still uses token classes directly.
- Accessibility inside the canvas is Glide's implementation (it has keyboard nav + screen-reader support), not react-aria.
- If Glide ever stalls as a dependency, the escape hatch is contained: the data layer, REST surface, and toolbar are grid-agnostic; only the cell-matrix component swaps.

Alternatives considered and rejected:
- **TanStack Table + @tanstack/react-virtual (headless):** perfect design-system fidelity (your DOM, your token classes) and the right choice if the bar were Notion-level editing (click cell → popover editor; Notion has no ranges or fill handle either). Rejected because building Sheets-grade selection/clipboard/fill by hand is the most expensive path to the stated goal. Still fine to use elsewhere for ordinary tables.
- **AG Grid Community:** range selection, clipboard, and other sheet-grade features are Enterprise-paid — fails "100% open source".
- **Handsontable:** non-commercial license. Out.
- **Univer:** Apache-2.0 full Sheets clone (formulas included) but ships its own entire UI/design language — fights the token system head-on, and is far more dependency than a grid.

---

## Open questions (non-blocking, defaults chosen)

1. Naming: settled — feature = **Sheets**, tabs = **pages** (tables `sheets`, `sheet_pages`, …; package `internal/sheets`). Watch one soft collision: canvas has a `web_page` artifact type, and any future Notion-style doc feature would also want the word "pages"; "tabs" is the fallback rename if that materializes.
2. Full per-cell version history: intentionally skipped. The `sheet_operations` log (§0) provides operation-level revert + audit trail; per-cell history can be derived from it later if demanded.
3. Kanban/gallery/calendar views: schema supports them (`sheet_views.type`), UI is grid-only in v1.
4. Scope addendum (2026-07-02): `attachment` and `relation` field types pulled INTO v1 by user decision; `formula` and filtering-through-relations remain deferred.
