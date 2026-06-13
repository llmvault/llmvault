# Known Issues

Derived from the triage in `fixes-todo.txt` (now retired) after three fix campaigns
(P0, P1, P2) landed on the `production-fixes` branch. Each item below was verified
against the current tree. Resolved items are noted inline where relevant context helps.

---

## Open Issues

### KI-07 — Reconnect sessions are not scoped to the current org
**Severity: P1 / Security**

`internal/handler/connections_session.go` line 91 loads the connection by
`id AND revoked_at IS NULL` only. A user in org A who knows a connection UUID
from org B can create a reconnect session for org B's connection.

**Fix:** Add `AND org_id = ?` to the WHERE clause, binding the query to the
authenticated user's org.

**Key file:** `internal/handler/connections_session.go:91`

---

### KI-04 — Shared subagents inherit the first agent's GitHub identity
**Severity: P0 / Security**

`internal/sandbox/orchestrator_git_identity.go:resolveGitIdentityAgent` (lines 35-57)
falls back to the first agent in the org by `created_at ASC` when the
executing agent is a subagent. If the dispatching agent differs from the
first agent, commits are attributed to the wrong GitHub account.

**Fix:** Pass the dispatching agent ID through sandbox-creation and git-credential
resolution; do not infer ownership from the first `agent_subagents` row.

**Key files:**
- `internal/sandbox/orchestrator_git_identity.go:35-57`
- `internal/handler/subagents.go`

---

### KI-10 — Old sandbox bridge keys still authenticate for git credentials
**Severity: P1 / Security**

`internal/handler/git_credentials.go` lines 93-108 load ALL sandboxes for
an agent (`WHERE agent_id = ?`) with no status filter. A stopped, error,
or archived sandbox retains its encrypted runtime secret and can still be used
to mint GitHub tokens.

**Fix:** Filter to `status IN ('running','starting')` when loading sandboxes for
bridge-key comparison. Rotate bridge keys on sandbox stop/archive.

**Key file:** `internal/handler/git_credentials.go:93-108`

---

### KI-13 — Webhook delivery IDs remain random, defeating task-level dedupe
**Severity: P1 / Reliability**

`internal/handler/nango_webhooks_dispatch.go` line 37 generates
`deliveryID = connectionID + ":" + uuid.New().String()`. For providers that
send a stable delivery header (e.g. GitHub's `X-GitHub-Delivery`), the random
UUID means each forward creates a new task even if the provider redelivers.

**Fix:** Extract the provider-supplied delivery header from the unwrapped Nango
headers map and use it as the delivery ID when present.

**Key files:**
- `internal/handler/nango_webhooks_dispatch.go:37`
- `internal/handler/nango_webhooks_infer.go` (header extraction helpers)

---

### KI-14 — Failed subagent sandbox setup/clone leaves live provider sandboxes running
**Severity: P1 / Resource leak**

`internal/sandbox/orchestrator_create_subagent.go` marks the DB row with
`status=error` on endpoint, runtime, setup-command, and clone failures but
never calls `o.provider.DeleteSandbox()`. The Daytona/Docker sandbox continues
consuming provider resources.

**Fix:** Call `o.provider.DeleteSandbox(ctx, info.ExternalID)` in each failure
branch after the provider sandbox has been created, or use a `defer`-based
rollback that fires on any non-nil error.

**Key file:** `internal/sandbox/orchestrator_create_subagent.go:88-155`

---

### KI-15 — Subagent task upload bearer is not restricted to the task folder
**Severity: P1 / Security**

`internal/handler/agent_uploads_stream.go` authenticates the bearer against
the agent's sandbox runtime secret but places no constraint on the upload
path. A compromised sandbox bearer can write or delete assets at arbitrary
paths under the agent drive.

**Fix:** Bind the bearer/sandbox to an allowed path prefix derived from the
task ID and reject writes or deletes outside that prefix.

**Key files:**
- `internal/handler/agent_uploads_stream.go`
- `internal/handler/subagents.go` (where upload URLs are issued)

---

### KI-19 — Nango webhook HMAC fails open when NANGO_WEBHOOKS_SECRET is empty
**Severity: P2 / Security**

`internal/config/config.go` line 87 declares `NangoWebhooksSecret` without a
`required` tag. If the env var is unset, `verifyNangoSignature` computes
HMAC-SHA256 with an empty key — an attacker who knows the key is empty can
craft valid signatures.

The handler correctly rejects missing signature headers, but does not reject
an empty configured secret at startup.

**Fix:** Add startup validation that errors (or disables the Nango webhook
route) when `NangoWebhooksSecret` is empty.

**Key files:**
- `internal/config/config.go:87`
- `internal/handler/nango_webhooks.go:34`
- `cmd/server/serve.go` (startup wiring)

---

### KI-21 — Revoked Railway connections can serve cached tokens
**Severity: P2 / Security**

`internal/handler/railway_proxy.go` lines 150-152 return a cached Railway token
without checking the connection's current revocation state. `connections_revoke.go`
does not evict the railway token cache on revoke.

**Fix:** Include a connection version or `revoked_at` in the cache key, or look
up the DB revocation state before returning a cached token. Purge the cache
entry when a connection is revoked.

**Key files:**
- `internal/handler/railway_proxy.go:140-205`
- `internal/handler/connections_revoke.go`

---

### KI-24 — Setup command failure can persist raw command output in sandbox error messages
**Severity: P2 / Security**

`internal/sandbox/orchestrator_helpers.go:156` formats the error as
`"setup command failed: <cmd>: <err>"`, which includes the raw command string.
`orchestrator_create_subagent.go:141` stores this verbatim in
`error_message` (visible via the sandboxes API). If the setup command contains
secrets in env-variable form (e.g. `--token=$SECRET`), they may appear in the
stored error.

**Fix:** Redact known secret env-var values from the command string before
formatting the error, and/or suppress raw command output from user-visible
`error_message` (replace with a generic "setup failed" message, log the
detail server-side).

**Key files:**
- `internal/sandbox/orchestrator_helpers.go:153-160`
- `internal/sandbox/orchestrator_create_subagent.go:138-143`

---

### KI-27 — Production compose omits HIVY_SESSION_SECRET and HIVY_API_URL for the web container
**Severity: P2 / Reliability**

`hosting/docker-compose.production.yml` web service (lines 201-223) sets only
`NODE_ENV` and `HOSTNAME`. `apps/web/lib/auth/session.ts` throws at runtime if
`HIVY_SESSION_SECRET` is absent; `apps/web/app/api/proxy/[...path]/route.ts`
requires `HIVY_API_URL`.

**Fix:** Add the required env vars to the web container's `environment` block:
- `HIVY_SESSION_SECRET: ${SESSION_SECRET}` (or equivalent)
- `HIVY_API_URL: https://${API_DOMAIN}`

**Key file:** `hosting/docker-compose.production.yml:210-212`

---

### KI-31 — rag_embedding_models table is never seeded at startup
**Severity: P1 / Correctness**

`internal/rag/embedder/seed.go:SeedRegistry` upserts the curated model catalog
into `rag_embedding_models`, but no server or worker boot path calls it. The
table exists (via migration `000012_rag.sql`) but stays empty.

**Fix:** Call `embedder.SeedRegistry(db)` in the RAG boot sequence (e.g. in
`cmd/server/serve_rag.go` or `cmd/server/worker_rag.go`), or remove the table
and its model references if the embedding-model registry is no longer planned.

**Key files:**
- `internal/rag/embedder/seed.go`
- `cmd/server/serve_rag.go` / `cmd/server/worker_rag.go`

---

### KI-33 — MCP scope resource validation passes nil connection resources
**Severity: P1 / Security**

`internal/mcp/scope.go` line 72 calls `cat.ValidateResources(provider, actions, resources, nil)`.
The fourth argument (connection-configured resource constraints) is always
`nil`; the TODO comment acknowledges this defers to Phase 4.

**Fix:** Load the connection's actual resource constraints from `Connection.Meta`
and pass them to `ValidateResources` so scopes cannot request resources the
connection is not configured to expose.

**Key file:** `internal/mcp/scope.go:70-74`

---

### KI-34 — Agent.Tools column carries deprecated data with no removal path
**Severity: P2 / Cleanup**

`internal/model/agent.go` line 29 retains the `Tools JSON` column (mapped to
`jsonb`). Functional built-in tool management now uses `ValidBuiltInTools` and
`SandboxTools`; the column appears unused in handler create/update paths. It
is still exposed in generated API types.

**Fix:** Audit any remaining writes to `agent.Tools`, archive data if non-empty
in production, drop the column in a migration, and regenerate API types.

**Key files:**
- `internal/model/agent.go:29`
- `docs/openapi.json`
- `apps/web/lib/api/schema.d.ts`

---

### KI-37 — No sandbox-runtime E2E coverage for the full agent lifecycle
**Severity: P1 / Testing**

`e2e/` contains `sandbox_exec_test.go` and `sandbox_provider_test.go` but no
test that exercises the full path: create agent, connect Slack/GitHub
profiles, provision sandbox, push `/config`, verify `/healthz` and `/readyz`,
and confirm model/proxy/env behavior.

**Fix:** Add an E2E test suite in `e2e/` that covers the agent + sandbox
runtime lifecycle end-to-end.

**Key files:**
- `e2e/` (new file)
- `internal/sandbox/orchestrator_create_agent.go`
- `internal/agentruntime/compile.go`

---

### KI-39 — Allowlisted Go files still exceed the 300-line ceiling
**Severity: P2 / Cleanup**

`scripts/file-length-allowlist.txt` permits four files over the 300-line limit:
- `internal/registry/models.go` (generated — acceptable)
- `internal/handler/connections_create_test.go` (342 lines — split opportunistically)
- `internal/handler/connections_revoke_test.go` (long integration test — split opportunistically)
- `internal/evals/setup.go` (342 lines)
- `internal/agentruntime/compile.go` (401 lines)

**Fix:** Split the non-generated files as they are touched in normal development
work, then remove their allowlist entries.

**Key files:**
- `scripts/file-length-allowlist.txt`
- `internal/evals/setup.go`
- `internal/agentruntime/compile.go`

---

## Resolved (for reference)

The following items from the original triage were resolved across the three fix
campaigns. Brief citations are included so readers can trace the fix.

| # | Summary | Where resolved |
|---|---------|----------------|
| 1 | API-key scope ceiling | `internal/handler/api_keys.go:scopesWithinCeiling`; `internal/middleware/apikeyauth.go` |
| 2 | Agent MCP integration connection org-validation | Verified via connection org-scoping in agent helpers; connection lookup gated by org context |
| 3 | Next proxy refresh lock could swap user sessions | `apps/web/lib/auth/refresh.ts:RefreshCoordinator` — per-token sha256-keyed singleflight |
| 5 | GitHub profiles bound to another user's org connection | `agent_profiles_github.go` feature not present in codebase; connection scope is org-only in current routes |
| 6 | GitHub repo selection limits | `agents_connection_resources.go` validates connections; resource constraints enforced before clone |
| 8 | Nango webhook routing hijack via duplicate nango_connection_id | `identify()` uses `ORDER BY created_at DESC` as tie-break; creation still accepts duplicates (partial mitigation only) |
| 9 | Public /spider routes | `cmd/server/serve_routes_aux.go:79` — `middleware.TokenAuth` added |
| 11 | Subscription renewal double-charge | `internal/billing/subscription/renew.go:184` — SELECT FOR UPDATE + deterministic `renewalChargeReference` |
| 12 | Credit grant idempotency race | Migration `000027_credit_ledger_idempotency_index.sql` + ON CONFLICT DO NOTHING in `internal/billing/credits.go` |
| 16 | GitHub webhook setup missing bridgeHost / agent webhook not mounted | Feature (`agent_profiles_github.go`, `github_agent_webhooks.go`) not implemented in this codebase; no partial wire to regress |
| 17 | Direct incoming webhooks use connection UUID as secret | `incoming_webhooks.go` documents this explicitly; Railway relies on the unguessable UUID (low-risk for current provider set) |
| 18 | Bridge/agent webhook replay protection | No `bridge_webhooks.go` in codebase; outbound webhook HMAC added in P1 campaign |
| 20 | CORS fails open when CORS_ORIGINS omitted | `internal/middleware/cors.go` — fail-closed in production mode |
| 22 | Revoked profiles feed agent runtime state | `internal/agentruntime/bugsink.go:45` and `compile_mcp.go:37` filter `revoked_at IS NULL` |
| 23 | Deleting agents leaves external sandboxes running | No DELETE /agents/{id} route exists; lifecycle managed via archive + sandbox cleanup task |
| 25 | Git author env overrides bypass GitHub identity | Git identity resolution goes through `setGitIdentityEnvVars`; user env is not expected to override runtime git config (no shell escaping issue found) |
| 26 | Legacy sandbox-drive resolves agent via wrong join | `internal/handler/sandbox_drive.go:83` now queries `WHERE sandbox_id = ?` (sandbox → agent) |
| 30 | RAG tables missing from migration path | Migration `000012_rag.sql` embedded in `internal/migrations/sql/`; included in standard goose run |
| 32 | RAG mutations not admin-gated | `cmd/server/serve_routes_v1.go:238` — `RequireOrgAdmin` applied to RAG mutation group |
| 35 | Agent category expansion | No category validation on POST /agents; all category strings accepted; decision deferred |
| 36 | Non-Slack agent startup profile | Slack-only is explicit v1 contract; `compile.go` and `agents_sync_runner.go` document this |
| 38 | DB cleanup findings from db-todos.txt | `sandboxes/runtime/db-todos.txt` retired (removed in this pass); open schema decisions moved to KI-34 |
| 40 | Trigger architecture docs stale | `trigger-architecture/` directory removed; `docs/contracts/streaming.md` and `delivery.md` are the current contracts |
