# Apps — Implementation Plan

Status: design locked, not started. Decided 2026-07-03.

**Apps** are agent-built, minimal web applications over a single sheet: a static React SPA plus a
small always-running backend, both produced by an app-builder agent from a strict template,
hosted in our own microsandbox infrastructure with the same auto-sleep/wake lifecycle as agent
sandboxes. Agents do work → store structured results in a sheet → build an interactive interface
(CRUD, dashboards, custom tooling) over that sheet without touching its schema. Apps also receive
the channel's custom env vars, so an app can be a custom Stripe dashboard or anything else the
org's secrets unlock.

## Philosophy (locked decisions)

- **App = one sheet.** An app is bound to exactly one sheet at creation. It reads the structure,
  never modifies it (no field/page DDL through the app surface). Blast radius of any app bug or
  weird agent code is full CRUD on that one sheet plus its own env — nothing else.
- **No SSR, no Node at runtime.** The frontend is a static Vite-built React SPA. The backend is
  agent-written but template-constrained, in a low-memory compiled language (**Go** — see
  Template section). Node exists only in the *builder* sandbox (already in the agent image) for
  the Vite build. Super fast builds are a hard requirement.
- **The browser never talks to the Hivy API or third parties.** SPA → app backend (same
  sandbox, same origin) → Hivy API / Stripe / anything. Latency cost accepted; it is the most
  secure default and keeps every secret server-side.
- **Two credentials, two jobs.**
  - The **launch JWT** (RS256, short-lived, one-time) only says "this user may enter this app in
    the browser" and carries user/org display metadata. The app exchanges it for its own cookie
    session.
  - The **app secret** (per-app runtime secret, generated and encrypted by us, injected into the
    app sandbox) is what actually authenticates the app backend to the Hivy API — exactly the
    sandbox-runtime-secret pattern (`agent_uploads_stream.go` `authAgent`). Mutations are
    attributed to users for audit by the app forwarding the session's user ID; the platform
    trusts it because the request is app-secret-authenticated and the session was
    JWT-established by template code.
- **Aliases are a microsandbox feature, not an app feature.** The control plane owns
  alias → (sandbox, port) mappings, validates availability, and propagates to the gateway/Caddy.
  The app record just stores its alias string. Anything else that needs a stable sandbox URL
  later gets aliases for free.
- **Observability is ephemeral by design.** No durable log store. Every app sandbox exposes a
  logs endpoint authorized by the app secret; the agent reaches it through a Hivy tool
  (tool → Go API → wake sandbox → fetch). Logs live on the persistent `/workspace` volume so
  they survive sleep/wake; loss on delete-and-redeploy is a non-issue.
- **Deferred deliberately:** XSS/CSP hardening pass, custom domains, wake-latency UX polish,
  multi-sheet apps, SSE/live updates in apps.

## Existing primitives (what we build on)

- **Wake-on-request hosting, end to end:** Caddy wildcard edge (`ansible/roles/caddy-preview-proxy/templates/Caddyfile.j2`)
  → `forward_auth` to the Python gateway `/v1/lookup` (`services/microsandbox-gateway/src/microsandbox_gateway/app.py`)
  → control-plane `ensure-ready` wake with Redis wake-lock (`internal/microsandbox/control/handlers_sandbox_gateway.go`)
  → `reverse_proxy` to `runner_host:hostPort`. Auto-sleep default 300s, activity-extended
  (`handlers_sandbox_activity.go`, `preview_cache.go`). Preview routes are pushed control→gateway
  with `route_generation` + leases (`preview_cache.go`) — aliases ride the same mechanics.
- **systemd is PID1 inside every microVM** (`sandboxes/runtime/Dockerfile.runtime`,
  `internal/microsandbox/runner/microsandbox_init.go`); `/workspace` is a persistent named
  volume that survives sleep/wake (`microsandbox_backend.go`).
- **Sandbox sizing is parameterized** (`internal/model/template_size.go`,
  `internal/microsandbox/api/types.go`); runner exec exists (used by the `/logs` tail today,
  `microsandbox_exec.go`); scheduler bin-packs runners (`scheduler_test.go`).
- **RS256 auth tokens:** `internal/auth/jwt.go` (`IssueAccessToken`, golang-jwt/v5, claims +
  issuer/audience/exp/jti), verification needing only the public key (`internal/auth/validate.go`,
  `serve.go:59` derives it). Encrypted-cookie session reference implementation:
  `apps/web/lib/auth/session.ts` (AES-256-GCM + HKDF).
- **Per-user app authorization is already written:** `internal/access` (`CanUseChannel`, org
  roles, team-scoped channels). Apps are channel-scoped like sheets; "who can launch this app" =
  "who can use its channel".
- **Runtime-secret bearer pattern:** `internal/handler/agent_uploads_stream.go` `authAgent` —
  path-scoped resource, decrypt stored secret (`SandboxEncKey`), constant-time compare. The app
  secret and the internal app API copy this exactly.
- **Sheets service layer:** `internal/sheets.Service` — the validated single entry point both
  REST and MCP use (coercion, limits, org+channel guards, operation log with
  `actor_user_id`/`actor_agent_id`). The app API calls this directly, like MCP tools do.
- **S3 + versioning:** streaming drive upload from builder sandboxes
  (`agent_uploads_stream.go`, no MIME allowlist, transfer-manager); immutable sha256-in-key
  object layout + `archived_at` version rows precedent in `internal/canvasartifact/sync.go`;
  presigned GET primitives (`internal/storage/publicassets.go`). Safe archive extraction
  reference: `cmd/agent-debug-pack/archive.go`.
- **Channel env vars** (`internal/model/channel_env_var.go`, migration 000061): AES-encrypted,
  name-validated (`HIVY_` reserved), decrypted at injection time
  (`internal/agentruntime/runtime_env.go` `mergeChannelEnvVars`). App sandboxes reuse the store;
  no `__ENV__` prefix dance needed (no Rust runtime in app sandboxes — env goes straight into
  the unit environment).

## 0. Data model (Hivy side)

```sql
-- 000063_apps.sql
apps (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  channel_id    uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  sheet_id      uuid NOT NULL REFERENCES sheets(id) ON DELETE CASCADE,  -- the ONE bound sheet
  slug          text NOT NULL,             -- also the default alias stem
  name          text NOT NULL,
  description   text NOT NULL DEFAULT '',
  icon          text NOT NULL DEFAULT '',
  alias         text NOT NULL DEFAULT '',  -- microsandbox alias hostname stem (see §2); '' until claimed
  sandbox_id    uuid,                      -- current app sandbox (nullable: not yet deployed)
  encrypted_app_secret bytea NOT NULL,     -- SandboxEncKey AES-GCM, like sandboxes.encrypted_runtime_secret
  active_version_id    uuid,               -- FK app_versions, SET NULL semantics via app code
  status        text NOT NULL DEFAULT 'draft',  -- draft|deploying|running|stopped|failed
  template_version text NOT NULL DEFAULT '',    -- template rev the bundle was built against
  created_by_agent_id uuid, created_by_user_id uuid, source_session_id uuid,
  archived_at   timestamptz, created_at, updated_at
);
CREATE UNIQUE INDEX ON apps (org_id, slug) WHERE archived_at IS NULL;
CREATE INDEX ON apps (channel_id, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX ON apps (sheet_id) WHERE archived_at IS NULL;

app_versions (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id        uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  source_object_key text NOT NULL,   -- zip of source code
  bundle_object_key text NOT NULL,   -- zip of build output (server binary + public/)
  source_sha256 text NOT NULL,
  bundle_sha256 text NOT NULL,       -- verified by appd before activation
  source_bytes  bigint NOT NULL DEFAULT 0,
  bundle_bytes  bigint NOT NULL DEFAULT 0,
  notes         text NOT NULL DEFAULT '',   -- agent changelog for the version
  template_version text NOT NULL DEFAULT '',
  created_by_agent_id uuid, source_session_id uuid,
  archived_at   timestamptz, created_at
);
CREATE INDEX ON app_versions (app_id, created_at DESC) WHERE archived_at IS NULL;
```

Object keys follow the canvas-artifact immutable layout:
`pub/o/{orgID}/apps/{appSlug}/{sha256}/source.zip` and `.../bundle.zip`. New content = new key;
`app_versions` rows are the history (soft-archive for pruning, never overwrite). Conventions
honored: uuid PKs, `org_id` CASCADE everywhere, `archived_at` + partial indexes, GORM model in
`internal/model/app.go` + hand-written goose migration, `SandboxEncKey` for the secret.

App secret format: `hvapp_` + 32 random bytes hex (mirrors `GenerateAPIKey`), generated at app
creation, encrypted at rest, decrypted only for sandbox injection and rotation. Rotation endpoint
re-generates + re-injects (sandbox env update or redeploy).

## 1. Auth

### 1.1 Launch flow (user → app)

1. `GET /v1/apps/{appID}/launch` — normal web-app auth (JWT + `ResolveUser` + org resolution).
   Authorize via the app's channel: `canUseChannel` with the caller's org role (same predicate
   family as sheets/sessions — `internal/handler/channel_access.go`).
2. Mint the **launch JWT**: RS256 with the existing `AuthHandler` private key, but a distinct
   claims struct + audience so it is useless against the main API:
   - `aud: "hivy-app"`, `iss` unchanged, `exp = now + 60s`, `jti` (one-time)
   - claims: `app_id`, `org_id`, `user_id`, `role`, plus display metadata: `user_name`,
     `user_email`, `user_avatar`, `org_name` — this is what the app renders and what makes
     in-app actions user-attributable.
3. 302 to `https://{alias}.{appsBaseDomain}/auth/callback?token=…`.
4. Template code in the app verifies (RS256 **public key only**, injected as
   `HIVY_AUTH_PUBLIC_KEY` env at deploy — the app never holds a signing secret), checks
   `app_id` matches its own `HIVY_APP_ID`, enforces one-time `jti` (in-memory LRU is fine at
   this scale), then writes its **own cookie session**: AES-256-GCM + HKDF encrypted payload
   (port of `apps/web/lib/auth/session.ts` to the template's Go core), `HttpOnly`,
   `SameSite=Lax`, `Secure`, TTL ~7 days, containing the claims above.
5. Unauthenticated requests to the app redirect to `HIVY_LAUNCH_URL`
   (= `{frontend}/…/apps/{id}/launch`); if the user's Hivy session is alive this round-trip is
   invisible, so session expiry inside the app is a transparent re-auth, not a login wall.

### 1.2 App backend → Hivy API (the app secret)

The SPA never calls us. The app's Go backend calls a **dedicated internal app API**, mirroring
the drive-endpoint shape rather than the public `/v1` surface:

```
/internal/apps/{appID}/v1/sheet                      GET    structure (pages, fields, row counts)
/internal/apps/{appID}/v1/pages/{pageID}/rows/query  POST   sheets.Query body (filter AST, sorts, cursor)
/internal/apps/{appID}/v1/pages/{pageID}/rows        POST   insert (≤100/call)
/internal/apps/{appID}/v1/pages/{pageID}/rows        PATCH  update
/internal/apps/{appID}/v1/pages/{pageID}/rows        DELETE archive
/internal/apps/{appID}/v1/pages/{pageID}/attachments/download-url  POST
```

- Auth: `Authorization: Bearer {app secret}` → load app by path ID, decrypt
  `encrypted_app_secret`, constant-time compare (copy `authAgent`,
  `agent_uploads_stream.go:24-85`). No API keys, no JWT, no org header.
- Handlers are thin shells over `internal/sheets.Service` (exactly how MCP tools consume it),
  with the page-belongs-to-bound-sheet check via `sheets` access helpers
  (`internal/sheets/access.go` `PageInChannel`-style, keyed on `app.sheet_id`). **No schema
  mutation surface exists on this API at all** — structure is read-only by construction.
- Actor attribution: the app forwards `X-Hivy-App-Actor: {user_id}` from its cookie session on
  every mutating call; the handler resolves it against `OrgMembership` (fail-closed like
  `access.Resolve`) and records `actor_user_id` on `sheet_operations`. Absent header = recorded
  as app-only mutation with the app's `created_by_agent_id` provenance.
- The app reaches the API at `HIVY_APP_API_URL`
  (= `{RuntimeControlPlaneBaseURL}/internal/apps/{appID}`, same config source as
  `HIVY_DRIVE_UPLOAD_URL` — `internal/config/control_plane.go`).

**Related hardening (independent, do alongside Phase 1):** close the existing sheets API-key
holes — `canUseSheetChannel` returns true for any API key (`internal/handler/sheets_auth.go:26`,
full cross-channel org access) and `/v1/sheets` has no scope gate (no `"sheets"` in
`model.ValidAPIKeyScopes`). Add the scope + honor channel restrictions for keys. Apps don't use
that surface, but it's the same data.

## 2. Aliases (microsandbox control plane)

A first-class control-plane concept: **alias → (sandbox_id, guest_port)**, so apps (and anything
else later) get stable URLs that survive sandbox recreation.

- **State:** `aliases` in the control-plane store: `alias` (unique, lowercase DNS label),
  `sandbox_id`, `port`, `created_at`, `updated_at`. Validation: DNS-label charset, length,
  reserved-word list (www, api, admin, preview, static, …), availability check at claim time.
- **API:** `PUT /v1/aliases/{alias}` (claim/repoint — body `{sandbox_id, port}`),
  `GET /v1/aliases/{alias}`, `DELETE /v1/aliases/{alias}`. Claim/repoint is atomic; repointing to
  a new sandbox is the whole point (redeploy into a fresh sandbox = one alias update).
- **Gateway:** `/v1/lookup` currently parses `^([0-9]{1,5})-(.+)$` from the Host
  (`app.py:142-154` `parse_preview_host`). Add the alias path: when the host is
  `{alias}.{apps_base_domain}` (or the port-pattern match fails), resolve alias → sandbox+port
  from the route store, then reuse the exact same ensure-ready/wake/lease/activity machinery.
  Control pushes alias routes to the gateway store alongside preview routes
  (`preview_cache.go` `syncPreviewRoute` pattern, same `route_generation` bumping on wake).
- **Caddy:** add the apps base domain to the wildcard vhost list in the Caddyfile template —
  nothing else changes (it already forwards Host to the gateway and proxies to the returned
  upstream). **Locked: apps get their own base domain** (`*.apps.usehivy.com`, config
  `HIVY_MICROSANDBOX_APPS_BASE_DOMAIN`) — cookie isolation from agent previews and a clean seam
  for custom domains later.
- **Go client:** extend the provider driver (`internal/sandbox/microsandbox/driver.go`) with
  alias CRUD; only the microsandbox provider implements it (interface addition returns
  not-supported elsewhere, like warm pool).
- **App level:** the deploy orchestrator claims `{app.slug}` (suffix on collision), stores the
  final string on `apps.alias`, and repoints it whenever the app's sandbox is recreated.

## 3. App sandbox, image, and `hivy-appd`

- **New minimal image** `Dockerfile.app`: Debian slim + systemd (same masking as the runtime
  image) + `hivy-appd`. None of the agent image's node/Chromium/ffmpeg bulk — this collapses
  disk footprint and cold-create time. New template registered like existing images.
- **New size tier `micro`** (256 MB RAM / 1 CPU / small disk) in `model.TemplateSizeSpec` +
  `microsandbox/api/types.go` — app density per runner is the real economics; nano's 1 GB is
  4× what an app needs.
- **`hivy-appd`** — a tiny platform-owned Go daemon (~10 MB RSS), its own systemd unit,
  listening on the reserved control port (7080, same convention as the agent runtime, so
  the existing default preview-port set already exposes it). It is the deploy/observe agent of
  the sandbox, independent of the app process (survives app crashes). Bearer auth = the app
  secret, constant-time. Endpoints:
  - `POST /deploy` `{bundle_url (presigned GET), sha256, version_id}` — download, verify sha256,
    extract to `/workspace/app/releases/{sha}/` (path-traversal-guarded, per
    `cmd/agent-debug-pack/archive.go`), atomically repoint `/workspace/app/current` symlink,
    write the env file, `systemctl restart hivy-app`.
  - `POST /rollback` `{sha256}` — repoint symlink + restart.
  - `GET /logs?lines=&grep=&since=&stream=app|appd` — reads the size-rotated log files under
    `/workspace/logs/` (+ can shell out to `journalctl -u hivy-app` for crash forensics).
  - `GET /health` — systemd unit state + app `/healthz` probe result.
  - `POST /env` — re-write env file + restart (env var changes without a redeploy).
- **`hivy-app.service`** runs `/workspace/app/current/server` on guest port 8080
  (in `exposed_ports`), `WorkingDirectory=/workspace/app/current`,
  `EnvironmentFile=/workspace/app/env`, stdout/stderr → `/workspace/logs/app.log` with size-cap
  rotation. `/workspace` persistence is what makes "logs always present across sleep/wake" true.
- **Injected env (platform, `HIVY_` reserved):** `HIVY_APP_ID`, `HIVY_APP_SECRET`,
  `HIVY_APP_API_URL`, `HIVY_AUTH_PUBLIC_KEY`, `HIVY_LAUNCH_URL`, `HIVY_SESSION_SECRET`
  (generated per app), `HIVY_SHEET_ID`, plus org/app display fields. **Channel env vars** of the
  app's channel are decrypted server-side at deploy/env-update and written into the same env
  file — name validation already forbids `HIVY_*` collisions
  (`channel_environment_variables_types.go`).

Auto-sleep/wake needs zero new work: gateway HTTP activity already keeps sandboxes awake and
`ensure-ready` already handles cold wake; apps have no Rust runtime posting `runtime_busy`, and
don't need it.

## 4. Publish & deploy flow

1. **Builder agent** (in a normal agent sandbox: has node for Vite, Go toolchain added to the
   agent image or installed by the skill) scaffolds from the template, builds
   `source.zip` + `bundle.zip` (bundle = `server` binary, `public/` SPA assets, `app.json`
   manifest with `template_version`).
2. Uploads both via the existing **drive streaming PUT** (bearer = its own sandbox runtime
   secret) — no new upload infra.
3. Calls the **`app_publish` MCP tool** with the two drive keys + shas + notes. Go side: verify
   the keys belong to this agent (`AuthorizeObjectKeys` precedent,
   `internal/sheets/relations_resolve.go`), server-side copy into the immutable
   `pub/o/{orgID}/apps/...` layout, create the `app_versions` row, then deploy:
   - ensure app sandbox exists (create with app image, micro size, exposed port 8080; claim/
     repoint alias) — `CreateSandboxOpts` flow in `orchestrator_create_agent.go` generalized for
     non-agent workloads;
   - presign the bundle key (15-min GET) and call `hivy-appd` `POST /deploy` through the
     sandbox's preview URL (wake-on-request makes this work even if asleep);
   - flip `apps.status` + `active_version_id` on appd success.
4. **Rollback** = `app_rollback` tool → appd `/rollback` to a previous version's sha (bundle
   still on disk) or re-deploy an older `app_versions` row.

## 5. Template (`hivy-app-template`)

- **Stack: Go backend + Vite/React SPA.** Go is the locked backend language: ~10–20 MB RSS
  static binary, 1–2 s incremental compiles (satisfies "super fast builds"; Rust fails this),
  matches the platform team's stack, agents write it reliably. One process serves both the API
  (`/api/*`, agent-written handlers) and the SPA static files (everything else → `public/`,
  SPA-fallback to `index.html`).
- **Layout:**
  ```
  hivycore/        # DO-NOT-EDIT platform core, vendored: session (AES-GCM cookie), JWT
                   # callback + jti guard, auth middleware (on by default for /api/*),
                   # sheets client (typed over the internal app API, actor header automatic),
                   # env access, logging setup, healthz
  api/             # agent-written handlers (registered on an authed router)
  web/             # agent-written React SPA (Vite; calls /api/* only, same origin)
  app.json         # manifest: name, template_version, build commands
  ```
- The sheets client is the only path to data the template offers, and it burns the bound sheet
  in: `hivycore.Sheets().QueryRows(pageID, query)` — the agent cannot construct a call to a
  different sheet because the API surface doesn't have one.
- Build speed: template ships a locked `package.json`; the builder skill pre-installs
  `node_modules` from the lockfile once per sandbox (cached in `/workspace`), so iterative
  builds are `go build` (~1–2 s) + `vite build` (~ seconds).
- `template_version` stamped into `app.json` and recorded on `apps`/`app_versions` — the fleet
  migration handle for when `hivycore` gets a security patch.
- Distribution: template lives in-repo under `global/apps/template/`; the builder skill fetches
  it (same mechanism as other global assets).

## 6. Agent surface (plugin + tools + skill)

New `apps` plugin (`global/plugins/apps/`), gated like the sheets plugin
(`agent_plugin_installs`, session-derived channel scoping — `internal/sheets/mcptools.go:49-78`
pattern):

- `app_create` — name/description/icon + bound `sheet_id` (must be in the session's channel);
  generates the secret, returns app ID + alias preview.
- `app_publish` — drive keys + shas + notes → version + deploy (§4). Returns the live URL.
- `app_status` — status, active version, sandbox state, alias URL.
- `app_logs` — args pass through to appd `/logs` (lines/grep/since/stream); Go side wakes the
  sandbox implicitly by calling through the gateway. This is the whole observability story.
- `app_rollback` — previous version.
- `app_env_sync` — re-push channel env vars (after the user edits them) via appd `/env`.

Skill (`skills/apps/SKILL.md`): scaffold-from-template workflow, the hivycore contract
(what is off-limits), sheet-structure-first development (call `sheet_describe`, generate typed
accessors), build commands, publish flow, debugging-with-`app_logs` loop.

## 7. Web app surface (v1, minimal)

- Channel → Apps tab: list (`GET /v1/apps?channel_id=`), card with status + Open button
  (`/launch` redirect), version history, env vars pointer to the existing channel env UI.
- CRUD: `POST/PATCH/DELETE /v1/apps` (channel-authorized like sheets routes,
  `RequireChannelAccess` pattern from `sheets_auth.go`). Admin actions: rotate secret, rollback.
- (Note: `make openapi` clobbers hand-added spec paths — sheets learned this; keep app spec
  generation in the standard flow from the start.)

## Phases

| Phase | Scope | Depends on |
|---|---|---|
| 0 | **Microsandbox aliases**: control-plane state + API, gateway alias lookup, Caddy apps domain, driver client. Independently shippable/testable with any sandbox. | — |
| 1 | **Data model + internal app API**: `apps`/`app_versions` migration, app secret, `/internal/apps/{id}/v1/*` sheets surface with actor attribution. (+ the independent sheets API-key hardening.) | — |
| 2 | **Image + appd + deploy**: `Dockerfile.app`, `micro` size, `hivy-appd` (deploy/logs/health/env), publish pipeline (drive keys → immutable S3 layout → presigned pull → systemd). | 0, 1 |
| 3 | **Auth handoff**: launch endpoint + launch JWT, template `hivycore` session/callback. | 1 |
| 4 | **Template + builder plugin**: `global/apps/template/`, `apps` MCP tools, skill; first end-to-end agent-built app over a real sheet. | 2, 3 |
| 5 | **Web UI + env vars**: apps tab, launch button, version history, channel env injection + `app_env_sync`. | 4 |

Exit criterion for the whole plan: an agent, given a populated sheet and "build me an app to
manage this", ships a working authenticated CRUD app to a stable URL, then diagnoses a runtime
error in it using only `app_logs` — no human touching the sandbox.

## Open items (decide during implementation, none block Phase 0/1)

- Session-cookie TTL for apps (proposed 7d) and whether launch JWTs should also be redeemable
  for API-side session introspection later.
- Alias claim ownership: control plane is org-agnostic today; decide whether aliases carry an
  org tag for quota/abuse control (proposed: yes, `org_id` column, per-org alias quota).
- Whether `app_create` requires an existing populated sheet or may create one (proposed:
  require existing — keeps "reads structure, never modifies" clean).
- Where the Go toolchain for builders lives: bake into the agent image (simplest, +size) vs.
  mise-install in the skill (proposed: bake — build speed is a requirement).
- appd log endpoint pagination/caps (proposed: hard cap 2000 lines per call, `grep` applied
  before cap).
