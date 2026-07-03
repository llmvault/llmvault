# QA Agent — Design Plan

Status: exploration + research + concurrency experiment complete (2026-07-03). Composes **existing** platform capabilities — channel-scoped sheets, the `browser` CLI, subagents, drive uploads, skills, triggers — into a proactive QA agent. v1 needs zero platform changes; §8 lists small enhancements worth building, ranked.

Design philosophy (from industry research, §4): **deterministic cached replay is the product, the LLM is the exception handler, assertions are the immune system, and every heal is a logged proposal.** Green runs should cost almost nothing.

## 0. Verified ground truth

| Topic | Reality |
|---|---|
| **Sheets channel scoping** | Exists (migration `000060_sheets_channel_scope.sql`; every sheets MCP tool derives channel from `_hivy_session_id`, `PageInChannel` guards). The QA registry lives natively in the `#qa` channel. The agent must operate from sessions in that channel. |
| **Registry shape** | One sheet, three pages — NOT sheet-per-run: relation fields pin a fixed `options.target_page_id` (`internal/sheets/fields.go:180-194`), so dynamic sheets can't be relation-linked. |
| **Subagent MCP** | Runtime keeps ONE process-global MCP registry gated only by each definition's `mcp_tool_filter` (`crates/runtime/src/main.rs:139-146`, `runner.rs` `build_all_tools`); the compiled `McpServers: []` is never consulted. **Empty filter = subagent inherits ALL parent MCP tools.** Sheets-scoped subagent = config-only. ⚠️ Security finding to raise beyond QA: unset filters currently leak everything. |
| **Tool parallelism** | Non-subagent tool calls run strictly sequentially (`runner.rs` ~826-861); only contiguous `subagent_task` calls `join_all`. Moot for writes: `rows_write` batches ≤100 rows/call. Subagent fan-out: all calls in ONE turn, 15-min cap each, worker batch 10, one nesting level, freeform-text results. |
| **Browser** | Chromium + `browser` CLI in every sandbox (`Dockerfile.runtime:139-153`), Bash-driven. First-class skill: `global/plugins/runtime/skills/browser/SKILL.md` (snapshots with `@eN` refs, auth vault, `state save/load`, `record`, network mocking, `batch --bail`). |
| **Concurrency (EXPERIMENT, agent-browser 0.31.1, macOS)** | Each `--session` = its **own headless Chrome instance + profile** (not shared tabs). Cookie isolation between sessions: verified. `state save` → `state load` into another session carries auth: verified. 3 concurrent persona sessions × 15 interleaved ops: 0 failures, 0 cross-talk. 10 simultaneous sessions launch fine. **Limit is memory** (~hundreds of MB–1 GB RSS per instance) → ~3-5 concurrent sessions on small sandboxes. Re-verify once inside the runtime container (microVM memory limits). |
| **Artifacts** | Fully built: sandbox `PUT $HIVY_DRIVE_UPLOAD_URL/<path>` (runtime-secret bearer, no size cap) → `{key, asset_url}`; `pub/e/{agentID}/…` keys are valid in sheet **attachment cells** (thumbnails in grid; documented in sheets SKILL.md:207,232-241); `asset_url` auto-renders in chat. Caution: `agent_assets.SandboxID` cascades on delete. ≤10 keys/cell. |
| **Sheets surface** | 8 MCP tools; backlinks already efficient (`contains` on relation → GIN `@>`); no join-through/formulas (agent computes); `rows_query` returns `{id, data}` only — no timestamps, no fetch-by-id (§8). Same-row concurrent updates lose writes (§8). |
| **Screenshots readable by subagents** | Builtin `read` on an image calls the control-plane describe API (`crates/tools/src/read.rs:131-140`); in subagent defaults; 5 MiB cap. Used sparingly (failure triage only — §3). |

## 1. Architecture

```
        #qa channel (DefaultAgentID = QA agent; registry sheet lives here)
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
   │executor │   │executor │      │executor │     Tier 0: batch replay (no
   │admin ×3 │   │editor ×2│      │viewer ×3│     reasoning) → Tier 1/2 only
   └────┬────┘   └────┬────┘      └────┬────┘     on step failure
        └──── fenced-JSON results ─────┘
                          │
                          ▼
      failures only → triage subagent (classify: regression /
      UI-change-healed / environment / flake; verify heals;
      read screenshots only here)
                          │
                          ▼
      coordinator: batched rows_write, heal-review report in channel
```

**Roles:**
- **Coordinator** — plans, authors cases, bootstraps persona auth states, fans out, writes the registry (single writer), reports, manages quarantine.
- **`test-executor`** (subagent; bash+read+write; deny-all MCP filter) — runs a batch of 1–3 cases in an isolated `--session`; Tier-0 replay first; heals within budget; uploads artifacts; returns structured results.
- **`test-triage`** (subagent; read tools; deny-all MCP filter) — **failures only** (this replaces the every-run judge, which would be needlessly expensive and is not industry practice): classifies each failure (app regression / UI change / environment / flake), verifies any heal the executor performed actually serves the original intent, reads screenshots (image-description API) only here.
- **`results-writer`** (optional, sheets-allow-listed, config-only) — only if runs outgrow the coordinator's batched writes. Not deployed v1.

## 2. The registry: one sheet, three pages

Sheet **`QA Test Registry`** in `#qa` (bootstrap via `sheet_create` if absent).

**Page `Test Cases`** (`display_field_id` = Name)
| Field | Type | Notes |
|---|---|---|
| Name | text | |
| Suite | select | login, checkout, … |
| Persona | select | admin / editor / viewer — which saved auth state to load |
| Priority | select | P0/P1/P2 |
| Status | select | draft → active → quarantined / deprecated |
| Preconditions | long_text | |
| Steps | long_text | **NL intent (source of truth) + cached command per step** (format §3) |
| Expected | long_text | explicit, observable assertions — the immune system; never healed |
| Last Result | select | passed / flaky-pass / failed / blocked |
| Last Run At | date | |
| Heal Pending Review | checkbox | a heal was applied and awaits human ack (§3 heal policy) |
| Consecutive Passes | number | for auto-unquarantine (10 = release, Testim convention) |

**Page `Test Runs`**: Run (text, display), Started/Finished (date), Trigger (select: chat/cron/deploy-hook/manual), Target (url), Build (text), Status (select: running/passed/failed/partial), Passed/Failed/Flaky/Skipped (number), Summary (long_text — includes heal digest).

**Page `Test Results`** — one row per (case × run)
| Field | Type | Notes |
|---|---|---|
| Case / Run | relation | |
| Status | select | passed / flaky-pass / failed / blocked |
| Failure Class | select | regression / ui-change / environment / flake / none — triage verdict |
| Duration (s) | number | |
| Transcript | long_text | steps executed |
| Failure | long_text | assertion diff, console errors, triage reasoning |
| Heals | long_text | structured heal events: step, old cmd, new cmd, confidence, why |
| Screenshots | attachment | drive keys; failure states + healed steps only |
| Artifacts Index | url | run-folder `asset_url` for videos / >10 files |

**Query recipes:** results of a run / history of a case = `contains` filter on the relation field (GIN-indexed); runnable cases = `and[Status eq active, Suite eq X]`.

## 3. Execution policy — the tiered ladder

**Step format** (in `Steps`): NL intent is authoritative; the cached command is a replay cache, one per step:

```
1. Open the login page
   → open $QA_BASE_URL/login
2. Fill the email field with the QA admin email
   → find label "Email" fill $HIVY_ORG_QA_EMAIL
3. Submit
   → find role button "Sign in" click
4. ASSERT url contains /w
   → get url   [expect: contains "/w"]
5. ASSERT the workspace name is visible in the sidebar
   → is visible [data-testid=workspace-name]   [expect: true]
```

**Tier 0 — deterministic replay (no reasoning, the default).** The executor materializes the cached commands into one `browser batch --bail` call (assertions become `get`/`is` commands checked by the wrapper script from the qa-execution skill). A green case = **1–2 bash tool calls, zero snapshots, zero screenshots, zero judge** — this is where ~95%+ of steady-state executions should land (Momentic reports ~99% cache-hit step rates; Stagehand's act-caching is the same pattern).

**Tier 1 — cheap non-LLM recovery.** On a failed step: bounded wait/reload (timing causes ~30% of failures per QA Wolf's taxonomy), dismiss known blockers (cookie banner, modal), retry the cached command once. Still no reasoning beyond orchestrating bash.

**Tier 2 — heal (LLM engages).** Only now does the executor think: `snapshot -i`, re-resolve the step's **NL intent** against the accessibility tree (screenshot only if the tree is ambiguous). Heal rules:
- The new target must semantically match the stored intent — not merely resemble the old element.
- **Assertions are never healed.** A failed ASSERT is a failure, full stop (mabl/Applitools rule — this is what stops "healing over a real regression": the wrong-button heal fails the next assertion).
- Heal budget: **max 3 heals per case run** (Momentic's number); budget exceeded → fail.
- Every heal is recorded as a structured event (old cmd, new cmd, confidence, reasoning, screenshot) in the result's Heals field.

**Tier 3 — fail immediately, no healing**, when: 5xx / console exception / crash / CAPTCHA (Momentic's non-recoverable list); an assertion failed; the intent itself cannot be completed ("the Delete button is genuinely gone"); or the step is human-pinned (`[no-heal]` annotation).

**Heal persistence — loud, never silent (Octomind's "Zero Silent Commits"):**
- The Steps cache is patched **only if the whole case run passes after the heal** (mabl's rule — a heal followed by downstream failure was a wrong heal).
- Patching sets `Heal Pending Review` on the case; the channel report includes a heal-review section (old → new, confidence, screenshot). Human ack clears the flag; rejection reverts the cache and marks the case broken. The registry grid doubles as the review queue.

**Retry & flake policy:** one automatic retry of a failed case in a fresh session; pass-on-retry = `flaky-pass`, recorded as a flake event, never a clean pass (Testim's definition). Distinguish per mabl's two axes: inconsistent results = flaky → quarantine + investigate; consistent failure = broken → report as regression, never quarantine. Quarantined cases keep running but don't fail the run; auto-release after 10 consecutive passes.

**Triage (failures only):** the `test-triage` subagent gets the failed case's result JSON + transcript + screenshots and returns `{failure_class, reasoning, heal_verdicts[]}` — classifying regression vs ui-change vs environment vs flake, and double-checking each heal against the original intent (the "adjoint check"). This is the only place screenshots are LLM-analyzed. Coordinator diffs the failing transcript against the case's last passing one for the report ("step 3 previously landed on /w, now /login?error=…").

## 3b. Run protocol (coordinator)

1. Intake → query registry (bootstrap if missing). No case? **Authoring mode**: announce, explore the flow live (`snapshot -i` to learn stable locators), codify NL steps + cached commands + **explicit assertions for every meaningful outcome** (an agentic run that merely "finished" is not a test — Momentic), insert as `draft`, run once through the real pipeline, promote to `active` on green.
2. `rows_write` run row (`running`). Persona auth bootstrap: log in once per persona needed, `state save /workspace/qa/state-<persona>.json` (or auth vault).
3. Fan out executors — all `subagent_task` calls in one turn; 1–3 cases each; ≤3-5 concurrent (session memory, per experiment); goals are self-contained (steps, expected, persona state file, `--session case-<id>`, artifact dir, JSON contract).
4. Collect fenced-JSON results (unparseable → blocked, one serial retry). Failures → fan out `test-triage` calls.
5. Record: batched `rows_write` results; run totals; case Last Result / Last Run At / Consecutive Passes; apply passing heals + set review flags; upload nothing itself (executors already ran `upload-run.sh`).
6. Report in channel: pass/fail/flaky table, failures with triage class + transcript diff + inline screenshots, **heal-review section**, authored/promoted cases.
7. `retain_memory` (org scope): environment quirks, flaky patterns, seed-data facts.

Secrets only as env vars (`HIVY_ORG_QA_*`) — never in steps, transcripts, goals, or cells.

## 4. Research grounding (2024–26 industry survey)

Full survey in the research transcript; load-bearing conclusions:
- **Heal/fail line**: universal convergence — element *location* may heal, the *verification layer* may not; intent-impossible = fail (testRigor: "succeed when and only when the intended action is possible from the end-user's perspective"). No serious tool heals silently; all keep review trails (Octomind zero-silent-commits, mabl insights, Testim AI-flagged revisions).
- **Cost**: the LLM-native leaders (Momentic, Stagehand) cache resolved actions and replay with **zero LLM calls** until a locator misses; code-generating tools (QA Wolf, Octomind, Checkly) have zero steady-state LLM cost by construction. Vision is a fallback, not a default (a11y tree ≈ 10× cheaper than screenshots).
- **Judge**: nobody runs a blanket LLM judge per green run. Deterministic assertions decide pass/fail; LLM appears only as failure-triage/root-cause and heal-verifier. LLM-judge research shows inconsistency and bias; capped scoped "AI-check" assertions exist but are opt-in and rare.
- **Flake**: retries are 1–3 max industry-wide; quarantine on metrics with auto-release; "a suite known to lie green destroys trust in the whole system."

## 5. Agent + subagent definitions

**Coordinator**: plugins `sheets` + `runtime` (browser/drive skills) + `qa-toolkit`; tools baseline + memory + `cron` + `create_http_trigger`; sandbox tool `chrome`; org env vars `QA_BASE_URL`, per-persona `QA_<P>_EMAIL/PASSWORD`; home = `#qa` channel (`default_agent_id`).

**Subagents** (all with explicit `McpToolFilter` — empty filter leaks everything, §0):
| | builtin tools | MCP filter | notes |
|---|---|---|---|
| `test-executor` | bash + read + write (explicit; defaults are read-only) | deny-all | tiered ladder; JSON contract; `browser close` on exit |
| `test-triage` | read (+ grep/glob) | deny-all | failures only; adversarial stance |
| `results-writer` | read | `Allow: [rows_write, rows_query, sheet_describe]` | shelf until needed |

## 6. Skills (`qa-toolkit` org plugin)

- **`qa-registry`**: schema, bootstrap payload, query recipes (incl. backlink `contains`), write batching, "never update the same row concurrently".
- **`qa-execution`**: the tiered ladder as executor playbook; **`scripts/replay.sh`** (turns a Steps cell into a `browser batch --bail` run with assertion checks, emits step-level exit info) and **`scripts/upload-run.sh`** (walks artifact dir, PUTs to `$HIVY_DRIVE_UPLOAD_URL`, prints key+asset_url pairs); goal templates; JSON contracts; heal policy + budget; retry/flake/quarantine rules.
- **`qa-authoring`**: explore → codify (NL + cached command + assertions per outcome), locator-stability guidance (prefer roles/labels/testids), persona conventions.

Executor/triage goals inline everything (they can't `skill_view` under deny-all filters).

## 7. Triggers

Chat in `#qa`; nightly regression via `cron` (self-scheduled); deploy smoke via `create_http_trigger` from CI.

## 8. Platform enhancements — ranked (v1 needs none)

1. **Timestamps in `rows_query` results** (small, ~2 lines in `rowObjects`, `internal/sheets/mcptools.go:241-249` + SKILL/contract) — agent can't currently see when a result was written.
2. **`rows_get` by row ID** (small) — no way to fetch a row by UUID today (filter AST is `fld_`-only).
3. **`FOR UPDATE` on same-row updates** (small, `internal/sheets/rows.go:117-204`) — concurrent same-row updates lose writes.
4. **Structured `subagent_task` output** (small: prompt-convention spec field in `compile_subagents.go`; medium: validated `output_schema` with retry through domain/storage/worker) — do small now, medium if fenced-JSON parse failures show up.
5. **Subagent MCP defense-in-depth** (medium, Rust) — enforce declared `mcp_servers`/require explicit filter. Security fix beyond QA.
6. **Docs**: backlink recipe + agent-attachment keys in sheets SKILL.md (tiny).
7. **Upsert on `rows_write`** (medium) — idempotent re-runs; query-then-branch meanwhile.
8. Skip: filter-through-relations, rollup/computed fields, unique cell constraints, parallel MCP dispatch in runner.

**Decision needed**: artifact durability — `agent_assets.SandboxID` cascade means QA artifacts die with sandbox cleanup; either accept, or small change to detach QA uploads (null `SandboxID`).

## 9. Remaining verification + next steps

Done locally (agent-browser 0.31.1, macOS): session-per-Chrome process model, cookie isolation, state save/load handoff, 3-way parallel operation (45/45 ops), 10 simultaneous sessions. Remaining:
1. Same checks inside the actual runtime container (microVM memory ceilings → confirm safe concurrency; Linux headless footprint).
2. `record start/stop` output → `upload-run.sh` → working `asset_url` round-trip.
3. `HIVY_ORG_*` visibility inside `subagent_task` child sessions (same process — should hold).
4. Attachment key written via `rows_write` thumbnails in the grid.

Then: write coordinator Instructions, the three skills + two scripts, executor/triage instructions; acceptance = the login-page scenario end-to-end (author → replay → heal → report).
