# Microsandbox Fleet Service

The Microsandbox fleet runs as its own Hivy service binary. The main Hivy API keeps using `internal/sandbox.Provider` and talks to the control plane over HTTP.

## Build

```sh
make microsandbox-build
```

Release binaries:

```sh
make microsandbox-release-linux-amd64
make microsandbox-release-linux-arm64
make microsandbox-release-darwin-arm64
```

The Linux release targets use Docker Linux builders because the Microsandbox Go SDK uses cgo/FFI. This avoids macOS cross-compilation failures.

## Control Plane

Command:

```sh
/usr/local/bin/microsandbox control
```

Required environment file: `/etc/hivy/msb.env`

```sh
HIVY_MICROSANDBOX_ADDR=:8080
HIVY_MICROSANDBOX_DATABASE_DSN=postgres://...
HIVY_MICROSANDBOX_LOG_LEVEL=info
HIVY_MICROSANDBOX_LOG_FORMAT=json
HIVY_MICROSANDBOX_API_TOKEN=...
HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET=...
HIVY_MICROSANDBOX_RUNNER_API_TOKEN=...
HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN=preview.usehivy.com
HIVY_MICROSANDBOX_PREVIEW_JWT_SECRET=...
HIVY_MICROSANDBOX_PREVIEW_PASSWORD_KEY=...
HIVY_MICROSANDBOX_PREVIEW_CACHE_URL=https://preview.usehivy.com/_microsandbox/preview-cache
HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN=...
HIVY_MICROSANDBOX_PREVIEW_CACHE_SYNC_INTERVAL=1m
```

Systemd unit:

```ini
[Unit]
Description=Hivy Microsandbox Control Plane
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/hivy/msb.env
ExecStart=/usr/local/bin/microsandbox control
Restart=always
RestartSec=3
User=hivy
Group=hivy

[Install]
WantedBy=multi-user.target
```

Run SQL migrations before starting the control plane:

```sh
msb migrate up
```

The fleet service requires a Postgres `HIVY_MICROSANDBOX_DATABASE_DSN` and has its own embedded migrations under `internal/microsandbox/migrations`. Do not run the main Hivy API migrations for this service.

## Runner

Install Microsandbox on the bare-metal runner, install the Hivy `microsandbox` binary, and run:

```sh
/usr/local/bin/microsandbox runner
```

Required environment file: `/etc/hivy/microsandbox-runner.env`

```sh
HIVY_MICROSANDBOX_CONTROL_URL=https://msb.usehivy.com
HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET=...
HIVY_MICROSANDBOX_RUNNER_API_TOKEN=...
HIVY_MICROSANDBOX_RUNNER_NAME=runner-1
HIVY_MICROSANDBOX_RUNNER_PUBLIC_URL=https://runner-1.sandboxes.usehivy.com
HIVY_MICROSANDBOX_RUNNER_PREVIEW_BASE_URL=http://10.80.1.2
HIVY_MICROSANDBOX_ADDR=127.0.0.1:8081
HIVY_MICROSANDBOX_RUNNER_TOTAL_CPU=32
HIVY_MICROSANDBOX_RUNNER_TOTAL_MEMORY_MB=131072
HIVY_MICROSANDBOX_RUNNER_TOTAL_DISK_GB=2000
HIVY_MICROSANDBOX_RUNNER_CPU_OVERCOMMIT=1.5
HIVY_MICROSANDBOX_RUNNER_DISK_OVERCOMMIT=4
HIVY_MICROSANDBOX_RUNNER_PREVIEW_PORT_RANGE_START=30000
HIVY_MICROSANDBOX_RUNNER_PREVIEW_PORT_RANGE_END=60999
```

Systemd unit:

```ini
[Unit]
Description=Hivy Microsandbox Runner
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/hivy/microsandbox-runner.env
ExecStart=/usr/local/bin/microsandbox runner
Restart=always
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
```

Runner production install is native binary plus systemd. Docker images are optional and not the primary deployment path.

## Runtime Image Docker Support

All agent runtime images are expected to support Docker-in-Microsandbox by default:

- `docker info` must connect to an in-sandbox Docker daemon.
- `docker compose version` must work.
- Docker data must be stored on the dedicated `/var/lib/docker` mount, not on the sandbox root filesystem.

The runtime Dockerfiles install Docker engine, Docker CLI, and Compose v2. The runtime entrypoint starts `dockerd` before starting `hivy-sandboxes-runtime` and fails startup if Docker cannot become ready.

The runner creates two named volumes for every sandbox:

- `/workspace`: user workspace data.
- `/var/lib/docker`: disk-backed Docker data volume, following the Microsandbox Docker recipe's guidance to avoid Docker storage on the overlay-backed root filesystem.

The Docker data volume is carved out of the requested sandbox disk budget and capped at 20 GiB. For example, a 40 GiB sandbox gets a 30 GiB workspace volume and a 10 GiB Docker data volume.

After building the runtime image, run:

```sh
make sandbox-runtime-image-test
```

That smoke test verifies the runtime API and checks `docker info` plus `docker compose version` inside the image. The live Microsandbox E2E should additionally run `docker run` and `docker compose up` inside a real sandbox after the image is published.

## Preview Routing

Preview traffic does not go through the Railway control plane and does not go through the runner API. The deployed hot path is:

```text
browser
  -> preview Caddy VPS
  -> local Microsandbox gateway
  -> Redis route cache or Microsandbox control lookup/ensure-ready
  -> runner private IP + published sandbox host port
  -> sandbox guest port
```

The wildcard host shape is:

```text
{port}-{sandbox_id}.preview.usehivy.com
```

Microsandbox control remains the source of truth. The Caddy gateway keeps Redis as a local cache, but falls back to control for empty or stale routes and calls control `ensure-ready` for runtime port `7080` before forwarding. Route payloads store full upstream URLs:

```json
{
  "sandbox_id": "sbx_123",
  "status": "running",
  "upstreams": {
    "3000": "http://10.80.1.2:30000",
    "5173": "http://10.80.1.2:30001"
  }
}
```

The gateway lookup endpoint returns `X-Microsandbox-Upstream: 10.80.1.2:30001` to Caddy. Caddy preserves the original request path and query and proxies directly to that private upstream. Caddy strips `Authorization` from the lookup subrequest so runtime bearer tokens are not exposed to the gateway, then preserves the original authorization header on the final sandbox request.

Browser preview password/JWT enforcement is not implemented in the direct Caddy route path yet. Keep preview domains operationally private or treat them as bearer-by-URL until that auth layer is added.

## Flagship E2E

Run the canonical Vite preview lifecycle test from the repository root:

```sh
export HIVY_MICROSANDBOX_CONTROL_URL=https://msb.usehivy.com
export HIVY_MICROSANDBOX_API_TOKEN=...
export HIVY_MICROSANDBOX_E2E_PREVIEW_RESOLVE_IP=46.62.169.26 # optional when DNS is already fresh

scripts/microsandbox/e2e-vite-preview.py --size medium
```

The script creates a sandbox, installs a Vite React app, waits for preview `200`, stops the sandbox, verifies the next preview request auto-wakes back to `200`, then deletes it and waits for preview `404`.
