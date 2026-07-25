# Hivy

## Local development

Docker is the only application toolchain required. From a fresh clone:

```bash
./start.sh
```

The script prepares the sandbox runtime images, starts the complete local
infrastructure, runs database migrations, builds the application services,
and seeds a login-ready organization with one team and its team-scoped Hivy
agent.

If `make` is installed, `make up` runs the same workflow.

Use `./start.sh --build-local-images` (or `make up-build` when Make is
available) when changing the sandbox runtime or app image. Those builds also
run entirely through Docker.

See [CONTRIBUTING.md](CONTRIBUTING.md) for credentials, service URLs, manual
LLM provider setup, data persistence, and troubleshooting.
