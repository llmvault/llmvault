# Authorization — Connections, Credentials & Database Integrations

## 1. Overview

This feature covers the three resource families that hold **secret material**, and are therefore the highest-blast-radius surface in the app:

- **Connections** — third-party OAuth/App connections brokered by **Nango** (github, github-app, slack, linear, notion, vercel, railway, apify, bugsink, glitchtip, etc.). A `connections` row (`model.Connection`) stores `org_id`, `user_id`, `integration_id`, a `nango_connection_id` pointer, and `meta` JSON. The actual OAuth access/refresh tokens live **inside Nango**, not in Postgres — the row is a reference, but it is the key that lets the runtime mint provider tokens and lets discovery endpoints call the provider API with the org's token.
- **Credentials** — encrypted LLM/API keys (`model.Credential`: `EncryptedKey` + KMS-`WrappedDEK`), used by the LLM proxy. Secret is AES-encrypted with a KMS-wrapped DEK.
- **Database integrations** — encrypted DB connection strings (`model.DatabaseConnection`: `EncryptedDSN` + `WrappedDEK` + `AccessPolicy`), used by the sandbox DB proxy.
- **Integrations** (`model.Integration`) — the org/instance-level provider *config* (Nango provider config incl. OAuth `client_id`/`client_secret`). Managed only via admin-secret routes; included here because it is the parent of connections and holds client secrets.
- **Nango webhook receiver** — inbound events from Nango (Slack forwards, sync/auth notifications), authenticated by HMAC signature, no human role.

Principals who interact: Org Owner/Admin (should own creation/revocation), Members (should only *use* connected integrations inside channels, never manage secrets), automated/API-key actors (scoped separately), and the Nango service (signature axis).

The routes split across three files: `cmd/server/serve_routes_v1.go` (credentials, database-integrations), `cmd/server/serve_routes_connect.go` (all connection + integration-admin routes), and `cmd/server/serve_routes.go` (public/webhook/sandbox-proxy routes).

## 2. Backend endpoint inventory

### Connections (`serve_routes_connect.go`) — gate stack: `RequireAuth` + `RequireEmailConfirmed` + `ResolveUser` + `ResolveOrgFlexible`. **No role middleware.**

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| POST | `/v1/integrations/{id}/connect-session` | `connections_session.go:25` | Mutates (mints Nango connect-session token) | any org member | ❌ HIGH |
| POST | `/v1/integrations/{id}/connections` | `connections_create.go:29` | Mutates (creates connection) | any org member | ❌ HIGH |
| GET | `/v1/connections` | `connections_list.go:20` | Reads (org-wide list) | any org member | ⚠️ MEDIUM |
| GET | `/v1/connections/{id}` | `connections_get.go:13` | Reads | any org member | ⚠️ MEDIUM |
| PUT | `/v1/connections/{id}/resources` | `connections_resources.go:103` | Mutates (default resources) | any org member | ❌ HIGH |
| GET | `/v1/connections/{id}/resources/{type}` | `connections_resources.go:43` | Reads (calls provider API w/ org token) | any org member | ❌ HIGH |
| POST | `/v1/connections/{id}/reconnect-session` | `connections_session.go:77` | Mutates (mints Nango reconnect token) | any org member **AND no org filter** | ❌ **CRITICAL** |
| PATCH | `/v1/connections/{id}/webhook-configured` | `connections_webhook.go:21` | Mutates | any org member | ❌ MEDIUM |
| DELETE | `/v1/connections/{id}` | `connections_revoke.go:24` | Mutates (revokes; deletes in Nango) | any org member | ❌ HIGH |

### Integration config — admin-secret (instance operator), gate: `RequireAdminSecret` (only if `cfg.AdminEnabled`)

| Method | Path | Handler | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | `/v1/admin/integrations` | `integrationHandler.ListAdmin` | Reads | admin secret | ✅ |
| PUT | `/v1/admin/integrations/{id}` | `integrationHandler.UpsertAdmin` | Mutates (stores client_secret) | admin secret | ✅ |
| DELETE | `/v1/admin/integrations/{id}` | `integrationHandler.DeleteAdmin` | Mutates | admin secret | ✅ |
| GET | `/v1/admin/system-credentials` + POST/PATCH/DELETE | `credentialHandler.*System` | Mutates/Reads system creds | admin secret | ✅ |
| GET | `/v1/admin/llm-providers` | `credentialHandler.ListLLMProviders` | Reads | admin secret | ✅ |
| GET | `/v1/integrations/available` | `integrationHandler.ListAvailable` | Reads (availability only) | **public/no auth** | ✅ (no secrets) |

### Credentials (`serve_routes_v1.go:178-189`) — scope-gate `RequireAPIKeyScopeOrJWT("credentials")`; write sub-group adds `RequireOrgAdminOrAPIKey`

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | `/v1/credentials` | `credentials_list.go:29` | Reads (metadata + usage, **no key**) | any JWT / scoped key | ⚠️ MEDIUM |
| GET | `/v1/credentials/{id}` | `credentials_list.go:136` | Reads (metadata, **no key**) | any JWT / scoped key | ⚠️ MEDIUM |
| POST | `/v1/credentials` | `credentials.go:86` | Mutates (stores encrypted key) | org admin / API key | ✅ |
| DELETE | `/v1/credentials/{id}` | `credentials_revoke.go:28` | Mutates (revoke) | org admin / API key | ✅ |

### Database integrations — LIST at `serve_routes_v1.go:113` (no gate); writes at `:204-212` behind `RequireAPIKeyScopeOrJWT("agents")` + `RequireOrgAdmin`

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | `/v1/database-integrations` | `database_integrations.go:113` | Reads (metadata + schema snapshot, **no DSN**) | any org member | ⚠️ MEDIUM |
| POST | `/v1/database-integrations` | `database_integrations.go:61` | Mutates (stores encrypted DSN) | org admin | ✅ |
| POST | `/v1/database-integrations/{id}/test` | `database_integrations_actions.go:25` | Reads (decrypts DSN, connects) | org admin | ✅ |
| POST | `/v1/database-integrations/{id}/introspect` | `database_integrations_actions.go:54` | Mutates (schema snapshot) | org admin | ✅ |
| PUT | `/v1/database-integrations/{id}/policy` | `database_integrations_actions.go:93` | Mutates (access policy) | org admin | ✅ |
| DELETE | `/v1/database-integrations/{id}` | `database_integrations.go:147` | Mutates (revoke) | org admin | ✅ |

### Webhook / signature axis (`serve_routes.go`) — no human role

| Method | Path | Handler (file:line) | Auth | Correct? |
|---|---|---|---|---|
| POST | `/internal/webhooks/nango` | `nango_webhooks.go:79` | HMAC `X-Nango-Hmac-Sha256`, fail-closed on empty secret (`nango_webhooks_identify.go:28`) | ✅ |
| POST | `/incoming/webhooks/{provider}/{connectionID}` | `IncomingWebhookHandler.Handle` | per-connection webhook auth (out of role axis) | ✅ (verify separately) |
| POST | `/internal/git-credentials/{agentID}` + other `*-proxy/{agentID}` | `serve_routes.go:84-119` | sandbox **runtime-secret** bearer (`git_credentials.go:72-82`) | ✅ (separate axis, as noted) |

## 3. Frontend screens & actions

The connection/credential/database UI is **not** under `settings/`; it lives in the chat-level **Plugins** area, which is reachable by every workspace user with no role gate.

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---|---|---|---|---|
| `app/w/(chat)/plugins/page.tsx` | Browse plugins, see connected state | `GET /v1/plugins` | No | Viewable by members; connect actions admin-only |
| `app/w/(chat)/plugins/[slug]/page.tsx` | Connect / reconnect / configure resources | `use-connect-integration.ts` → connect-session, connections, reconnect-session | **No** | **Org Admin** to connect/reconnect |
| `app/w/(chat)/plugins/[slug]/disconnect-connection-confirm-dialog.tsx` | Disconnect | `DELETE /v1/connections/{id}` | **No** | **Org Admin** |
| `app/w/(chat)/plugins/[slug]/required-connections-section.tsx` / `resource-requirements-section.tsx` | Pick default repos/projects | `GET/PUT /v1/connections/{id}/resources*` | **No** | **Org Admin** |
| `app/w/(chat)/plugins/integration-credentials-form.tsx` | Enter provider client creds for connect | connect flow | **No** | **Org Admin** |
| `app/w/(chat)/plugins/database-connection-modal-content.tsx` + `database-policy-configuration.tsx` + `redis-database-configuration.tsx` | Add DB integration / set policy | `POST /v1/database-integrations*` | **No** (backend `RequireOrgAdmin` blocks non-admins — UI shows a form that will 403) | **Org Admin** (mirror gate + hide) |
| `app/w/settings/environments/page.tsx` | Sandbox preview ports (org update) | `PATCH /v1/orgs/current` | No | Org Admin (separate; org settings doc) |
| `app/w/settings/channels/[id]/page.tsx` | Channel env-vars (secret values) | channel env-var endpoints | No | member of the channel's team, or Org Admin (covered in channels doc) |

For comparison, `settings/teams` correctly gates with `isAdmin = activeOrg?.role === "owner" || "admin"` (`teams/page.tsx:24`). The role signal (`activeOrg.role`) already exists on the client; the connections/plugins UI simply never consults it.

## 4. Ambiguities & lapses (ranked)

1. **CRITICAL — `CreateReconnectSession` is not org-scoped (cross-tenant).** `connections_session.go:91` loads the connection by `id = ? AND revoked_at IS NULL` with **no `org_id` predicate** (every other connection handler scopes by `org.ID`). Any authenticated, email-confirmed user in *any* org who learns/guesses a connection UUID can mint a Nango **reconnect session token** for another org's connection. Blast radius: cross-org OAuth re-authorization surface against a victim org's connection. Mitigated only by UUIDv4 unguessability — this is a missing tenant check, not a designed boundary. Fix: add `AND org_id = ?` and gate to admin.

2. **HIGH — Any member can create/revoke connections and drive provider APIs.** The entire `/v1/connections*` + `/v1/integrations/{id}/connect-session|connections` block has **no role gate** (only auth + org resolution). A plain **Member** can: connect a new integration to the org; **revoke** any org connection (`connections_revoke.go` scopes by `org_id` only, not `user_id`) — a denial-of-service that also deletes the connection in Nango and breaks every agent/channel relying on it; call `GET /connections/{id}/resources/{type}`, which uses the connection's stored token to **enumerate the provider account** (e.g. list all GitHub repos, Slack channels, Linear projects) — a data-exfil path over the org's third-party account. Per the role model, creating/deleting a resource that holds secrets = **Org Admin**.

3. **MEDIUM — Credential metadata is member-readable.** `GET /v1/credentials[/{id}]` sits under `RequireAPIKeyScopeOrJWT("credentials")`, which lets **any org member's JWT** through (the admin gate is only on the write sub-group). Responses (`credentialResponse`) expose `label`, `base_url`, `provider_id`, quota/refill, `meta`, and usage stats — **not** the encrypted key (`EncryptedKey`/`WrappedDEK` are never serialized). No secret leaks, but the *inventory of what LLM/API providers the org holds keys for* and their endpoints/quotas is visible to members. `_MODEL.md:39` already flags this JWT-passthrough as the member-read leak point.

4. **MEDIUM — Database-integration list is member-readable and exposes schema.** `GET /v1/database-integrations` has no gate; `databaseConnectionResponse` returns `provider`, `display_name`, `access_policy`, and the full introspected **`schema_snapshot`** (table/column/collection structure) — but **not** the DSN (`EncryptedDSN`/`WrappedDEK` never serialized). Members learn the org's connected databases and their schemas. No secret leak, but more than a member needs.

5. **MEDIUM — Connection reads are org-wide, not user-scoped.** `List`/`Get`/`Revoke`/resources scope by `org_id` only, so any member sees and can act on connections created by *other* users (including admins). Even after adding a role gate, note that the `user_id` on the row is currently informational, not an ownership boundary.

6. **LOW — Frontend never mirrors intended gates.** Plugins/connections/database UI is shown in full to members with functioning connect/disconnect buttons. For database integrations the backend already 403s (RequireOrgAdmin), so it's a confusing dead-end; for connections the buttons actually work (see #2). Hide/disable for non-admins.

**No CRITICAL secret-material leak found in response bodies:** encrypted keys, wrapped DEKs, DSNs, and OAuth tokens are never serialized in any read endpoint. `safeConnectionMeta` (`helpers.go:50`) strips `credentials` from connection `meta`, and `Create` also deletes `resources`/`credentials` keys before persisting. The Nango webhook is correctly HMAC-gated and fail-closed. The single true tenant-boundary bug is #1.

## 5. Recommendation

**Target principals (per role model):**
- Create / revoke / reconnect / configure-resources on **connections, credentials, database-integrations** → **Org Admin** (catalog resource holding secrets). None of these have a natural team owner or channel-grant step, so there is no team-scoped tier here — they are strictly org-level.
- Reading connection/credential/database **metadata** → restrict to **Org Admin** as well. These are org-catalog config, not channel content; members consume the *effect* of a connection inside a channel (via an assigned agent), never the connection record. There is no member use-case for `GET /v1/connections` or `GET /v1/credentials`.
- Nango/incoming webhooks → **signature axis**, no human role (already correct).
- Instance integration config + system credentials → **admin secret** (operator), already correct.

**Enforcement mechanism:**
1. **Fix the cross-tenant bug first (#1):** add `AND org_id = ?` to the `CreateReconnectSession` query in `connections_session.go:91`. This is a straight tenant-scoping fix independent of the role work.
2. **Wrap the connection routes in an admin gate.** In `serve_routes_connect.go`, put the connection mutation + read routes inside a `r.Group` with `middleware.RequireOrgAdmin(database)` (these routes are JWT-only — no API-key auth is wired in this file — so plain `RequireOrgAdmin` is the right predicate, not `RequireOrgAdminOrAPIKey`). If any runtime/API-key caller needs read access, split reads under `RequireAPIKeyScopeOrJWT` + explicit admin check like the credentials block; otherwise keep it all admin.
3. **Tighten credential + database reads.** Move `GET /v1/credentials[/{id}]` and `GET /v1/database-integrations` behind admin (for JWT) — i.e. use the same `RequireOrgAdminOrAPIKey` pattern already on the credential writes, applied to the reads too, so API-key/runtime callers keep working but human members do not. This closes lapses #3 and #4.
4. **Prefer the shared layer over ad-hoc checks.** Per `_MODEL.md`, express these as `(role, resource, action)` predicates in `internal/access` (e.g. an `Actor.IsOrgManager()` guard reused by a `RequireOrgManager` middleware) rather than three separately-gated route files. Connections especially are currently the only secret-holding family with *zero* gate, precisely because they live in a third route file that the `RequireOrgAdmin` sweep of `serve_routes_v1.go` never touched — a single shared predicate prevents this class of miss.
5. **Frontend:** gate the plugins connect/reconnect/disconnect/resource/database actions on `activeOrg?.role` being `owner|admin` (the signal already used in `settings/teams`); hide or disable for members. Backend remains authoritative.

No new role/column/migration is required — everything here is **Org Admin**, which already exists (`org_memberships.role`). No team-scoped tier applies.

## 6. Deviations from the baseline model

- **No team dimension.** The baseline's org-vs-team split does not apply: connections/credentials/database-integrations have no channel-grant step of their own — they are consumed indirectly when an agent (assigned to a channel by a member of the channel's team) runs. So the entire family is **Org-Admin-only for create AND read**, with no team-scoped tier at all. This is a legitimate deviation, not a gap.
- **Member reads recommended OFF.** The baseline says "reads follow visibility rules (members see usable channels + assigned agents)." For secret-holding catalog config there is no member-visible slice at all — recommend members get *nothing* here, unlike agents/knowledge where members see assigned items. Stricter than the generic read rule, intentionally.
- **API-key axis is uneven.** Credentials/database writes use `RequireOrgAdminOrAPIKey` (keys pass), but the connection routes have no API-key auth wired at all in `serve_routes_connect.go`. If runtime callers ever need connection reads, that axis must be added explicitly rather than inherited.

## 7. Open questions for the operator

1. **Should Members see *any* connection/credential inventory?** Recommendation is no (admin-only reads). Confirm no product surface (e.g. a member picking which connected integration an agent uses) depends on member read access to `/v1/connections`.
2. **Connection ownership:** when an admin revokes/reconnects, should the original connecting `user_id` matter, or is org-level admin authority sufficient? (Recommendation: org-admin authority; `user_id` stays informational.)
3. **Reconnect after the fix:** reconnect-session currently lets *anyone* re-auth a connection. After scoping to org + admin, should a **non-admin who originally created** a connection still be allowed to reconnect their own (self-service token refresh), or is that admin-only too?
4. **API-key access to connections:** do any automated/runtime flows call `/v1/connections*` with an API key? If so, an explicit scope (e.g. `"connections"`) must be added before the admin gate, or those callers will break.
