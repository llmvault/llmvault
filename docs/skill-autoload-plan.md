# Skill Auto-Load — Implementation Plan

**Status:** awaiting approval · **Date:** 2026-07-03 · **Owner:** QA-agent program (but the feature is agent-generic)

Agents and sub-agents can declare skills that the runtime loads into the session automatically — the runtime invokes `skill_view` itself before the first model call, so the model starts with the skill content already in context and the `.skills/` files already materialized. No model round trips, no "remember to load your skills" ambiguity.

## Motivation (measured)

From the QA flagship-run forensics (sessions `8ea2fb83…` deepseek, `09c03413…` mimo):
- Every `test-executor` spends 2–3 model round trips loading the `browser` skill + its commands reference (~10–20s per executor, every run).
- The coordinator spends 1–2 round trips loading the three QA skills.
- Total: ~30–60s per run of pure ritual with a known-in-advance outcome, plus the risk class of "agent forgot to load the skill and guessed syntax" (the deepseek CLI-flailing run cost 6+ minutes exactly this way).

## Design

### 1. Config surface

New optional field `auto_load_skills` on agents and sub-agents, everywhere agent config lives:

```jsonc
// entry forms: string shorthand loads the skill root; object form adds linked files
"auto_load_skills": [
  "qa-registry",
  { "name": "browser", "files": ["references/commands.md"] }
]
```

- **Catalog manifest** (`internal/agentcatalog/manifest.go`): on the top-level `Manifest` and on `SubAgentManifest`. Validated at load: every named skill must be resolvable for the agent (in `skill_filter` allow when set; from an attached/required plugin), every `files` entry must be a relative path with no traversal. Invalid manifest = sync error, same as other fields.
- **DB**: migration `000063_agent_auto_load_skills.sql` — `agents.auto_load_skills jsonb`, `agent_catalog.auto_load_skills jsonb` (default `'[]'`). Catalog install copies it, same flow as `model` / `default_reasoning_effort`. Sub-agent rows (`agents` with `type='subagent'`) use the same column.
- **Agents CRUD API**: accept/return `auto_load_skills` on create/update (validated), including in `sub_agents` payloads (`internal/handler/agents_crud.go`, `agents_update.go`, `agents_subagents.go`).

### 2. Compile (Go → AgentDefinition)

- `internal/agentruntime/compile.go`: `AgentDefinition` gains `auto_load_skills` (serialized list of `{name, files[]}`, string shorthand normalized to object at compile time).
- `internal/agentruntime/compile_subagents.go`: same field on each compiled sub-agent definition.
- Compile-time validation mirrors manifest validation; unknown skill = compile error logged, entry dropped (config push must not fail the whole session for one bad slug — log loudly instead).

### 3. Runtime (Rust) — the actual loading

- `crates/domain/src/agent_definition.rs`: `auto_load_skills: Vec<AutoLoadSkill>` where `AutoLoadSkill { name: String, files: Vec<String> }` (serde default empty — old configs unaffected).
- **Bootstrap hook** — on the FIRST turn of a session (main session in the runner/handler; subagent child sessions in `subagent_worker.rs` before goal injection):
  1. For each entry, invoke the hivy MCP `skill_view` tool through the existing MCP registry — the exact same code path a model-initiated call takes, so the `materialize` side effect (writing `.skills/<slug>/…`) happens identically. One call for the skill root, one per `files` entry (`skill_view {name, file_path}`).
  2. Inject a synthetic assistant-message-with-tool-calls + tool-result pair(s) into the transcript ahead of the first user message, exactly the shape the model would have produced (synthetic `tool_call_id`s). This is the context-faithful option: downstream turns see a normal history. (Alternative considered and rejected for v1: baking content into the system prompt — changes prompt shape, inflates every turn for rarely-used skills, drifts from load-on-demand semantics.)
  3. Record `auto_load_done` in session state so resumed/subsequent turns never re-inject; failures (skill missing at runtime) log + skip — the agent can still load manually.
- **Strict-schema providers** (deepseek `strict_tool_schema: true`): verify the synthetic pair passes provider validation in the model adapters; if any provider rejects synthetic assistant tool_calls, fall back per-profile to a single `user`-role "preloaded skill content" message (flagged in the injected text as system-provided). Implementation detail to settle in code review.

### 4. qa-engineer adoption (the reference user)

- **Coordinator** `global/agents/qa-engineer/agent.json`:
  `"auto_load_skills": ["qa-registry", "qa-execution", "qa-authoring"]` — NOT the browser skill (fallback-only, loaded manually per `browser_fallback`).
- **test-executor** sub-agent:
  `"auto_load_skills": [{ "name": "browser", "files": ["references/commands.md"] }]`.
- **Instruction updates** (coordinator + executor + the three QA skills): replace "load X before any work" steps with "X is preloaded in your context"; executor flow loses its two loading steps; `browser_fallback` keeps the manual `skill_view` instructions (fallback path is not auto-loaded).
- **Open question for the user**: MiMo's coordinator loaded `browser references/commands.md` at drafting time — a technical violation with a good rationale (drafting syntactically correct Commands). Option: add the commands reference (only the reference, not the whole skill) to the coordinator's auto-load so drafts are grammar-accurate without any violation. Default: per current instruction (not included).

### 5. Rider (approved): remove the progress nudge from subagent sessions entirely

The "You have called N tool calls without providing a user-facing summary" mechanism must be REMOVED completely for subagent sessions — not conditionally suppressed: subagent-scope sessions never track toward it and never receive it (measured cost: 2 forced round trips per executor per run). Main-session behavior unchanged.

## Conflict avoidance (other agents work in this repo)

- Before every edit, implementing agents run `git status`/`git diff` on target files and MUST NOT revert existing uncommitted changes (the whole QA program is uncommitted in this working tree).
- Files touched here: `internal/agentcatalog/{manifest,load,fields}.go`, one new migration (take the NEXT free number at write time — check `internal/migrations/sql/`, currently 000063), `internal/handler/agents_{crud,update,subagents,response}.go` + `agents_catalog_helpers.go`, `internal/agentruntime/{compile,compile_subagents}.go`, `internal/testdb/migrations.go` (bump latest version), `crates/domain/src/agent_definition.rs`, `crates/runtime/src/{handler,subagent_worker}.rs` or the runner bootstrap seam, `global/agents/qa-engineer/**`, `global/plugins/qa/skills/**` (wording). NOT touched: e2e tests, Makefile, sheets, env plumbing.
- Work splits into two non-overlapping agent tasks: (A) Go config plumbing + migration + manifest + qa-engineer manifest/wording; (B) Rust runtime bootstrap + nudge rider. A defines the JSON contract first (documented in this plan) so B codes against it without waiting.

## Test plan

1. Unit: manifest validation (good/bad slugs, traversal paths), compile output contains normalized entries, catalog install copies the column.
2. Rust: bootstrap test — definition with auto_load_skills → transcript contains synthetic pair before first user message; subagent worker path covered; resume does not duplicate; `cargo test` for touched crates.
3. Full check: `go build ./... && go vet ./... && go test ./internal/agentcatalog/... ./internal/handler/... (targeted)`; runtime image rebuild (arm64 target).
4. Acceptance: flagship `TestAgentSessionsQAAgentE2E` — with skill loading gone from the transcript, executor turn count should drop by 2–3 each and the coordinator by 1–2. **Per the user: acceptance runs on a NEW model (not deepseek, not mimo) as a model-independence check.**

## Rollout order

Go schema/plumbing (A) → Rust runtime (B) → runtime image rebuild → api/worker restart → qa-engineer manifest flip + instruction wording → flagship e2e on the new model.
