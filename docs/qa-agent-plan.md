# QA Agent — Design Plan

Status: exploration complete (2026-07-03). This plan composes **existing** platform capabilities — sheets, `agent-browser`, subagents, skills, triggers — into a proactive QA agent. No new UI. A short list of optional platform changes is at the end, ranked.

## 0. Verified ground truth (what the code actually supports)

Facts this design is built on, with the three corrections to the original sketch called out:

| Idea in the sketch | Reality in code | Design consequence |
|---|---|---|
| "Sheets are assigned per channel" | Sheets are **org-scoped**. No `channel_id` anywhere in `000055_sheets.sql` / `internal/model/sheet.go`; no channel param in the 8 MCP tools. | The QA channel is the agent's conversational + trigger home, not a data boundary. Scope tests with a naming convention + `Suite` select field (§2). Optional `channel_id` column later (§7). |
| "New sheet per test run, linked back to the parent sheet" | Relation fields pin a **fixed** `options.target_page_id` at field-creation time (`internal/sheets/fields.go:180-194`). A relation cannot point at a dynamically created sheet/page. | **One registry sheet, three pages.** Runs are rows in a `Test Runs` page, not new sheets. Relations then work: results → cases, results → runs. |
| "Subagents use the same browser session" | Subagents run **in-process, same sandbox, shared `/workspace`** (`sandboxes/runtime/crates/runtime/src/subagent_worker.rs`). Browser is the `agent-browser` CLI over Bash (Chromium baked into `Dockerfile.runtime:139-153`), with `auth save/login`, `state save/load`, `--session`, `--state` flags. | Coordinator logs in **once**, `state save`s to a workspace file; each executor loads that state into its **own isolated browser session** (`--session case-<id>`). Shared login, isolated tabs — no clobbering. |
| (implicit) subagents write results to sheets | **Subagents get zero MCP tools** — `internal/agentruntime/compile_subagents.go:172` compiles `McpServers: []`. Only runtime builtins (bash/read/write, read-only by default). | Executors cannot touch sheets. This is actually a clean separation: executors run browsers and emit JSON; **the coordinator is the only writer** to the registry. No write races, one place to enforce schema. |

Other load-bearing facts:

- **Parallelism**: multiple `subagent_task` calls emitted in **one assistant turn** run concurrently via `join_all` (`runner.rs:831-853`); worker batch is 10; each call blocks with a **15-minute** foreground timeout. Sequential calls = sequential execution — the prompt must teach batching.
- **Subagent output is freeform text** (`handler.rs:816`). Structure is a prompt convention (fenced JSON), parsed by the coordinator.
- **One nesting level**: subagents can't spawn subagents.
- **Credentials**: org env vars are AES-encrypted per agent and injected as `HIVY_ORG_<NAME>` (`internal/agentruntime/runtime_env.go:115`, write-only API). Same-process subagents inherit them.
- **Sheets MCP surface** (needs the `sheets` plugin installed on the agent): `sheet_create`, `sheet_list`, `sheet_describe`, `sheet_manage`, `rows_query`, `rows_write`, `sheet_import_csv`, `sheet_operations`. Filter AST, 100-row write batches, keyset pagination, `resolve_relations` hydration. **No filter-through-relations, no formulas, no backlinks** — coordinator resolves cross-page links itself and computes totals into plain number columns.
- **Persona levers**: `agents.Instructions` (unbounded prose), org-plugin skills (`create_org_plugin`/`create_skill` — requires default agent or explicit `McpToolFilter.Allow`), `chrome` sandbox tool flag, `SetupCommands`, `cron` + `create_http_trigger` + provider webhooks, memory (`retain_memory`/`search_memories`, org-scope auto-recalled into prompt).
- **UI**: sheets render in the existing Glide grid in the chat right-panel — human-readable for free, tabular only.

## 1. Architecture at a glance

```
                #qa channel (DefaultAgentID = QA agent)
                     │  chat / cron / http-trigger
                     ▼
            ┌─────────────────────┐
            │  QA Coordinator     │  sheets MCP · memory · cron ·
            │  (top-level agent)  │  agent-browser (login + state save)
            └────────┬────────────┘
        one turn, N subagent_task calls (parallel)
     ┌───────────────┼───────────────┐
     ▼               ▼               ▼
┌──────────┐   ┌──────────┐   ┌──────────┐
│ executor │   │ executor │   │ executor │   subagent "test-executor"
│ case #12 │   │ case #7  │   │ case #31 │   bash+read+write only
└────┬─────┘   └────┬─────┘   └────┬─────┘   own browser --session,
     │              │              │          shared saved auth state
     └── fenced JSON results ──────┘
                     │
                     ▼
        coordinator: rows_write results,
        update run totals + case status,
        post summary in channel
```

## 2. The registry: one sheet, three pages

Sheet **`QA Test Registry`** (bootstrap on first run via `sheet_create` if `sheet_list` search finds none). If multiple products/channels need separation later, one registry sheet per suite named `QA Test Registry — <suite>`.

**Page `Test Cases`** (`display_field_id` = Name)
| Field | Type | Notes |
|---|---|---|
| Name | text | "Login — happy path" |
| Suite | select | login, checkout, settings… (stands in for channel scoping) |
| Priority | select | P0/P1/P2 |
| Status | select | `draft` → `active` → `quarantined` / `deprecated` |
| Preconditions | long_text | "logged-out state", seed data needs |
| Steps | long_text | **numbered, deterministic steps** — see step format below |
| Expected | long_text | observable assertions |
| Last Result | select | passed/failed/skipped/blocked |
| Last Run At | date | RFC3339, written by coordinator |
| Flaky | checkbox | set after N inconsistent runs |

**Page `Test Runs`** (`display_field_id` = Run)
| Field | Type |
|---|---|
| Run | text (e.g. `run-2026-07-03-1432`) |
| Started / Finished | date |
| Trigger | select: chat / cron / deploy-hook / manual |
| Target | url (base URL tested) |
| Build | text (commit/tag if known) |
| Status | select: running / passed / failed / partial |
| Passed / Failed / Skipped | number (coordinator-computed; no formulas exist) |
| Summary | long_text |

**Page `Test Results`** — one row per (case × run)
| Field | Type |
|---|---|
| Case | relation → Test Cases |
| Run | relation → Test Runs |
| Status | select: passed / failed / skipped / blocked |
| Duration (s) | number |
| Transcript | long_text — steps actually executed, as reported by the executor |
| Failure | long_text — assertion diff, console errors, last URL |
| Artifacts | long_text — screenshot/video paths or asset URLs |

**Query patterns** (no join-through, so):
- Results of a run: `rows_query(Test Results, filter: Run contains <run_row_id>, resolve_relations: true)`
- History of a case (regression baseline): `filter: Case contains <case_row_id>`, sort `created_at desc`, limit 5
- Cases to run: `rows_query(Test Cases, filter: and[Status eq active, Suite eq login])`

**Step format** (stored in `Steps`, ≤16 KB/cell): numbered natural-language steps, each optionally annotated with the semantic-locator command that worked last time:

```
1. Open {BASE_URL}/login
   → browser open $QA_BASE_URL/login
2. Fill the email field with the QA account email
   → browser find label "Email" fill $HIVY_ORG_QA_EMAIL
3. Fill password, submit
   → browser find role button "Sign in" click
4. ASSERT: URL is /w and the sidebar shows the workspace name
```

Natural language is the contract; the command hints are a cache. If the UI changed and a hint fails, the executor falls back to the natural-language intent (snapshot → find the new locator), reports the drift, and the coordinator updates the `Steps` cell — **self-healing test cases**.

## 3. Execution protocol (the coordinator's loop)

**Intake** — "can you test the login page?"
1. `sheet_list`/`sheet_describe` the registry (bootstrap if missing).
2. `rows_query` Test Cases for the requested area.

**Authoring mode** (no matching case — the proactive path):
3. Say so: "No login test case exists — I'll create one, run it, and record it."
4. Explore: coordinator (which has full tools) drives `agent-browser` itself — logs in with `$HIVY_ORG_QA_*` creds, performs the flow, `snapshot -i` at each step to learn stable locators, screenshots key states.
5. Codify: write the case row (`Status: draft`) with numbered steps + locator hints + assertions derived from what it observed.
6. Run it once as a real executor pass (below); on green, promote `draft → active`.

**Run mode**:
7. `rows_write` a Test Runs row (`Status: running`, `Started` now).
8. **Auth bootstrap** (once per run): `agent-browser open $QA_BASE_URL` → perform login → `agent-browser state save /workspace/qa/auth.json`. Skip for logged-out suites.
9. **Fan-out**: partition active cases into groups of 1–3 (15-min cap per subagent; worker batch is 10 — cap at ~8 concurrent). Emit **all `subagent_task` calls in a single turn**. Each goal is self-contained (executors have no MCP, no skills — everything they need travels in the goal):
   - case id + verbatim Steps + Expected + Preconditions
   - `QA_BASE_URL` and instruction to use `--state /workspace/qa/auth.json` loaded into an **isolated session** `--session case-<id>`
   - artifact dir `/workspace/qa/runs/<run>/<case>/` (screenshot on failure, `record start/stop` for P0 cases)
   - the result contract: final message MUST end with one fenced ```json block: `{"case_id", "status", "duration_s", "transcript": [...], "failure": null|{...}, "artifacts": [...], "step_drift": null|{"step": n, "old_hint", "new_hint"}}`
10. **Collect**: parse each fenced JSON. Unparseable/timeout → `blocked`, retry once serially.
11. **Record**: batched `rows_write` to Test Results (≤100/row batches); update the run row (`Status`, totals, `Finished`, `Summary`); update each case's `Last Result` / `Last Run At`; apply any `step_drift` back into `Steps` (note the change in the run Summary).
12. **Report** in the channel: pass/fail table, each failure with its assertion diff + artifact paths, and any test cases it authored or healed.

**Regression intelligence**:
- Before declaring a failure, fetch the case's last passing Transcript and diff — "step 3 previously landed on /w, now lands on /login?error=..." is a much better report than "assert failed".
- `retain_memory` (org scope) for durable environment quirks: "staging resets Mondays", "checkout depends on Stripe test mode", flaky selectors. Auto-recalled into future prompts.
- Two inconsistent results in a row → set `Flaky`, quarantine (`Status: quarantined`), and say so rather than crying wolf.

**Secrets hygiene**: credentials exist only as `HIVY_ORG_*` env refs — never written into Steps, Transcripts, Failures, or chat.

## 4. Agent + subagent definition

**Coordinator** (top-level agent):
- Plugins: `sheets` (installed on the agent — hard requirement for the 8 tools) + the `qa-toolkit` org plugin (§5).
- Tools: baseline + `search_memories`, `retain_memory`, `cron`, `create_http_trigger`; sandbox tool `chrome`.
- Env vars (org env-var API, write-only): `QA_BASE_URL`, `QA_EMAIL`, `QA_PASSWORD` (→ `HIVY_ORG_QA_*`).
- Channel: dedicated `#qa` channel with the agent as `default_agent_id` (set on the agent page — not settable via agent-builder tools).

**Subagent `test-executor`** (via the sub-agents field / `sub_agents` payload):
- Tools: **explicitly grant bash + write + read** — subagent builtins default to read-only if none selected (`compile_subagents.go:161-183`), and bash is how it reaches `agent-browser`.
- Model: inherit (or a cheaper model once the protocol is stable — executors follow instructions, they don't plan).
- Instructions: the executor playbook — load shared state into an isolated `--session`, execute numbered steps, prefer locator hints but fall back to natural-language intent and report drift, screenshot failures into the given artifact dir, close the browser session when done, and end with exactly one fenced JSON result block.

## 5. Prompt/skill split

Keep `agents.Instructions` to the persona + protocol outline (~1 page). Put the long, versionable material in an org plugin **`qa-toolkit`** (authored via `create_org_plugin`/`create_skill` — requires the default agent or explicit allow-listing of the skill-manager tools in `McpToolFilter.Allow`):

- `qa-registry` skill: exact registry schema (pages/fields/choices), bootstrap `sheet_create` payload, query recipes.
- `qa-execution` skill: auth-bootstrap recipe, the subagent goal template, the JSON result contract, artifact conventions, flaky/quarantine policy.
- `qa-authoring` skill: how to explore a flow and codify it into the step format; assertion-writing guidance.

The coordinator loads these via `skills_list`/`skill_view`. Executors can't (no MCP) — the goal template inlines whatever they need. Updating the contract = `update_skill`, no agent redeploy.

## 6. Triggers

- **Chat**: messages in `#qa` ("test checkout on staging").
- **Cron**: the agent self-schedules nightly regression via the `cron` MCP tool ("run all active P0/P1 cases nightly at 03:00").
- **Deploy hook**: `create_http_trigger` returns a POST URL for CI — deploy pipeline calls it → smoke suite against the fresh deploy, results in the same registry.

## 7. Platform gaps — optional follow-ups, ranked

None block v1. Ranked by value/effort:

1. **Structured subagent output** (small): optional JSON-schema param on `subagent_task`, validated before completion. Kills the fenced-JSON parsing fragility. Interim: the prompt convention.
2. **Artifact upload path** (small/medium): a way for the coordinator to publish workspace files (screenshots, recordings) as `agent_assets` and get a `PublicURL` into the Artifacts cell — today artifacts live only in the sandbox workspace and die with it. Worth verifying whether an existing assets-upload path is already reachable from the sandbox before building.
3. **Scoped MCP for subagents** (medium): `compile_subagents.go` hardcodes `McpServers: []`. Even a read-only floor (skills) would let executor instructions live in skills instead of inflating every goal. Full sheets access for executors is *not* wanted — single-writer is a feature.
4. **`channel_id` on sheets** (small migration + query filter): makes "sheets in this channel" literal. The `Suite` select + naming convention covers it until then.
5. **`subagent_task` timeout/async knobs** (medium): 15-min cap forces 1–3 cases per executor; fine at first, limiting for long E2E suites.

## 8. Verify before writing the final prompts

`agent-browser`'s source is **not in this repo** (external npm package; only its ~55 subcommands are inventoried in `apps/web/app/w/(chat)/_lib/browser-bash-commands.ts`). Before finalizing the executor playbook, smoke-test in a real sandbox:
1. `state save` → `--state` load into a **different** `--session` actually carries cookies/login.
2. Two concurrent `--session`s don't fight over one daemon/profile.
3. What `record start/stop` emits and where.
4. That `HIVY_ORG_*` vars are visible to bash inside `subagent_task` child sessions (they should be — same process — but confirm).

Next steps after verification: write the coordinator `Instructions`, the three `qa-toolkit` skills, and the `test-executor` subagent instructions, then run the login-page scenario end-to-end as the acceptance test.
