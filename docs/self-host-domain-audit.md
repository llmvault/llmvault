# Self-Host Domain Hardcoding Audit

**Date:** 2026-07-05
**Goal:** Enumerate every hardcoded `usehivy.com` (and subdomain) reference that blocks self-hosting, rank by migration complexity, and propose a config-driven design so a self-hoster's own domain adapts automatically.

**Self-host scope (in):** main app, API, frontend, Docker sandbox provider.
**Out of scope:** Ansible deployment (`ansible/`, 12 files — maintainer infra only).

---

## TL;DR

The architecture is **already ~85% config-driven**. Most of the ~245 raw matches are test fixtures, OCI labels, comments, marketing links, and env-vars-with-defaults that are *already overridable*. The real work is a **short list of genuine blockers**, dominated by two structural problems:

1. **Frontend `NEXT_PUBLIC_*` values are baked into the JS bundle at build time** → a self-hoster pulling a prebuilt image cannot repoint the API/preview/assets hosts without rebuilding.
2. **The wildcard preview stack (`*.preview.<domain>` DNS + TLS + routing) is simply absent from the shipped self-host `hosting/` stack** — not hardcoded, just not there.

Everything else is Easy/Trivial: add a few missing env vars, delete a few hardcoded fallbacks that bypass env vars that already exist, and neutralize sample/default strings.

### Raw counts by subsystem

| Subsystem | Matches | Files | Functional blockers | Rest (test/label/comment/marketing) |
|---|---:|---:|---:|---:|
| Go backend (`internal/`, `cmd/`) | 93 | 34 | ~9 | ~84 |
| Frontend (`apps/web`, `apps/emails`) | 29 | 13 | ~6 | ~23 |
| Rust runtime + infra (`sandboxes/runtime`, `services/`, `hosting/`, compose, Caddy) | 30 | 15 | 1 code + 1 structural gap | ~28 |
| Skills / global / docs (`global/`, `skills/`, `docs/`, `scripts/`, `.github/`) | 93 | 25 | 4 behavioral | ~89 |
| **Total** | **~245** | **~87** | **~20 real** | **~225 noise/cosmetic** |

---

## Existing configuration mechanisms (the good news)

Self-hosting is close because most layers already read env:

- **Go backend** — `internal/config/config.go`, one `Config` struct via `caarlos0/env` (`env:"..." envDefault:"..."`). Existing domain env vars: `HIVY_FRONTEND_URL` (required), `HIVY_CORS_ORIGINS`, `HIVY_API_WEBHOOK_BASE_URL` (→ `RuntimeControlPlaneBaseURL()`), `HIVY_PROXY_HOST`, `HIVY_AUTH_AUDIENCE`, `HIVY_RESEND_FROM`, `HIVY_PREVIEW_CNAME_TARGET`, `HIVY_ACME_DNS_API_URL`, `HIVY_CADDY_ADMIN_URL`, `HIVY_MCP_BASE_URL`.
- **Microsandbox service** — separate `internal/microsandbox/config/config.go` + gateway `services/microsandbox-gateway/src/microsandbox_gateway/config.py`. Preview host is env-driven: `HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN`.
- **Rust runtime** (`sandboxes/runtime/`) — pure env config. **No domain is compiled into the binary** (only `.env.example` samples + test fixtures). Reads `HIVY_CONTROL_PLANE_URL`, `HIVY_WEBHOOK_URL`, etc.
- **Caddy** (`hosting/caddy/Caddyfile`) — fully placeholder-driven: `{$API_DOMAIN}`, `{$MCP_DOMAIN}`, `{$APP_DOMAIN}`, `{$CONNECT_DOMAIN}`, `{$NANGO_DOMAIN}`, `{$ACME_EMAIL}`. No hardcoded host.
- **Nginx edge** (`proxy.nginx.conf`, `docker/local-proxy.nginx.conf`) — host-agnostic (regex/default server_name).
- **Frontend** — `NEXT_PUBLIC_HIVY_API_URL`, `HIVY_API_URL` (server proxy, runtime), `NEXT_PUBLIC_HIVY_CONNECTIONS_HOST`, `NEXT_PUBLIC_HIVY_ASSETS_URL` (declared but unused), `HIVY_EMAIL_ASSET_URL`, `HIVY_SENTRY_URL`.

**Gaps in the config surface:** there is no env var for (a) the sandbox **preview base domain** used by the Go server + agent system prompt, (b) the sandbox/app **image repositories** (only the tag is configurable), (c) the frontend **preview domain** + **asset-preview API host** classifiers. And the frontend's public vars are build-time only.

---

## Ranked findings by migration complexity

### TIER 1 — HARD (architectural / structural)

#### H1. Frontend `NEXT_PUBLIC_*` build-time inlining
- **Where:** `apps/web/Dockerfile:16-27` (ARG→ENV in build stage), consumed at `apps/web/lib/api/client.ts:5`, `apps/web/app/w/(chat)/plugins/use-connect-integration.ts:49,90`, `apps/web/app/(admin)/admin/page.tsx:5`.
- **Problem:** Next.js string-inlines every `NEXT_PUBLIC_*` at `pnpm build`. The hosted `api.usehivy.com` / `connections.usehivy.com` are frozen into the compiled bundle. A self-hoster pulling a prebuilt image **cannot** repoint them at runtime; only `HIVY_API_URL` (the server-side proxy route) is runtime-settable.
- **Fix options:**
  - **(Recommended) Runtime config injection:** serve a small `/config.js` or `window.__HIVY_ENV__` from the Next server (or the `/api/proxy` origin) read at container start, and have `apiUrl()` + connect host read from it with the build-time value as fallback. Makes the *same image* self-hostable.
  - **(Cheap) Document rebuild:** publish build args and require self-hosters to `docker build` with their own `NEXT_PUBLIC_*`. Zero code, but no "pull-and-run".
- **Complexity:** Hard (touches the app's config bootstrap + every `NEXT_PUBLIC_` reader).

#### H2. Wildcard preview stack absent from self-host `hosting/`
- **Where (absence):** `hosting/caddy/Caddyfile` (no `*.preview.<domain>` site block, no on-demand-TLS / DNS-01), `hosting/docker-compose.production.yml` (microsandbox-gateway **not deployed**; default provider is `docker`), `hosting/.env.example` (no `HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN`), stock `caddy:2-alpine` image has **no DNS plugin** so DNS-01 wildcard certs aren't even possible without a custom build.
- **Problem:** The preview *host string* `{port}-{sandbox_id}.{base_domain}` is already config-driven (see M1/`config.py:53`). What's missing is the **plumbing**: wildcard DNS, wildcard TLS, and a Caddy site block that looks up the sandbox via the gateway and reverse-proxies. Self-hosters get no cloud-style preview path out of the box.
- **Fix:** Ship, behind a documented "previews" profile: (1) a custom Caddy image with a DNS provider plugin **or** an on-demand-TLS `ask` endpoint; (2) a `*.preview.{$PREVIEW_DOMAIN}` Caddyfile block calling the gateway `/v1/lookup`; (3) the microsandbox-gateway service added to the compose stack; (4) `PREVIEW_DOMAIN` / `HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN` in `hosting/.env.example`; (5) docs for the `*.preview` DNS record.
- **Complexity:** Hard (new infra components + custom image), but self-contained and optional (a self-hoster who doesn't need previews can skip it).

### TIER 2 — MEDIUM (multiple call sites / behavioral contracts)

#### M1. Go sandbox preview base domain (feeds the agent system prompt)
- **Where:** `internal/agentruntime/system_prompt_base.go:17` (`defaultPreviewBaseDomain = "preview.usehivy.com"`, injected into the system prompt at lines 90-92), `internal/handler/apps_preview_env.go:23` (`appsPreviewDefaultBaseDomain`, used at :124 `fmt.Sprintf("https://%d-%s.%s", port, externalID, baseDomain)`).
- **Problem:** STRUCTURAL. Domain is derived from `sandbox.RuntimeURL` at runtime (good) but falls back to the hardcoded const when derivation fails — and there is **no env var** for the fallback. Because it lands in the **agent system prompt**, a self-hoster's agents will literally tell users to visit `*.preview.usehivy.com`.
- **Fix:** Add `HIVY_PREVIEW_BASE_DOMAIN` to `config.go`; thread it into both packages; delete the two duplicated consts.
- **Complexity:** Medium (two packages, and it touches the system-prompt contract — tests in `system_prompt_base_test.go` assert the value).

#### M2. Frontend chat URL classifiers
- **Where:** `apps/web/app/w/(chat)/_lib/asset-preview-links.ts:5` (`PREVIEW_HOST = api.usehivy.com`), `apps/web/app/w/(chat)/_lib/preview-browser-links.ts:14` (`PREVIEW_DOMAIN = preview.usehivy.com`).
- **Problem:** These gate whether agent-shared asset/preview URLs render as rich cards (and port parsing). Self-hosted hosts won't match → previews silently don't render.
- **Fix:** Derive both from the configured API/preview base (ties into H1's runtime config). Update coupled test fixtures (`*.test.ts`).
- **Complexity:** Medium (depends on H1's config source; test fixtures coupled).

#### M3. `next/image` remotePatterns hardcodes assets host
- **Where:** `apps/web/next.config.mjs:9` (`assets.usehivy.com`). The declared `NEXT_PUBLIC_HIVY_ASSETS_URL` is **unused**.
- **Problem:** `next/image` rejects images from any other host. Runs at build → needs a build arg or wildcard.
- **Fix:** Feed `NEXT_PUBLIC_HIVY_ASSETS_URL` (build arg) into `remotePatterns`, or widen the pattern for self-host.
- **Complexity:** Medium (build-time config).

#### M4. Fixed container image repositories
- **Where:** `internal/sandbox/runtime_snapshot_ref.go:13-14` (`ghcr.io/usehivy/hivy-sandboxes-runtime[-developers]`), `internal/apps/image.go:11` (`ghcr.io/usehivy/hivy-app`). Only the **tag** is env-configurable (`HIVY_SANDBOXES_RUNTIME_IMAGE_TAG`), the repo is a fixed `const`.
- **Problem:** A self-hoster publishing their own runtime/app images cannot point to them without editing code. Not a domain per se, but a hard self-host blocker.
- **Fix:** Add `HIVY_SANDBOXES_RUNTIME_IMAGE_REPO` / `HIVY_APP_IMAGE_REPO`.
- **Complexity:** Medium (image-ref plumbing; defaults keep the hosted path working).

#### M5. Custom-domain automation assumes usehivy cloud topology
- **Where:** `internal/config/config.go:124` + `.env.example:207` `HIVY_PREVIEW_CNAME_TARGET` (`preview-proxy.usehivy.com`), `.env.example:206` `HIVY_ACME_DNS_API_URL`, `.env.example:209` `HIVY_CADDY_ADMIN_URL`. Go pokes Caddy's admin API to add per-customer preview certs.
- **Problem:** All default to usehivy infra and assume the cloud Caddy-admin + acme-dns topology. Overridable, but meaningless without the H2 stack.
- **Complexity:** Medium (couples to H2; document as an advanced/optional feature).

### TIER 3 — EASY (swap to / consolidate onto existing env; delete stray fallbacks)

| # | Where | Issue | Fix |
|---|---|---|---|
| E1 | `internal/config/control_plane.go:5`, `internal/handler/triggers.go:55`, `internal/mcpserver/http_trigger_tool.go:156` | Three independent `https://api.usehivy.com` fallbacks that bypass the canonical `HIVY_API_WEBHOOK_BASE_URL`; these build **agent-visible** webhook/trigger URLs | Route all through `config.RuntimeControlPlaneBaseURL()`; make the env effectively required in prod |
| E2 | `apps/web/components/integration-logo.tsx:17` | Hardcoded `connections.usehivy.com` while `NEXT_PUBLIC_HIVY_CONNECTIONS_HOST` already exists | Swap to the existing env var |
| E3 | `global/apps/template/web/vite.config.ts:18,26` | `allowedHosts: [".preview.usehivy.com"]` **ships into every agent-built app**; Vite dev/preview server refuses a self-hoster's preview host | Derive from a `PREVIEW_DOMAIN` env / `allowedHosts: true` in the template |
| E4 | `apps/emails/lib/hivy-email.tsx:17` | `siteUrl = https://usehivy.com` fallback → logo/footer/Terms/Privacy links + default asset base | Add `HIVY_EMAIL_SITE_URL` env |
| E5 | `internal/config/config.go:56` (`HIVY_RESEND_FROM`) + `.env.example:190` | Default `betty@notifications.usehivy.com` silently fails DKIM/SPF if unset | Neutral default + doc as required |
| E6 | `apps/web/app/w/(chat)/_lib/internal-app-links.ts:15-17` | `APP_HOSTS` hardcodes `usehivy.com/www/app`; runtime same-origin still cards (partial) | Add configured app origin to the set |
| E7 | `services/microsandbox-gateway/.../config.py:53` | Default `preview.usehivy.com` | Neutral default + document `HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN` (tests already use `preview.test`) |
| E8 | `scripts/public-assets-cors.json:5,6` | CORS allowlist `usehivy.com`, `*.usehivy.com` | Parameterize for self-host origins |
| E9 | `global/plugins/skill-manager/skills/skill-creator/references/sandbox-environment.md:27`, `compatibility-checklist.md:32` | Skill reference docs teach the agent the `{port}-{id}.preview.usehivy.com` scheme as fact | Replace with a placeholder / instruct to use tool-provided URL (see the good pattern in `global/agents/hakaree-software-engineer/prompts/instructions.md:36`) |

### TIER 4 — TRIVIAL (already overridable → just document, or purely cosmetic)

**Already env-overridable (document in a self-host `.env` guide — no code change):**
- `internal/config/config.go:44` `HIVY_AUTH_AUDIENCE` (`https://api.usehivy.com`)
- `internal/config/config.go:109` `HIVY_API_WEBHOOK_BASE_URL`
- `internal/config/config.go:110` + `proxy.go:8` `HIVY_PROXY_HOST` (`proxy.usehivy.com`)
- `internal/microsandbox/config/config.go:61` `HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN` (`.local`)
- `sandboxes/runtime/.env.example:4,7` sample `control.usehivy.com` + `aria-prod` runtime id
- Nango image refs: `scripts/ensure-nango-image.sh`, `scripts/ci-start-nango.sh`, `docker-compose.yml:265` (`HIVY_NANGO_IMAGE`)

**Cosmetic / no functional impact (optional cleanup for a clean grep):**
- `internal/providerheaders/openrouter.go:9` OpenRouter `HTTP-Referer` (`https://usehivy.com`) — analytics attribution only
- `sandboxes/runtime/crates/agent/src/runner.rs:40` out-of-credits message link (`usehivy.com/w`)
- Marketing: `apps/web/app/page.tsx:7` (GitHub link), `plans-page.tsx:477` (`mailto:hello@`), 8× `global/plugins/*/plugin.json` `"website"`
- Generated API docs host `api.dev.usehivy.com`: `docs/swagger.yaml`, `docs/swagger.json`, `docs/openapi.json`, `docs/docs.go`, `cmd/server/main.go:29` (`@host`) — regenerated by swag, fix generator config not by hand
- OCI `image.source` labels in all Dockerfiles — provenance metadata, leave as-is
- Comments: `config.go:125-126`, `proxy.nginx.conf:36,86,136`, `cmd/verify-devbox/main.go:7`, `cmd/fake-nango/html.go:6`
- Illustrative "never share a bare …" prohibitions: `global/plugins/apps/skills/apps/SKILL.md:162,243`, `global/agents/ricky-app-builder/prompts/instructions.md:11`
- `skills/fake-nango/SKILL.md:3` (names the hosted Nango tenants the skill avoids)

**Test fixtures (~100 total across Go/TS/Python/Rust) — none break self-host:**
- Go: ~70 `_test.go` + 5 `testdata/railway_agent.json` (`@test.usehivy.com`, `api.usehivy.com`, `assets.usehivy.com`, `app.usehivy.com`, `mcp.usehivy.com`)
- Frontend: 14 lines across `preview-browser-links.test.ts`, `internal-app-links.test.ts`, `asset-preview-links.test.ts`, `session-workspace-store.test.ts`
- Rust: 8 (`config_snapshot_integration.rs`, `repos/service.rs`), smoke-test scripts
- Python: `scripts/gen_rag_test_data.py` (11 synthetic), gateway tests already use neutral `preview.test`
- Emails: PreviewProps sample data (`org-invite.tsx:41`, `auth-password-reset.tsx:35`)
- Several tests already use placeholder domains (`usehivy.test`, `preview.test`, `usehivy.local`) — the pattern to standardize on.

**Maintainer CI/deploy (out of self-host scope — self-hosters fork the pipeline):**
- `.github/workflows/release.yml`, `publish-api-image.yml`, `publish-runtime-manifests.yml` — `ghcr.io/usehivy/*` images, `RAILWAY_SERVICES`, `runner-N.sandboxes.usehivy.com` deploy targets
- `scripts/release/*.sh`, `scripts/ci-test-shard.sh` (Go module import paths `github.com/usehivy/hivy` — repo identity, not a domain)

---

## Proposed config-driven self-host design

### Principle: one base domain, derived subdomains, with per-service overrides

Introduce a single canonical knob and derive the rest, while keeping explicit per-service overrides for anyone who splits hosts:

```
HIVY_BASE_DOMAIN=example.com          # the only thing most self-hosters set
# Derived defaults (each independently overridable):
#   API            -> api.${HIVY_BASE_DOMAIN}
#   APP/FRONTEND   -> app.${HIVY_BASE_DOMAIN}
#   PREVIEW        -> preview.${HIVY_BASE_DOMAIN}   (wildcard *.preview.${HIVY_BASE_DOMAIN})
#   ASSETS         -> assets.${HIVY_BASE_DOMAIN}
#   CONNECT/NANGO  -> connect.${HIVY_BASE_DOMAIN}
#   MCP            -> mcp.${HIVY_BASE_DOMAIN}
#   PROXY          -> proxy.${HIVY_BASE_DOMAIN}  (or point at hosted LLM proxy)
```

### Workstreams (map to the tiers above)

1. **Add the missing env vars** (Easy/Medium): `HIVY_PREVIEW_BASE_DOMAIN` (M1), `HIVY_SANDBOXES_RUNTIME_IMAGE_REPO` / `HIVY_APP_IMAGE_REPO` (M4), `HIVY_EMAIL_SITE_URL` (E4). Delete the stray `api.usehivy.com` fallbacks (E1). Wire `HIVY_BASE_DOMAIN` derivation in `config.go`.
2. **Frontend runtime config** (Hard, H1): serve `window.__HIVY_ENV__` (or `/config`) at container start; make `apiUrl()`, connect host, preview/asset classifiers (M2), and `next/image` (M3) read it. Unblocks pull-and-run.
3. **Preview wildcard profile** (Hard, H2): custom Caddy image + wildcard site block + gateway in compose + `hosting/.env.example` entries + DNS docs. Optional profile.
4. **Template + skill hygiene** (Easy): env-driven `vite.config.ts allowedHosts` (E3), neutralize skill-reference docs (E9), gateway default (E7).
5. **Documentation** (Trivial): a `SELF-HOSTING.md` env reference covering every already-overridable var in Tier 4.

### Suggested sequencing
1. Tier 3 (E1, E2, E3, E4, E7) — quick wins, low risk, immediately reduce breakage.
2. M1 + M4 — add the two missing Go env vars (preview domain, image repos).
3. H1 (frontend runtime config) — the single highest-leverage change; also unblocks M2/M3.
4. H2 (preview wildcard profile) — the largest infra effort; ship as optional.
5. Tier 4 docs + fixture cleanup — finalize.

### Placeholder-domain convention
Standardize tests/samples on the already-present placeholders (`example.com`, `usehivy.test`, `preview.test`, `usehivy.local`) so the grep for `usehivy.com` eventually returns only intentional maintainer references.
