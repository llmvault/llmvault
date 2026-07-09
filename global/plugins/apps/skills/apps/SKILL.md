---
name: apps
description: Use when the user wants an interactive web app — a CRUD tool, dashboard, or custom interface — over data in a sheet, or asks to build, preview, publish, debug, or roll back a Hivy app. This is the execution skill for the app MCP tools and the template workflow: scaffold from the app template, implement Go API handlers and a React SPA, run a live preview the user can try, and — only with their explicit authorization — publish and verify with status and logs.
---

# Apps

A Hivy app is a small production web application over exactly **one sheet**: a Go backend serving a JSON API under `/api/*` plus a static Vite/React SPA, built from a strict template, deployed to a stable URL on Hivy's app hosting. Users open it from the channel with one click and are signed in automatically; the app reads the sheet's structure and does full CRUD on its rows, but can never change the schema — that surface does not exist on its API.

You build the app in your own sandbox from the template, run it there as a live preview the user can open and test, and — once they authorize deployment — zip the source and the build output, upload both to your drive, and publish. The platform copies the artifacts, provisions the app's sandbox, and deploys — synchronously, so `app_publish` returning means the app is live (or failed, with a status telling you so).

## The contract (non-negotiable)

- **App = one sheet.** Bound at `app_create`, forever. Structure is read-only; rows are full CRUD. All data access goes through `app.Sheets()` — the template's client is scoped server-side to the bound sheet and there is no way to reach another one.
- **`hivycore/` is platform core — never edit, delete, or add files in it.** It owns auth (launch callback, encrypted session cookie, 401 handling), the sheets client, config, logging, and static serving. Security patches ship as new template versions; local edits get lost and can break auth.
- **Auth is iframe-first and entirely hivycore's.** Users open apps — live and preview alike — from the Hivy frontend, which mounts them in an iframe with a one-time launch token. Unauthenticated HTML visits get hivycore's built-in 401 page; unauthenticated `/api/*` calls get 401 JSON with `launch_url`, and the template's `fetchJSON` sends the **top-level window** there automatically. You never build login UI, never handle tokens or sessions, never redirect to Hivy yourself.
- **You write `api/` and `web/` only** (plus the `name` in `app.json`). **Never change `template_version` in `app.json`** — it records which platform template revision the app was built from.
- **The SPA calls only `/api/*` on its own origin.** Never the Hivy API, never third parties, never a secret in browser code. Anything external goes through your Go handlers.
- **Never invent field IDs.** Row data and filters key by field ID (`fld_…`), never by field name. `sheet_describe` (or `Structure()` in handlers) first, always.
- **Deployment needs explicit authorization.** `app_publish` ships to the app's stable production URL and runs only after the user explicitly authorizes it — the hard rule in step 8.

## Tools

| Tool | Args | Returns |
|---|---|---|
| `app_create` | `name`, `description?`, `icon?`, `sheet_id` | `app_id`, `slug`, `status` |
| `app_publish` | `app_id`, `source_key`, `bundle_key`, `source_sha256`, `bundle_sha256`, `notes?` | `version_id`, `url`, `status` |
| `app_status` | `app_id` | `status`, `url`, active version, live health report |
| `app_logs` | `app_id`, `lines?`, `grep?`, `since?` | recent production logs |
| `app_rollback` | `app_id`, `version_id` | redeploys that previous version |

`app_publish` is synchronous: it copies your uploaded zips into the platform's immutable version storage, provisions the app sandbox if needed, and deploys. `app_logs` works even if the app sandbox was asleep — the platform wakes it. Pass `notes` on every publish; it is the version's changelog and what makes `app_rollback` targets identifiable later.

## Workflow

### 1. Sheet first

The sheet is the app's entire data model, so pin it down before writing any code (sheets plugin tools):

- `sheet_list` → find the sheet the user means; `sheet_create` (+ `rows_write` for seed data) only if the app is over data that does not exist yet.
- `sheet_describe` → capture the **sheet ID, page IDs, and the full field legend** (id / name / type / options). Every handler you write keys by these field IDs.

### 2. Create the app record — early

```json
{ "name": "Leads Manager", "description": "Browse, qualify, and edit the leads sheet.", "sheet_id": "5f0b6c1e-…" }
```

`app_create` binds the sheet and returns `app_id` and `slug`. Create it before you write code: `make preview` fetches the app's runtime environment by `app_id`, so the record must exist for your first preview. Keep both values — the slug names your workspace directory and drive upload folder; the `app_id` goes into every later tool call.

### 3. Fetch and unpack the template

The template zip is served next to the drive endpoint: take `$HIVY_DRIVE_UPLOAD_URL`, replace the trailing `/drive` with `/apps-template.zip`, GET it with the drive bearer:

```bash
template_url="${HIVY_DRIVE_UPLOAD_URL%/drive}/apps-template.zip"
curl -fsS --retry 3 -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
  -o /tmp/apps-template.zip "$template_url"
mkdir -p /workspace/apps/leads-manager
unzip -q /tmp/apps-template.zip -d /workspace/apps/leads-manager
cd /workspace/apps/leads-manager
```

Layout of what you just unpacked:

```
hivycore/        Platform core — DO NOT EDIT
api/             Your Go handlers, registered in api.Register
web/             Your React SPA (Vite + TypeScript, calls /api/* only)
main.go          Wiring: hivycore.MustNew() → api.Register(app) → app.Run() — leave as is
app.json         Manifest: set name; NEVER touch template_version
Makefile         deps / web / bundle / source / all / test / preview
scripts/         preview.sh — powers make preview; leave as is
public/          Vite build output (generated — never edit by hand)
dist/            Build artifacts: server, bundle/, bundle.zip, source.zip
```

Read the template's `README.md` — it is the authoritative reference for everything below.

### 4. Implement the API (`api/`)

Handlers register in `api.Register` with Go `ServeMux` patterns relative to `/api`; auth is pre-applied, so the session user is always present:

```go
func Register(app *hivycore.App) {
    app.HandleAPI("GET /me", handleMe(app))
    app.HandleAPI("GET /leads", handleListLeads(app))
    app.HandleAPI("PATCH /leads/{id}", handleUpdateLead(app))
}
```

`app.Sheets()` is the only data path — a typed client over the internal app API, scoped to the bound sheet, forwarding the session user as the mutation actor automatically when you pass the request context:

```go
// Structure: pages, fields (id/name/type/options), row counts. Call it once
// at startup or per request — it is how you resolve page and field IDs.
structure, err := app.Sheets().Structure(r.Context())

// Query: filter AST + sorts + cursor paging. Limit clamps to 100 — follow
// NextCursor until empty when you need everything. Fields are field IDs.
result, err := app.Sheets().QueryRows(r.Context(), pageID, hivycore.Query{
    Filter: &hivycore.Filter{And: []hivycore.Filter{
        {Field: "fld_2j9xc4hb", Op: "eq", Value: "qualified"},
    }},
    Sorts: []hivycore.Sort{{Field: "created_at", Desc: true}},
    Limit: 100,
})

// Mutations: ≤100 rows per call; updates partial-merge only the keys sent.
rows, err := app.Sheets().InsertRows(r.Context(), pageID, []hivycore.RowInsert{
    {Data: map[string]any{"fld_8k2mx1q9": "Acme"}},
})
rows, err = app.Sheets().UpdateRows(r.Context(), pageID, []hivycore.RowUpdate{
    {ID: rowID, Data: map[string]any{"fld_2j9xc4hb": "won"}},
})
archived, err := app.Sheets().DeleteRows(r.Context(), pageID, []string{rowID})

// Attachment cells hold object keys; presign downloads for the browser:
urls, err := app.Sheets().AttachmentDownloadURLs(r.Context(), pageID, keys)
```

Handler conventions (copy `api/api.go`):

- `hivycore.UserFrom(r.Context())` → the signed-in user (`user_id`, `user_name`, `user_email`, `user_avatar`, `org_id`, `org_name`, `role`) for display and app-level permission checks.
- `hivycore.WriteJSON(w, status, v)` for responses; `app.WriteError(w, r, err)` to relay errors — an `*hivycore.APIError` from the sheets client passes the platform's status and message through to the SPA.
- `app.Log()` for structured JSON logs to stdout — this is exactly what `app_logs` reads later, so log the things future-you needs to debug. Never log secrets.
- Filter ops: `eq neq contains not_contains starts_with gt gte lt lte is_empty is_not_empty in`. `Query.Search` fuzzy-matches across the row; `ResolveRelations: true` hydrates relation cells into `{id, label}` via `QueryResult.Relations`.

Hardcoding the page and field IDs you captured in step 1 as named constants is fine and normal — they are stable. Generating a small typed accessor layer (`leadFromRow(row)`) keeps handlers readable.

### 5. Implement the SPA (`web/`)

The template ships with routing, data fetching, and error handling pre-wired. Build every screen to the **Frontend quality bar** below — those are instructions, not suggestions. Plus:

- Everything through `/api/*`, same origin, via `fetchJSON` — which carries the 401 → top-window relaunch, so re-auth is invisible while the user's Hivy session is alive. Never build a login screen.
- `web/package.json` is locked and `package-lock.json` committed. Add dependencies only when genuinely needed, pinned — every add slows the build.
- Vite builds into `../public`, which the server serves with SPA fallback to `index.html`, so client-side routing just works.
- Do not shadow the platform paths `/healthz` and `/auth/callback` in routes or handlers.

### 6. Build

```bash
make deps    # cd web && npm ci — ONCE per sandbox; node_modules is cached after
make all     # bundle + source → dist/bundle.zip + dist/source.zip
make test    # go test ./... AND cd web && npm test (vitest) — needs make deps for the web half
```

`make all` runs the Vite build plus a 1–2 s Go compile — that is your whole iteration cost after the first `make deps`. App sandboxes are **linux/amd64**, and the Makefile defaults `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`, so a plain `make all` produces the right `server` binary from any builder — no cross-compile flags to set.

`make test` runs both suites: `go test ./...` and, in `web/`, `npm test` (`vitest run --passWithNoTests`, so it needs `make deps` for node_modules). The delivered template zip strips all test files (`*_test.go`, `*.test.*`, `vitest.config.*`, `__tests__/`, `testdata/`, `fixtures/`), so out of the box both halves pass with nothing to run — `make test` is a build/type check, not a quality gate. It only exercises real tests once you write your own.

Fix every build failure before previewing or publishing. Do **not** write tests for the app unless it has grown genuinely complex — multi-page flows, tricky server logic; the template's own machinery is already tested by the platform. Your verification is the running app: preview it and drive it in the headless browser (step 7).

### 7. Preview with the user

Run the app inside your own sandbox with its real platform environment and let the user try it before anything ships:

```bash
make preview APP_ID=<app_id> PORT=3000
```

`make preview` rebuilds the server, fetches the app's runtime env over an authenticated side channel, writes it to a 0600 file, (re)starts the preview supervised (a systemd unit when systemd is booted, a background process otherwise), waits for `/healthz`, and prints the public preview URL — carrying the `?app=<app_id>` hint the Hivy frontend needs to route the launch — as the **last line of stdout**, right after a line telling you to share ONLY that URL. All progress goes to stderr.

- **Read only that final URL line, and share it exactly, hint included.** Never strip the `?app=<app_id>` query, and never share a bare `{port}-{id}.<preview-domain>` URL without it — the frontend can't route an unhinted link. Never print, read, or inspect the env file (under `/workspace/.hivy/`), the preview-env response, or the process environment — they contain the app secret and the channel's secrets.
- Verify the server itself before you share — not the authenticated UI: `curl {preview_url}/healthz` (expect `{"status":"ok"}`) and `curl {preview_url}/` (expect 200 HTML) against the raw preview URL. You can also `browser open <preview-url>` to eyeball it, `browser snapshot -i`, click/fill through key interactions, `browser console` for errors — but you have no user session, so a direct open lands on hivycore's "you're not signed in" page (or a 401 with `launch_url` for `/api/*` calls). That's expected, not a bug — don't report it as one; it just isn't how you exercise the signed-in flows the user will see.
- Share the preview URL with the user exactly as printed and ask them to test it — and tell them it's ephemeral: tied to this builder sandbox, it stops working once the session ends, unlike a deployed app's link. Preview URLs authenticate through the Hivy frontend exactly like the live app; opened raw, or without the `?app=` hint, they show the 401 page.
- Iterate: edit code → `make web` (only if `web/` changed; preview rebuilds the Go server itself) → `make preview APP_ID=<app_id> PORT=3000` again → share the URL. Reruns replace the previous preview.

### 8. Deploy — only with explicit authorization

**HARD RULE — deployment authorization.** `app_publish` ships to the app's stable production URL and runs ONLY after the user explicitly authorizes deployment. Explicit authorization = the user says to deploy/ship/publish in the conversation — including in their original request ("build X and deploy it" counts; previewing first is still good practice when the change is substantial, but you may then proceed to deploy without re-asking). If authorization is absent or ambiguous: preview, share the URL, and ASK. Re-deploying an already-live app also requires explicit authorization for THAT change.

Once authorized: `make all`, compute sha256 for both zips (`app_publish` verifies them):

```bash
sha256sum dist/source.zip dist/bundle.zip   # or: shasum -a 256 …
```

Upload both via the drive streaming PUT (same flow as CSV uploads in the sheets skill) and capture the returned keys:

```bash
source_key=$(curl -fsS --retry 3 -X PUT \
  -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
  -H "Content-Type: application/zip" \
  --upload-file dist/source.zip \
  "$HIVY_DRIVE_UPLOAD_URL/apps/leads-manager/source.zip" | jq -r '.key')

bundle_key=$(curl -fsS --retry 3 -X PUT \
  -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
  -H "Content-Type: application/zip" \
  --upload-file dist/bundle.zip \
  "$HIVY_DRIVE_UPLOAD_URL/apps/leads-manager/bundle.zip" | jq -r '.key')
```

Then publish:

```json
{
  "app_id": "…",
  "source_key": "pub/e/…/apps/leads-manager/source.zip",
  "bundle_key": "pub/e/…/apps/leads-manager/bundle.zip",
  "source_sha256": "…",
  "bundle_sha256": "…",
  "notes": "Initial version: leads table with status filter and inline qualify/won editing."
}
```

### 9. Verify

`app_publish` returns `version_id`, the live `url` — carrying the same `?app=<app_id>` hint as the preview URL — and `status`. Then:

1. `app_status` — confirm the app is running and the new version is active.
2. `curl -fsS <url>/healthz` — liveness plus the template version, no auth needed.
3. `app_logs` with a small `lines` value — confirm clean startup, no error lines.

Give the user the live `url` exactly as returned, hint included — never a bare alias — only after all three check out. A direct open (or a curl of the app UI itself rather than `/healthz`) may still show "invalid preview host" until the alias is fully routed; that's expected, not a failed publish. If publish or startup failed, `app_logs` (add `grep` for `error`/`panic`) is your debugger — production logs survive sleep/wake and the tool wakes the sandbox for you.

### 10. Iterate

Feedback loops through the preview: edit `api/` or `web/` → rebuild (`make web` if the SPA changed) → `make preview APP_ID=<app_id> PORT=3000` → share the URL again. Publish a new version only when the user authorizes shipping that change: `make all` → `sha256sum` → re-upload both zips (a new upload path or filename per version keeps keys unambiguous) → `app_publish` with fresh keys, hashes, and `notes` describing the change → `app_status` + `app_logs` to verify. Publishing never mutates a previous version — each publish is a new version row.

If a deploy regresses — new version erroring, feature broken — `app_rollback` with the last good `version_id` restores it immediately. Roll back first, debug at leisure, republish when fixed.

## Frontend quality bar

These are requirements for every screen you ship — the template already wires them; do not hand-roll alternatives:

- **Routing (wouter, browser-history mode).** `src/App.tsx` is the shell: a nav plus a `<Switch>` of `<Route>`s. Each screen is a component in `src/pages/` with one `<Route path="/thing" component={Thing} />` (plus a nav `<Link>` if it belongs in the header). Keep the routeless `<Route component={NotFound} />` last in the `<Switch>` — it is the catch-all 404. Never switch to hash routing; deep links work because the server falls back to `index.html`.
- **Data fetching (TanStack Query).** Components never call `fetch` or use `useEffect` for data. Every endpoint gets a hook in `src/hooks/queries.ts` built on `useQuery`/`useMutation` and `fetchJSON` from `src/lib/query.ts` (`fetchJSON` is `api.ts`'s `apiFetch`, so the 401 → top-window relaunch applies to every request automatically).
- **Loading and error states, every screen.** Render `isPending` with `<Loading />` and `isError` with `<ErrorNotice error={q.error} />` (both in `src/components/Feedback.tsx`) before touching `q.data` — copy `src/pages/Welcome.tsx`.
- **Mutations invalidate.** Every `useMutation`'s `onSuccess` invalidates the query keys the write affects (the commented example at the bottom of `src/hooks/queries.ts` is the pattern), so the UI reflects the write without a reload.
- **Root ErrorBoundary stays.** `src/components/ErrorBoundary.tsx` is mounted once in `src/main.tsx` so a render crash shows a friendly panel instead of white-screening the iframe. Leave it; add feature-level boundaries only if a screen must fail independently.
- **QueryClient defaults** (retry 1, no focus refetch, 30 s staleTime) live in `src/lib/query.ts` — override per-hook, don't edit the defaults casually.

## Rules

- Never edit, delete, or add files under `hivycore/`. Never change `template_version` in `app.json`.
- `app_publish` only after the user explicitly authorizes deployment (step 8's hard rule) — for the first deploy and for every re-deploy of a live app.
- `make preview` output: read only the last stdout line (the hinted preview URL). Never print, read, or paste the env file, the preview-env response, or the process environment.
- Never invent page or field IDs — `sheet_describe` / `Structure()` first, then key everything by `fld_…` IDs, never display names.
- All data access through `app.Sheets()`; all browser traffic through `/api/*` same-origin. No secrets in the SPA, no third-party calls from the browser. No login UI, tokens, or session handling of your own — hivycore owns auth.
- The app reads sheet structure and CRUDs rows; it never modifies schema. Schema changes are sheets-plugin work (`sheet_manage`), done outside the app.
- `make deps` once per sandbox, then `make all` per iteration; deploy target is linux/amd64.
- Verify before reporting: `app_status`, `/healthz`, and a clean `app_logs` read after every publish. Never hand the user a URL you have not checked — previews included. Check the raw URL's `/healthz` and `/` with `curl`; a direct browser open or curl of `/api/*` hitting the sign-in wall or a 401 with `launch_url` is expected there, not a check failure.
- Always share the `?app=<app_id>`-hinted URL `make preview` prints or `app_publish` returns — never a bare `{port}-{id}.<preview-domain>` or alias without it. Preview links are ephemeral (die with the builder session); say so when sharing one — only a deployed app's link is durable.
- No test suites for the app unless it has grown genuinely complex (multi-page flows, tricky server logic) — verify in the headless browser on the preview instead; `make test` (`go test ./...` plus `vitest`) is a build/type check on the test-stripped template, not your quality gate.
- Rows in mutations cap at 100 per call; queries clamp to 100 with cursor paging — handle `NextCursor` in handlers that need full data.
- Env: the channel's custom env vars are injected into the app (read with `os.Getenv`); `HIVY_*` names are platform-reserved. Never echo or log secret values.
- Do not shadow `/healthz` or `/auth/callback`.

## Final response checklist

When handing over a preview, state: the preview URL (hint included) and that it's ephemeral to this session, what the app does (routes/views), what you want the user to try, that you already verified it serves (`/healthz`, `/`, and a headless-browser look — expecting the sign-in wall, not a rendered app), and that you will deploy once they give the go-ahead.

After a deploy, state:

- The app name and the live URL, and which sheet (and pages) it is bound to.
- What the app does — the routes/views a user will find.
- The version published (`version_id`, your `notes`) and that status, healthz, and logs were verified.
- Any assumptions (field semantics, which page is primary, permissions applied by role) the user may want to adjust.
