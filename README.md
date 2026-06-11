# Hivy

Please read AGENTS.md file before starting.

## Local Development

After copying `.env.example` to `.env` and filling required secrets, run:

```bash
make dev
```

`make dev` uses Docker Compose as the source of truth, but runs development
processes: the Go API and worker restart with Air when backend files change,
and `apps/web` runs `next dev` with the app directory mounted into the
container.

The default Postgres host port for `make dev` is `15432` so it does not collide
with the test stack on `5433`. Compose-only knobs use the `HIVY_COMPOSE_*`
prefix; override values like `HIVY_COMPOSE_POSTGRES_PORT` in `.env` if you need
different host ports.
Redis binds to `16379` on the host for the same reason; inside Compose, Hivy
still uses `redis:6379`.

## Local Env Secrets

Start from the checked-in template:

```bash
cp .env.example .env
```

Generate the common local secrets with:

```bash
{
  echo "HIVY_SESSION_SECRET=$(openssl rand -base64 32)"
  echo "HIVY_JWT_SIGNING_KEY=$(openssl rand -base64 32)"
  echo "HIVY_AUTH_RSA_PRIVATE_KEY=$(openssl genrsa 2048 | base64 | tr -d '\n')"
  echo "HIVY_KMS_TYPE=aead"
  echo "HIVY_KMS_KEY=$(openssl rand -base64 32)"
  echo "HIVY_SANDBOX_ENCRYPTION_KEY=$(openssl rand -base64 32)"
  echo "HIVY_NANGO_ENCRYPTION_KEY=$(openssl rand -base64 32)"
}
```

Paste those values into `.env`. Provider credentials and LLM keys still need to be real values for flows that call external services.

## Quality Gates

`make check` (and the `ci-quality` CI target) enforces the following gates. All must pass before merging.

| Gate | Script / Command | What it checks |
|------|------------------|----------------|
| go vet | `go vet ./internal/... ./cmd/server/...` | Standard Go static analysis |
| golangci-lint | `golangci-lint run` | forbidigo (log hygiene), depguard, errcheck, staticcheck, and more (see `.golangci.yml`) |
| Go file length | `scripts/check-go-file-length.sh` | Hand-written `.go` files must be ≤300 lines; exceptions in `scripts/file-length-allowlist.txt` |
| TS file length | `scripts/check-ts-file-length.sh` | Hand-written `.ts`/`.tsx` files must be ≤600 lines; exceptions in `scripts/ts-file-length-allowlist.txt` |
| Log budget | `scripts/check-log-budget.sh` | Total log emit-site count must stay under a declared budget |
| Comment density | `scripts/check-go-comment-density.sh` | PR diff must not exceed 10% comment lines (PR-diff check, CI only) |
| Bare goroutine gate | `scripts/check-bare-goroutines.sh` | Forbids bare `go` statements outside `scripts/goroutine-allowlist.txt`; new goroutines must use `goroutine.Go` or `conc` pools |
| Migrations sanity | `scripts/check-migrations.sh` | Migration files must be contiguously numbered; when `TEST_DATABASE_URL` is set, also verifies the test DB version matches the highest migration file |

### Goroutine policy

Bare `go func` or `go someFunc()` calls are forbidden outside an explicit allowlist because unrecovered panics in bare goroutines crash worker processes silently.

**Use instead:**
- `goroutine.Go(ctx, func(ctx context.Context) { ... })` for singleton background goroutines (cleanup loops, flushers).
- [`sourcegraph/conc`](https://github.com/sourcegraph/conc) pools for fan-out work.

**Allowed exceptions** (in `scripts/goroutine-allowlist.txt`): channel-producer goroutines whose lifetime is bounded by a `close(ch)` + `ctx.Done()`, drain goroutines with their own `defer recover()` + `wg.Done()`, and fixed-size worker pools with `wg.Wait()`. Each entry requires a justification comment.
