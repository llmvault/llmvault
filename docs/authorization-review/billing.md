# Authorization — Billing & Subscription

## 1. Overview

Everything money: plan catalog, checkout, payment verification, subscription lifecycle (upgrade/downgrade/cancel/resume), credit balance, and usage/spend reporting. Provider is **Paystack** (`internal/billing/paystack`), fronted by a provider-agnostic `billing.Registry`. Resources:

- **Subscription** (`model.Subscription`) — one active row per org; carries the payment-method snapshot (authorization code, card last4/brand/exp, bank/account name), current period, cancel-at-period-end flag, pending plan change.
- **Org.plan_slug** — the org's active plan; set only by the payment flow.
- **Credits** (`internal/billing/credits*`) — granted on subscribe/renew, spent by usage.
- **Plan catalog** (`model.Plan`, `internal/billing/plancatalog`) — public.
- **Quotes** — server-signed proration quotes for plan changes.

Principals per `_MODEL.md`: money + org lifecycle → **Org Owner** only. Roles today are `org_memberships.Role ∈ {owner, admin, member}` (org creator = `owner`, see `internal/handler/auth_signup.go:46`, `orgs.go:131`). **There is no owner-only predicate anywhere in the codebase** — `RequireOrgAdmin` accepts `owner|admin` (`internal/middleware/auth.go:162`), and `access.IsOrgManager` is likewise `owner|admin`. So an owner-vs-admin distinction for billing does not yet exist as an enforceable gate.

**Headline finding: billing has NO role gate at all today — backend or frontend.** Every billing route is mounted inside the plain JWT/org group with only `MultiAuth + RequireEmailConfirmed + ResolveOrgFlexible + RateLimit + Audit` (`cmd/server/serve_routes_v1.go:135` → `serve_routes_billing.go`). No `RequireOrgAdmin`, no inline role check. Any authenticated org member — including a plain `member` — can change the plan, pay, cancel, and view the payment method. This is the single most exposed money surface in the app.

Payment-provider callback axis: **there is no inbound Paystack webhook route.** The `paystack.go` header comment references a `webhook.go`/`VerifyWebhook`, but no such file/function exists in the tree. Payment confirmation happens via the authenticated `POST /v1/billing/verify` (client polls after the popup, server calls Paystack `/transaction/verify` and asserts amount+currency+org). Recurring renewals run in an internal worker (`internal/tasks/billing_renewal.go`, `internal/billing/subscription/sweep.go`) with no HTTP surface. So the "webhook = signature-verified, no human role" axis is **not present** for billing; see §6.

## 2. Backend endpoint inventory

All `/v1/billing/*`, `/v1/usage`, `/v1/dashboard`, `/v1/reporting`, `/v1/generations*` inherit exactly: `MultiAuth`, `RequireEmailConfirmed`, `ResolveOrgFlexible`, `RateLimit`, `Audit`. "member" below = any authenticated member of the resolved org; org-scoping (`org.ID`) is enforced but role is not.

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | `/v1/plans` | `plans.go:29` List | Reads (catalog) | **None (public, no auth)** — `serve_routes.go:70` | ✅ non-sensitive catalog |
| POST | `/v1/billing/checkout` | `billing.go:55` CreateCheckout | Mutates (creates provider customer + checkout session) | member | ❌ owner-only |
| POST | `/v1/billing/verify` | `billing_verify.go:55` Verify | Mutates (provisions Subscription, sets `org.plan_slug`, grants credits) | member | ❌ owner-only |
| GET | `/v1/billing/subscription` | `billing_subscription_get.go:45` GetSubscription | Reads (incl. **payment-method snapshot**: card last4/brand/exp, bank/account name) | member | ⚠️ payment PII to any member |
| POST | `/v1/billing/subscription/preview-change` | `billing_subscription.go:60` PreviewChange | Mutates lightly (persists a signed quote) | member | ⚠️ part of change flow |
| POST | `/v1/billing/subscription/init-upgrade` | `billing_subscription_init.go:38` InitUpgrade | Mutates (initialises Paystack txn for upgrade) | member | ❌ owner-only |
| POST | `/v1/billing/subscription/apply-change` | `billing_subscription.go:133` ApplyChange | Mutates (**charges saved card / switches plan**) | member | ❌ owner-only (CRITICAL) |
| POST | `/v1/billing/subscription/cancel` | `billing_subscription.go:201` Cancel | Mutates (**cancels subscription**) | member | ❌ owner-only (CRITICAL) |
| POST | `/v1/billing/subscription/resume` | `billing_subscription.go:241` Resume | Mutates (clears cancel flag) | member | ❌ owner-only |
| GET | `/v1/usage` | `usage.go:136` Get | Reads (usage) | member | ⚠️ view — admin+? |
| GET | `/v1/dashboard` | `dashboard.go:58` Get | Reads (credit balance, spend, connections, schedules) | member | ⚠️ view — admin+? |
| GET | `/v1/reporting` | `reporting.go:69` Get | Reads (org-wide reporting) | member | ⚠️ view — admin+? |
| GET | `/v1/generations` | `generations.go:136` List | Reads (org generation history) | member | ⚠️ view — admin+? |
| GET | `/v1/generations/{id}` | `generations.go:95` Get | Reads (single generation) | member | ⚠️ view — admin+? |

Notes on the existing safety rails (org-scoping, not role): quotes are bound to the requesting org (`ErrQuoteWrongOrg` → 403, `billing_subscription.go:155`, `billing_subscription_init.go:61`); `Verify` passes `ExpectedOrgID` and asserts `PaidAmountMinor == plan.PriceCents` and currency match (`billing_verify.go:83,107,116`). These stop cross-org tampering and popup-amount tampering, but do nothing to stop a low-privilege member from acting on **their own** org's money.

## 3. Frontend screens & actions

Role IS available in the client (`useAuth().activeOrg.role`; the Teams screens gate on `activeOrg?.role === "owner" || activeOrg?.role === "admin"`, `settings/teams/page.tsx:24`). **No billing screen references it.** The billing link lives under the "Personal" nav section (`settings/_components/nav.tsx:18`) and is shown to everyone; the only nav filter is a text search. No `middleware.ts` route guard exists for these paths.

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---|---|---|---|---|
| `settings/_components/nav.tsx` + `settings/layout.tsx` | Show "Usage & billing" link | — | **No** (static item, text-search only) | Owner sees change actions; admin may see usage |
| `settings/billing/page.tsx` | Layout container | — | No | — |
| `settings/billing/_components/your-plan-section.tsx` | View plan / "View plans" | GET `/v1/billing/subscription`; nav to `/w/billing/plans` | **No** | View: owner (+admin?) |
| `settings/billing/_components/credits-balance-section.tsx` | Buy credits | GET `/v1/dashboard`; nav to plans | **No** | Owner |
| `settings/billing/_components/credits-usage-section.tsx` | View usage this period | GET `/v1/dashboard` | **No** (read-only) | Admin+ (view) |
| `settings/billing/_components/cancel-plan-section.tsx` | Cancel subscription | POST `/v1/billing/subscription/cancel` | **No** (only `if (!hasPaidSubscription) return null`) | Owner |
| `settings/billing/_components/cancel-plan-section.tsx` | Resume subscription | POST `/v1/billing/subscription/resume` | **No** | Owner |
| `w/billing/plans/page.tsx` → `_components/plans-page.tsx` | Checkout (new sub) | POST `/v1/billing/checkout` → POST `/v1/billing/verify` (via `hooks/use-paystack-pop.ts`, only guard `if (!user?.email)`) | **No** | Owner |
| `_components/plans-page.tsx` | Preview upgrade/downgrade | POST `/v1/billing/subscription/preview-change` | **No** | Owner |
| `_components/plans-page.tsx` | Confirm upgrade (pay) | POST `/v1/billing/subscription/init-upgrade` | **No** | Owner |
| `_components/plans-page.tsx` | Apply plan change | POST `/v1/billing/subscription/apply-change` | **No** | Owner |

## 4. Ambiguities & lapses (ranked)

1. **CRITICAL — a plain member can cancel the org's subscription.** `POST /v1/billing/subscription/cancel` has no role gate. Any member can cancel the org's plan (default `at_period_end: true`, but `at_period_end: false` is accepted → immediate loss of service). Blast radius: denial of the whole org's paid capability by any single low-trust member; trivially reachable from the UI (button shown to all) and the API.

2. **CRITICAL — a plain member can change the plan and trigger a charge.** `apply-change` (+ `preview-change`/`init-upgrade`) lets any member downgrade (revenue/feature loss, deferred) or upgrade — and an upgrade charges the org's saved card via the verified reference. A member can move the org onto a different plan and cause a real-money charge without owner consent.

3. **CRITICAL — a plain member can start a fresh paid subscription.** `checkout` + `verify` provision a subscription and set `org.plan_slug` + grant credits. A member can subscribe the org to a paid plan (paying with their own card, but committing the org to a plan/state) with no owner involvement.

4. **HIGH — payment-method details exposed to every member.** `GET /v1/billing/subscription` returns the card brand/last4/expiry and bank/account name of whoever pays. Any member (and the UI, unconditionally) can read partial payment instrument details for the org. This is money-adjacent PII with no role gate.

5. **MEDIUM — org-wide financial/usage reads are member-visible.** `/v1/usage`, `/v1/dashboard`, `/v1/reporting`, `/v1/generations` expose org-wide credit balance, spend, connection inventory, and generation history to every member. Lower blast radius (read-only, own org) but inconsistent with treating spend/billing posture as privileged.

6. **LOW — frontend advertises money actions to everyone.** The billing nav item and every mutate button render for all roles; role is available but unused. Even once the backend is gated, the UI will show actions that 403, a poor and confusing UX. Defense-in-depth + UX gap.

## 5. Recommendation

Target principal per action (money + org lifecycle = **Owner**; usage *viewing* may be broadened to **Admin**):

| Action(s) | Target principal | Enforcement |
|---|---|---|
| `checkout`, `verify`, `preview-change`, `init-upgrade`, `apply-change`, `cancel`, `resume` | **Owner** | New route gate on the `/v1/billing/*` mutating group |
| `GET /v1/billing/subscription` (payment-method snapshot) | **Owner** to see payment instrument; consider a redacted admin/member view (plan + status + credits only) | Split response by role, or gate the whole route to owner and expose plan/status via a member-safe field elsewhere |
| `GET /v1/usage`, `/v1/dashboard`, `/v1/reporting`, `/v1/generations` | **Admin+** (view usage/spend), owner included | Route gate `RequireOrgManager` (owner|admin) — operator to confirm whether members should keep usage visibility |
| `GET /v1/plans` | anyone (public catalog) | No change |

**Mechanism (buildable):**

- **Introduce an owner-only predicate.** Add `access.IsOrgOwner()` (role == `owner`) alongside `IsOrgManager`, and a `middleware.RequireOrgOwner(db)` mirroring `RequireOrgAdmin` (`internal/middleware/auth.go:144`) but checking `m.Role == "owner"`. No schema change needed — the `owner` role already exists on `org_memberships`.
- **Gate the billing mutation routes.** In `cmd/server/serve_routes_billing.go`, wrap checkout/verify + all `subscription/*` mutations in a `r.Group` with `middleware.RequireOrgOwner(database)`. Keep this consistent with the shared-layer goal in `_MODEL.md` (extend `internal/access` rather than inline checks).
- **Gate the reads.** Wrap `/v1/usage`, `/v1/dashboard`, `/v1/reporting`, `/v1/generations` in `RequireOrgAdmin` (owner|admin) if the operator wants usage visibility to be admin+; leave `/v1/plans` public.
- **Redact payment PII for non-owners** if `GET /v1/billing/subscription` must remain member-visible for plan/credits display: strip `card_*`, `payment_bank_name`, `payment_account_name` unless caller is owner (`fillSubscriptionResponse`, `billing_subscription_get.go:76`).
- **Frontend mirrors the gate.** Hide/disable checkout/upgrade/cancel/resume unless `activeOrg.role === "owner"` (reuse the exact pattern from `settings/teams/page.tsx:24`); optionally hide the billing nav item's mutating affordances for non-owners. Backend stays authoritative.

No new table/column/migration is required — this is entirely a predicate + route-wrapping change, because `owner` is already a persisted role.

## 6. Deviations from the baseline model

- **Payment-provider webhook axis does not exist here.** `_MODEL.md` and the task anticipate signature-verified provider callbacks with no human role. The billing subsystem has **no inbound Paystack webhook route** (despite a stale `paystack.go` comment describing a non-existent `webhook.go`/`VerifyWebhook`). Payment state is instead reached through the authenticated `POST /v1/billing/verify` (client-driven, org-scoped, amount/currency/org asserted) and the internal renewal worker (`internal/tasks/billing_renewal.go`, `subscription/sweep.go`) which has no HTTP surface. So there is nothing to "document but not role-gate" on the webhook axis; flagging its absence is itself a finding — if a Paystack webhook is added later it must be HMAC-verified (raw body, `X-Paystack-Signature`) and remain role-free.
- **Owner vs Admin is not separable today.** The baseline reserves billing for Owner and usage-view potentially for Admin, but the only manager gate that exists (`RequireOrgAdmin`/`IsOrgManager`) collapses owner and admin. Implementing the recommendation requires the new `RequireOrgOwner` predicate above; until then "owner-only" is unenforceable and billing is either fully open (today) or admin-or-owner (if you reuse the existing gate as a stopgap).

## 7. Open questions for the operator

1. **Usage/spend visibility for plain members:** should `/v1/usage`, `/v1/dashboard`, `/v1/reporting`, `/v1/generations` be admin-only, or is org-wide usage visibility to every member intended? (These currently leak org-wide spend/connection/generation data to members.)
2. **Payment-method visibility:** owner-only, or admin-visible (redacted) for support/ops? Determines whether we split the subscription response or fully gate it.
3. **Admin (non-owner) and billing:** can an Org Admin ever change the plan or cancel, or is that strictly Owner? The baseline says Owner-only; confirm before we choose `RequireOrgOwner` vs `RequireOrgAdmin` for the mutating group.
4. **Interim stopgap:** given billing is fully ungated in production today, do you want an immediate wrap in the existing `RequireOrgAdmin` (owner|admin) as a hotfix, then tighten to owner-only once `RequireOrgOwner` lands?
