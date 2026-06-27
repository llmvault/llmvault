# Sandbox Provisioning Latency

## Summary

The sandbox timing issue has three separate cases:

1. `3fokz7r7` - first large Hakaree developer sandbox was cold and took about 42 seconds to register ports, and about 62 seconds before first model usage.
2. `0fa6dq5f` - later large Hakaree developer sandbox using the same image was hot and registered ports in about 4 seconds.
3. `0l3rn8sg` - a reused Hivy sandbox was already running; the later delay was config rejection from an old runtime, not provisioning.

Runner-level evidence shows runner-1 was healthy and Docker daemon bootstrap inside the developer sandboxes took only about 3 seconds. The remaining cold-start time is therefore mostly Microsandbox VM/image/runtime startup before port registration, plus Hivy runtime health/config/repo work after ports exist.

## Primary Evidence

- [Microsandbox DB snapshot](/tmp/hivy-prod-session-forensics-20260620/msb_db_snapshot.json)
- [Microsandbox control logs](/tmp/hivy-prod-session-forensics-20260620/msb_logs_4h.jsonl)
- [Runner 1 evidence](/tmp/hivy-prod-session-forensics-20260620/runner1_forensics.txt)
- [Runner 1 `3fokz7r7` window](/tmp/hivy-prod-session-forensics-20260620/runner1_3fokz7r7_window.txt)
- [Runner 1 `0fa6dq5f` window](/tmp/hivy-prod-session-forensics-20260620/runner1_0fa6dq5f_window.txt)
- [Runner 1 wake window](/tmp/hivy-prod-session-forensics-20260620/runner1_wake_window.txt)
- [Runner 1 reused sandbox exec evidence](/tmp/hivy-prod-session-forensics-20260620/runner1_0l3_exec.json)

Relevant code paths:

- Agent sandbox orchestration: `internal/sandbox/orchestrator_create_agent.go`
- Microsandbox provider driver: `internal/sandbox/microsandbox/driver.go`
- Microsandbox runner create path: `internal/microsandbox/runner/microsandbox_backend.go`
- Docker daemon bootstrap inside sandbox: `internal/microsandbox/runner/docker_daemon.go`
- Runtime health wait: `internal/sandbox/orchestrator_agent_health.go`
- Warm-pool claim path: `internal/sandbox/orchestrator_warm_claim.go`

## Shared Production Context

All three target sandboxes were on runner-1:

- Runner ID: `e9501206-c9c1-476d-8430-3a119b1a6aa7`
- Runner name: `1-runner-sandboxes-hivy-com`
- API URL: `https://runner-1.sandboxes.usehivy.com`
- Status: `healthy`
- Drain: `false`
- Last heartbeat during investigation: `2026-06-20T11:58:42.970906Z`

Read-only runner host check:

- Service `microsandbox-runner`: active
- Service `caddy`: active
- Local health: OK
- Root filesystem: 906 GB total, 68 GB used, 792 GB available
- Memory: 64 GB total, about 54 GB available
- Load average: around 2.7-3.2

No evidence was found that host CPU, disk, memory, or service health caused the incident timing.

## Case 1: Cold Developer Sandbox `3fokz7r7`

Sandbox details:

- Hivy sandbox ID: `1cdeda92-98d2-48d0-a12c-2340f904b0ad`
- Microsandbox ID: `3fokz7r7`
- Agent: Hakaree
- Image: `ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.7.0-amd64`
- Size: `large`
- Status later: stopped

Timeline:

| Time | Event |
| --- | --- |
| 2026-06-20 10:55:42.766 | Hivy `sandboxes` row created. |
| 2026-06-20 10:55:42.782 | Microsandbox `microsandbox_sandboxes` row created. |
| 2026-06-20 10:56:24.936 | Microsandbox ports registered for guest ports 3000, 5173, 7080, 8000, 8080, 30112. |
| 2026-06-20 10:56:24 | Runner logged `sandbox docker daemon bootstrap completed sandbox_id=3fokz7r7 duration_ms=2948 status=started`. |
| 2026-06-20 10:56:26.613 | Microsandbox control synced preview routes. |
| 2026-06-20 10:56:32.931 | User-visible session `7b336...` created. |
| 2026-06-20 10:56:36.536 | Runtime emitted `turn_started`. |
| 2026-06-20 10:56:44.764 | First `model_usage` row recorded. |

Observed deltas:

- Microsandbox create to ports registered: about 42.15 seconds.
- Hivy sandbox row to user-visible session creation: about 50.17 seconds.
- Microsandbox create to first model usage: about 61.98 seconds.
- Docker daemon bootstrap: about 2.95 seconds.

Interpretation:

The long first-start latency is before ports are registered and is not explained by dockerd bootstrap. It is most likely cold Microsandbox VM/image/runtime startup and image/materialization work.

## Case 2: Hot Developer Sandbox `0fa6dq5f`

Sandbox details:

- Hivy sandbox ID: `81ce2946-9da4-41d0-8a91-e357eb485c30`
- Microsandbox ID: `0fa6dq5f`
- Agent: Hakaree
- Image: `ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.7.0-amd64`
- Size: `large`
- Status later: stopped

Timeline:

| Time | Event |
| --- | --- |
| 2026-06-20 11:29:04.297 | Hivy `sandboxes` row created. |
| 2026-06-20 11:29:04.345 | Microsandbox row created. |
| 2026-06-20 11:29:08.301 | Ports registered. |
| 2026-06-20 11:29:08 | Runner logged `sandbox docker daemon bootstrap completed sandbox_id=0fa6dq5f duration_ms=2947 status=started`. |
| 2026-06-20 11:29:08.470 | Preview route synced. |
| 2026-06-20 11:29:16.185 | Session `1964...` created. |
| 2026-06-20 11:29:19.779 | Runtime emitted `turn_started`. |

Observed deltas:

- Microsandbox create to ports registered: about 3.96 seconds.
- Hivy sandbox row to session creation: about 11.89 seconds.
- Hivy sandbox row to `turn_started`: about 15.48 seconds.
- Docker daemon bootstrap: about 2.95 seconds.

Interpretation:

The same runner and same developer runtime image were much faster later. That strongly suggests the earlier long latency was cold cache/image/VM startup, not a structural 1-minute minimum.

## Case 3: Reused Hivy Sandbox `0l3rn8sg`

Sandbox details:

- Hivy sandbox ID: `7c0cfafd-e6e9-48a3-b512-322f13937d3b`
- Microsandbox ID: `0l3rn8sg`
- Agent: Hivy
- Image: `ghcr.io/usehivy/hivy-sandboxes-runtime:v3.6.0-amd64`
- Size: `small`
- Status: running
- Created: `2026-06-19T06:33:53.718964Z`

Timeline:

| Time | Event |
| --- | --- |
| 2026-06-19 06:33:53 | Sandbox created. |
| 2026-06-20 11:27:14 | Runner logged `duration_ms=7 status=no-dockerd`; the sandbox did not need Docker bootstrap. |
| 2026-06-20 11:27:15 | Preview route synced for `0l3rn8sg`. |
| 2026-06-20 11:30:13 | Session `6887...` created. |
| 2026-06-20 11:30:15-11:42:51 | Delivery repeatedly failed while pushing runtime config due to unsupported `builtin.file_search`. |

Live read-only exec inside `0l3rn8sg` showed:

- Kernel alive.
- Process `/usr/local/bin/hivy-sandboxes-runtime` running.
- `/healthz` returned OK.
- Root overlay: 3.9 GB total, 3.5 GB available.
- `/workspace`: host-sized mount, 906 GB total, 792 GB available.
- `/var/lib/docker`: 9.8 GB total, 9.8 GB available.

Interpretation:

The reused sandbox problem was not startup latency. It was a config/runtime schema mismatch. See [Old Runtime Rejected New Config](./01-runtime-config-version-skew.md).

## Warm Pool Finding

The main production DB `sandbox_warm_slots` table was empty at pull time.

The Microsandbox provider driver does not implement the warm-pool interface in the observed code path. In `internal/sandbox/orchestrator_create_agent.go`, warm pool is only used when:

```go
if _, usesWarmPool := o.provider.(WarmPoolCapable); usesWarmPool && templateRef == "" {
    ...
}
```

The Microsandbox direct driver path calls:

```go
d.post(ctx, "/v1/sandboxes", body, &out)
```

So these production Microsandbox sessions were direct creates, not warm-pool claims.

## Root Cause

The first large developer sandbox paid cold Microsandbox/image/runtime startup. The second same-image sandbox was hot and much faster. The runner host was healthy and dockerd bootstrap inside the sandbox was short in both cases.

The exact uninstrumented 42-second cold portion is between:

```text
microsandbox_sandboxes.created_at
```

and:

```text
microsandbox_sandbox_ports.created_at
```

Current telemetry does not split that interval into:

- image pull/cache/materialization
- microVM creation
- volume creation/mount
- init process startup
- port binding
- runtime binary startup

## Contributing Factors

- Timing logs are not phase-level enough to tell exactly where the cold 42 seconds went.
- Warm pool was not available for these Microsandbox creates.
- Large developer image `hivy-sandboxes-runtime-developers:v3.7.0-amd64` likely has a materially larger cold-start footprint than the default runtime image.
- Hivy API waits for runtime health/config/repository setup after provider create returns, adding more user-visible delay after ports exist.

## Recommended Fix Areas

1. Add phase-level timing:
   - Control-plane create request received.
   - Runner selected.
   - Runner create started.
   - Image available.
   - Volumes created.
   - MicroVM started.
   - Ports allocated.
   - Runtime `/healthz` OK.
   - Runtime config pushed.
   - Runtime `/readyz` OK.
   - Repository clone complete.

2. Add Microsandbox warm capacity:
   - Implement a warm-pool-capable Microsandbox provider or pre-started runtime slots.
   - Keep at least one large developer sandbox warm if that is a common path.

3. Pre-pull/pre-materialize runtime images:
   - Ensure `ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.7.0-amd64` is hot on every runner.

4. Separate "sandbox exists" from "runtime ready":
   - Store and expose per-phase status in DB/UI.
   - Users should see whether they are waiting on VM/image, runtime health, config, or repo clone.

5. Avoid confusing reused sandbox failures with provisioning:
   - Schema/config failures should surface as runtime compatibility errors, not as generic startup slowness.

## Acceptance Criteria For A Fix

- For every sandbox create, logs and DB metadata can answer where each second was spent.
- Cold and hot startup paths are separately measured.
- The first large developer sandbox startup is consistently under a target SLO, or a warm slot is used.
- Wake/config failures on existing sandboxes are classified separately from provisioning latency.
- Runner dashboards show image cache and active warm capacity.

## Suggested Owner Brief

Own production sandbox startup latency and observability. Start with `internal/sandbox/orchestrator_create_agent.go`, `internal/sandbox/microsandbox/driver.go`, `internal/microsandbox/runner/microsandbox_backend.go`, and `internal/microsandbox/runner/docker_daemon.go`. The first deliverable should be phase-level instrumentation that proves exactly where cold starts spend time.
