---
name: qa-authoring
description: Use when a requested test case does not exist yet — draft NL Steps, Commands JSON, and assertions, then validate through an executor. Also for updating cases after intentional product changes.
---

# QA Authoring

When someone asks you to test something and no matching case exists in the registry: say so, draft the case, validate it through the executor pipeline, and record it. One validated draft becomes a durable test; every future run is a cheap deterministic replay.

Authoring does NOT require you to open a browser. You draft; an executor validates and — through its heal mechanism — corrects your locator guesses against the real page. You only touch a browser yourself in the coordinator's fallback path, after fresh executors have repeatedly failed.

## Before drafting

- Confirm you have a **target URL** (from the request, the channel's env vars, or the registry's existing cases). If you can't determine it, ask. Never guess a URL.
- Confirm **credentials** if the flow needs them: the user's instructions name the env vars, or inspect what's available. Ask if ambiguous. You will reference them as `${NAME}` — never copy values anywhere.
- Check the registry for a near-miss first (same suite, different scenario) — extend or clone rather than duplicating.

## Draft the case

For each step of the flow, write:

1. **The intent** — what a user is trying to do ("submit the login form"). Intents are the healing anchor; write them precisely.
2. **A best-effort command** — semantic locators inferred from what the flow must contain, in preference order:
   - `find testid <id>` (if the team uses data-testid and you know the ids)
   - `find label "<label>"` (form fields — infer from the flow: "Email", "Password")
   - `find role <role> <action> --name "<accessible name>"` (buttons, links — e.g. `["find", "role", "button", "click", "--name", "Sign in"]`; the accessible name goes in the `--name` flag, never as a bare positional)
   - `find text "<visible text>"` (last semantic resort)
   - CSS selector only for well-known stable structure (`h1`, `[data-testid=...]`)
   - **Never `@eN` snapshot refs** — they are ephemeral and meaningless outside a live snapshot.
   Your guesses do not need to be perfect: the executor heals locator misses against the real page and returns the corrected commands. Wrong *intents* or missing *assertions* cannot be healed — spend your care there.
3. **What success observably looks like** — the URL after the action, text that appears, an element that becomes visible. These become assertions.

## Write assertions — the test IS its assertions

A flow that merely "finished" is not a test. Every meaningful outcome gets an `assert: true` step with an `expect` check:

- Navigation: `["get", "url"]` + `{"op": "contains", "value": "/dashboard"}`
- Content: `["get", "text", "<selector>"]` + `contains`/`equals`
- Visibility: `["is", "visible", "<selector>"]` + `{"op": "equals", "value": "true"}`

At minimum: one assertion for the end state, plus one after any step whose silent failure would make later steps lie (e.g. assert login worked before testing things behind login). Prefer asserting things a real user cares about, not implementation details. Remember: **assertions are never healed** — they are the tripwire that stops a broken product from being "healed around" — so make them specific enough to catch regressions and stable enough not to fire on cosmetic changes.

## Codify

Write the case into the registry (see `qa-registry` for fields):

- `Steps`: numbered plain-language intents — the human-readable mirror.
- `Commands`: the JSON array (schema in `qa-execution`), one object per step, `${NAME}` placeholders for anything secret or environment-specific.
- `Expected`: the assertions in plain language.
- `Suite`/`Persona`/`Priority` set; **`Status: draft`**.

Keep cases small and single-purpose: "login happy path" and "login wrong password" are two cases, not one. Aim for under ~15 steps; a giant case is slow to replay, hard to heal, and fails ambiguously.

## Validate through an executor, then promote

Send the draft through the real pipeline (one executor, per `qa-execution`) — never validate by hand. The executor proves the Commands JSON is schema-valid, heals your locator guesses against the real page, and checks the assertions.

- Green → persist the accepted heals into the draft's Commands, set `Status: active`, record the result row, report what you created (including which locators the executor corrected). Green-after-heal is final; do not send the case out again to re-confirm the healed commands.
- Red because the draft is wrong (bad intent, missing step, wrong assertion) → fix the draft from the executor's evidence and re-validate with a fresh executor.
- Red because the product is broken → report the bug, leave the case in `draft` with a note in the run summary.
- Executors repeatedly unable to establish the flow at all → the coordinator's browser fallback is the last resort.

## Updating existing cases

After an intentional product change (user says "we redesigned login"): redraft the affected `Commands`/`Steps`/`Expected` directly, clear `Heal Pending Review`, reset `Consecutive Passes` to 0, and validate through an executor again. This is an authored edit, not a heal — no review flag needed beyond saying what you changed in the channel.
