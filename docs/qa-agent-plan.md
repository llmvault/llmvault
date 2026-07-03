# QA Agent — Design Plan

Status: **PRODUCTIONIZED 2026-07-03** — catalog agent at `global/agents/qa-engineer/` (instructions in `prompts/`, sub-agents `test-executor`/`test-triage`), plugins `sheets`+`qa` required, `runtime.reasoning_effort: "low"` (new agent-level default, plumbed through catalog→agents→sessions), skills hardened with browser-CLI discipline from the latency forensics. Temporary e2e (`e2e/qa_agent_e2e_test.go`, `make test-agent-qa-e2e`) deleted per direction — testing goes through the flagship e2e suite. Earlier history below. Composes **existing** platform capabilities — channel-scoped sheets, channel-scoped env vars, the `browser` CLI, subagents, drive uploads, skills, triggers — into a proactive QA agent. v1 needs zero platform changes; §8 lists small enhancements, ranked.

Design philosophy (industry-validated, §4): **deterministic cached replay is the product, the LLM is the exception handler, assertions are the immune system, and every heal is a logged proposal.** Green runs cost almost nothing.

## 0. Verified ground truth

| Topic | Reality |
|---|---|
| **Sheets channel scoping** | Exists (migration `000060_sheets_channel_scope.sql`; MCP tools derive channel from `_hivy_session_id`, `PageInChannel` guards). The registry lives natively in `#qa`; the agent must operate from sessions in that channel. |
| **Env vars (CHANGED 2026-07-03)** | **Channel-scoped** now (commit `bac14cc67`; plan `docs/channel-env-vars-plan.md`). Old org-wide `HIVY_ORG_*` store dropped. Per-channel vars via `/v1/channels/{id}/environment-variables` (write-only, AES-GCM), pushed into the runtime as `__ENV__<NAME>`. **The skill prescribes NO variable names**: users define whatever they like; the agent discovers what's available and follows the user's instructions. ⚠️ **BLOCKER found by verification (must fix before QA v1)**: channel vars never reach bash children. They live only in the config-pushed `runtime_env` map as `__ENV__NAME`; the bash tool overlays onto the inherited OS env only its `env_passthrough` whitelist, and the compiled whitelist is always the 7 fixed platform names (`ensureBashEnvPassthrough`, `internal/agentruntime/compile_tool_config.go:112` — so the strip-all empty-passthrough branch in `bash.rs:107-113` is dead code in practice). Platform `HIVY_*` vars are fine (inherited from the container OS env — that's how the drive skill works). **Fix (recommended, small)**: in `bash.rs`'s whitelist branch, additionally overlay every `__ENV__*` key from `runtime_env` as its stripped clean name (user names can't collide with `HIVY_*` — validation forbids the prefix) + test. Alternative: Go-side, append the channel's var names to every compiled bash `env_passthrough` (the `__ENV__` fallback lookup at `bash.rs:119` already resolves them). |
| **Registry shape** | One sheet, three pages — NOT sheet-per-run: relation fields pin a fixed `options.target_page_id` (`internal/sheets/fields.go`), so dynamic sheets can't be relation-linked. |
| **Subagent MCP** | One process-global MCP registry gated only by `mcp_tool_filter` (`crates/runtime/src/main.rs:139-146`); compiled `McpServers: []` is never consulted. **Empty filter = subagent inherits ALL parent MCP tools.** Sheets-scoped subagent = config-only. ⚠️ Security finding beyond QA: unset filters leak everything. |
| **Tool parallelism** | Non-subagent tools run sequentially (`runner.rs` ~826-861); only contiguous `subagent_task` calls `join_all`. Moot for writes: `rows_write` batches ≤100 rows/call. Fan-out: all calls in ONE turn, 15-min cap, worker batch 10, one nesting level, freeform-text results. |
| **Browser** | Chromium + `browser` CLI in every sandbox (`Dockerfile.runtime:139-153`), Bash-driven. Skill: `global/plugins/runtime/skills/browser/SKILL.md` (snapshots with `@eN` refs, auth vault, `state save/load`, `record`, `batch --bail`). |
| **Concurrency (EXPERIMENT, agent-browser 0.31.1, macOS)** | Each `--session` = own headless Chrome instance/profile. Cookie isolation: verified. `state save`→`load` handoff: verified. 3 concurrent personas × 15 ops: 0 failures. 10 simultaneous sessions: fine. **Limit is memory** (~0.3–1 GB RSS/instance) → ~3-5 concurrent on small sandboxes. Container re-check pending. |
| **Artifacts** | Fully built: `PUT $HIVY_DRIVE_UPLOAD_URL/<path>` (runtime-secret bearer, no size cap) → `{key, asset_url}`; `pub/e/{agentID}/…` keys valid in sheet **attachment cells** (grid thumbnails); `asset_url` auto-renders in chat. Cautions: `agent_assets.SandboxID` cascade; ≤10 keys/cell. |
| **Sheets surface** | 8 MCP tools; backlinks efficient (`contains` → GIN `@>`); no join-through/formulas; `rows_query` returns `{id, data}` only (§8); same-row concurrent updates lose writes (§8). |
| **Screenshots readable by subagents** | Builtin `read` on an image → control-plane describe API; in subagent defaults; 5 MiB cap. Failure triage only. |

## 1. Architecture

```
        #qa channel (DefaultAgentID = QA agent; registry sheet + env vars live here)
                          │  chat / cron / deploy webhook
                          ▼
               ┌─────────────────────────┐
               │     QA Coordinator      │ sheets MCP · memory · cron ·
               │                         │ persona logins + state save
               └───────────┬─────────────┘
          one turn, N subagent_task calls (parallel)
        ┌─────────────┬────┴────────────┐
        ▼             ▼                 ▼
   ┌─────────┐   ┌─────────┐      ┌─────────┐
   │executor │   │executor │      │executor │    replay (no reasoning) →
   │admin ×3 │   │editor ×2│      │viewer ×3│    heal only on step failure
   └────┬────┘   └────┬────┘      └────┬────┘
        └──── fenced-JSON results ─────┘
                          │
          failures only → triage subagent (classify + verify heals;
          screenshots LLM-read only here)
                          │
                          ▼
      coordinator: batched rows_write, heal-review report in channel
```

**Roles:** **Coordinator** (plans, authors, persona auth bootstrap, fan-out, single registry writer, reporting, quarantine). **`test-executor`** (bash+read+write; deny-all MCP filter; replay → heal ladder; uploads artifacts; JSON results). **`test-triage`** (read tools; deny-all filter; failures only: classify regression/ui-change/environment/flake, verify heals against intent). **`results-writer`** (optional, sheets-allow-listed; shelf until needed).

## 2. The registry: one sheet, three pages

Sheet **`QA Test Registry`** in `#qa` (bootstrap via `sheet_create` if absent).

**Page `Test Cases`** (`display_field_id` = Name)
| Field | Type | Notes |
|---|---|---|
| Name | text | |
| Suite | select | |
| Persona | select | admin / editor / viewer — which saved auth state to load |
| Priority | select | P0/P1/P2 |
| Status | select | draft → active → quarantined / deprecated |
| Preconditions | long_text | |
| Steps | long_text → **array** when §8.1 ships | NL intents only, one per step — human-readable |
| Commands | long_text | JSON array of argv step objects — the replay cache (format §3) |
| Expected | long_text → array | explicit observable assertions — never healed |
| Last Result | select | passed / flaky-pass / failed / blocked |
| Last Run At | date | |
| Heal Pending Review | checkbox | grid doubles as the heal-review queue |
| Consecutive Passes | number | auto-unquarantine at 10 |

**Page `Test Runs`**: Run (text, display), Started/Finished (date), Trigger (select), Target (url), Build (text), Status (select: running/passed/failed/partial), Passed/Failed/Flaky/Skipped (number), Summary (long_text incl. heal digest).

**Page `Test Results`**: Case/Run (relations), Status (select: passed/flaky-pass/failed/blocked), Failure Class (select: regression/ui-change/environment/flake/none), Duration s (number), Transcript (long_text → array), Failure (long_text), Heals (long_text — structured events: step, old cmd, new cmd, confidence, why), Screenshots (attachment — failure states + healed steps only), Artifacts Index (url).

**Query recipes:** run results / case history = `contains` on the relation field (GIN-indexed); runnable = `and[Status eq active, Suite eq X]`.

## 3. Execution policy — the ladder (simplified: replay → heal → fail)

**Test-case data — two fields, no prose parsing.** `Steps` holds the NL intents (human-readable, authoritative). `Commands` holds the replay cache as **structured JSON — argv arrays, schema-validated, nothing is ever parsed out of prose**:

```json
[
  { "intent": "Open the login page",
    "command": ["open", "${STAGING_URL}/login"] },
  { "intent": "Fill the email field with the staging account email",
    "command": ["find", "label", "Email", "fill", "${STAGING_EMAIL}"] },
  { "intent": "Submit the login form",
    "command": ["find", "role", "button", "Sign in", "click"] },
  { "intent": "URL is the workspace shell",
    "assert": true,
    "command": ["get", "url"], "expect": { "op": "contains", "value": "/w" } },
  { "intent": "Workspace name visible in the sidebar",
    "assert": true,
    "command": ["is", "visible", "[data-testid=workspace-name]"],
    "expect": { "op": "equals", "value": "true" } }
]
```

`${NAME}` placeholders keep secrets out of the sheet; the names are **whatever the user configured** (here `STAGING_*` is the user's choice, not ours). Replay substitutes from the environment and treats an unset variable as a setup error — it never guesses. `"no_heal": true` pins a step; `"assert": true` marks verification steps.

**Tier 0 — deterministic replay (the default).** The executor runs `replay.mjs` (Node; executes each argv via `execFile` — no shell, no quoting, no eval; per-step expect checks; bail on first failure; JSON verdict + failure screenshot). A green case = **one bash tool call, zero snapshots/screenshots/judging** — where ~95%+ of steady-state executions land (Momentic ~99% cache-hit rates; Stagehand act-caching). The CLI's built-in auto-waiting covers ordinary timing; there is no separate recovery tier.

**Tier 1 — heal (LLM engages).** On a failed step the executor thinks: `snapshot -i`, re-resolve the step's **NL intent** against the accessibility tree (screenshot only if the tree is ambiguous). Rules:
- New target must semantically match the stored intent — not merely resemble the old element.
- **Assertions are never healed.** A failed ASSERT = failure, full stop (this is what stops healing over a real regression — the wrong-button heal fails the next assertion).
- Heal budget: **max 3 per case run**; exceeded → fail.
- Every heal recorded as a structured event (old cmd, new cmd, confidence, reasoning, screenshot) in Heals.
- After a heal, resume replay from the healed step.

**Tier 2 — fail immediately, no healing**, when: 5xx / console exception / crash / CAPTCHA; an assertion failed; the intent itself cannot be completed; or the step is pinned (`"no_heal": true`).

**Heal persistence — loud, never silent:** the Commands cache is patched **only if the whole case passes after the heal** (a heal followed by downstream failure was wrong); patching sets `Heal Pending Review`; the channel report shows old → new + confidence + screenshot; human ack clears, rejection reverts and marks broken.

**Retry & flake:** one automatic retry in a fresh session; pass-on-retry = `flaky-pass` (a flake event, never a clean pass). Inconsistent results = flaky → quarantine + investigate; consistent failure = regression → report, never quarantine. Quarantined cases keep running without failing the run; release after 10 consecutive passes.

**Triage (failures only):** `test-triage` gets result JSON + transcript + screenshots → `{failure_class, reasoning, heal_verdicts[]}`; the only place screenshots are LLM-analyzed. Coordinator diffs failing vs last-passing transcript for the report.

## 3b. Run protocol (coordinator)

1. Intake → query registry (bootstrap if missing). No case? **Authoring mode**: announce, explore live (`snapshot -i` for stable locators), codify NL steps + cached commands + explicit assertions per meaningful outcome, insert as `draft`, run through the real pipeline, promote on green.
2. `rows_write` run row. Persona bootstrap: login per persona, `state save /workspace/qa/state-<persona>.json`.
3. Fan out executors — one turn, 1–3 cases each, ≤3-5 concurrent (memory); self-contained goals (steps, expected, persona state file, `--session case-<id>`, artifact dir, JSON contract).
4. Collect fenced JSON (unparseable → blocked, one serial retry). Failures → fan out `test-triage`.
5. Record: batched `rows_write`; run totals; case Last Result / Last Run At / Consecutive Passes; apply passing heals + review flags.
6. Report in channel: pass/fail/flaky table, failures with triage class + transcript diff + inline screenshots, heal-review section, authored/promoted cases.
7. `retain_memory` (org): environment quirks, flaky patterns, seed data.

Secrets only via channel env vars, referenced as `${NAME}` placeholders (user-chosen names) — values never appear in steps, transcripts, goals, or cells.

## 4. Research grounding (2024–26 survey)

- **Heal/fail line**: universal — element *location* may heal, the *verification layer* may not; intent-impossible = fail (testRigor: "succeed when and only when the intended action is possible from the end-user's perspective"). Nobody heals silently (Octomind zero-silent-commits, mabl insights, Testim flagged revisions).
- **Cost**: LLM-native leaders (Momentic, Stagehand) cache resolved actions and replay with zero LLM calls until a miss; code-generating tools (QA Wolf, Octomind, Checkly) have zero steady-state LLM cost. Vision is fallback (a11y tree ≈ 10× cheaper).
- **Judge**: nobody LLM-judges green runs. Deterministic assertions decide; LLM appears only as failure triage and heal verifier.
- **Flake**: 1–3 retries max industry-wide; metric-based quarantine with auto-release; "a suite known to lie green destroys trust."

## 5. Agent + subagent definitions

**Coordinator**: plugins `sheets` + `runtime` + `qa-toolkit`; tools baseline + memory + `cron` + `create_http_trigger`; sandbox tool `chrome`; home = `#qa` (`default_agent_id`). Credentials/target URLs come from **user-defined channel env vars + the user's instructions** — the skill teaches discovery (`env` minus `HIVY_*`/shell vars) and `${NAME}` referencing, and prescribes no names.

**Subagents** (explicit `McpToolFilter` mandatory — empty filter leaks everything):
| | builtin tools | MCP filter |
|---|---|---|
| `test-executor` | bash + read + write (explicit) | deny-all |
| `test-triage` | read (+ grep/glob) | deny-all |
| `results-writer` (shelf) | read | `Allow: [rows_write, rows_query, sheet_describe]` |

## 6. Skills — WRITTEN (2026-07-03) as global plugin `global/plugins/qa/`

- **`qa-registry`**: schema + exact bootstrap payload (relations added post-create via `sheet_manage`), field semantics (Commands = sanctioned JSON-in-cell exception), query recipes, write rules.
- **`qa-execution`**: Commands schema; replay verdict handling; the failure ladder; heal rules + budget; sessions/personas/artifacts; fan-out limits; executor + triage goal templates with fenced-JSON contracts; retry/flake/quarantine; reporting. Ships **`scripts/replay.mjs`** and **`scripts/upload-run.mjs`** (both tested; smoke-tested again from the plugin path).
- **`qa-authoring`**: explore → stable locators (testid > label > role > text; never `@eN` refs) → assertions-first codify → validate via the real pipeline → promote; authored-edit vs heal distinction.

Agent + subagent definitions live in the catalog: `global/agents/qa-engineer/`. Executor/triage goals inline everything except the `browser` skill, which executors load themselves (their skill_filter allows it; the read-only MCP floor grants `skill_view`).

## 7. Triggers

Chat in `#qa`; nightly regression via `cron`; deploy smoke via `create_http_trigger` from CI.

## 8. Platform enhancements — ranked (v1 needs none)

1. **`array` field type for sheets** (small backend + small-medium web) — free-form ordered string lists. Backend: `fieldTypeSpecs` entry (`array: true, cast: castText`, ops contains/not_contains/is_empty/is_not_empty), a `coerceArray` (= `coerceMultiSelect` minus choice validation, `internal/sheets/coerce.go:129`), limits, SKILL.md + contract tests. Web: `case "array"` in `cells.ts`/`cell-editor.tsx` (reuse multi-select bubbles; editor = one item per line). Payoff: Steps/Expected/Transcript become human-readable ordered lists in the grid instead of markdown walls. QA uses long_text until this ships, then migrates.
2. **Timestamps in `rows_query` results** (small, ~2 lines in `rowObjects`, `internal/sheets/mcptools.go:241-249`).
3. **`rows_get` by row ID** (small) — filter AST is `fld_`-only today.
4. **`FOR UPDATE` on same-row updates** (small, `internal/sheets/rows.go:117-204`) — lost-update race.
5. **Structured `subagent_task` output** (small prompt-convention field; medium validated schema) — small now, medium if parse failures show up.
6. **Subagent MCP defense-in-depth** (medium, Rust) — security fix beyond QA.
7. **Docs**: backlink recipe + agent-attachment keys in sheets SKILL.md (tiny).
8. Skip: upsert (query-then-branch works), filter-through-relations, rollups, unique cell constraints, parallel MCP dispatch.

**Decision needed**: artifact durability — `agent_assets.SandboxID` cascade; accept, or detach QA uploads (null `SandboxID`).

## 9. Verification + next steps

Done locally: session process model, cookie isolation, state handoff, 3-way parallel (45/45), 10 sessions; `replay.mjs`/`upload-run.mjs` written + tested (pass/fail/pinned/schema-violation/missing-env/auth-failure paths); `record` produces a well-formed WebM at the requested path; runtime image Node is `lts` (fetch available); code-verified that platform `HIVY_*` vars (incl. drive upload) reach bash via OS-env inheritance.

**Blocker FIXED (2026-07-03, uncommitted)**: channel env vars now reach bash children — `overlay_user_env` in `bash.rs` overlays all `__ENV__*` keys as clean names in the whitelist branch; test `explicit_passthrough_still_overlays_user_env_prefix` added; all 10 bash tests pass.

**E2E ACCEPTANCE PASSED (2026-07-03)** — `TestAgentSessionsQAAgentE2E` green in 46s on `gemini-3.1-flash-lite` against the full `make up` stack: registry sheet bootstrapped with all 3 pages, Login Test case authored from exploration, run + passing result rows recorded, marker emitted. **Channel env vars proven end-to-end**: the fixture only accepts the exact credentials, which existed solely as encrypted channel vars → config push → `__ENV__` → bash overlay → browser fill. Fixes landed on the way: migration 60/61 applied (60 assumes empty sheets table — dev e2e leftovers had to be cleared), api restart needed post-migration (Postgres cached-plan errors), `make sandbox-runtime-image` defaults to x86_64 on arm64 hosts (use `sandbox-runtime-image-arm64` + retag; Makefile papercut worth fixing).

Observed cheap-model deviations (prompt/model-quality tuning, not design failures):
- It **echoed `${LOGIN_EMAIL}`/`${LOGIN_PASSWORD}` values via bash `echo`** into the transcript — instructions forbid printing secrets but flash-lite ignored it. Harden instructions ("never echo/printenv secret values") and consider a runtime-side redaction of `__ENV__` values in tool results (platform enhancement).
- It did everything **inline in the main loop** — no `test-executor` fan-out, no `replay.mjs` (authored, browsed with `@eN` refs, and wrote the sheet itself). Acceptable for a single-case authoring run; multi-case runs and replay discipline need a stronger model or firmer skill wording.

Remaining (verify on a stronger model / real target):
1. Executor fan-out + `replay.mjs` path actually exercised (give it an existing case to re-run).
2. Concurrency + memory ceilings inside the runtime container.
3. `record` → `upload-run.mjs` → `asset_url` round-trip; attachment keys thumbnailing in the grid.

Then: coordinator Instructions, three skills, executor/triage instructions; acceptance = login-page scenario end-to-end (author → replay → heal → report).
