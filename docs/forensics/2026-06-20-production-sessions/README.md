# Production Session Forensics - 2026-06-20

This folder contains handoff-ready forensic notes for the latest production session incidents investigated on 2026-06-20.

The investigation was read-only. It used Railway production service metadata/logs, production Postgres snapshots, Microsandbox control-plane rows, and read-only SSH checks against runner hosts. No code or production state was changed.

## Source Evidence

- [Main production DB snapshot](/tmp/hivy-prod-session-forensics-20260620/main_db_snapshot.json)
- [Latest web sessions and linked runtime sessions snapshot](/tmp/hivy-prod-session-forensics-20260620/web_and_linked_sessions_snapshot.json)
- [Microsandbox production DB snapshot](/tmp/hivy-prod-session-forensics-20260620/msb_db_snapshot.json)
- [API logs](/tmp/hivy-prod-session-forensics-20260620/api_logs_4h.jsonl)
- [Worker logs](/tmp/hivy-prod-session-forensics-20260620/asynq_logs_4h.jsonl)
- [Microsandbox control logs](/tmp/hivy-prod-session-forensics-20260620/msb_logs_4h.jsonl)
- [Runner 1 host evidence](/tmp/hivy-prod-session-forensics-20260620/runner1_forensics.txt)
- [Runner 1 always-on sandbox exec evidence](/tmp/hivy-prod-session-forensics-20260620/runner1_0l3_exec.json)
- [Session 7b event table](/tmp/hivy-prod-session-forensics-20260620/session_7b_events.tsv)

## Organization And Sessions

Only one organization was found:

- Org ID: `630567c0-663f-41ca-81a0-6c49ce5aea03`
- Name: `Frantz Kati's Workspace`
- Plan: `business-25000`

Latest user-visible web sessions:

| Session | Name | Agent | Result | Primary Issue |
| --- | --- | --- | --- | --- |
| `6887bdb9-b683-4f12-85c5-145abaf1f2a2` | `summary-of-pr-191` | Hivy | failed before runtime turn | Old always-on runtime rejected new tool config |
| `7b3367b3-d81d-431a-b4dc-1c2e1a2af105` | `codebase-locally-latest-commit` | Hakaree | completed, high cost | Large repeated context plus subagent follow-up turn reuse |

The raw third latest session in the DB was `deba8afc-c8a4-57c7-b4ff-4068787cd405`, a runtime-created subagent session linked to `7b3367b3-d81d-431a-b4dc-1c2e1a2af105`, not a user-visible web session.

## Handoff Documents

Assign these independently:

1. [Old Always-On Runtime Rejected New Config](./01-runtime-config-version-skew.md)
   - Session: `6887bdb9-b683-4f12-85c5-145abaf1f2a2`
   - Root cause: API pushed `builtin.file_search` to an older `v3.6.0` runtime whose schema did not support it.

2. [High LLM Spend And Subagent Turn Reuse](./03-llm-spend-subagent-turn-reuse.md)
   - Session: `7b3367b3-d81d-431a-b4dc-1c2e1a2af105`
   - Root causes: high-volume real model usage, no hard spend cap, large uncached prompts, and synthetic subagent follow-up work reused a completed `turn_id`.

3. [Sandbox Provisioning Latency](./04-sandbox-provisioning-latency.md)
   - Sandboxes: `3fokz7r7`, `0fa6dq5f`, `0l3rn8sg`
   - Root cause: first large developer sandbox paid cold Microsandbox/runtime startup; later same-image sandbox was hot. Always-on delay was not provisioning, but config rejection.

## Cross-Cutting Themes

- Runtime schema compatibility is not enforced before pushing config.
- Runtime/billing has no hard per-session or per-turn spend guardrail.
- Session cost attribution misses linked subagent costs when joining only on canonical `sessions.id`.
- Sandbox timing telemetry is too coarse to separate VM image startup, port registration, runtime health, config readiness, and repo clone phases.
