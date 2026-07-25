# Contributing to Hivy

## Start the complete local environment

Install Docker Desktop or Docker Engine with the Compose v2 plugin, then run:

```bash
./start.sh
```

No host Go, Node.js, pnpm, Rust, PostgreSQL, Redis, Nango, Qdrant, or MinIO
installation is needed to run the application. The first start downloads large
agent runtime images; subsequent starts reuse Docker's local image cache.

The script reports each phase and exits only after the application services are
healthy and the development workspace has been reconciled. Independent image
downloads, builds, and core infrastructure startup run concurrently.

## Seeded workspace

The default local login is:

```text
Email:    dev@hivy.local
Password: local-development
```

It contains:

- one active organization named `Hivy Development`;
- one team named `Development`;
- the owner as a member of that team;
- the team's own Hivy agent;
- the normal local welcome-credit grant; and
- completed onboarding so the workspace opens without requiring a connection.

The seed is idempotent. Running `./start.sh` again does not create another
account, organization, team, or Hivy agent. It also does not rename a team the
developer has subsequently changed.

The seed values can be overridden for the first start:

```bash
HIVY_DEV_SEED_EMAIL=me@example.test \
HIVY_DEV_SEED_PASSWORD='a-local-password' \
HIVY_DEV_SEED_USER_NAME='Local Developer' \
HIVY_DEV_SEED_ORG_NAME='My Local Workspace' \
HIVY_DEV_SEED_TEAM_NAME='Product' \
./start.sh
```

These values are local application credentials, not production secrets.

## Add an LLM provider

No model-provider API keys are read from environment variables or seeded into
the database.

After startup:

1. Open `http://localhost:30112/admin`.
2. Enter the local admin secret `local-development-admin`.
3. Add at least one system LLM credential.
4. Open `http://localhost:30112`, log in, and start an agent.

Set `HIVY_ADMIN_SECRET` when starting the stack to use a different local admin
secret. The script will not print a custom secret.

## Sandboxes

Local development selects the Docker sandbox provider. API and worker
containers use the host Docker socket to create isolated agent sandbox
containers. The startup workflow prepares both the default and developer
runtime images, plus the app sandbox image.

Startup checks the published image manifests against the Docker daemon's
platform. It pulls the requested published tags when native images are
available and automatically builds native images from the checkout otherwise.
This avoids relying on optional CPU emulation. Set `HIVY_DEV_RUNTIME_IMAGE_TAG`
or `HIVY_DEV_APP_IMAGE_TAG` to test another published tag.

When changing sandbox runtime or app-image source, build native images from the
checkout:

```bash
./start.sh --build-local-images
```

The local builder compiles Rust and Go in Docker, builds the default runtime,
developer runtime, and app images for the Docker daemon's architecture, and
starts Hivy against their `local` tags. `make up-build` is an alias for this
mode. No host Rust, Go, Zig, or Node.js installation is required.

`HIVY_DEV_SKIP_RUNTIME_IMAGES=1 ./start.sh` skips runtime-image preparation
when working only on control-plane or frontend code.

## Nango and connections

The stack runs real Nango backed by the local PostgreSQL volume. Startup does
not configure integrations and does not create connections.

Create integrations and connections manually through Hivy when a change needs
them. Nango and Hivy data persist across normal shutdowns, so an OAuth
connection only needs to be established once unless the environment is reset.

Never commit provider credentials, OAuth tokens, Nango database exports, or a
populated `.env`.

## Service addresses

The startup summary resolves the actual published ports. With the default
configuration:

| Service | Address |
| --- | --- |
| Hivy web | `http://localhost:30112` |
| Admin panel | `http://localhost:30112/admin` |
| Hivy API | `http://localhost:8080` |
| Nango | `http://localhost:23003` |
| MinIO console | `http://localhost:9001` |

Port overrides remain available through `.env`.

Published development ports bind to `127.0.0.1` by default so local
credentials and data services are not exposed on shared or cloud hosts. Set
`HIVY_COMPOSE_BIND_IP` explicitly only when another machine must reach the
stack, and protect that interface with an appropriate firewall.

## Daily commands

```bash
./start.sh
./start.sh --build-local-images
docker compose down
docker compose logs -f api worker web
```

`docker compose down` stops containers while preserving databases,
connections, uploads, and local indexes.

The following command is destructive:

```bash
docker compose down -v
```

It removes all Compose volumes, including manually created Nango connections.
The next `./start.sh` recreates an empty environment.

If `make` is installed, `make up`, `make up-build`, `make down`, and
`make reset` are convenience aliases for the same workflows.

## Hot reload

The repository is mounted into the API and worker containers. Air rebuilds the
Go processes after source changes. The web container runs the Next.js
development server with the web source mounted.

If dependencies or development container definitions change, rerun
`./start.sh`; Docker rebuild caching keeps unchanged layers.

## Troubleshooting

The startup script prints container status and recent service logs when a
phase fails. Additional inspection:

```bash
docker compose ps
docker compose logs --tail=200 api worker web nango
docker compose exec postgres pg_isready -U hivy
```

If local state is no longer useful, run `docker compose down -v` and start
again. Do not use it merely to restart services because it deletes manually
authorized connections.
