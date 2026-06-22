# Agent Onboarding

This repository is set up so a new agent or developer can start the full local stack with Docker Compose and make live changes to the web app, API, and worker.

## CI Quality Gates — Run These Before Creating a PR

**40% of CI failures come from the `ci-quality` job.** Every agent MUST run these checks on their changes before pushing or opening a PR. Skipping them wastes pipeline minutes and reviewer attention.

### Prerequisite

```sh
# Ensure golangci-lint is installed (the CI version is v1.64.8)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
```

### Run the full quality check locally

This mirrors the `ci-quality` Makefile target exactly:

```sh
cd /workspace/repos/hivy

# 1. Check for whitespace / merge-conflict markers in the diff
git diff --check

# 2. Go vet — structural issues the compiler won't catch
go vet ./internal/... ./cmd/server/...

# 3. File-length ceiling — Go files ≤300 lines, TS/TSX files ≤600 lines
./scripts/check-go-file-length.sh
./scripts/check-ts-file-length.sh

# 4. Comment density — comments must be ≤10% of added Go lines in your PR.
#    This rule is specifically aimed at AI agents: delete comments that
#    restate what well-named code already says, step-by-step narration,
#    "added for X" notes, placeholder TODOs, and decorative banners.
#    Keep only comments that explain non-obvious WHY — hidden constraints,
#    subtle invariants, workarounds for specific bugs.
./scripts/check-go-comment-density.sh

# 5. Log budget — total log call sites must stay under 475. Don't add
#    net-new slog.Info/Warn/Error or fmt.Print calls without a good reason.
#    Prefer slog.Debug or logging.Capture(ctx, err) for non-critical paths.
./scripts/check-log-budget.sh

# 6. Bare goroutine gate — forbid bare 'go ' statements outside the
#    explicit allowlist. Use goroutine.Go(ctx, fn) or sourcegraph/conc
#    pools instead.
./scripts/check-bare-goroutines.sh

# 7. Migration sanity — migration files must be contiguously numbered
#    with no gaps or duplicates.
./scripts/check-migrations.sh
```

Alternatively, run the aggregate `check` target which also builds and runs tests:

```sh
make check
```

### Also: golangci-lint (separate CI job)

The lint workflow runs `golangci-lint` scoped to new code only (`--new-from-rev=origin/main`). Run it locally before pushing:

```sh
golangci-lint run --timeout=5m --new-from-rev=origin/main ./internal/... ./cmd/server/...
```

### Common failure patterns and how to fix them

| CI Failure | Cause | Fix |
|---|---|---|
| `Go files exceeding 300 lines` | A Go file you created or edited is too long | Split the file. Do NOT add it to `scripts/file-length-allowlist.txt` unless it's genuinely generated code. |
| `Comment density N% exceeds the 10% ceiling` | You added too many comment lines vs code lines | Delete low-signal comments: step-by-step narration, obvious restatements, "added for X" notes, decorative banners. Keep only non-obvious WHY explanations. |
| `Log emit count N exceeds budget 475` | You added net-new log call sites | Replace with `slog.Debug` or `logging.Capture(ctx, err)` if not critical. |
| `Bare 'go' statements found` | You wrote `go func()` without panic recovery | Use `goroutine.Go(ctx, func(ctx context.Context) { ... })` or a `sourcegraph/conc` pool. |
| `golangci-lint` reports issues | Linter caught style/safety problems | Run `golangci-lint run --new-from-rev=origin/main ./internal/... ./cmd/server/...` locally and fix the reported issues. |

### Why these checks exist

These quality gates exist because past incidents (unrecovered panics in bare goroutines, log noise drift, excessive commenting from AI agents, file-length bloat) caused real production issues and wasted review cycles. The checks are fast — run them before `git push` and keep CI green.

## First Run

Prerequisites:

- Docker Desktop or Docker Engine running.
- Go available locally for tests and migration commands.
- Internet access for the first Docker image pull and dependency download.

Start from a fresh clone:

```sh
cp .env.example .env
make up
```

The `.env.example` file includes local-only placeholder secrets that are valid enough for Compose interpolation, migrations, auth key parsing, Nango startup, and sandbox encryption. Replace provider API keys only when testing behavior that calls external providers.

Expected local services:

- Web app: `http://localhost:30112`
- API: `http://localhost:8080`
- API health: `http://localhost:8080/healthz`
- Worker health: `http://localhost:8090/healthz`
- Local proxy health: `http://localhost:18082/health`
- Nango: `http://localhost:23003`
- MinIO API: `http://localhost:9000`
- MinIO console: `http://localhost:9001`
- Postgres: `localhost:15432`
- Redis: `localhost:16379`

The first `make up` can take a few minutes because Docker pulls images and the API/worker images run `go mod download` into fresh Docker volumes.

## Local Development Loop

`make up` starts the stack in the background:

- `api` runs with `air` using `.air.api.toml`.
- `worker` runs with `air` using `.air.worker.toml`.
- `web` runs `pnpm exec next dev --turbopack`.
- Source directories are bind-mounted, so local edits are picked up by the running containers.

Useful commands:

```sh
docker compose ps
docker compose logs -f api worker web proxy
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8090/healthz
curl -fsS http://localhost:18082/health
```

Use `make down` to stop the stack. It runs `docker compose down -v`, so it deletes local Compose volumes and database state.

## Ports

The default ports are fixed in `.env.example`. If another local stack is already running, either stop it with `make down` or override the `HIVY_COMPOSE_*_PORT` values in `.env` before running `make up`.

## Migrations

New SQL migrations live in `internal/migrations/sql` and must include Goose markers:

```sql
-- +goose Up
-- migration SQL

-- +goose Down
-- rollback SQL
```

When adding a migration, update `latestMigrationVersion` in `internal/testdb/migrations.go` so tests run against the full schema.

## Tests

There are three broad test modes in this repository:

- Plain Go tests that run against local process fixtures.
- Compose-backed tests that expect the `make up` stack to already be running.
- Sandbox/runtime tests that build or use the sandbox runtime image and create real Docker containers.

Run the full Go suite:

```sh
go test ./...
```

Some packages exercise Docker-backed behavior, so keep Docker running. For focused backend checks, prefer package-level tests while iterating:

```sh
go test ./internal/connectionaccess ./internal/handler ./internal/tasks ./internal/sandbox ./internal/mcpserver ./internal/token
```

Frontend tests are run separately with:

```sh
make ci-test-web
```

Before running any E2E target, start the local stack:

```sh
cp .env.example .env
make up
```

The generic E2E target runs the `e2e` package against the live local API:

```sh
make test-e2e
```

Docker sandbox provider tests exercise the local Docker provider directly:

```sh
make test-sandbox-docker
```

The flagship agent-session E2E is the strongest local confidence test for the runtime path:

```sh
make test-agent-sessions-e2e
```

That target:

- builds the Linux sandbox runtime binary,
- builds the `hivy-sandboxes-runtime:runtime` Docker image,
- calls the local API and worker from the running Compose stack,
- provisions real Docker-backed agent sandbox containers,
- opens the direct sandbox SSE stream,
- verifies live runtime events such as `tool_call`,
- verifies participant sharing, final responses, and canonical event ingestion.

After the runtime image has already been built, use this faster form:

```sh
AGENT_SESSIONS_E2E_BUILD_RUNTIME_IMAGE=0 make test-agent-sessions-e2e
```

This test makes a real model request through the local proxy. `.env.example` intentionally does not include a real provider key, so a fresh database must have a valid system OpenRouter credential before this test can complete. `make up` should still work without that credential; the credential is only required for live LLM E2E behavior.

For local Compose, keep these networking rules in mind:

- Browser-facing sandbox runtime URLs use `HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN=http://127.0.0.1`.
- Sandbox-to-control-plane URLs inside Compose use `host.docker.internal` defaults for API webhooks, MCP, proxy, and presigned MinIO URLs.
- Docker-backed runtime containers need enough startup time for the embedded Docker daemon and runtime control server before the agent is ready.
