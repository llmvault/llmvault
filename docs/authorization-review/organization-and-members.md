# Authorization — Organization & Members

## 1. Overview

This feature is the **identity / access-control substrate** of the product: the organization itself and everything that grants a human or machine principal access to it.

Resources:
- **Org** (`orgs`) — the tenant. Profile fields (name, logo, website, company prompt), sandbox port config, plan/active flags. Created by any authenticated user; the creator is seeded as `owner`.
- **Org membership** (`org_memberships`) — the `(user, org) → role` join. Role is one of `owner | admin | member | viewer` (`internal/model/org_membership.go:13`, default `owner`). This is the row that `RequireOrgAdmin`, `access.Actor.OrgRole`, and every downstream gate read.
- **Org invites** (`org_invites`) — pending invitations carrying a target email + role (+ optional team links). The only mechanism by which a second person joins an org.
- **API keys** (`api_keys`) — long-lived `hvl_sk_*` org-scoped service credentials with a scope set (`internal/model/api_key.go:51`). A machine principal, not a human role.
- **Proxy tokens** (`tokens`) — short-lived `ptok_*` JWTs minted against a credential for sandbox/MCP egress. Machine principals.

Principals that interact: **Owner** (creator, sole role that should touch org lifecycle/billing), **Admin** (manages members/invites/settings/keys/tokens), **Member/Viewer** (consume), and the **API-key** machine axis (scope-bounded).

Headline finding: the org has **no member-lifecycle API at all** — no role change, no member removal, no ownership transfer, no org deletion. Access can be *granted* (invite) but never *revoked or altered*. The owner/admin boundary is effectively nonexistent because no owner-only route exists among identity operations.

## 2. Backend endpoint inventory

Gate abbreviations: **Auth** = `MultiAuth` + `RequireEmailConfirmed` (any authenticated principal); **OrgAdmin** = `RequireOrgAdmin` (role `owner|admin`); **AdminOrKey** = `RequireOrgAdminOrAPIKey` (admin JWT, or *any* API key subject to a handler ceiling); **ScopeOrJWT(x)** = `RequireAPIKeyScopeOrJWT("x")` (API key needs scope `x`/`all`; *any* JWT passes ungated).

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|--------|------|---------------------|---------------|--------------|----------|
| POST | /v1/orgs | orgs.go:98 | Mutates (creates org + owner membership) | Auth only | ✅ self-serve tenant creation; creator=owner |
| GET | /v1/orgs/current | orgs.go:169 | Reads | Auth (any member) | ✅ |
| PATCH | /v1/orgs/current | orgs.go:193 | Mutates (name/logo/website/prompt/ports) | OrgAdmin | ✅ |
| GET | /v1/orgs/current/members | org_invites_list.go:52 | Reads (all members' email/name/role) | Auth (any member) | ⚠️ member PII exposed to all |
| POST | /v1/orgs/current/invites | org_invites_create.go:33 | Mutates (create invite) | OrgAdmin | ✅ (role capped admin/member/viewer) |
| GET | /v1/orgs/current/invites | org_invites_list.go:20 | Reads | OrgAdmin | ✅ |
| DELETE | /v1/orgs/current/invites/{id} | org_invites_revoke.go:28 | Mutates (revoke) | OrgAdmin | ✅ |
| POST | /v1/orgs/current/invites/{id}/resend | org_invites_revoke.go:79 | Mutates (new token + email) | OrgAdmin | ✅ |
| GET | /v1/invites/{token} | org_invites_public.go:27 | Reads (org name, inviter, role, email) | **Public** (unauthenticated; serve_routes.go:67) | ✅ token is the secret; 404s uniformly |
| POST | /v1/invites/{token}/accept | org_invites_public.go:60 | Mutates (creates membership at invite.Role) | Auth + email must match invite | ✅ |
| POST | /v1/invites/{token}/decline | org_invites_public.go:163 | Mutates (revoke) | Auth + email must match | ✅ |
| GET | /v1/api-keys | api_keys_list.go:28 | Reads (all keys: name/prefix/scopes/expiry/last-used) | **Auth only — no admin/scope gate** | ❌ any member enumerates key inventory |
| POST | /v1/api-keys | api_keys.go:67 | Mutates (mint key) | AdminOrKey (+ scope-ceiling for keys) | ✅ |
| DELETE | /v1/api-keys/{id} | api_keys_list.go:104 | Mutates (revoke) | AdminOrKey | ✅ |
| GET | /v1/tokens | tokens_list.go:23 | Reads (all proxy tokens: jti/cred/scopes/meta) | ScopeOrJWT("tokens") → **any JWT** | ⚠️ any member lists org tokens |
| POST | /v1/tokens | tokens_mint.go:30 | Mutates (mint proxy token) | ScopeOrJWT("tokens") + AdminOrKey | ✅ |
| DELETE | /v1/tokens/{jti} | tokens_revoke.go:26 | Mutates (revoke) | ScopeOrJWT("tokens") + AdminOrKey | ✅ |
| — | **PATCH /orgs/current/members/{id}** (role change) | **does not exist** | — | — | ❌ missing (see §4.1) |
| — | **DELETE /orgs/current/members/{id}** (remove) | **does not exist** | — | — | ❌ missing (see §4.1) |
| — | **transfer ownership** | **does not exist** | — | — | ❌ missing (see §4.1) |
| — | **DELETE /orgs/current** (delete org) | **does not exist** | — | — | ❌ missing (see §4.1) |

Cross-cutting note (belongs to the **billing** doc, flagged here because it is the sharpest owner-boundary break): `mountBillingRoutes` (serve_routes_billing.go) registers `POST /billing/checkout|verify|subscription/*` (apply-change, cancel, resume) with **no `RequireOrgAdmin`** — they sit in the plain member group. Any member (or viewer) can change or cancel the subscription. Per _MODEL.md billing is Owner-only. **CRITICAL**, deferred to the billing review.

### How the machine axis nests

`RequireOrgAdminOrAPIKey` (auth.go:174) lets **any** API-key request through and only admin-gates JWTs. The escalation guard for keys is *in the handler*: `APIKeyHandler.Create` calls `scopesWithinCeiling` (api_keys.go:99-104,160) so a key can only mint scopes ⊆ its own (`all` mints anything). This is tested (`api_keys_scope_ceiling_test.go`). For credentials/tokens the scope gate (`ScopeOrJWT`) is the ceiling — a `tokens`-scoped key can mint tokens, etc. So the key→key and key→token escalation vectors are closed. The residual concern is human→key (§4.4), not key→key.

## 3. Frontend screens & actions

Role source: `apps/web/lib/auth/auth-context.tsx` — `useAuth().activeOrg.role` from `GET /auth/me`. There is **no shared `isOrgAdmin`/`canManage` helper**; each screen re-inlines `role === "owner" || role === "admin"`, so it is easy to omit — and it *was* omitted on the two most sensitive org-mutation screens.

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---------------|--------|-------|-------------------------|-----------|
| settings/general/page.tsx | Edit org name/website/logo/company prompt | PATCH /v1/orgs/current | ❌ **No gate** — editable by any role incl. viewer | Admin-only (hide/disable for non-admin) |
| settings/environments/page.tsx | Edit org env vars | PATCH /v1/orgs/current | ❌ **No gate** | Admin-only |
| settings/teams/page.tsx (Members section) | View members + roles | GET /v1/orgs/current/members | ❌ query runs for everyone (not `enabled:isAdmin`); read-only chips | Admin-only view; no role/remove control exists |
| settings/teams/page.tsx (Invites section) | List/revoke/resend invites | GET/DELETE/POST …/invites… | ✅ inside `isAdmin` block | Admin ✅ |
| _components/team-settings-modals.tsx (InviteMemberModal) | Create invite | POST /v1/orgs/current/invites | ✅ (opened only from admin block); role options admin/member/viewer (**no owner**) | Admin ✅ |
| — API keys screen | — | — | **No UI exists** | Admin-only screen (to build) |
| — Tokens screen | — | — | **No UI exists** | Admin-only (or omit) |
| — Members role-change / remove | — | — | **No UI exists** (read-only chips only) | Admin (+ owner-touching → Owner) |
| — Org delete / transfer ownership / danger zone | — | — | **No UI exists** | Owner-only (to build) |

Settings nav (`_components/nav.tsx`) has no "Members", "API keys", "Tokens", "Security", or "Danger zone" entry — member/invite management lives entirely under the "Teams" screen; keys/tokens are backend-only.

## 4. Ambiguities & lapses (ranked)

### 4.1 — HIGH — No member-lifecycle management exists (cannot de-provision access)
There is **no endpoint** to change a member's role, remove a member, let a member leave, transfer ownership, or delete the org (confirmed: no `members/{id}` PATCH/DELETE, no org DELETE anywhere in `cmd/server` or `internal/handler`). Consequences:
- A **compromised or departed admin cannot be demoted or removed.** Their access is permanent until the DB is edited by hand.
- The **single creator-owner is immutable** — if that person leaves, the org has no owner-capable successor and no way to appoint one.
- Blast radius: every org. This is the opposite of privilege *escalation* — it is the inability to *de-escalate*, which is itself a serious access-control gap (no revocation path).

### 4.2 — MEDIUM — `GET /v1/api-keys` is ungated
`r.Get("/api-keys", …)` (serve_routes_v1.go:126) sits **outside** the `RequireOrgAdminOrAPIKey` group (which starts at :129). Any member/viewer JWT lists every API key's name, prefix, scopes, expiry, and last-used time. No secret leaks, but it exposes the org's full service-credential inventory (including which powerful `all`-scoped keys exist) to non-admins. `api_keys_list_test.go` asserts no role gate today. Should be admin-only.

### 4.3 — MEDIUM — `GET /v1/tokens` visible to any member
`ScopeOrJWT("tokens")` passes any JWT (auth.go:145-151), so any member lists all org proxy tokens (jti, credential_id, scopes, meta, remaining). Metadata leak of the credential-egress surface. Should be admin, matching mint/revoke.

### 4.4 — MEDIUM — Admin can mint an `all`-scoped, unattributed, admin-equivalent key
An admin (JWT) may create a key with scope `all`, which via `RequireOrgAdminOrAPIKey` bypasses the human admin gate on credentials/tokens/api-keys writes and, via `ScopeOrJWT`, all scoped writes. The `api_keys` row has **no `created_by`** (`internal/model/api_key.go:13-25`) — keys are purely org-scoped with no minting-user attribution. So: (a) a leaked `all` key = full org write compromise with no per-key owner in the audit trail, and (b) combined with 4.1, such a key survives its creator's (hypothetical) removal. Not an escalation *above* admin, but a durable, unattributed admin-power bearer credential. Recommend recording `created_by` and considering whether `all` should require owner.

### 4.5 — LOW/MEDIUM — Owner vs Admin boundary does not exist for identity ops
`RequireOrgAdmin` treats `owner` and `admin` identically and **no owner-only route exists**. So an admin is functionally equal to the owner for everything the API currently exposes (org profile, invites, keys, tokens). Admins can also invite unlimited additional admins (`isValidInviteRole` allows `admin`). This is safe *today* only because the owner-sensitive operations (role change, transfer, delete, billing) simply aren't implemented — the moment any of them ships it must be Owner-gated, and the current `owner|admin` predicate is the wrong gate for them.

### 4.6 — MEDIUM — Two org-profile mutations are UI-ungated (backend saves the org)
`settings/general` and `settings/environments` fire `PATCH /v1/orgs/current` with no `isAdmin` check in the component. The **backend correctly rejects** non-admins (`RequireOrgAdmin`), so this is defense-in-depth / UX, not an exploit — but a viewer sees a fully editable form that 403s on save. Mirror the gate client-side.

### 4.7 — LOW — Member roster (email/name/role) readable by any member
`GET /v1/orgs/current/members` is intentionally open ("Any member may call this"). Acceptable for most orgs but is member-PII exposure to viewers; worth an explicit decision.

### Positives (no escalation vector found)
- Invite roles are capped at `admin|member|viewer` — **`owner` cannot be granted by invite** (`org_invites.go:131`), and accept creates membership at exactly `invite.Role` with an email-match check. No invite→owner escalation.
- API-key scope ceiling is enforced and tested — no key→broader-key escalation.
- Org profile, invite CRUD, and token/key mint/revoke are all admin-gated.

## 5. Recommendation

Target principal per the role model, with mechanism:

| Action | Target principal | Mechanism |
|--------|------------------|-----------|
| Create org | any authenticated user | unchanged |
| Edit org profile | Org Admin | `RequireOrgAdmin` (already ✅); mirror in `general`/`environments` UI |
| List members | Org Admin (recommend) or member (operator's call) | add `RequireOrgAdmin` if roster is considered sensitive |
| Invite / list / revoke / resend | Org Admin | `RequireOrgAdmin` (already ✅) |
| **Change member role member↔admin** | **Org Admin** | NEW `PATCH /orgs/current/members/{userID}` gated `RequireOrgAdmin`; forbid setting/removing `owner` |
| **Promote to owner / demote owner / transfer ownership** | **Owner** | same route but require caller role `owner`; enforce **exactly one owner** invariant (transfer = atomic swap) |
| **Remove member** | **Org Admin** (but only Owner may remove an owner/last admin) | NEW `DELETE /orgs/current/members/{userID}`; block removing the sole owner |
| Create/list/revoke API key | Org Admin (human) / key within ceiling | keep `RequireOrgAdminOrAPIKey` for writes; **add `RequireOrgAdmin` (or admin-or-key) to `GET /api-keys`** — fix 4.2 |
| List tokens | Org Admin | move `GET /tokens` under an admin gate — fix 4.3 |
| Mint/revoke token | Org Admin / `tokens`-scoped key | unchanged ✅ |
| **Delete org** | **Owner** | NEW `DELETE /orgs/current` requiring `owner`; hard-confirm |
| Billing changes | **Owner** | (billing doc) add `RequireOrgOwner` to `/billing/*` |

Concrete building blocks:
- **Add an owner predicate.** Extend `internal/access` with `Actor.IsOwner()` and add a `middleware.RequireOrgOwner(db)` mirroring `RequireOrgAdmin` but checking `role == "owner"`. Route all ownership/lifecycle/billing mutations through it. This is the single missing gate the model demands.
- **Enforce the one-owner invariant** at the membership layer (partial unique index or a transactional check in the role-change handler) so transfer is an atomic demote-old + promote-new, and neither remove nor demote can leave the org ownerless.
- **Attribute API keys**: add `created_by_user_id` to `api_keys` for audit; consider requiring `owner` for the `all` scope.
- **Frontend**: introduce one shared `useOrgRole()` / `isOrgAdmin`/`isOrgOwner` helper in `auth-context.tsx` and consume it everywhere instead of re-inlining the string compare; add Members (role/remove), API-keys, and a Danger-zone (transfer/delete) screen, each gated by the matching predicate.

## 6. Deviations from the baseline model

- **No team-scoped tier here.** Org/membership/invite/key/token administration is inherently org-level; none of it composes into a channel, so it stays **Org Admin** and there is no team-scoped tier in the model for it. (Team management — including the team *member* PUT/DELETE routes — is likewise Org Admin, gated `RequireOrgAdmin` — flagged in the teams doc, not here.)
- **The model assumes owner-only operations exist to protect.** They currently don't (no role change, transfer, delete, or billing gate wired to `owner`). This doc therefore recommends *creating* the owner boundary rather than merely re-gating it — the baseline's "role changes touching OWNER + org deletion = OWNER" is a target state, not a present reality.
- **API keys as an admin-creatable powerful credential** matches the model's "creating them = admin," but the model's implicit expectation of attribution/scoping is unmet (no `created_by`, no user binding) — noted as 4.4.

## 7. Open questions for the operator

1. **Owner semantics on transfer/removal:** exactly one owner always, or allow multiple owners? (Determines the invariant and whether "promote to owner" is additive or a swap.)
2. **Who may remove a member — admin, or owner only?** And may an admin remove another admin? Recommend: admin removes member/viewer; only owner removes/demotes an admin or owner.
3. **Is the member roster (`GET /orgs/current/members`, emails included) intentionally readable by all members/viewers**, or should it be admin-only?
4. **Should the `all` API-key scope require owner** (not just admin), given it confers full org write power in one unattributed credential?
5. **Org deletion policy:** soft-delete/anonymize vs hard cascade, and grace period — before wiring `DELETE /orgs/current`.
6. **Should `viewer` be able to reach `PATCH /orgs/current` UI at all** — i.e. do we want client gating purely for UX, accepting the backend is authoritative?
