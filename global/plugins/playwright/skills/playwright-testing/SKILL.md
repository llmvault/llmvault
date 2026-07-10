---
name: playwright-testing
description: Use for planning, writing, repairing, or stabilizing Playwright end-to-end tests in a real repository and application.
---

# Playwright testing

Test real user-facing behavior with the repository's existing Playwright conventions. Read its setup, fixtures, configuration, and nearby tests before introducing a pattern.

## Build reliable tests

1. Reproduce the flow or failure in the running application before writing a new test or changing a selector.
2. Keep one user goal per test and assert observable outcomes: visible content, URL, enabled state, or persisted result.
3. Use the repository's fixture and authentication approach. Persist `storageState` when login is reused; never commit credentials or tokens.
4. Prefer accessible locators in this order: role, label, visible text, test ID. Add a stable test ID when none exists; CSS/XPath is a last resort.
5. Use Playwright's web-first `expect` assertions and condition-based waits. Do not use blind sleeps or weaken an assertion to obtain green.
6. Keep tests isolated: seed prerequisite state through supported API/fixtures where possible, use unique data, and do not depend on execution order.
7. Mock services the product does not control. Test the application's behavior, not a payment provider, inbox, analytics vendor, or third-party outage.

## Diagnose failures

First reproduce and classify the evidence:

- Application regression: preserve the assertion and report the defect.
- Legitimate UI change: update the locator or expected user-visible path.
- Flake: fix setup, isolation, or the waited-for condition.

Use trace, screenshot, video, browser evidence, and test output before changing a test. A retry is evidence to investigate, not a correctness fix.

## Completion

Run the focused test to green and repeat it when flake is plausible. Capture the passing trace/video or useful failure evidence. Match the repository's naming, file layout, and CI configuration; load a linked reference only for an exact advanced pattern.
