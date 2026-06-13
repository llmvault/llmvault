# hivy-sandboxes-runtime

Rust runtime for AI agents running inside sandbox workspaces. One sandbox = one workspace = one agent runtime. Built on a provider-agnostic Rig-based agent runtime and configured live via a control-plane HTTP API.

## Layout

```
crates/
  domain/           # shared types: AgentDefinition, SessionId, InboundEvent, ConfigStore
  agent/            # AgentRunner trait, Rig-based agent runtime
  storage/          # SQLite repos + migrations
  tools/            # bash, read_file, write_file, ...
  mcp/              # MCP server registry
  skills/           # skill prompt composition
  webhooks/         # outbound webhook outbox
  api/              # axum control-plane HTTP
  hivy-sandboxes-runtime/  # binary
```

## Sentry

Sentry is disabled unless `SENTRY_DSN` or `SENTRY_SPOTLIGHT=true` is set.

Useful environment variables:

- `SENTRY_DSN`: production Sentry DSN.
- `SENTRY_ENVIRONMENT`: defaults to `APP_ENV`, `RUST_ENV`, then `development`.
- `SENTRY_RELEASE`: release name; otherwise the Rust SDK release default is used.
- `SENTRY_TRACES_SAMPLE_RATE`: defaults to `0.0`, or `1.0` when Spotlight is enabled.
- `SENTRY_ENABLE_LOGS`: defaults to `false`, or `true` when Spotlight is enabled.
- `SENTRY_SPOTLIGHT=true`: mirror Sentry envelopes to local Spotlight.
- `SENTRY_SPOTLIGHT_URL`: defaults to `http://localhost:8969/stream`.

Local Spotlight smoke test:

```bash
npx -y @spotlightjs/spotlight server --port 8969 --debug
env -u SENTRY_DSN SENTRY_SPOTLIGHT=true cargo test -p hivy-sandboxes-runtime sentry_support -- --nocapture
```
