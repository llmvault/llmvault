---
name: qa-registry
description: Use when finding, creating, or querying the QA Test Registry sheet — test cases, runs, and results. Load before checking which tests exist, recording results, or bootstrapping the registry.
---

# QA Test Registry

One sheet per channel named **`QA Test Registry`**, three pages: `Test Cases`, `Test Runs`, `Test Results`. Sheets are channel-scoped — you only see this channel's registry, and you must create/read it from a session in the channel it belongs to.

**You (the coordinator) are the only writer.** Executors never touch the registry; they return results and you record them.

## Find or bootstrap

1. `sheet_list` with `search: "QA Test Registry"`. If found → `sheet_describe` to load page IDs and the field-ID legend. Do this once per session and keep the legend; all filters and writes use `fld_` IDs, never names.
2. If missing, create it with exactly this payload (then `sheet_describe`):

```json
{
  "name": "QA Test Registry",
  "description": "Test cases, runs, and results maintained by the QA agent.",
  "pages": [
    { "name": "Test Cases", "fields": [
      { "name": "Name", "type": "text" },
      { "name": "Suite", "type": "select", "options": { "choices": [] } },
      { "name": "Persona", "type": "select", "options": { "choices": [] } },
      { "name": "Priority", "type": "select", "options": { "choices": ["P0", "P1", "P2"] } },
      { "name": "Status", "type": "select", "options": { "choices": ["draft", "active", "quarantined", "deprecated"] } },
      { "name": "Preconditions", "type": "long_text" },
      { "name": "Steps", "type": "long_text" },
      { "name": "Commands", "type": "long_text" },
      { "name": "Expected", "type": "long_text" },
      { "name": "Last Result", "type": "select", "options": { "choices": ["passed", "flaky-pass", "failed", "blocked"] } },
      { "name": "Last Run At", "type": "date" },
      { "name": "Heal Pending Review", "type": "checkbox" },
      { "name": "Consecutive Passes", "type": "number" }
    ]},
    { "name": "Test Runs", "fields": [
      { "name": "Run", "type": "text" },
      { "name": "Started", "type": "date" },
      { "name": "Finished", "type": "date" },
      { "name": "Trigger", "type": "select", "options": { "choices": ["chat", "cron", "deploy-hook", "manual"] } },
      { "name": "Target", "type": "url" },
      { "name": "Build", "type": "text" },
      { "name": "Status", "type": "select", "options": { "choices": ["running", "passed", "failed", "partial"] } },
      { "name": "Passed", "type": "number" },
      { "name": "Failed", "type": "number" },
      { "name": "Flaky", "type": "number" },
      { "name": "Skipped", "type": "number" },
      { "name": "Summary", "type": "long_text" }
    ]},
    { "name": "Test Results", "fields": [
      { "name": "Status", "type": "select", "options": { "choices": ["passed", "flaky-pass", "failed", "blocked"] } },
      { "name": "Failure Class", "type": "select", "options": { "choices": ["regression", "ui-change", "environment", "flake", "none"] } },
      { "name": "Duration Seconds", "type": "number" },
      { "name": "Transcript", "type": "long_text" },
      { "name": "Failure", "type": "long_text" },
      { "name": "Heals", "type": "long_text" },
      { "name": "Screenshots", "type": "attachment" },
      { "name": "Artifacts Index", "type": "url" }
    ]}
  ]
}
```

3. After creating, add the two relation fields on `Test Results` with `sheet_manage` (relations need the target page IDs, which only exist after creation):
   - `add_field` on the Test Results page: name `Case`, type `relation`, options `{ "target_page_id": "<Test Cases page id>" }`
   - `add_field`: name `Run`, type `relation`, options `{ "target_page_id": "<Test Runs page id>" }`
4. `Suite` and `Persona` start with empty choices — add choices as you need them via `sheet_manage` `update_field` (choices must exist before a row can use them). Add choices for every new suite or persona before writing rows.

## Field semantics

- **Steps** (Test Cases): numbered natural-language intents, one per line — for humans reading the grid. Never parsed by machines.
- **Commands** (Test Cases): the machine replay cache — a JSON array of step objects (schema in the `qa-execution` skill). This is what actually runs. Steps and Commands describe the same test; keep them in sync. (The general sheets guidance forbids JSON in cells; `Commands` is the one sanctioned exception — it is a machine cache by design, and humans read `Steps`.)
- **Expected** (Test Cases): the observable assertions in plain language.
- **Screenshots** (Test Results): attachment cell of drive object keys (`pub/e/...`), max 10 per cell. Put overflow behind an `Artifacts Index` URL.
- Dates are RFC3339 UTC strings. Relation cells are arrays of row UUIDs.

## Query recipes

Always filter by `fld_` IDs from your legend. `rows_query` returns `{rows: [{id, data}], fields_legend, next_cursor}`.

- **Runnable cases for a suite**: `filter: {"and": [{"field": "<fld Status>", "op": "eq", "value": "active"}, {"field": "<fld Suite>", "op": "eq", "value": "login"}]}`
- **Does a case exist for this area?** `search: "login"` on the Test Cases page (matches any cell text), or filter on Suite/Name.
- **All results of one run** (backlink — indexed and fast): `filter: {"field": "<fld Run relation>", "op": "contains", "value": "<run row uuid>"}` on Test Results.
- **A case's history, newest first**: same `contains` on the Case relation, `sorts: [{"field": "<fld you sort by>", "direction": "desc"}]` — note rows do not expose created_at; keep your own ordering via the Run's `Started` date (fetch the linked runs) or the run-name convention below.
- **Hydrate relation chips**: pass `resolve_relations: true` to get `{id, label}` for linked rows.
- There is no fetch-row-by-id and no join-through-relations: to answer "results of last night's run", first find the run row (filter Test Runs by Run name or Started), then `contains`-filter Test Results by that row's id.

## Write rules

- Run names: `run-<YYYY-MM-DD-HHMM>` (e.g. `run-2026-07-03-0300`) — unique, sortable, human-readable.
- `rows_write` batches up to 100 rows per call; one page per call. Insert all of a run's results in as few batched calls as possible.
- **Never update the same row from concurrent calls** — updates merge whole cells and concurrent writes to one row lose data. Batch instead.
- Update, don't re-insert: after a run, update each case's `Last Result`, `Last Run At`, `Consecutive Passes` (increment on pass, reset to 0 on fail).
- Deleting archives; `sheet_operations` can revert your last operation on a page if you make a mistake.
