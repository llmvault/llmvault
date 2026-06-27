# Microsandbox Performance Deep Dive

Date: 2026-06-20

Scope: why production sandbox creation took about one minute cold, why a later create still took about 15 seconds, why messages to an already-running sandbox feel like 3-4 seconds, and what production-grade fixes are needed.

This was a read-only investigation. Evidence was pulled from production Postgres, the Microsandbox Postgres, Railway logs, Railway service containers, and the bare-metal runner/proxy hosts. Supporting local snapshots are under `/tmp/hivy-prod-microsandbox-perf-20260620` and `/tmp/hivy-prod-session-forensics-20260620`.

## Executive Finding

The slow path is not one bottleneck. It is a stack of fixed costs:

1. Cold Microsandbox OCI image/materialization is the large first-create cost. Production evidence shows a cold `hivy-sandboxes-runtime-developers:v3.7.0-amd64` sandbox needed 42.155s from MSB row creation to first exposed port.
2. Agent sandboxes are not actually warmed. `HIVY_SANDBOX_WARM_POOL_AGENT_SIZE=0`, and the Microsandbox provider does not implement `WarmPoolCapable`, so `internal/sandbox/orchestrator_create_agent.go` always goes through direct `provider.CreateSandbox`.
3. Even after MSB exposes ports, Hivy blocks on runtime health, config push, and selected repository clone. For hot sandbox `0fa6dq5f`, MSB exposed ports by `2026-06-20T11:29:08.301Z`, API saw runtime live by `11:29:08.986Z`, but `agent sandbox created` was not logged until `11:29:16.182Z`.
4. User messages often pay the worker queue delay. Delivered queue rows in the last 14 days have p50 queue wait about 3.004s. After delivery, runtime `turn_started` is fast: p50 0.663s.
5. Runtime control traffic crosses regions over public domains. Railway API/worker are in `us-west2`; runners/proxy are Hetzner. API container ping to both `runner-1.sandboxes.usehivy.com` and `preview.usehivy.com` was about 175ms RTT. A simple API-container health request to the runtime was about 0.77s. Runner-local runtime health was 0.000756s.
6. `/sandbox-access` performs repeated provider/status/health work and can duplicate waits after wake. Logs show requests taking 2.728s, 5.454s, 3.685s, and 5.586s even when the runtime health probe itself succeeded on the first attempt.

The sub-five-second target is reachable, but not with cold OCI creation, full config push per message, synchronous repo clone, Asynq handoff on the hot path, and cross-region runtime calls.

## Evidence Files

- `/tmp/hivy-prod-microsandbox-perf-20260620/main_session_timings.csv`
- `/tmp/hivy-prod-microsandbox-perf-20260620/msb_sandbox_timings.csv`
- `/tmp/hivy-prod-microsandbox-perf-20260620/main_event_counts.txt`
- `/tmp/hivy-prod-microsandbox-perf-20260620/msb_event_counts.txt`
- `/tmp/hivy-prod-microsandbox-perf-20260620/recent_pending_queue.txt`
- `/tmp/hivy-prod-microsandbox-perf-20260620/agent_runtime_shape.txt`
- `/tmp/hivy-prod-microsandbox-perf-20260620/connection_resource_shape.txt`
- `/tmp/hivy-prod-session-forensics-20260620/api_logs_4h.jsonl`
- `/tmp/hivy-prod-session-forensics-20260620/asynq_logs_4h.jsonl`
- `/tmp/hivy-prod-session-forensics-20260620/msb_logs_4h.jsonl`
- `/tmp/hivy-prod-session-forensics-20260620/runner1_forensics.txt`

## Timeline: Cold Developer Sandbox `3fokz7r7`

Agent: Hakaree. Image: `ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.7.0-amd64`. Size: large, 4 CPU / 8192 MB / 40 GB.

| Time UTC | Event | Evidence |
| --- | --- | --- |
| 2026-06-20 10:55:42.766 | Hivy `sandboxes` row created | `main_session_timings.csv` |
| 2026-06-20 10:55:42.782 | MSB `microsandbox_sandboxes` row created | `msb_sandbox_timings.csv` |
| 2026-06-20 10:56:24.936 | First MSB port row created | `msb_sandbox_timings.csv` |
| 2026-06-20 10:56:25.102 | Preview route synced | `msb_logs_4h.jsonl` |
| 2026-06-20 10:56:24 local runner log | Docker daemon bootstrap completed in 2948ms | `runner1_forensics.txt` |
| 2026-06-20 10:56:32.931 | First Hivy session using sandbox created | `main_session_timings.csv` |
| 2026-06-20 10:56:35.972 | First message delivered to runtime | `main_session_timings.csv` |
| 2026-06-20 10:56:36.536 | Runtime emitted `turn_started` | `main_session_timings.csv` |
| 2026-06-20 10:56:44.764 | First `model_usage` event | `main_session_timings.csv` |

Breakdown:

- MSB create-to-first-port: 42.155s.
- Docker bootstrap inside sandbox: 2.948s.
- Hivy row to first model usage: about 62.0s.
- The 42s happened before Docker bootstrap and before first port registration. This points to Microsandbox image/rootfs/microVM materialization, not app runtime code or dockerd.

Important inference: Microsandbox itself documents very fast boot for hot/materialized workloads, but an open Microsandbox issue describes standard OCI cold starts requiring image download and extraction before boot. Their benchmark shows a standard OCI cold start around 20.1s versus OverlayBD around 3.9s. Our 42.155s cold row is consistent with a large developer OCI image being materialized cold on the runner.

## Timeline: Hot Developer Sandbox `0fa6dq5f`

Agent: Hakaree. Same image and same runner as `3fokz7r7`.

| Time UTC | Event | Evidence |
| --- | --- | --- |
| 2026-06-20 11:29:04.297 | Hivy `sandboxes` row created | `main_session_timings.csv` |
| 2026-06-20 11:29:04.345 | MSB sandbox row created | `msb_sandbox_timings.csv` |
| 2026-06-20 11:29:08.301 | First MSB port row created | `msb_sandbox_timings.csv` |
| 2026-06-20 11:29:08.465 | Preview route synced | `msb_logs_4h.jsonl` |
| 2026-06-20 11:29:08.473 | API started waiting for runtime | `api_logs_4h.jsonl` |
| 2026-06-20 11:29:08.986 | API observed runtime live | `api_logs_4h.jsonl` |
| 2026-06-20 11:29:08 local runner log | Docker daemon bootstrap completed in 2947ms | `runner1_forensics.txt` |
| 2026-06-20 11:29:16.182 | API logged `agent sandbox created` | `api_logs_4h.jsonl` |
| 2026-06-20 11:29:16.185 | Session created | `main_session_timings.csv` |
| 2026-06-20 11:29:19.154 | Queue row delivered | `main_session_timings.csv` |
| 2026-06-20 11:29:19.779 | Runtime `turn_started` | `main_session_timings.csv` |

Breakdown:

- MSB create-to-first-port: 3.956s.
- Runtime reachable by API: about 4.64s after MSB row creation.
- API did not finish sandbox creation until about 11.84s after Hivy row creation.
- The extra roughly 7.2s after runtime health is Hivy orchestration: config push and repository clone/fetch.

The repo clone path is active. Hakaree has GitHub connections whose metadata selects `usehivy/hivy`:

```text
{"repository": [{"id": "usehivy/hivy", "name": "hivy", "type": "repository", "full_name": "usehivy/hivy"}]}
```

`cloneRepositories` runs `mkdir -p` and then `git clone --depth=1` or `git fetch --depth=1` synchronously before `CreateAgentSandbox` returns. That is on the user-facing provisioning path.

## Message Delivery Findings

Production queue rows from the last 14 days:

| Metric | n | p50 | p90 | max |
| --- | ---: | ---: | ---: | ---: |
| `queue_wait_ms` | 31 | 3004ms | 16091ms | 80091ms |
| `delivered_to_turn_ms` | 31 | 663ms | 846ms | 1097ms |
| `turn_to_first_model_usage_ms` | 42 | 4804ms | 10742ms | 21681ms |

Interpretation:

- The runtime starts the turn quickly after message delivery.
- The 3-4s user-visible delay before a sandbox responds is usually before runtime injection: inline dispatch fallback plus worker scheduling and config sync.
- `SendMessage` tries inline delivery with `WithoutProvisioning()`. If the runtime is not already ready with accepted config, it queues to Asynq.
- `deliverClaim` calls `ensureRuntimeClient`, `PushAgentRuntimeConfigForSessionModel`, `GetRuntimeClient`, and then `PostHTTPMessage`.
- `PushAgentRuntimeConfigForSessionModel` calls `/healthz`, `PUT /config`, and `/readyz` every delivered message.
- Runtime `/config` persists the definition, syncs skills, reloads MCP specs, reloads outbound channels, replaces config, and marks config loaded. This is heavy and should not be on every message when config has not changed.

Latest reused Hivy sandbox session `6887bdb9-b683-4f12-85c5-145abaf1f2a2` never delivered:

- Queue row `ec6d4379-031a-4608-8ff2-57fb44a49614`.
- Attempt count: 7.
- Last error: runtime config rejected `builtin.file_search` because the running runtime image expected an older tool enum.
- This is a separate correctness issue from latency, but it also proves why inline dispatch falls back/retries when an existing sandbox is alive but not config-compatible.

## Network Measurements

Railway API/worker environment:

- `HIVY_RAILWAY_REGION=us-west2`
- `HIVY_SANDBOX_PROVIDER_ID=microsandbox`
- `HIVY_SANDBOX_WARM_POOL_AGENT_SIZE=0`

Runner/proxy inventory:

- runner1: `157.180.98.55`, `runner-1.sandboxes.usehivy.com`, private preview base `10.80.1.3`
- preview proxy: `46.62.169.26`, private IP `10.80.0.2`

Measurements:

| Probe | Result |
| --- | ---: |
| API container ping to `7080-0l3rn8sg.preview.usehivy.com` | avg 175.987ms |
| API container ping to `runner-1.sandboxes.usehivy.com` | avg 175.284ms |
| API container `wget` runtime `/healthz` | 0.77s |
| API container `wget` runner `/health` | 0.79s |
| MSB container `wget` runner `/health` | 0.81s |
| Runner host to local runtime port `127.0.0.1:30002/healthz` | 0.000756s |
| Preview proxy to private runner port `10.80.1.3:30002/healthz` | 0.001467s |
| Preview proxy to public preview URL | 0.073989s |
| Runner host to public preview URL | 0.022128s |
| Runner-local harmless exec via runner API | 0.005-0.006s |
| Runner to public runner exec | 0.106s |

Private `10.80.1.3` probes from Railway containers hung and had to be cancelled. Railway services do not have the same private path that the Hetzner preview proxy has.

Conclusion: the runner and preview proxy are fast locally. The control plane is paying public cross-region RTT and TLS/public routing on every runtime call.

## `/sandbox-access` Findings

Observed API log latencies:

- `0l3rn8sg` reused sandbox access: 2728ms.
- `3fokz7r7` sandbox access: 5454ms, 3685ms, 5586ms, with later repeated calls around 178-192ms.
- `0fa6dq5f` sandbox access after hot create: 177ms.

Code path:

- `SandboxAccess` calls `ensureSessionSandboxReady`.
- Runtime readiness now assumes an already-provisioned sandbox remains active, refreshes an expired runtime URL when needed, then probes the runtime directly.
- The deleted Go wake path no longer starts stopped sandboxes or performs duplicate readiness waits from browser access.

So a wake path can perform provider start/refresh plus a health wait, then `EnsureSandboxRuntimeReady` performs another health wait. Concurrent UI calls can also queue behind the lifecycle lock and repeat the same checks.

## Root Causes

### Root Cause 1: Cold OCI image materialization on the request path

The cold create was dominated by MSB create-to-first-port time. Dockerd took only about 3s and runtime health was fast after ports existed. This is the part that must be eliminated from the user request path.

Supporting external source: Microsandbox advertises average boot under 100ms for the microVM itself, but its own issue tracker identifies standard OCI image download/extraction as a cold-start bottleneck and discusses on-demand image loading as a fix.

### Root Cause 2: No agent warm pool for Microsandbox

Two independent blockers:

- Production config has `HIVY_SANDBOX_WARM_POOL_AGENT_SIZE=0`.
- The Microsandbox provider does not implement `WarmPoolCapable`, so the warm claim branch in `CreateAgentSandbox` cannot run for Microsandbox anyway.

### Root Cause 3: Hot create blocks on config plus repo clone

For `0fa6dq5f`, MSB had done its part in about 4s and API saw runtime live by about 4.6s. The next roughly 7.2s was Hivy work after runtime health. The selected `usehivy/hivy` repo clone/fetch is synchronous in the create path.

### Root Cause 4: Message delivery uses Asynq and full config push on the hot path

The runtime can start a turn within about 1s after delivery, but the message queue wait p50 is about 3s. The inline fast path falls back when readiness/config is not already acceptable. On worker delivery, full config push happens every message.

### Root Cause 5: Control-plane traffic is cross-region and public

Runtime health/config/message calls are sequential and remote. With a 175ms RTT and public TLS/proxy overhead, each call adds noticeable fixed cost. The message path can do 4-5 runtime HTTP calls before the model even starts.

### Root Cause 6: Access/wake path repeats readiness checks

`/sandbox-access` is not just token minting. It reconciles status, may wake, refreshes URL, and probes health; on wake it can probe health twice. This explains multi-second access calls even for an eventually healthy sandbox.

## Production-Grade Fixes

### P0: Remove cold create from the request path

Implement `WarmPoolCapable` for the Microsandbox provider and set a nonzero agent warm pool per `(agent image, size, runtime version)`. Warm slots must be real running sandboxes with ports assigned, runtime process booted, and `/healthz` passing. Claim should only bind the slot to an agent/session and push agent-specific config.

Use separate pools for:

- `default/small`
- `developer/large`
- runtime image tag, because v3.6/v3.7 mismatches caused real failures

### P0: Stop full config push per message

Add a config version/hash contract:

- Control plane computes `agent_config_hash` from compiled definition, tools, MCP specs, skills, outbound channels, and env.
- Runtime exposes current hash through `/readyz` or `/config/status`.
- `SessionMessageDeliver` only pushes config when hash differs.
- Session model override should be carried in `POST /sessions/{id}/messages` as `model_definition`; it should not require a full `/config` push.

### P0: Move runtime control traffic onto a low-latency/private path

Do not use public preview URLs for API/worker runtime control. Options:

- Co-locate API/worker/MSB with runners/proxy in the same region/network.
- Move runners/proxy near Railway `us-west2`.
- Run a small Hetzner-side control gateway that accepts one authenticated RPC from Railway and performs local health/config/message calls to runner/private ports.
- Store two runtime URLs: a private control URL for API/worker and a public preview URL for browsers.

The current private `10.80.1.3` path is only usable from the Hetzner preview proxy, not Railway containers.

### P0: Prebake or decouple repository clone

Do not block first user message on `git clone --depth=1 usehivy/hivy`:

- Build agent templates/snapshots with selected repos already cloned.
- Maintain per-runner shared read-only repo mirrors and create per-sandbox working copies from local disk.
- Or let the sandbox become interactive immediately and clone in background, with tools surfacing repo-not-ready until clone completes.

### P1: Optimize image/rootfs strategy

Shrink the developer image and pre-materialize it on every runner. Investigate Microsandbox snapshot/fork/restore and OverlayBD/on-demand image loading. The target state is a snapshot after:

- runtime binary loaded
- `/healthz` passes
- optional dockerd already started for developer image
- base repo mirror available locally

This matches how production sandbox platforms avoid request-time cold starts: prebuilt templates and memory/filesystem snapshots.

### P1: Make `/sandbox-access` idempotent and cheap

For running sandboxes with non-expired runtime URL and recent health:

- Skip provider `GetStatus`.
- Skip URL refresh.
- Skip duplicate `/healthz`.
- Return cached access token/URL or mint JWT only.

For browser access:

- Do not ask the provider for live sandbox status.
- Singleflight concurrent browser calls for the same session/sandbox.
- Frontend should not fan out multiple `/sandbox-access` calls on page load.

### P1: Add phase instrumentation before more tuning

Add structured timings for:

- Hivy `CreateAgentSandbox`: save row, prepare startup, provider create, endpoint, DB update, health wait, config push, repo sync, total.
- MSB control: runner selection, runner create POST duration, port insert, preview route sync.
- Runner: volume create, port allocation, `microsandbox.CreateSandbox`, detach, dockerd bootstrap, state publish.
- Message delivery: claim wait, readyz, config hash check, config push, post message.

The current logs are insufficient inside `microsandbox.CreateSandbox`; we can bound the cold phase but not split image pull/extract/rootfs/VM start without new instrumentation.

## Architecture Research Notes

- Firecracker advertises microVM startup in as little as 125ms and uses snapshots to serialize/restore running microVM state.
- E2B snapshots capture filesystem and memory state; its template `start`/`ready` flow snapshots after startup work so later sandbox creates avoid that wait.
- Modal uses warm container retention (`scaledown_window`) and sandbox/filesystem snapshots to avoid cold work on request paths.
- Cloudflare Workers avoid container/VM cold starts by using V8 isolates and keeping isolate/runtime work off the request path.
- Microsandbox advertises sub-100ms average boot for the microVM itself, but its issue tracker explicitly calls out standard OCI image download/extraction as a cold-start problem.

## What To Assign

1. Warm pool and snapshot owner: implement Microsandbox warm slots, nonzero agent warm pool, and image/version-specific pool accounting.
2. Message hot path owner: config hash/etag, no full config push per message, inline delivery reliability.
3. Network/topology owner: private control URL or co-location, remove public preview URL from API/worker runtime control.
4. Repo/image owner: prebaked repo/template snapshots, local repo mirrors, developer image slimming.
5. Access path owner: cheap `/sandbox-access`, remove duplicate health wait, frontend singleflight.
6. Observability owner: per-phase timing instrumentation across API, MSB control, runner, and runtime.

## Source Links

- Microsandbox README: https://github.com/superradcompany/microsandbox
- Microsandbox cold-start issue: https://github.com/superradcompany/microsandbox/issues/601
- Microsandbox SDK overview: https://github.com/superradcompany/microsandbox/blob/main/docs/sdk/overview.mdx
- Firecracker: https://firecracker-microvm.github.io/
- Firecracker snapshotting: https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md
- E2B sandbox snapshots: https://e2b.dev/docs/sandbox/snapshots
- E2B start/ready command snapshots: https://e2b.dev/docs/template/start-ready-command
- Modal sandbox snapshots: https://modal.com/docs/guide/sandbox-snapshots
- Modal cold starts and warm retention: https://modal.com/docs/guide/cold-start
- Cloudflare Workers architecture: https://developers.cloudflare.com/workers/reference/how-workers-works/
