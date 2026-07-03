---
name: qa-execution
description: Use when running QA test cases — deterministic replay, locator healing, failure classification, artifact upload, and the executor subagent contract. Load before any test run.
---

# QA Execution

The rule that makes test runs cheap and trustworthy:

> **Deterministic replay is the product. You (the LLM) are the exception
> handler. Assertions are the immune system. Every heal is a logged proposal.**

A green test costs one bash call and zero reasoning. You only think when a step fails — and even then, you never "heal" an assertion.

## The Commands cache (step schema)

Each test case's `Commands` cell is a JSON array of step objects. Commands are **argv arrays** for the `browser` CLI — never shell strings, never prose:

```json
[
  { "intent": "Open the login page",
    "command": ["open", "${STAGING_URL}/login"] },
  { "intent": "Fill the email field",
    "command": ["find", "label", "Email", "fill", "${LOGIN_EMAIL}"] },
  { "intent": "Submit the login form",
    "command": ["find", "role", "button", "click", "--name", "Sign in"] },
  { "intent": "Lands on the dashboard",
    "assert": true,
    "command": ["get", "url"],
    "expect": { "op": "contains", "value": "/dashboard" } }
]
```

Fields per step:
- `intent` (string, required) — what this step means, in plain language. The healing anchor: when a command fails, you re-resolve THIS, not the old selector.
- `command` (string[], required) — `browser` CLI argv. Cache **semantic locators** (`find testid|label|role|text ...`) or stable CSS selectors. **Never cache `@eN` snapshot refs** — they are ephemeral per-snapshot.
- `expect` (optional) — `{"op": "contains"|"equals", "value": "..."}` checked against the command's output.
- `assert: true` — marks a verification step. **Assertion steps are never healed.**
- `no_heal: true` — pins a step; a pinned failure is final.

`${NAME}` placeholders are substituted from the environment at run time. **There are no prescribed names** — users define env vars on the channel with whatever names they like, and their instructions tell you which to use. If the instructions don't say, inspect what's available (`env` filtered of `HIVY_*` and standard shell vars) and ask the user if it's ambiguous. Store only `${NAME}` references — never a secret value — in sheets, transcripts, goals, or reports. A referenced variable that is unset at replay time is a **setup error to report, never something to guess**.

## Browser CLI discipline (read before any browser use)

Wrong CLI syntax is the #1 time sink in QA runs. Rules:

- The `browser` skill and its commands reference (`.skills/browser/references/commands.md`) are preloaded into every executor's context — executors never load them. The coordinator loads them manually only on the fallback path. Take syntax from those files, never from memory.
- Batch related browser commands into one bash call; pass `timeout_seconds: 15` on interaction calls so a bad locator costs seconds, not a whole default timeout.
- Snapshot refs (`@eN`) die on any page change; re-snapshot before reusing.
- Two-strikes rule: the same action failing twice means STOP retrying variants — re-read the docs or re-snapshot, form a hypothesis, then act. The runtime hard-caps tool calls per turn; retry loops end runs partially complete.
- Bad browser state → one `browser close --all`, then a fresh session.

## Tier 0 — replay (default; no reasoning)

```bash
node .skills/qa-execution/scripts/replay.mjs <commands.json> <session-name> <artifact-dir>
```

Write the case's Commands JSON to a file, run the script, read the last stdout line:

- `{"status":"passed","steps_run":N}` (exit 0) — done. Report and move on.
- `{"status":"failed","failed_step":N,"intent":...,"assert":...,"pinned":...,"command":[...],"expect":...,"output":...,"screenshot":...}` (exit 1) — go to the failure ladder below.
- `{"status":"error","message":...}` (exit 2) — schema or setup problem (malformed JSON, unset `${VAR}`). Fix the setup or report it; this is not a test failure.

The script validates the JSON schema, substitutes env vars, executes each argv via execFile (no shell), checks expects, bails on first failure, and screenshots the failure state into the artifact dir.

## The failure ladder

On `"status":"failed"`, classify **before** touching anything:

**Fail immediately — never heal — when:**
- the failed step has `assert: true` or `no_heal: true`;
- the output shows a server error (5xx), page crash, or CAPTCHA;
- `browser console` / `browser errors` show an application exception;
- the *intent itself* is impossible (the element is genuinely gone from the product, not moved — e.g. the feature was removed).

**Heal — only for locator drift — when** the intent is still achievable but the cached locator no longer matches (renamed button, moved field, changed testid):

1. `browser snapshot -i --session <session>` — re-resolve the step's `intent` against the accessibility tree. Only screenshot if the tree is ambiguous.
2. The new target must **semantically match the intent** — a button labeled "Log in" heals a step whose intent is "Submit the login form"; a different button that merely sits in the same spot does not.
3. Patch that one step's `command` in your working copy of the JSON, re-run the replay script from the top.
4. **Budget: max 3 heals per case per run.** A 4th failure = fail.
5. Record every heal: step number, old command, new command, your confidence, one-line reasoning. Include it in your result (`heals` array).

Heals are proposals, not silent edits: the coordinator persists a healed command back to the sheet **only if the whole case then passed**, always sets `Heal Pending Review` on the case, and reports old → new in the channel. A pass-after-heal is final — the executor already re-replayed the full case to green; the coordinator never re-dispatches a healed case to "confirm" it. Only rejected heals send a case back out.

## Sessions, personas, artifacts

- Every case runs in its **own isolated browser session**: `--session case-<case-id>` (the replay script appends this automatically — just pass the session name). Isolated sessions are separate browser instances; they never share cookies.
- Shared login: the coordinator delegates each persona login to an executor once per run — a small login Commands JSON that asserts the login succeeded and ends with `["state", "save", "/workspace/qa/state-<persona>.json"]`. The saved file is shared across sessions. Executors with a persona load it into their own session before replay (`browser open <target-url> --session case-<id>` then `browser state load /workspace/qa/state-<persona>.json --session case-<id>`); state is per-session and never leaks between sessions. Logged-out tests skip this.
- Artifact dir per case: `/workspace/qa/runs/<run-name>/<case-id>/`. Failure screenshots land there automatically; add `browser record start <dir>/rec.webm` / `record stop` around P0 cases.
- After the case, upload artifacts: `node .skills/qa-execution/scripts/upload-run.mjs <artifact-dir> qa/runs/<run-name>/<case-id>` — prints one JSON line per file: `{"file","key","asset_url"}`. The `key` values go into the result's `screenshots`; `asset_url`s render inline when pasted in chat. Requires `HIVY_DRIVE_UPLOAD_URL`/`HIVY_DRIVE_UPLOAD_BEARER` (present in every sandbox).
- Always `browser close --session case-<id>` when the case is done — live sessions cost hundreds of MB each.

## Fan-out (coordinator)

- **One `test-executor` per test case.** Emit all `subagent_task` calls in ONE message — sequential calls run sequentially. Each call blocks up to 15 minutes.
- At most 5 executors in flight (each runs its own headless browser, hundreds of MB apiece). More than 5 cases → waves of 5, recording results between waves.
- The executor's own instructions define its behavior and output contract; the goal carries only the case DATA. Do not restate rules in goals.

### Executor goal template (data only)

```
Execute this test case.

case_id: <id>
target_url: <url>
persona_state_file: <path under /workspace/qa/ or "none">
artifact_dir: /workspace/qa/runs/<run>/<case-id>
remote_prefix: qa/runs/<run>/<case-id>

Expected:
<the case's Expected text, verbatim>

Commands JSON:
<the case's Commands cell, verbatim>
```

The executor returns one fenced json object: `{"case_id", "status": "passed|failed|blocked", "duration_s", "failed_step", "failure", "transcript": [...], "heals": [...], "screenshots": [...], "artifacts_index"}`.

## Failure classification (coordinator)

Classify every failed case yourself from the executor's evidence — failed step + intent, failure output, transcript, heal log, and the last passing Transcript from the registry. Decision order:

1. An assertion failed and the output confirms the expected outcome did not happen → `regression`.
2. The step's intent was still achievable and only a locator or cosmetic expectation was stale (the heal log usually shows this) → `ui-change`.
3. Target unreachable, 5xx/timeout before the flow started, unset `${NAME}` variable, missing test data → `environment`.
4. Concrete evidence of non-determinism (pass-on-retry, same step passing and failing within one run) → `flake`. Never use `flake` to mean "unsure".

Torn between `regression` and `ui-change` → prefer `regression` (a human looks at it). Read the failure screenshot only as a LAST resort — when the written evidence is genuinely inconclusive AND the verdict changes what you record. Image reads are expensive; the text evidence should almost always be enough.

Audit each heal: does the new command still do what the step's intent says? Reject any heal that changed the meaning of the test; rejected heals are stripped and the case is failed. When uncertain, reject.

## Retry, flake, quarantine

- One automatic retry of a failed case, in a fresh session. Pass-on-retry = `flaky-pass` — record it as a flake, never as a clean pass.
- Inconsistent results across runs → flaky: quarantine the case (`Status: quarantined`) and say so. Consistent failure → regression: report it as a bug, never quarantine it.
- Quarantined cases still run but do not fail the run. Release back to `active` after 10 consecutive passes (`Consecutive Passes` field).

## Reporting

The channel report after a run: pass/fail/flaky table; each failure with its failure class, the transcript diff against the last passing run ("step 3 previously landed on /dashboard, now stays on /login"), and screenshot `asset_url`s inline; a heal-review section (old → new, confidence, screenshot) for anything that healed; any cases you authored or promoted.
