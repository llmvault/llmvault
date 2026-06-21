# Old Always-On Runtime Rejected New Config

## Summary

Session `6887bdb9-b683-4f12-85c5-145abaf1f2a2` failed before the runtime could start a turn. The API/worker attempted to push a runtime config containing `builtin.file_search` into an existing always-on Hivy sandbox created from `ghcr.io/usehivy/hivy-sandboxes-runtime:v3.6.0-amd64`. That old runtime did not know the `builtin.file_search` tool type and rejected the config with HTTP 422.

This was not an LLM failure and not a sandbox provisioning timeout. The sandbox was reachable and `/healthz` was live. The runtime schema was older than the API-generated config.

## Impact

- The user message was accepted and persisted.
- The message queue retried delivery multiple times.
- No `turn_started`, model call, tool call, final response, or `turn_failed` runtime event was recorded for the session.
- The queue remained `pending` after final retry, with `attempt_count=7`.
- The user-visible session failed without useful runtime output.

## Primary Evidence

- [Main DB snapshot](/tmp/hivy-prod-session-forensics-20260620/main_db_snapshot.json)
- [Latest web sessions snapshot](/tmp/hivy-prod-session-forensics-20260620/web_and_linked_sessions_snapshot.json)
- [Worker logs](/tmp/hivy-prod-session-forensics-20260620/asynq_logs_4h.jsonl)
- [Microsandbox DB snapshot](/tmp/hivy-prod-session-forensics-20260620/msb_db_snapshot.json)
- [Runner 1 always-on sandbox exec evidence](/tmp/hivy-prod-session-forensics-20260620/runner1_0l3_exec.json)

Relevant code paths:

- Runtime delivery/config push: `internal/tasks/session_message_deliver_runtime.go`
- Current runtime tool enum: `sandboxes/runtime/crates/domain/src/tool_specs.rs`
- Current agent configs that enable file search: `global/agents/hivy/agent.json`
- Agent tool compilation: `internal/agentruntime/compile_tools.go`

## Timeline

All times are UTC.

| Time | Event |
| --- | --- |
| 2026-06-19 06:33:53 | Microsandbox sandbox `0l3rn8sg` was created for Hivy always-on agent from `ghcr.io/usehivy/hivy-sandboxes-runtime:v3.6.0-amd64`. |
| 2026-06-20 10:54:10 | Production API latest successful deployment was created. |
| 2026-06-20 10:54:15 | Production worker/latest `asynq` deployment was created. |
| 2026-06-20 11:27:14 | Runner logged `sandbox docker daemon bootstrap completed sandbox_id=0l3rn8sg duration_ms=7 status=no-dockerd`; this sandbox was alive and did not need Docker bootstrap. |
| 2026-06-20 11:27:15 | Microsandbox control synced preview route for `0l3rn8sg`. |
| 2026-06-20 11:30:13 | Session `6887...` was created with user message: "hey hivy. can you give me a summary of this pr ? https://github.com/usehivy/hivy/pull/191". |
| 2026-06-20 11:30:15 | Worker attempted `session:message_deliver`. |
| 2026-06-20 11:30:15-11:42:51 | Delivery retried repeatedly and failed with the same config deserialization error. |
| 2026-06-20 11:42:51 | Queue row was last updated with final `last_error`; `attempt_count=7`, `status=pending`. |

## Exact Failure

The message queue row for `6887...` has this `last_error`:

```text
sync agent runtime: agent runtime put config: put config: 422 Unprocessable Entity:
Failed to deserialize the JSON body into the target type:
definition.tools[2].type: unknown variant builtin.file_search,
expected one of builtin.bash, builtin.read_file, builtin.write_file, builtin.cron,
builtin.subagent_task, builtin.check_bash_status,
builtin.wake, builtin.skills_list, builtin.skill_view, builtin.skill_manage,
builtin.search_sessions, builtin.request_user_input, builtin.update_plan
```

Current source supports `builtin.file_search`:

```rust
#[serde(rename = "builtin.file_search")]
FileSearch(SearchConfig),
```

But the running sandbox's old `v3.6.0` runtime listed its accepted variants in the error, and `builtin.file_search` was absent.

## State Observed In Production

Session:

- `id`: `6887bdb9-b683-4f12-85c5-145abaf1f2a2`
- `name`: `summary-of-pr-191`
- `source`: `web`
- `agent`: Hivy, `4c092c84-a200-4823-84df-21720ede9986`
- `sandbox_strategy`: `always_on`
- `model`: `deepseek-v4-flash`
- `sandbox_id`: `7c0cfafd-e6e9-48a3-b512-322f13937d3b`
- `external_id`: `0l3rn8sg`
- `agent_turn_last_outcome`: `failed`

Queue:

- `status`: `pending`
- `attempt_count`: `7`
- `created_at`: `2026-06-20T11:30:13.178463Z`
- `updated_at`: `2026-06-20T11:42:51.329712Z`
- `runtime_turn_id`: empty
- `runtime_trace_id`: empty

Events:

- One `user.message`
- No `turn_started`
- No `model_usage`
- No `final`
- No runtime terminal event

Sandbox:

- Hivy DB sandbox ID: `7c0cfafd-e6e9-48a3-b512-322f13937d3b`
- Microsandbox ID: `0l3rn8sg`
- Runner: `e9501206-c9c1-476d-8430-3a119b1a6aa7` / `runner-1.sandboxes.usehivy.com`
- Image: `ghcr.io/usehivy/hivy-sandboxes-runtime:v3.6.0-amd64`
- Status: `running`
- Created: `2026-06-19T06:33:53.718964Z`

Live read-only sandbox exec confirmed:

- `/healthz` returned OK.
- Process was `/usr/local/bin/hivy-sandboxes-runtime`.
- Root overlay was live.
- `/readyz` returned 401 from inside the sandbox without runtime auth, which is expected and was not forced.

## Root Cause

The API/worker generated config for the current code and agent catalog. The always-on sandbox was already running an older runtime binary. There is no compatibility gate between:

- the API-generated runtime config schema, and
- the schema accepted by a reused always-on runtime process.

The delivery path treats a non-ready runtime by pushing config:

```go
if err := client.Readyz(ctx); err != nil {
    if err := agentruntime.PushAgentRuntimeConfig(ctx, h.compileDeps, agent, sb); err != nil {
        return nil, nil, fmt.Errorf("sync agent runtime: %w", err)
    }
}
```

That is correct for a compatible runtime, but fatal when the runtime is too old to deserialize the config.

## Why It Repeated For 12 Minutes

`session:message_deliver` is retried by the worker. Each retry loaded the same running always-on sandbox and attempted the same config push. Since the runtime binary did not change, every retry failed identically.

The logs show retry counts `0` through `5` on the same Asynq task and then `final_attempt=true`.

## Contributing Factors

- Always-on sandboxes can outlive API/runtime protocol changes.
- Runtime config push does not first query supported tool types or protocol version.
- Agent config enabled a newly supported tool, `file_search`.
- The session queue stayed pending after final retry, so the system did not convert the failed delivery into a terminal user-visible failure event.

## Recommended Fix Areas

1. Runtime capability handshake:
   - Runtime exposes protocol version and supported tool types.
   - API compares generated config requirements before `PutRuntimeConfig`.

2. Automatic stale runtime replacement:
   - If the runtime is too old, mark the sandbox stale and recreate/restart it with the current image.
   - This should be especially strict for always-on sandboxes.

3. Config downgrade fallback:
   - If safe, omit unsupported non-critical tools for older runtimes.
   - This is less desirable than upgrade/recreate because behavior diverges silently.

4. Queue terminalization:
   - After final retry, write a clear terminal session event and mark the queue/session as failed instead of leaving a pending queue row with only `last_error`.

5. Deployment guard:
   - When deploying runtime schema changes, either pre-drain/recreate always-on sandboxes or make the first message recreate them automatically.

## Acceptance Criteria For A Fix

- A `v3.6.0` always-on sandbox cannot repeatedly receive an unsupported `builtin.file_search` config.
- A reused sandbox with unsupported capabilities is recreated or clearly failed in one attempt.
- The user sees a terminal error event, not an indefinitely pending message.
- New runtime tool additions include compatibility tests against previous runtime schemas or a forced runtime version bump.

## Suggested Owner Brief

Own the runtime/API compatibility boundary for always-on sandboxes. Start with `internal/tasks/session_message_deliver_runtime.go`, `internal/agentruntime/compile_tools.go`, and `sandboxes/runtime/crates/domain/src/tool_specs.rs`. The target fix is not just to handle `file_search`; it is to make runtime config schema changes safe for all reused sandboxes.
