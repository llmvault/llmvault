---
name: playwright-testing
description: Use for anything about Playwright end-to-end tests — planning coverage for a flow, writing or refactoring spec files, or fixing flaky/failing tests and hardening a suite for CI. Covers what to test, test-plan format, locator strategy (getByRole first), web-first assertions, fixtures vs page objects, storageState auth, killing flakiness, mocking, the trace viewer, and healing tests broken by UI changes.
---

# Playwright end-to-end testing: plan, write, stabilize

Great e2e suites are planned, written against the real app, and kept stable — not improvised.
This skill covers the full lifecycle in three phases that mirror Playwright's own agent model
(**plan → generate → heal**):

1. **Plan** — explore the running feature and write a precise test plan.
2. **Write** — turn the plan into production-grade spec files.
3. **Stabilize** — eliminate flakiness, harden for CI, and heal broken tests.

The single overriding rule across all three: **drive the real application first, never guess.**
Use the `browser` skill (live accessibility-tree automation) or an existing seed test to reach a
feature before you plan or write anything about it, so every scenario, selector, and assertion
references states, copy, and elements that actually exist.

---

## Phase 1 — Plan before you write

Skipping straight to code produces tests that only cover the happy path, miss the failure modes
that break in production, and duplicate each other. Plan first.

### Explore the running app

Drive the flow you're about to cover and identify:

- The **entry points** and preconditions (auth? seeded data? a feature flag?).
- The **happy path** — the sequence a normal user takes to success.
- Every **branch** — validation errors, empty states, permissions, expired sessions, network
  failures, concurrent edits, boundary values.
- The **observable outcomes** at each step (visible text, URL, toast, the row that appears).
  Tests assert on what the user sees, so the plan must name it.

### What to test (and what not to)

Test **user-visible behaviour**, not implementation details — never reference internal function
names, private state, or CSS classes in the plan. Prioritise, in order:

1. **Critical revenue/trust paths** — login, signup, checkout, publish, the core job-to-be-done.
2. **Destructive or irreversible actions** — delete, pay, send, cancel subscription.
3. **The failure branches of the above** — wrong password, declined card, permission denied.
   This is where real bugs hide and where teams under-invest.
4. **Regressions** — every bug you reproduce becomes a permanent test so it never returns.

Deliberately keep **out** of an e2e plan:

- Third-party sites/services you don't control (mock them — see Phase 3).
- Pure unit logic a fast unit test covers better. E2e is expensive; spend it on integration
  through the real UI.
- Exhaustive input permutations — pick representative boundary values, not the cross-product.

Keep the suite a **pyramid**: a few broad journey tests, more focused feature tests, and push
combinatorial detail down to unit tests.

### Isolation is a planning constraint

Every scenario must run **alone, in any order, in parallel** with a clean context (own storage,
cookies, data). For each scenario, plan its setup and teardown:

- What state must exist first? Create it via **API/DB setup** (fast, reliable), not by clicking
  through prerequisite UI.
- Make data **unique per run** (unique emails, ids) so parallel tests can't collide.
- Never let scenario B depend on scenario A having run first. Shared mutable state is the #1
  cause of order-dependent flakiness.

### The test-plan format

Write plans as Markdown in a `specs/` directory (one file per feature), e.g.
`specs/guest-checkout.md` — this is the artifact Phase 2 consumes:

```markdown
# Feature: Guest checkout

## Overview
One paragraph: what the feature does and why it matters. Note the entry URL,
auth requirement, and any feature flags.

## Preconditions / test data
- Seeded product `SKU-123` in stock, price $19.00 (create via API in setup).
- No authenticated user (guest context).

## Seed / setup reference
- Reaches checkout via `tests/seed.spec.ts` pattern (cart pre-loaded fixture).

## Scenarios

### 1. Guest completes checkout with a valid card  [happy path]
1. Go to `/cart` with SKU-123 in cart.
   - Expect: cart shows "1 item", subtotal "$19.00".
2. Click "Checkout as guest".
   - Expect: shipping form is visible.
3. Fill shipping + valid test card `4242…`, submit.
   - Expect: URL is `/order/confirmed`; heading "Thank you"; order number visible.

### 2. Declined card shows an inline error  [failure branch]
1. …reach payment step…
2. Submit card `4000000000000002`.
   - Expect: inline error "Your card was declined"; user stays on payment step;
     no order is created.

### 3. Empty cart cannot reach checkout  [edge]
...

## Out of scope
- Real payment-processor behaviour (mocked).
- Email delivery (asserted via API, not inbox).
```

Rules for good scenarios:

- **One scenario = one user goal.** If a scenario has "and then also…", split it.
- Every step lists an **explicit expected result** phrased as what's visible. "Expect: toast
  'Saved'" — not "Expect: it works".
- Name the **category** (happy path / failure / edge / permission) so coverage gaps are obvious.
- Give scenarios stable names — they become `test('...')` titles.

Recommend a `tests/seed.spec.ts` that demonstrates reaching the app's authenticated, ready state
(fixtures, hooks, storageState auth) so every generated test starts from the same known-good
setup. Get the plan agreed **before** any test code is written.

---

## Phase 2 — Write production-grade tests

Turn the plan into clean, maintainable `@playwright/test` spec files — code you'd merge, not
throwaway scripts.

### Rule 0: match the repo's existing conventions FIRST

Before writing a line, inspect how this repo already writes tests — conventions beat generic
advice:

```bash
find . -path ./node_modules -prune -o -name '*.spec.ts' -print -o -name '*.test.ts' -print | head
cat playwright.config.ts 2>/dev/null || cat playwright.config.js 2>/dev/null
ls tests/ e2e/ 2>/dev/null            # where do specs live?
```

Match their file layout, naming, fixture style (POM vs plain fixtures), auth approach, and
`baseURL`. Only introduce a new pattern when the repo has none.

### Verify selectors and assertions live

Don't write locators from imagination. As you author each step, confirm the selector resolves and
the assertion holds against the running app (via the `browser` skill or by running the test).
Write a scenario, run it green, then write the next.

### Locators: the stability hierarchy

Prefer user-facing, accessibility-anchored locators — they survive redesigns because they're tied
to what the user perceives, not to DOM structure. Use, in order:

1. `page.getByRole('button', { name: 'Sign in' })` — semantic + doubles as an a11y check. Default.
2. `getByLabel()` / `getByPlaceholder()` — form fields.
3. `getByText()` / `getByAltText()` / `getByTitle()` — visible content.
4. `getByTestId()` — deliberate fallback. Add a `data-testid` to the app when nothing above is
   stable, rather than reaching for CSS.
5. CSS / XPath — last resort only. `page.locator('button.btn-icon.x')` breaks silently on
   restyle; text selectors break the moment a PM edits copy.

Chain and filter instead of brittle compound selectors:

```ts
await page.getByRole('listitem')
  .filter({ hasText: 'Product 2' })
  .getByRole('button', { name: 'Add to cart' })
  .click();
```

`npx playwright codegen <url>` generates locators using this same priority — use it to discover
good locators, then clean up the recording.

### Assertions: always web-first

Use auto-retrying `expect()` matchers. They poll until the condition holds or time out — the
single biggest source of non-flaky tests.

```ts
// ✅ waits and retries
await expect(page.getByText('Welcome')).toBeVisible();
await expect(page).toHaveURL(/\/dashboard/);
await expect(page.getByRole('row')).toHaveCount(3);

// ❌ reads state once — races the UI, flakes forever
expect(await page.getByText('Welcome').isVisible()).toBe(true);
```

Assert on **outcomes the user observes** (visible text, URL, counts, enabled state), matching the
"Expect:" lines from the plan. Use `expect.soft()` to collect several independent UI checks in one
test without aborting at the first failure.

### Structure: fixtures first, page objects when they earn it

For small-to-medium suites, **composable fixtures usually beat the Page Object Model** — same
isolation, far less ceremony. Build fixtures around **business state / actions**, not thin wrappers
over Playwright:

```ts
// fixtures.ts — extend the base test with ready-to-use state
import { test as base } from '@playwright/test';

export const test = base.extend<{ pageWithCart: Page }>({
  pageWithCart: async ({ page, request }, use) => {
    await seedCartViaApi(request);          // set state via API, not UI clicks
    await page.goto('/cart');
    await use(page);
  },
});
```

```ts
// checkout.spec.ts — reads like the plan
import { test } from './fixtures';
import { expect } from '@playwright/test';

test('guest completes checkout with a valid card', async ({ pageWithCart: page }) => {
  await page.getByRole('button', { name: 'Checkout as guest' }).click();
  await fillShipping(page);
  await payWith(page, '4242424242424242');
  await expect(page).toHaveURL(/\/order\/confirmed/);
  await expect(page.getByRole('heading', { name: 'Thank you' })).toBeVisible();
});
```

Reach for the **Page Object Model** when interaction logic for a page is duplicated across many
tests. Keep page objects **user-centric** (`login(user)`, `addToCart(sku)`) — capture selectors in
one place, never expose raw DOM queries to the test. Extract it when the duplication is real, not
preemptively.

### Auth: log in once with storageState, never per test

Re-logging in every test wastes minutes and adds a flaky step to every scenario. Authenticate once
in global setup, persist the storage state, and load it into every context:

```ts
// auth.setup.ts (a setup project / global setup)
await page.goto('/login');
await page.getByLabel('Email').fill(process.env.TEST_USER!);
await page.getByLabel('Password').fill(process.env.TEST_PASS!);
await page.getByRole('button', { name: 'Sign in' }).click();
await expect(page).toHaveURL('/dashboard');
await page.context().storageState({ path: '.auth/user.json' });
```

```ts
// playwright.config.ts
projects: [
  { name: 'setup', testMatch: /auth\.setup\.ts/ },
  {
    name: 'chromium',
    use: { ...devices['Desktop Chrome'], storageState: '.auth/user.json' },
    dependencies: ['setup'],
  },
]
```

Regenerate the state file per CI run; **never commit tokens** — add `.auth/` to `.gitignore`.

### File organisation & hooks

- Organise specs **by user journey / feature**, not one-file-per-page.
- Use `test.beforeEach` for shared navigation/setup, but keep each test **isolated** — no state
  leaks between tests.
- Give tests the scenario names from the plan: `test('declined card shows an inline error')`.
- Enable TypeScript + ESLint `@typescript-eslint/no-floating-promises` to catch missing `await`
  on Playwright calls — a classic silent bug.

### Never weaken a test to make it pass

If a test fails because the app is wrong, **the test is doing its job** — report the bug (with a
reproduction) rather than loosening the assertion, adding a sleep, or deleting the check. A test
softened until it's green protects nothing. Run the suite to green before handing back, and capture
a trace/screenshot as evidence.

---

## Phase 3 — Stabilize, harden, and heal

The most expensive test is the one that fails intermittently, gets retried, eventually passes, and
convinces everyone the suite is healthy when it isn't. Kill flakiness at the source.

### The cardinal rule: never sleep, always wait for a condition

`waitForTimeout` / `sleep` is the root of most flakiness — too short and it races, too long and the
suite crawls. Replace every hard wait with a wait on the actual condition:

```ts
// ❌ guesses at timing — flaky and slow
await page.waitForTimeout(2000);

// ✅ waits for the real signal, no longer than needed
await expect(page.getByText('Saved')).toBeVisible();
await page.getByRole('button', { name: 'Save' }).click(); // auto-waits for actionability
```

Playwright **auto-waits**: actions wait for elements to be attached, visible, stable, enabled, and
for navigations/network to settle. Web-first `expect()` matchers retry until true. Lean on both
instead of manual waiting. For eventually-consistent state not tied to one element, use
`expect.poll` (never a while-loop + sleep):

```ts
await expect.poll(async () => (await api.getOrderStatus(id))).toBe('shipped');
await expect.poll(() => page.getByRole('row').count()).toBeGreaterThan(0);
```

### Isolation: the other half of stability

Order-dependent tests are flaky by construction. Each test gets its own browser **context** (fresh
storage, cookies) automatically — don't defeat it with shared mutable state. Seed prerequisite data
via **API/DB** (not by clicking through other features), make test data **unique per run**, clean
up in teardown, and never assume database state — pin it as a fixture.

### Mock everything you don't control

Don't test third-party sites/servers. Intercept their network calls so tests are deterministic and
don't fail when an external service hiccups:

```ts
await page.route('**/api.stripe.com/**', route =>
  route.fulfill({ status: 200, json: { id: 'pi_test', status: 'succeeded' } }));
```

Mock analytics, payment processors, email, flaky widgets. Assert your *own* backend behaviour
through the UI; assert side effects (email sent, webhook fired) via API, not by watching a real
inbox.

### Config knobs that stop CI flakiness

```ts
// playwright.config.ts
export default defineConfig({
  use: {
    actionTimeout: 8_000,          // fail fast per-action instead of a 30s test-wide ceiling
    navigationTimeout: 15_000,
    trace: 'on-first-retry',       // capture a full trace only when something flakes
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  retries: process.env.CI ? 2 : 0,
  fullyParallel: true,
  reporter: [['html'], ['list']],
});
```

Four common CI-only failure causes and their fixes:

1. **Missing action timeouts** → set `actionTimeout`.
2. **Hydration races** → wait on a specific element after load, or `waitUntil: 'networkidle'` for
   hydration-heavy apps, rather than assuming the page is ready.
3. **No failure evidence** → `screenshot: 'only-on-failure'` + trace.
4. **Environment assumptions** → pin **timezone and locale** via CI env vars; seed data as
   fixtures. "Passes locally, fails in CI" is almost always locale, timezone, or data state.

**Retries are a diagnostic, not a fix.** A retry budget of 0 forces you to fix the real cause. Use
`retries: 2` in CI to keep the pipeline moving, but treat any test that only passes on retry as a
**bug to investigate**. Reserve genuine tolerance for truly non-deterministic external systems
(OAuth, email, payments) — and prefer mocking those away entirely.

### Parallelism and sharding

- **Workers** (one machine): `--workers=4` or `fullyParallel: true`. Requires real isolation.
  Start here.
- **Sharding** (many machines): `--shard=1/4` in CI. Add only when a single machine exceeds ~5 min.
  Don't reach for sharding before isolation and workers are solid.

### Debugging: trace viewer first, not console.log

When a test fails, open the trace before touching the test code — it gives you DOM snapshots, the
network timeline, console, and video at the exact failure moment:

```bash
npx playwright test --trace on        # capture locally
npx playwright show-trace trace.zip   # or download the artifact from CI
npx playwright test --debug           # step through with the inspector
npx playwright show-report            # the HTML report
```

This replaces `console.log`, `page.pause()`, and `waitForTimeout` guessing.

### Healing a test broken by a UI change

When a test fails because the app *legitimately* changed (renamed button, new step, moved field):

1. **Reproduce** — replay the failing step and inspect the current UI (trace viewer / `--debug` /
   the `browser` skill). Confirm it's a UI change, not a real bug.
2. **Diagnose the class of fix** — locator update, a new/removed step, a wait/assertion adjustment,
   or a data fix.
3. **Apply the minimal patch** using the locator hierarchy above — prefer updating to a
   `getByRole`/label locator over reintroducing a brittle CSS selector.
4. **Re-run until green.** If the feature is genuinely broken, don't paper over it — mark the test
   `test.skip` with a comment linking the bug, or report it, rather than deleting the assertion.

Never heal by weakening: adding sleeps, loosening assertions, or `try/catch`-ing failures makes the
test pass while protecting nothing. A healed test still asserts the real outcome — just against the
updated UI.

### Hardening checklist

- [ ] No `waitForTimeout` / `sleep` anywhere — all waits are on conditions.
- [ ] Every assertion is a web-first `expect()` (auto-retrying), not a one-shot boolean check.
- [ ] Tests pass in a **random order** and in **parallel** (`fullyParallel: true`).
- [ ] All third-party/network dependencies are mocked or seeded.
- [ ] Trace/screenshot/video on failure enabled; timezone + locale pinned in CI.
- [ ] Any test that only passes on retry is filed as a flake to fix, not left green.
