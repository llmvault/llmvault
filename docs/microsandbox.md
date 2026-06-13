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

## Control Plane

Command:

```sh
/usr/local/bin/microsandbox control
```

Required environment file: `/etc/hivy/msb.env`

```sh
HIVY_MICROSANDBOX_ADDR=:8080
HIVY_MICROSANDBOX_DATABASE_DSN=postgres://...
HIVY_MICROSANDBOX_API_TOKEN=...
HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET=...
HIVY_MICROSANDBOX_RUNNER_API_TOKEN=...
HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN=preview.usehivy.com
HIVY_MICROSANDBOX_PREVIEW_JWT_SECRET=...
HIVY_MICROSANDBOX_PREVIEW_PASSWORD_KEY=...
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
HIVY_MICROSANDBOX_RUNNER_PUBLIC_URL=http://10.0.0.12:8081
HIVY_MICROSANDBOX_ADDR=:8081
HIVY_MICROSANDBOX_TOTAL_CPU=32
HIVY_MICROSANDBOX_TOTAL_MEMORY_MB=131072
HIVY_MICROSANDBOX_TOTAL_DISK_GB=2000
HIVY_MICROSANDBOX_CPU_OVERCOMMIT=1.5
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

## Hivy API Provider Config

Set the Hivy API/worker to use the control plane through the existing sandbox provider abstraction:

```sh
HIVY_SANDBOX_PROVIDER_ID=microsandbox
HIVY_MICROSANDBOX_CONTROL_URL=https://msb.usehivy.com
HIVY_MICROSANDBOX_CONTROL_API_TOKEN=...
HIVY_MICROSANDBOX_DEFAULT_PREVIEW_PORTS=3000,3001,5173,7080,8000,8080
HIVY_SANDBOXES_RUNTIME_BASE_IMAGE=ghcr.io/usehivy/hivy-sandboxes-runtime:latest
```

Agent runtime traffic uses signed runtime endpoint URLs from the control plane. Browser previews keep the password/JWT cookie flow.

## Caddy

Use `hosting/caddy/microsandbox.Caddyfile` for wildcard preview routing. The wildcard host shape is:

```text
{port}-{sandbox_id}.preview.usehivy.com
```

Caddy forwards wildcard preview requests to the control plane, which resolves the sandbox runner and proxies to the runner API.
