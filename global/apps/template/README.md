# Hivy App Template

This is the strict template every Hivy app is built from: a Go backend that
serves an agent-written JSON API under `/api/*` and a static Vite/React SPA
for everything else, over exactly one bound sheet. One process, one port, no
Node at runtime.

An app's job is an interactive interface — CRUD, dashboards, custom tooling —
over data agents already put in a sheet. The app reads the sheet's structure
and reads/writes rows; it can never change the schema (that surface does not
exist on its API).

## Layout

```
hivycore/        Platform core — DO NOT EDIT (see contract below)
api/             Your Go handlers, registered in api.Register
web/             Your React SPA (Vite + TypeScript, calls /api/* only)
main.go          Wiring: hivycore.MustNew() → api.Register(app) → app.Run()
app.json         Manifest: name, template_version, build commands
Makefile         Build tooling (deps / bundle / source / test)
public/          Vite build output (generated — never edit by hand)
dist/            Build artifacts: server, bundle/, bundle.zip, source.zip
```

## The hivycore contract

`hivycore/` is platform-owned and vendored into your app. **Never edit,
delete, or add files in it.** Security patches ship as new template versions
keyed on `template_version`; local edits will be lost and can break auth or
data access. Everything you need is exported:

- `hivycore.MustNew()` — load config, fail fast on bad env (already in main.go).
- `app.HandleAPI(pattern, handler)` — register a handler under `/api` with
  authentication pre-applied. Patterns are Go `ServeMux` patterns relative to
  `/api`: `app.HandleAPI("GET /todos/{id}", h)` serves `GET /api/todos/{id}`.
- `hivycore.UserFrom(r.Context())` — the signed-in user (always present in
  `/api` handlers): `user_id`, `user_name`, `user_email`, `user_avatar`,
  `org_id`, `org_name`, `role`.
- `app.Sheets()` — the typed sheets client (below). This is the only data
  path an app has.
- `hivycore.WriteJSON(w, status, v)` and `app.WriteError(w, r, err)` —
  responses and error relay (an `*hivycore.APIError` from the sheets client
  passes the platform's status and message through to the SPA).
- `app.Log()` — structured JSON logger to stdout. The platform captures
  stdout into the app's logfile; `app_logs` reads it. Never log secrets.

Auth is entirely hivycore's problem, and the model is iframe-first: apps are
opened only from the Hivy frontend, which fetches a one-time launch token
from the Hivy API and mounts the app in an **iframe** at
`/auth/callback?token=…`. hivycore verifies the token (RS256, one-time jti),
sets the encrypted session cookie, and 302s to `/` — the app's single
internal redirect. Nothing in the app ever redirects out to Hivy:

- Unauthenticated HTML navigations and failed callbacks get a minimal
  **401 HTML page** ("You're not signed in") with a plain link to
  `HIVY_LAUNCH_URL`.
- Unauthenticated `/api/*` requests get **401 JSON** `{error, launch_url}`.
- The session cookie is `HttpOnly; Secure; SameSite=None; Partitioned` so it
  works inside the cross-origin Hivy iframe. (Browsers accept Secure cookies
  over plain http on `localhost`/`127.0.0.1`, so local dev works.)
- Every HTML response carries `Content-Security-Policy: frame-ancestors
  'self' <hivy-frontend-origin>` (derived from `HIVY_LAUNCH_URL`), so only
  the Hivy frontend can embed the app. `X-Frame-Options` is never set.

Do not build login screens, tokens, or session handling of your own.

## Config (injected env)

The platform injects these at deploy; locally you must set them all
(`hivycore` fails fast, naming what's missing):

| Var | Meaning |
|---|---|
| `HIVY_APP_ID` | This app's ID (launch tokens must match it) |
| `HIVY_APP_SECRET` | Backend → Hivy API credential (never expose) |
| `HIVY_APP_API_URL` | Internal app API base URL |
| `HIVY_AUTH_PUBLIC_KEY` | PEM RSA key verifying launch tokens |
| `HIVY_LAUNCH_URL` | Hivy frontend launch endpoint (linked from the 401 page; its origin is the allowed iframe embedder — nothing redirects to it) |
| `HIVY_SESSION_SECRET` | Session-cookie encryption secret |
| `HIVY_SHEET_ID` | The bound sheet (informational — API is already scoped) |
| `PORT` | Listen port, default 8080 |

The channel's custom env vars are injected alongside these; read them with
`os.Getenv` as usual. `HIVY_*` names are reserved for the platform.

## Working with the sheet

`app.Sheets()` talks to the internal app API, authenticated with the app
secret and scoped server-side to the one bound sheet. Row data and filters
key by **field ID** (`fld_…`), never by field name — call `Structure()`
first and generate typed accessors for the fields you use.

```go
// Structure: pages, fields (id/name/type/options), row counts.
structure, err := app.Sheets().Structure(r.Context())

// Query: filter AST + sorts + cursor paging (limit clamped to 100 —
// follow NextCursor until empty when you need everything).
result, err := app.Sheets().QueryRows(r.Context(), pageID, hivycore.Query{
    Filter: &hivycore.Filter{And: []hivycore.Filter{
        {Field: "fld_2j9xc4hb", Op: "eq", Value: "qualified"},
    }},
    Sorts: []hivycore.Sort{{Field: "created_at", Desc: true}},
    Limit: 100,
})

// Mutations: ≤100 rows per call. Pass the request context — hivycore
// forwards the session user as the actor so the platform's operation log
// attributes the change to the human using the app.
rows, err := app.Sheets().InsertRows(r.Context(), pageID, []hivycore.RowInsert{
    {Data: map[string]any{"fld_8k2mx1q9": "Acme"}},
})
rows, err = app.Sheets().UpdateRows(r.Context(), pageID, []hivycore.RowUpdate{
    {ID: rowID, Data: map[string]any{"fld_2j9xc4hb": "won"}}, // partial merge
})
archived, err := app.Sheets().DeleteRows(r.Context(), pageID, []string{rowID})

// Attachment cells hold object keys; presign downloads for the browser:
urls, err := app.Sheets().AttachmentDownloadURLs(r.Context(), pageID, keys)
```

Filter ops: `eq neq contains not_contains starts_with gt gte lt lte is_empty
is_not_empty in` (per field type). `Query.Search` does a fuzzy match across
the whole row. `ResolveRelations: true` hydrates relation cells into
`{id, label}` pairs via `QueryResult.Relations`.

Handler style — copy `api/api.go`:

```go
app.HandleAPI("GET /pages", func(w http.ResponseWriter, r *http.Request) {
    structure, err := app.Sheets().Structure(r.Context())
    if err != nil {
        app.WriteError(w, r, err)
        return
    }
    hivycore.WriteJSON(w, http.StatusOK, structure)
})
```

## Frontend rules

- The SPA calls **only** `/api/*` on its own origin. Never call the Hivy API,
  never embed secrets, never talk to third parties from the browser — route
  everything through your Go handlers.
- On a 401 the backend includes `launch_url`; send the **top-level window**
  there (`web/src/api.ts` already does) — the app runs in an iframe, so
  navigating the iframe itself would not re-run the Hivy launch flow.
- `web/package.json` versions are locked and `package-lock.json` is
  committed. Add dependencies only when genuinely needed and keep them
  pinned; every add slows the build.
- Vite builds into `../public` (`web/vite.config.ts`), which the server
  serves with SPA fallback to `index.html`. Client-side routing just works.

## Build & publish

```bash
make deps      # cd web && npm ci           — once per sandbox (cache node_modules)
make test      # go test ./...              — hivycore + your handler tests
make bundle    # vite build + go build → dist/bundle/{server,public/,app.json} + dist/bundle.zip
make source    # dist/source.zip            — sources, excluding node_modules/dist/public
make all       # bundle + source            — the two zips app_publish consumes
```

Iterative loop: after `make deps` once, `make bundle` is a Vite build plus a
1–2 s Go compile. Build in the builder sandbox (linux) so the `server` binary
matches the deploy target.

Local dev: run the server with the env above (`go run .`), then
`cd web && npm run dev` — Vite proxies `/api` and `/auth` to `:8080`.

`app.json` carries `name`, `template_version`, and the build commands. Update
`name` to your app; **never change `template_version`** — it identifies the
platform template revision this app was built from.

## Preview loop

Before publishing, run the app **inside this builder sandbox** with its real
platform environment and let the user try it:

```bash
make web                            # rebuild the SPA when web/ changed
make preview APP_ID=<app uuid>      # PORT=3000 by default
```

`make preview` rebuilds the server, fetches the app's runtime env over an
authenticated side channel (using this sandbox's ambient credentials), writes
it to a 0600 file under `/workspace/.hivy/`, (re)starts `dist/server`
supervised — a systemd unit (`hivy-app-preview.service`) when systemd is
booted, otherwise a background process with a pidfile — waits for `/healthz`,
and prints the public preview URL as the **last line of output**.

Rules for the preview loop:

- Read **only** that final URL line and share it with the user. Never print,
  read, or paste the env file, the endpoint response, or the process
  environment — they contain the app secret and the channel's secrets.
- Iterate: edit code → `make web` (if the SPA changed) → `make preview …`
  again → share the URL. Reruns replace the previous preview.
- **Deploy only after the user explicitly approves.** The preview URL is for
  the user to review the app; `app_publish` (which ships to the stable app
  URL) must wait for their explicit go-ahead.

## Endpoints the platform relies on

Provided by hivycore — do not shadow these paths in your handlers or SPA
routes: `GET /healthz` (unauthenticated liveness + template version) and
`GET /auth/callback` (launch-token exchange).

## Frontend patterns

The SPA ships with routing, data fetching, and error handling already wired.
Follow these patterns — don't hand-roll alternatives.

**Routing (wouter, browser-history mode).** `src/App.tsx` is the shell: a
nav plus a `<Switch>` of `<Route>`s. To add a screen, create a component in
`src/pages/` and add one `<Route path="/thing" component={Thing} />` (plus a
nav `<Link>` if it belongs in the header). Deep links work because the server
serves `index.html` for any non-file path — never switch to hash routing.
Keep the routeless `<Route component={NotFound} />` last in the `<Switch>`;
it is the catch-all 404.

**Data fetching (TanStack Query).** Components never call `fetch` or use
`useEffect` for data. Every endpoint gets a hook in `src/hooks/queries.ts`
built on `useQuery`/`useMutation` and `fetchJSON` from `src/lib/query.ts`
(`fetchJSON` is `api.ts`'s `apiFetch`, so the 401 → top-window relaunch
applies to every request automatically). Every screen must render its
query's `isPending` state with `<Loading />` and its `isError` state with
`<ErrorNotice error={q.error} />` (both in `src/components/Feedback.tsx`)
before touching `q.data` — copy `src/pages/Welcome.tsx`. Mutations follow
the commented example at the bottom of `src/hooks/queries.ts`: a
`useMutation` whose `onSuccess` invalidates the query keys the write
affects. The shared `QueryClient` defaults (retry 1, no focus refetch, 30s
staleTime) live in `src/lib/query.ts` — override per-hook, don't edit the
defaults casually.

**Error boundary.** `src/components/ErrorBoundary.tsx` is mounted once at
the root (`src/main.tsx`) so a render crash shows a friendly panel instead
of white-screening the iframe. Leave it in place; add feature-level
boundaries only if a screen needs to fail independently.

**Data is live by default — never poll, never build refresh buttons.** The
template streams sheet changes over a single SSE connection (`/api/_live`,
served by hivycore) and applies them straight to the TanStack Query cache:
edits patch the affected cells, deletes drop the rows, inserts append where
provably safe, and anything ambiguous triggers a coalesced background
refetch. Screens re-render the instant data changes with no code from you —
so do not add `refetchInterval`, polling loops, manual refresh buttons, or
`window.setInterval` refetches.

Reading rows through the canonical **`useRows(pageID, query?)`** hook is what
opts a screen into this. Its query key is `["rows", pageID, hash(query)]`
(via `rowsKey`), and the realtime engine keys off exactly that shape to find
and update your cached rows:

```tsx
import { useRows } from "../hooks/queries"

function RowsTable({ pageID }: { pageID: string }) {
  const rows = useRows(pageID)                        // whole page (live)
  // const rows = useRows(pageID, { filter, sorts })  // filtered/sorted (live)
  if (rows.isPending) return <Loading />
  if (rows.isError) return <ErrorNotice error={rows.error} />
  return <ul>{rows.data.rows.map((r) => <li key={r.id}>{/* … */}</li>)}</ul>
}
```

Always read rows through `useRows` (never a hand-rolled `useQuery` with a
different key) or live updates won't reach that screen. `useRows` is backed
by `POST /api/pages/{pageID}/rows/query` (see `api/api.go`).

The whole mechanism — `src/lib/realtime.ts`, its `startRealtime` call in
`src/main.tsx`, and the `/api/_live` relay in hivycore — is **template
infrastructure. Do not modify or remove it.** It works automatically; your
only job is to read rows through `useRows`.
