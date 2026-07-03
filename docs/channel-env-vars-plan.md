# Channel-scoped environment variables

**Status:** Planned · **Date:** 2026-07-03

Replace the org-wide environment-variable store with **per-channel** environment
variables. Every env var belongs to exactly one channel and holds a
**per-channel value**. At session start, the runtime config push pulls all env
vars for the session's channel and injects them into the sandbox. There is no
org-wide / all-channel layer.

---

## Goals & decisions

| Decision | Choice |
| --- | --- |
| Scope | Every var is scoped to exactly one channel. No org-wide vars. |
| Value model | Per-channel value. "Assign to N channels" = one row per channel (independently editable), driven from the UI. |
| Old org-wide store | **Dropped entirely** — handler, routes, API, and the `agents.encrypted_env_vars` column are removed. |
| Injection namespace | User vars are injected into `RuntimeEnv` as `__ENV__<NAME>`. The `__ENV__` sentinel marks *user-supplied* env, distinct from platform `HIVY_*` control-plane vars. |
| Platform vars | The ~40 `HIVY_*` control-plane vars are **untouched** (still read in ~152 places across the Rust runtime). `__ENV__` is *only* for user-supplied env. |
| Prefix stripping | The Rust runtime strips `__ENV__` when building the child-process env, so the workload sees the clean name (`DATABASE_URL`, not `__ENV__DATABASE_URL`). |
| Name validation | `^[A-Z_][A-Z0-9_]*$`, uppercased. Reject names starting with `__ENV__` or `HIVY_` (the latter defensively, so a stripped user var can't shadow a control-plane key). |
| Encryption | AES-256-GCM via the existing `envEncKey` (`crypto.SymmetricKey`), same as the old store. |

---

## Background: how it works today

- **Storage:** org env vars are a single AES-256-GCM-encrypted JSON blob in
  `agents.encrypted_env_vars` on the org's singleton "Hivy agent"
  (`internal/model/agent.go:51`), keyed as `HIVY_ORG_<NAME>`.
- **API:** `internal/handler/org_environment_variables.go`, routes at
  `cmd/server/serve_routes_v1.go:79–82`.
- **Injection:** `mergeAgentEnvVars` (`internal/agentruntime/runtime_env.go:155`)
  decrypts the blob and copies `HIVY_ORG_*` into the runtime env map inside
  `BuildRuntimeEnvWithProxyToken` (`runtime_env.go:114–117`), *before* the
  reserved `HIVY_*` control-plane keys are written (so reserved keys always win).
  The map becomes `ConfigUpdateRequest.RuntimeEnv` and is pushed to the sandbox.
- **Runtime consumption:** `sandboxes/runtime/crates/tools/src/bash.rs:101–119`
  copies the *entire* `runtime_env` into the child process env when
  `env_passthrough` is empty — so the old vars reached the workload verbatim as
  `HIVY_ORG_<NAME>`.
- **Frontend:** none. The endpoints exist only in generated `schema.d.ts`.

### Session → channel → runtime-config-push path

```
SessionHandler.Create                                    sessions_create.go:33
  ├─ loadUsableChannel  -> model.Channel                  :68   (CHANNEL AVAILABLE)
  ├─ newSessionRecord   -> model.Session{ChannelID}       :92   (not yet persisted)
  └─ provisionSessionSandbox(agent, model, effort)        :101  (channel NOT threaded)
       └─ orchestrator.CreateAgentSandboxWithRuntimeOptions(...)  sessions_sandbox.go:44
            └─ Orchestrator.pushAgentRuntimeConfig(sb, AgentRuntimeConfigPush{Agent, RuntimeOptions})
                 orchestrator_create_agent.go:148
                 └─ pusher callback                       serve_handlers_core.go:110
                      └─ PushAgentRuntimeConfigWithProxyTokenOptions(...)  config_push.go:68
                           └─ BuildAgentRuntimeConfigUpdateWithProxyTokenOptions(...)  runtime_env.go:61
                                └─ BuildRuntimeEnvWithProxyToken(...)         runtime_env.go:105
                                     └─ mergeAgentEnvVars(...)                runtime_env.go:155  ← merge point
```

**Key wrinkle:** at session creation the sandbox is provisioned (`:101`, first
config push) *before* the session↔sandbox link is persisted (`:108`
`session.SandboxID = &...`). So the channel cannot be recovered by
`sandbox_id → session.ChannelID` on that first push — it must be **threaded down
through the push payload**. The restart / wake / warm-claim paths *can* resolve
the channel from the session, which `resolveSandboxRuntimeConfigOptions` already
loads by `sandbox_id` (`config_push.go:160`).

---

## Data model

New migration `internal/migrations/sql/000061_channel_env_vars.sql` (next free
number; 000060 is the latest):

```sql
-- +goose Up
CREATE TABLE channel_env_vars (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL,
    channel_id      uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name            text NOT NULL,
    encrypted_value bytea NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_channel_env_vars_channel_name ON channel_env_vars (channel_id, name);
CREATE INDEX idx_channel_env_vars_channel ON channel_env_vars (channel_id);
CREATE INDEX idx_channel_env_vars_org ON channel_env_vars (org_id);

-- Drop the removed org-wide store.
ALTER TABLE agents DROP COLUMN IF EXISTS encrypted_env_vars;

-- +goose Down
ALTER TABLE agents ADD COLUMN encrypted_env_vars bytea;
DROP TABLE channel_env_vars;
```

- `name` stores the user's chosen name (already uppercased/validated), **without**
  the `__ENV__` prefix. The prefix is applied only at injection time.
- `(channel_id, name)` unique → no collisions within a channel; per-channel
  values fall out naturally.
- No data migration: the old store is dropped (product decision).

Model `internal/model/channel_env_var.go`:

```go
type ChannelEnvVar struct {
    ID             uuid.UUID
    OrgID          uuid.UUID
    ChannelID      uuid.UUID
    Name           string
    EncryptedValue []byte    // gorm:"type:bytea"
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
func (ChannelEnvVar) TableName() string { return "channel_env_vars" }
```

---

## Backend (Go)

### 1. Handlers — `internal/handler/channel_environment_variables.go`

Methods on `*ChannelHandler` (which already owns channel authorization). Add an
`envEncKey *crypto.SymmetricKey` field to `ChannelHandler` via a new
`ChannelHandlerOption` (mirroring how `OrgHandler` is wired in
`internal/handler/orgs.go:23,31`), and pass it in `NewChannelHandler(...)` at the
handler-construction site.

Reuse `authorizeChannel(w, r, requireManage)` (`channels_auth.go:62`):
- List → `requireManage = false` (viewers can see names).
- Create / Update / Delete → `requireManage = true`.

Endpoints (values **never** returned on list, matching the old contract):

| Method | Route | Handler |
| --- | --- | --- |
| GET | `/v1/channels/{id}/environment-variables` | `ListChannelEnvironmentVariables` |
| POST | `/v1/channels/{id}/environment-variables` | `CreateChannelEnvironmentVariable` |
| PATCH | `/v1/channels/{id}/environment-variables/{name}` | `UpdateChannelEnvironmentVariable` |
| DELETE | `/v1/channels/{id}/environment-variables/{name}` | `DeleteChannelEnvironmentVariable` |

Behavior mirrors the old org handlers (409 on duplicate name, 404 on missing,
rename + value on PATCH) but backed by rows instead of a blob:
- Create: encrypt value, insert; unique-violation → 409.
- Update: load by `(channel_id, name)`; optional rename (409 if target exists) and/or value change.
- Delete: delete by `(channel_id, name)`; 404 if absent.

Validation helper `normalizeChannelEnvName` (new
`channel_environment_variables_types.go`): uppercase, match
`^[A-Z_][A-Z0-9_]*$`, reject prefixes `__ENV__` and `HIVY_`.

Register routes in `cmd/server/serve_routes_v1.go` inside the existing
`channels/{id}` group (near lines 141–148), under `RequireAPIKeyScopeOrJWT("channels")`.

### 2. Remove the org-wide store

- Delete `internal/handler/org_environment_variables.go` and
  `internal/handler/org_environment_variables_types.go`.
- Remove routes `serve_routes_v1.go:79–82`.
- Remove `envEncKey` usages tied to org env on `OrgHandler` if now unused
  (keep the key wiring — it moves to `ChannelHandler`).
- Remove `mergeAgentEnvVars` and the `EncryptedEnvVars` field usage.

### 3. Runtime injection — `internal/agentruntime/runtime_env.go`

Thread a resolved `channelID uuid.UUID` into the env builder and merge channel
vars at the existing pre-reserved-keys point.

- Add `ChannelID uuid.UUID` to `sandbox.AgentRuntimeConfigPush`
  (`internal/sandbox/orchestrator.go:57`).
- Thread the channel from session creation:
  `sessions_create.go` (channel in scope at `:68`) → `provisionSessionSandbox`
  (`sessions_sandbox.go`) → `CreateAgentSandboxWithRuntimeOptions` →
  `pushAgentRuntimeConfig` (`orchestrator_create_agent.go:148`) → set
  `AgentRuntimeConfigPush.ChannelID`.
- In the pusher callback (`serve_handlers_core.go:110`, `worker.go:129`), pass
  `push.ChannelID` through to the builder.
- Sandbox-only paths (start/restart/wake/warm-claim): resolve `ChannelID` from
  the session already loaded in `resolveSandboxRuntimeConfigOptions`
  (`config_push.go:160`) and pass it down. When unknown, pass `uuid.Nil` → skip.
- New `mergeChannelEnvVars(deps, env, channelID)` replaces `mergeAgentEnvVars` at
  `runtime_env.go:114–117`:

```go
func mergeChannelEnvVars(deps CompileDeps, env map[string]string, channelID uuid.UUID) error {
    if channelID == uuid.Nil {
        return nil
    }
    if deps.EncKey == nil {
        return fmt.Errorf("runtime env decrypt: encryption key is required")
    }
    var vars []model.ChannelEnvVar
    if err := deps.DB.Where("channel_id = ?", channelID).Find(&vars).Error; err != nil {
        return err
    }
    for _, v := range vars {
        value, err := deps.EncKey.DecryptString(v.EncryptedValue)
        if err != nil {
            return err
        }
        env["__ENV__"+v.Name] = value
    }
    return nil
}
```

Precedence is unchanged: user vars merge *before* the reserved `HIVY_*` keys are
written (`runtime_env.go:119–147`), so control-plane keys still win. The
`__ENV__` names never collide with `HIVY_*` anyway.

> `BuildRuntimeEnvWithProxyToken` and
> `BuildAgentRuntimeConfigUpdateWithProxyTokenOptions` gain a `channelID`
> parameter (or carry it on `RuntimeConfigOptions`). Update all call sites,
> including the token-refresh task (`internal/tasks/agent_proxy_token_refresh.go:150`),
> which should resolve the channel from the session by `sandbox_id`.

---

## Runtime (Rust) — strip `__ENV__` for the workload

`sandboxes/runtime/crates/tools/src/bash.rs`, env construction (`:101–119`).
After building `env` from `runtime_env`, rewrite user vars so the workload sees
clean names and never the sentinel:

- **Empty passthrough branch** (`:102–107`, all of `runtime_env` copied): after
  the copy, for every key starting with `__ENV__`, insert the stripped name and
  drop the prefixed key.
- **Explicit passthrough branch** (`:108–118`, user lists names): a listed name
  `FOO` should also match `__ENV__FOO` in `runtime_env` — look up both.

Sketch:

```rust
// after populating `env` (both branches), normalize user-supplied vars:
let user: Vec<(String, String)> = env
    .iter()
    .filter_map(|(k, v)| k.strip_prefix("__ENV__").map(|real| (real.to_string(), v.clone())))
    .collect();
env.retain(|k, _| !k.starts_with("__ENV__"));
for (k, v) in user {
    env.insert(k, v); // clean name wins for user-supplied env
}
```

Notes:
- Do this *before* the `HOME`/`PATH` `or_insert` (`:120–123`) so a user
  `__ENV__PATH` is respected (their env, their choice) while still defaulting
  when unset.
- Update the existing bash env tests (`bash.rs:353+`) and add a case asserting
  `__ENV__FOO` in `runtime_env` yields `FOO` (and no `__ENV__FOO`) in the child
  env, while `HIVY_*` keys pass through untouched.

---

## Frontend

New "Environment variables" section in the channel details modal
(`apps/web/app/w/(chat)/_components/channel-details-modal.tsx`, already open in
the working tree):

- List rows (name + masked value placeholder; values never fetched).
- Add / edit (rename + value) / delete forms hitting the new API via generated
  `apps/web/lib/api/schema.d.ts` client types.
- Client-side name validation mirroring the server (uppercase, pattern, forbid
  `__ENV__` / `HIVY_` prefixes) for fast feedback.

---

## OpenAPI

Regenerate with `make openapi` after handlers land.

> ⚠️ Known caveat: `make openapi` deletes the hand-added `/v1/sheets` spec paths.
> Restore them in the same commit (verify `docs/openapi.json`, `docs/swagger.*`).

---

## Tests

- **Go handlers:** CRUD happy paths, 409 duplicate, 404 missing, rename
  collision, name validation (reject `__ENV__*`, `HIVY_*`, bad pattern),
  authorization (viewer can list, non-manager cannot mutate).
- **Go runtime:** `mergeChannelEnvVars` injects `__ENV__<NAME>` for the session's
  channel; reserved `HIVY_*` keys remain authoritative; `uuid.Nil` channel is a
  no-op. Cover both push entry paths (threaded channel vs. session-resolved).
- **Rust:** bash env strip test (above).

---

## Rollout / sequencing

1. Migration + model.
2. Go handlers + routes + `ChannelHandler` env key wiring; remove org-wide store.
3. Runtime threading (`ChannelID` through push) + `mergeChannelEnvVars`.
4. Rust `__ENV__` stripping + tests.
5. Frontend section.
6. `make openapi` (restore `/v1/sheets`), full test pass.

Steps 2–4 are one coordinated deploy (Go injects `__ENV__`, Rust strips it).
Until the Rust change ships, injected user vars would reach the workload as
`__ENV__<NAME>` — deploy Go+Rust together, or land Rust stripping first.
