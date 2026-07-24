# Production sandbox autosleep investigation — 2026-07-24 13:33 UTC — Degraded

## Run metadata

- Investigation started: `2026-07-24T13:27:49Z`
- Investigation ended: `2026-07-24T13:33:24Z`
- Investigation duration: 5 minutes 35 seconds
- Evidence window: `2026-07-24T12:00:00Z` through `2026-07-24T13:29:00Z`
- Scope: production agent-sandbox autosleep, wake, gateway routing, and status reconciliation

## Executive summary

Production autosleep is **not working exactly as intended**. The worker reliably
selects idle sandboxes and stops them, but browser session-stream reconnects wake
the same sandboxes within roughly 2–3 seconds. Because gateway-driven wakes are
mirrored back to the Go API only by a 2 minute 30 second reconciler, those
sandboxes commonly remain running until the next reconciliation and sleep sweep.

The immediate trigger is a configuration interaction: production agent
sandboxes use a 15-second idle timeout, while preview-route leases subtract a
fixed 15-second safety margin. Consequently, every observed gateway
`ensure_ready` result had a zero-second lease. Long-lived browser streams do not
generate new gateway lookups while connected; once autosleep closes a stream,
the browser reconnects and wakes the sandbox again.

The stop machinery itself appears healthy: 86 successful autosleep completions
were observed across six agent sandboxes, with no autosleep task, runner-stop, or
volume-cleanup errors in the evidence window.

## Severity summary

| Severity | Count | Environment |
|---|---:|---|
| High | 1 | Production |
| Medium | 1 | Production |
| Critical | 0 | — |

## Cluster and node health

Direct Kubernetes inspection could not be completed. The platform-engineering
setup script reported that the required Kubernetes credential variable was not
available in this environment. No attempt was made to inspect or bypass
credentials.

Grafana showed the production backend worker, microsandbox control, runner,
preview cache, and guest journald actively emitting logs throughout the evidence
window. This confirms the relevant services were operating, but it is not a
substitute for node, Pod, resource-pressure, or Kubernetes event checks.

## Service health

| Environment | Service | Autosleep-relevant status |
|---|---|---|
| Production | Backend worker | Sleep sweeps and per-sandbox tasks executing |
| Production | Microsandbox control | Stop and gateway wake paths executing |
| Production | Microsandbox runner | Guests stopping and subsequently booting |
| Production | Preview cache/gateway | Reconnects repeatedly invoking `ensure_ready` |
| Staging | Autosleep | Not assessed; production was the requested scope |

## Platform status

| Area | Status | Evidence |
|---|---|---|
| Networking/gateway | Degraded behavior | Zero-second route leases and repeated wake requests |
| Sandbox lifecycle | Degraded behavior | Stops succeed, but affected sandboxes wake again |
| Databases | Not assessed | Kubernetes/DB access unavailable |
| Storage | No autosleep cleanup warning observed | No volume-cleanup-pending logs in window |
| Observability | Sufficient for lifecycle diagnosis | Control, runner, gateway, worker, and guest logs correlated |
| Backups/certificates | Not assessed | Outside focused scope and Kubernetes access unavailable |

## Detailed findings

### High — idle agent sandboxes repeatedly sleep and wake

**Evidence**

- Six agent sandboxes recorded 86 successful autosleep completions.
- Persistent examples:

  | External sandbox | Successful sleeps | Guest boots | Observation |
  |---|---:|---:|---|
  | `rt24w6gs` | 36 | 37 | One initial boot plus one boot per sleep |
  | `eyw0b8qt` | 17 | 18 | One initial boot plus one boot per sleep |
  | `omu7ftlz` | 6 | 7 | One initial boot plus one boot per sleep |
  | `ia0jyzsj` | 1 | 2 | Slept once, then immediately booted again |

- For `omu7ftlz`, the first stop completed at `13:16:29.970Z`; the next guest
  boot began at `13:16:32.543Z`, approximately 2.6 seconds later.
- Subsequent `omu7ftlz` stop/boot cycles occurred around `13:16:52/13:16:54`,
  `13:19:14/13:19:16`, `13:21:45/13:21:47`, `13:24:15/13:24:17`, and
  `13:26:45/13:26:47`.
- Gateway logs show paired browser lookups invoking `ensure_ready` on every wake.
  The slow request waited approximately 3.4–4.5 seconds for the guest; a
  concurrent waiter then completed quickly.
- The status reconciler repeatedly corrected four running/stopped mismatches at
  its 2 minute 30 second cadence, after which the sleep sweep selected those
  sandboxes again.

**Impact**

Autosleep does not keep affected sandboxes asleep while a browser continues
reconnecting. This creates unnecessary VM boots, several seconds of repeated
stream reconnection latency, extra control-plane/runner load, and substantially
more running time than the configured 15-second idle policy implies.

**Likely cause**

High confidence. A browser stream is open through the preview gateway. The
sandbox is considered idle after its final session event and is stopped. The
stream disconnect causes browser reconnection, which invokes gateway
`ensure_ready` and starts the guest. The Go API still considers it stopped until
the periodic reconciler runs, so it cannot promptly sleep the now-idle guest.

**Recommended human action**

Treat gateway wakes as an immediate lifecycle state/activity signal to the Go
API, or move the authoritative autosleep decision into the control plane so the
2 minute 30 second status-mirror delay is not on the path. Add an integration
test that holds a browser session stream open for several idle periods and
asserts that the sandbox does not enter an endless stop/wake cycle.

### Medium — the 15-second timeout produces zero-second preview-route leases

**Evidence**

- Production lifecycle configuration logs consistently show
  `idle_timeout_ms=15000`.
- The route lease calculation subtracts a fixed 15-second safety margin from
  `sleep_after_at`.
- Every one of 189 observed gateway `ensure_ready` completions in the evidence
  window reported `lease_seconds=0`.
- A zero lease makes both the memory and Redis routes unusable, forcing
  `ensure_ready` instead of a cache hit.

**Impact**

Every direct-sandbox gateway request takes the expensive readiness path, even
immediately after a successful wake. It also makes gateway activity reporting
occur at the lease boundary rather than providing a useful active interval.

**Likely cause**

High confidence. The production idle timeout equals the fixed route safety
margin, so `sleep_after_at - safety_margin` is effectively the current time.

**Recommended human action**

Make the safety margin bounded relative to the configured timeout, for example
`min(fixed_margin, idle_timeout/3)`, and ensure the resulting lease is strictly
positive. Alternatively raise the production agent idle timeout above the
safety margin. Add tests for idle timeouts equal to and below the current margin.

## Capacity and reliability risks

- Repeated cold boots amplify runner CPU, storage, and systemd/container startup
  work and can create correlated load when multiple browser tabs reconnect.
- Wake latency of roughly 3–4.5 seconds is reintroduced on every cycle.
- The 2 minute 30 second reconciliation interval allows a gateway-woken sandbox
  to run far beyond the nominal 15-second idle threshold.
- Repeated reconnects generated simultaneous owner/waiter `ensure_ready` calls.
  Locking prevented duplicate owners, but the traffic and latency remain.

## Monitoring limitations

- Kubernetes API, node health, Pod readiness, resource pressure, and Warning
  events were not available because the required cluster credential was absent.
- Database rows were inferred from structured application logs; they were not
  queried directly.
- Grafana logs prove lifecycle behavior and timing but do not independently
  verify current resource reservations or billing records.

## Evidence appendix

Sanitized, read-only checks:

```text
Grafana/VictoriaLogs:
- production lifecycle configuration events
- auto-sleep sweep and completion events
- sandbox reconciler events
- gateway lookup results and lease durations
- guest kernel "Run /init.krun as init process" events
- autosleep/runner stop error and cleanup-warning searches

Repository:
- production idle-timeout default and lifecycle scheduling
- route lease calculation and gateway ensure-ready flow
- sleep candidate recheck and stop orchestration
- gateway-wake status reconciliation interval
```
