# Authorization Review — Master Synthesis & Specification

> **Read this first.** This is the authoritative map of "who can do what" across every Hivy API endpoint and the spec engineers build the unified authorization layer from. It (1) inventories every route and its gate today, (2) finalizes the unified role model and resolves the org-vs-team boundary question, (3) specifies the single enforcement layer to build, (4) ranks the worst authorization lapses product-wide, and (5) gives a phased roadmap. The shared vocabulary is [`_MODEL.md`](./_MODEL.md); per-feature detail lives in the sibling files linked in the [Table of Contents](#8-table-of-contents).

---

## 0. TL;DR — the decision

- **Adopt a four-principal model (three human roles + one non-human axis):** Org Owner ⊃ Org Admin ⊃ Member, plus Automated. There is **no Team Admin role** — authority is not tiered inside a team.
- **The crux, resolved:** *Org Admins own the org structure & shared catalog; any member of a team freely operates that team's channels and agents; Members consume; Owner touches money.* **Team management, knowledge sources, connections/credentials, members/invites** → **Org Admin**. A team's **channels and agents** (create/edit/delete/configure/assign) → **any Member of that team**, via `CanManageTeamResource(teamID) = IsOrgManager() || IsTeamMember(teamID)`. **Agent create/edit/delete** → **any member** (agents are org-global; the read-visibility model scopes which agents a member sees). Billing & org lifecycle → **Owner**.
- **No new role or migration is needed.** Team resources gate on `CanManageTeamResource(teamID)` — team membership already lives in `team_members`, so the inert `team_members.role` column stays unused (or can be dropped). Billing gets a new `IsOrgOwner()` predicate (none exists today; `RequireOrgAdmin` currently collapses owner+admin). Route channel-composition writes through a capability matrix.
- **Today's enforcement is ad-hoc and leaks.** `RequireAPIKeyScopeOrJWT(scope)` lets **any JWT (any org member, including `viewer`) pass unchecked**, so the scope gate only constrains API keys. Several org-level mutations (billing, plugin install, agent-plugin enable, connection create, sandbox exec, sandbox-template CRUD, system tasks) sit behind **no admin gate at all**.

---

## 1. Master endpoint → gate inventory

**Legend — CURRENT gate**

- `MultiAuth` — every `/v1` route requires a valid JWT **or** API key + `RequireEmailConfirmed`. Baseline, omitted from rows below.
- `ScopeOrJWT(x)` = `RequireAPIKeyScopeOrJWT("x")` — **API keys** must carry scope `x` (or `all`); **any JWT passes unchecked**. This is *not* a human-role gate.
- `OrgAdmin` = `RequireOrgAdmin` — JWT caller must be org `owner|admin`. (No API-key bypass.)
- `OrgAdminOrKey` = `RequireOrgAdminOrAPIKey` — JWT caller must be org `owner|admin`; **any API key passes** (subject to handler scope ceiling).
- `ResolveUser` — loads the user; **not** an authorization gate on its own (any member passes). Inline handler checks may follow.
- `AdminSecret` = `RequireAdminSecret` — static ops secret, not a user role.
- `Token`/`AppSecret`/`RuntimeSecret`/`HMAC` — machine auth for the runtime/proxy/webhook surface.
- **Correct?** ✅ matches role model · ⚠️ ambiguous/inconsistent · ❌ wrong (under-gated).

### 1.1 Organization, members, teams, invites — → [organization-and-members.md](./organization-and-members.md)

| Method | Path | Mutates? | CURRENT gate | Correct? |
|---|---|---|---|---|
| POST | /v1/orgs | mutate | JWT only (no org ctx) | ✅ any authed user may create an org |
| GET | /v1/orgs/current | read | any member | ✅ |
| PATCH | /v1/orgs/current | mutate | `OrgAdmin` | ✅ |
| GET | /v1/orgs/current/members | read | **any member** | ⚠️ member roster/email visible to all |
| POST | /v1/orgs/current/invites | mutate | `OrgAdmin` | ✅ |
| GET | /v1/orgs/current/invites | read | `OrgAdmin` | ✅ |
| DELETE | /v1/orgs/current/invites/{id} | mutate | `OrgAdmin` | ✅ |
| POST | /v1/orgs/current/invites/{id}/resend | mutate | `OrgAdmin` | ✅ |
| GET/POST | /v1/orgs/current/teams | read/mutate | `OrgAdmin` | ✅ |
| GET/PATCH/DELETE | /v1/orgs/current/teams/{id} | read/mutate | `OrgAdmin` | ✅ team mgmt = Org Admin (members may *read* own team — see teams.md) |
| PUT/DELETE | /v1/orgs/current/teams/{id}/members/{userID} | mutate | `OrgAdmin` | ✅ roster mgmt = Org Admin |
| POST | /v1/invites/{token}/accept·decline | mutate | JWT (user-scoped) | ✅ |
| GET | /v1/invites/{token} | read | public (token) | ✅ |
| GET | /v1/audit | read | `OrgAdmin` | ✅ |
| GET | /v1/usage · /dashboard · /reporting · /generations[/id] | read | **any member** | ⚠️ org-wide cost/usage to all members |

### 1.2 Channels — → [channels.md](./channels.md)

Route middleware `ScopeOrJWT("channels")`; **real gate is inline** via `authorizeChannel(requireManage)` where `canManage = apiKey || orgOwner|admin || channel_members.role=="owner"`. Channel "owner" = the creator (set at Create).

| Method | Path | Mutates? | Inline gate | Correct? |
|---|---|---|---|---|
| GET | /v1/channels | read | visibility-scoped (member sees usable) | ✅ |
| POST | /v1/channels | mutate | **any member** (creator→owner) | ⚠️ any member can spawn channels incl. team-scoped |
| GET | /v1/channels/{id} | read | view (member of team/channel) | ✅ |
| PATCH | /v1/channels/{id} | mutate | manage (owner/org-admin) | ✅ |
| DELETE | /v1/channels/{id} | mutate | manage | ✅ |
| POST | /v1/channels/{id}/join | mutate | view + public | ✅ |
| PUT | /v1/channels/{id}/members/{userID} | mutate | manage | ✅ (but see team-membership gap) |
| DELETE | /v1/channels/{id}/members/{userID} | mutate | self OR manage | ✅ |
| GET | /v1/channels/{id}/environment-variables | read | view | ⚠️ env-var **values** to any channel viewer |
| POST/PATCH/DELETE | /v1/channels/{id}/environment-variables[/{name}] | mutate | manage | ✅ |
| GET | /v1/channels/{id}/rag-sources | read | view | ✅ |
| PUT | /v1/channels/{id}/rag-sources | mutate | **manage** | ✅ (compose = manage) |
| GET | /v1/channels/{id}/agents | read | view | ✅ |
| POST | /v1/channels/{id}/agents | mutate | **view only** (`CanUseChannel`) | ❌ **inconsistent** — compose gated weaker than rag-sources |
| DELETE | /v1/channels/{id}/agents/{agentID} | mutate | **view only** | ❌ same |
| GET | /v1/channels/{id}/sessions | read | view | ✅ |

### 1.3 Sessions — → [channels.md](./channels.md) / sessions

Route `ScopeOrJWT("sessions")`; inline per-session channel-access checks (see feature file for confirmation).

| Method | Path | Mutates? | CURRENT gate | Correct? |
|---|---|---|---|---|
| GET | /v1/sessions · /sessions/{id} · /{id}/usage · /{id}/events · /{id}/name-updates | read | inline channel access | ✅/⚠️ (verify) |
| POST | /v1/sessions | mutate | inline channel access | ✅ |
| PATCH/DELETE | /v1/sessions/{id} | mutate | inline | ✅/⚠️ |
| POST | /v1/sessions/{id}/messages·input-responses·interrupt·transcriptions | mutate | inline | ✅ |
| POST | /v1/sessions/{id}/sandbox-access | mutate | inline | ⚠️ grants sandbox reach |
| POST/PUT/DELETE | /v1/sessions/{id}/participants[/{userID}] | mutate | inline | ⚠️ verify |

### 1.4 Agents, catalog, plugins, triggers, schedules, sandboxes — → [agents-plugins.md](./agents-plugins.md) / [automations.md](./automations.md)

All under `ScopeOrJWT("agents")`. A nested `OrgAdmin` group covers the agent/trigger/schedule **write block** and database-integration writes. Everything else in this group is **any member**.

| Method | Path | Mutates? | CURRENT gate | Correct? |
|---|---|---|---|---|
| GET | /v1/agents[...] · /agents/catalog[...] · /agents/models · /agents/{id} | read | any member | ✅ |
| GET | /v1/agents/{id}/plugins · /agents/{id}/trigger-deliveries[...] | read | any member | ✅ |
| POST | /v1/agents/{id}/plugins/{slug} (EnableForAgent) | mutate | **any member who can *view* the agent** (visibility gate, no admin) | ❌ modifies agent capability |
| DELETE | /v1/agents/{id}/plugins/{slug} (DisableForAgent) | mutate | same | ❌ |
| GET | /v1/triggers[/id] · /v1/schedules[/id] | read | any member | ✅ |
| POST | /v1/agents | mutate | `OrgAdmin` | ✅ |
| POST/DELETE | /v1/agents/catalog/{slug}/install | mutate | `OrgAdmin` | ✅ |
| PATCH/DELETE | /v1/agents/{id} · /{id}/model | mutate | `OrgAdmin` | ✅ |
| PUT | /v1/agents/{id}/connections/{connectionID}/resources | mutate | `OrgAdmin` | ✅ |
| POST/PATCH/DELETE | /v1/triggers[/{id}] | mutate | `OrgAdmin` | ✅ |
| POST/PATCH/DELETE | /v1/schedules[/{id}] | mutate | `OrgAdmin` | ✅ |
| GET | /v1/plugins · /plugins/{slug} | read | any member | ✅ |
| POST | /v1/plugins/{slug}/install | mutate | **any member** | ❌ installs org-wide plugin |
| DELETE | /v1/plugins/{slug}/install | mutate | **any member** | ❌ uninstalls org-wide plugin |
| POST/GET/PUT/DELETE | /v1/sandbox-templates[...] (Create/Update/Delete/build/retry) | mutate | **any member** | ❌ builds org-wide images |
| GET | /v1/sandboxes[/{id}] | read | **any member** (verify scoping) | ⚠️ |
| POST | /v1/sandboxes/{id}/stop·exec · DELETE /{id} | mutate | **any member** (verify) | ❌ if unscoped: cross-team code exec |
| POST | /v1/system/tasks/{taskName} | mutate | **any member** | ❌/⚠️ privileged job runner |
| GET | /v1/database-integrations | read | any member | ⚠️ |
| POST/PUT/DELETE | /v1/database-integrations[/{id}...] | mutate | `OrgAdmin` | ✅ |

### 1.5 Knowledge sources (RAG) — → [knowledge-sources.md](./knowledge-sources.md)

`mountRAGRoutes`: `ResolveUser`; reads any member; write group `OrgAdmin`.

| Method | Path | Mutates? | CURRENT gate | Correct? |
|---|---|---|---|---|
| GET | /v1/rag/integrations · /connections/{id}/scopes · /sources[...] · /sources/{id}/documents · /channels | read | any member (source-visibility scoped) | ✅ |
| POST | /v1/rag/search | read | any member (usable-source scoped) | ✅ |
| POST/PATCH/DELETE | /v1/rag/sources[/{id}] | mutate | `OrgAdmin` | ✅ create/delete = org catalog |
| PUT | /v1/rag/sources/{id}/channels (SetSourceChannels) | mutate | `OrgAdmin` | ✅ grant = Org Admin (knowledge stays centralized) |
| POST | /v1/rag/sources/{id}/sync·prune · /website/discover-sections | mutate | `OrgAdmin` | ✅ |

### 1.6 Connections & credentials & tokens — → [connections-credentials.md](./connections-credentials.md)

Connections live in a **separate router** (`setupConnectRoutes`): `RequireAuth + ResolveUser + ResolveOrgFlexible`, **no admin gate**.

| Method | Path | Mutates? | CURRENT gate | Correct? |
|---|---|---|---|---|
| POST | /v1/integrations/{id}/connect-session · /connections | mutate | **any member** | ❌ any member links OAuth connection |
| GET | /v1/connections[/{id}] · /{id}/resources/{type} | read | any member (metadata only — secrets stripped) | ⚠️ any member enumerates org connections |
| PUT | /v1/connections/{id}/resources | mutate | **any member** | ❌ |
| POST | /v1/connections/{id}/reconnect-session | mutate | **NOT org-scoped** (`id` only) | ❌❌ **cross-org IDOR** — see LAPSE-1 |
| PATCH | /v1/connections/{id}/webhook-configured | mutate | **any member** | ⚠️ |
| DELETE | /v1/connections/{id} | mutate | **any member** | ❌ any member revokes org connection |
| GET | /v1/credentials[/{id}] | read | any member (metadata only, no key material) | ⚠️ |
| POST/DELETE | /v1/credentials[/{id}] | mutate | `OrgAdminOrKey` | ✅ |
| GET | /v1/tokens | read | any member | ⚠️ |
| POST/DELETE | /v1/tokens[/{jti}] | mutate | `OrgAdminOrKey` | ✅ |
| GET/PUT/DELETE | /v1/admin/integrations[...] · /admin/system-credentials[...] · /admin/llm-providers | both | `AdminSecret` | ✅ ops-only |

### 1.7 Billing & subscription — → [billing.md](./billing.md)

`mountBillingRoutes`: **no gate beyond MultiAuth + org resolve** — any member *and any API key* passes; handlers do **no role check**.

| Method | Path | Mutates? | CURRENT gate | Correct? |
|---|---|---|---|---|
| POST | /v1/billing/checkout | mutate | **any member/key** | ❌ Owner-only per model |
| POST | /v1/billing/verify | mutate | **any member/key** | ❌ |
| GET | /v1/billing/subscription | read | any member | ⚠️ |
| POST | /v1/billing/subscription/preview-change·init-upgrade·apply-change·cancel·resume | mutate | **any member/key** | ❌ Owner-only |
| GET | /v1/plans | read | public | ✅ |

### 1.8 Brands, sheets, apps, memory, canvas, uploads — → [memories-content.md](./memories-content.md) & feature files

| Method | Path | Mutates? | CURRENT gate | Correct? |
|---|---|---|---|---|
| GET | /v1/orgs/current/brands[...] | read | `ScopeOrJWT("brands")` any member | ✅ |
| POST/PATCH/DELETE | /v1/orgs/current/brands[...] | mutate | `OrgAdminOrKey` | ✅ |
| * | /v1/sheets[...] | both | `ScopeOrJWT("sheets")` + `ResolveUser` + per-sheet `RequireChannelAccess` | ✅/⚠️ (verify channel binding) |
| GET | /v1/sheets/{sheetID}/live | read | live-token (self-validated) | ✅ |
| * | /v1/apps[...] | both | `ResolveUser` only + inline channel checks | ⚠️ verify create/launch/archive scoping |
| GET | /v1/memories[...] · /directives[...] · /observations[...] | read | any member, **visibility-scoped** to usable channels | ✅ |
| POST/PATCH/DELETE | /v1/memories[...] · /directives[...] · /observations[...] | mutate | **inline admin gate** (`canManageMemoryResources` = owner/admin/key) | ✅ directives (verbatim rules) are admin-only |
| GET/POST | /v1/canvas/* | both | any member + session-visibility scope | ⚠️ verify |
| POST | /v1/uploads/sign·upload · /assets/upload · /transcriptions · /images/describe | mutate | `ResolveUser` | ⚠️ member-level, acceptable |

### 1.9 Machine / runtime / public surface (non-human — separate axis)

Not human-role gated by design; listed for completeness. Auth = webhook HMAC, sandbox **runtime secret**, app **secret**, proxy **token**, or `AdminSecret`.

| Surface | Auth |
|---|---|
| `/healthz` `/readyz`, `/v1/providers*`, `/v1/models`, `/v1/integrations/available`, `/v1/catalog/*`, `/v1/plans` | public (no auth) |
| `/auth/*`, `/oauth/*` | credential/OAuth flows + rate limit |
| `/internal/webhooks/nango`, `/incoming/webhooks/{provider}/{connectionID}` | HMAC-verified |
| `/internal/git-credentials`, `/internal/*-proxy/{agentID}`, `/internal/database-proxy/*`, `/internal/agents/{agentID}/**` (drive, canvas, preview-env, apps-template) | sandbox runtime bearer secret |
| `/internal/apps/{appID}/v1/*` | app secret bearer (`authApp`) |
| `/v1/proxy/*` | proxy `Token` + credits |
| `/v1/admin/*` | `AdminSecret` |

---

## 2. The finalized unified role model

Four principals. The three human roles are a strict hierarchy (each a superset of the next); **Automated** is a separate non-human axis that cross-cuts all of them. There is **no Team Admin role** — authority over a team's resources is conferred by team membership itself.

### 2.1 Org Owner
The only principal that touches **money and org lifecycle**. Capabilities = all of Org Admin, **plus**: billing checkout/subscription change/cancel/resume, transfer ownership, delete org. There must be ≥1 owner; the earliest member is owner today (`org_memberships.role` default `owner`).

### 2.2 Org Admin
Owns the **org catalog** — the resource *types* teams compose from. Create/edit/delete: **agents** (incl. catalog install/uninstall, model, agent-plugin enable/disable, connection-resource grants), **knowledge sources**, **connections/credentials/tokens**, **database integrations**, **plugins** (install/uninstall), **sandbox templates**, **brands**, **members/invites/teams**, **org settings**, **API keys**. Reads all org data (audit, usage, all channels). **Not** billing.

### 2.3 Team resource authority *(no separate role — conferred by team membership)*
There is **no Team Admin role**. Any active member of a team fully manages that team's **channels and their composition**, for channels whose `team_id` is a team they belong to. Enforced by one predicate, `CanManageTeamResource(teamID) = IsOrgManager() || IsTeamMember(teamID)` — no new column, no migration; `team_members` membership is the grant:
- create/archive channels in the team; edit channel settings; channel membership;
- **assign existing org agents** to the team's channels; set channel env-vars, memory mission, directives;
- **create/edit/delete agents** — agents are org-global, so this is a plain org-member action; the read-visibility model scopes which agents a member sees.

**Cannot** (these stay Org Admin): create/delete a **knowledge source** or grant one to a channel, create/delete connections/credentials/plugins/templates, team management (create/delete teams, add/remove team members), touch billing, or reach channels of teams they don't belong to.

**Shared-agent caveat.** Because agents are org-global and shared across teams, a member of *any* team an agent is assigned to can edit/delete it. If strict per-team agent isolation is needed later, scope agents to teams (add ownership) — a future option, not built now.

### 2.4 Member
Uses channels they belong to (team member, or any member for team-less/external channels): create sessions, chat, invoke assigned agents, read channel-scoped content, self-join public channels, upload. Within any team they belong to, they also manage that team's channels/agents (§2.3). Beyond their teams, a member has **no org-level configuration** — no catalog/secrets, team management, or billing.

### 2.5 Automated actor / API key
Non-human. Gated by **API-key scopes** (`channels|agents|sessions|credentials|tokens|sheets|memories|brands|all`) and, on the agent-runtime path, by the injected `_hivy_actor_user_id` resolved to an `Actor` (`internal/access`). This axis is **orthogonal** to human role: an API key is not "an admin," it is a scoped machine principal, and its ceiling should be explicit per scope. Where a key currently bypasses an admin gate (`OrgAdminOrKey`, and billing/plugins where no gate exists), that bypass must be a deliberate, documented scope — not an accident of middleware.

### 2.6 The org-vs-team principle — resolved decisively

> **Org Admins own the org structure & shared catalog. Team members freely operate their teams' channels and agents. Members consume. Owner pays.**

The operator's recurring question is where team administration ends and org administration begins. The dividing line is **org structure/catalog (Org Admin) vs. a team's own channels & agents (any team member)**:

| Action | Principal | Why |
|---|---|---|
| Create/delete a **knowledge source**; **grant** a source to a channel | **Org Admin** | A source is org-wide ingestion (cost, external creds, PII surface). Both creation and channel-grant stay centralized. *(Today's source→channel PUT is org-admin — correct; the channel-side PUT bypasses it — see LAPSE-9.)* |
| Create/edit/delete an **agent** | **any Member** | Agents are org-global; the read-visibility model scopes which a member sees. *(Today: `OrgAdmin` — this is a **relaxation**; see agents-plugins.md + the shared-agent caveat in §2.3.)* |
| **Install a catalog agent** / **install a plugin** | **Org Admin** (plugin/connection-bearing) or **Member** (plain agent install) | Plugins wire org credentials → Org Admin; a plain catalog agent install can be a member action. Flagged decision — see agents-plugins.md §7. |
| **Assign** an existing agent to a channel | **any Member of that team**, or Org Admin | Composition inside the team's channel, via `CanManageTeamResource`. *(Today: **any channel user** — under-gated; see LAPSE-6.)* |
| Create a **connection/credential/plugin/sandbox template** | **Org Admin** | New capability/secret/spend. *(Today: connections & plugins & templates = any member — see LAPSES 4-6.)* |
| **Channel** create/settings, membership, env-vars, memory mission | **any Member of that team**, or Org Admin | Composition inside a team's channel, via `CanManageTeamResource`. |
| **Team management** (create/delete teams, add/remove members) | **Org Admin** | Defines the org structure teams sit in. |
| **Use** a channel (session, chat, invoke) | **Member** of that channel | Consumption. |
| **Billing / subscription / org lifecycle** | **Owner** | Money. *(Today: any member — see LAPSE-2.)* |

**Worked examples.**
- *Knowledge source added, then used by the support team's channel:* an Org Admin creates the source (`POST /rag/sources`) **and** grants it to `#support` — both the channel-side `PUT /channels/{id}/rag-sources` and the source→channel `PUT /rag/sources/{id}/channels` are Org-Admin actions. Today the channel-side PUT succeeds for mere channel *owners*, bypassing that gate — the fix is to require Org Admin there too (kill the channel-owner grant path).
- *Catalog agent install + channel assignment:* an Org Admin installs "Anna QA" org-wide (`POST /agents/catalog/{slug}/install`). **Any member of the Engineering team** assigns Anna to `#eng-reviews` (`POST /channels/{id}/agents`) via `CanManageTeamResource(team_id)` — today any channel *user* can, which is too weak (should require team membership).
- *Channel-agent assignment vs. billing:* assignment is a team-scoped compose → **team member**. Billing is money → **Owner**. Today both are effectively "any member," collapsing distinct authority levels into one.

### 2.7 Deviations from `_MODEL.md`
1. **None on the role model.** This review adopts `_MODEL.md`'s decision in full: no Team Admin role. The inert `team_members.role` column is *not* promoted to an authz signal — team membership alone gates team resources via `CanManageTeamResource`, so no schema change is needed; the column stays unused or can be dropped.
2. `_MODEL.md` frames scope-gate leakage as "member reads leak." **Refinement:** the same `ScopeOrJWT` pattern also fronts several **writes** that then have *no* inner admin gate (plugin install, agent-plugin enable, sandbox templates, system tasks) — so it is a write-escalation surface, not only a read leak.

---

## 3. Enforcement architecture recommendation

Replace scattered `RequireOrgAdmin` / inline role reads with **one capability layer** evaluated by shared predicates. Backend stays authoritative; frontend mirrors.

### 3.1 The capability matrix `(role, resource, action) → allow`

Model authority as data, not control flow. A single table (Go map or DB-backed) keyed by `(principal, resource, action)`:

```
resource       action            Owner Admin TeamMember(team) Member Key(scope)
billing        write               ✔     ✘        ✘             ✘      scope:billing?
org.settings   write               ✔     ✔        ✘             ✘      ✘
member/team    manage              ✔     ✔        ✘             ✘      ✘
agent          create/delete       ✔     ✔        ✔(any member) ✔      scope:agents
agent          assign→channel      ✔     ✔        own-team-chan ✘      scope:agents
knowledge      create/delete       ✔     ✔        ✘             ✘      scope?
knowledge      grant→channel       ✔     ✔        ✘             ✘      scope?
connection     create/revoke       ✔     ✔        ✘             ✘      scope?
plugin         install/enable      ✔     ✔        ✘             ✘      scope:agents
sandboxtmpl    create/build        ✔     ✔        ✘             ✘      scope:agents
channel        create/manage       ✔     ✔        own-team      ✘      scope:channels
channel        use                 ✔     ✔        own-team      member scope:channels
session/sheet  crud                ✔     ✔        own-team      chan-member scope
```

Note: **agent create/delete is a plain org-member action** (agents are org-global; visibility scoping, not the matrix, limits which agents a member sees), while **agent assign→channel** and all channel-scoped rows are gated on team membership of the target channel's team. **Team management and knowledge grants stay Org-Admin-only.** Evaluation returns `allow` and, for team/channel-scoped rows, calls the relevant predicate (`CanManageTeamResource(teamID)`, `CanUseChannel`). Shared HTTP middleware `Require(resource, action)` looks up the actor, resolves the resource's `team_id`/`channel_id`, and consults the matrix — one code path for the API surface, the MCP tool surface, and the frontend capability endpoint.

### 3.2 Extending `internal/access.Actor`

`Actor{UserID, OrgID, OrgRole}` gains team awareness:

```go
// Team-resource predicate — NO new column; reads existing team_members membership.
func (a *Actor) IsTeamMember(ctx, db, teamID uuid.UUID) bool {
    if a == nil { return false }
    // active row in team_members for (a.UserID, teamID), teams.archived_at IS NULL
}

func (a *Actor) CanManageTeamResource(ctx, db, teamID uuid.UUID) bool {
    return a.IsOrgManager() || a.IsTeamMember(ctx, db, teamID)
}

// Billing / org lifecycle — NEW predicate (RequireOrgAdmin collapses owner+admin today).
func (a *Actor) IsOrgOwner() bool { return a != nil && a.OrgRole == "owner" }

// Channel-composition predicate: may this actor configure THIS channel?
func (a *Actor) CanManageChannel(ctx, db, ch model.Channel) bool {
    if a.IsOrgManager() { return true }
    if ch.TeamID != nil { return a.IsTeamMember(ctx, db, *ch.TeamID) }
    return /* channel_members.role=="owner" (creator) — team-less fallback */
}
```

`CanManageChannel` becomes the single truth behind HTTP `canManageChannel` and the MCP path, closing the drift `_MODEL.md` warns about.

### 3.3 No migration — team membership is the grant

There is **no Team Admin migration**. Team-resource authority is realized entirely from existing data:

1. **No schema change.** `CanManageTeamResource(teamID)` reads the existing `team_members` rows; membership *is* the grant. The inert `team_members.role` column is not promoted to an authz signal — leave it unused or drop it in a later cleanup.
2. **(Optional) prune `channel_members.role`.** With team membership as the compose authority, per-channel "owner" is only meaningful for **team-less** channels (personal/external). Keep it solely for that fallback; document that team-scoped channels ignore it.
3. **Predicate + matrix**, as §3.1–3.2. No new table, no new column.
4. **New predicate for billing.** Add `Actor.IsOrgOwner()` (and a `RequireOrgOwner` middleware) so billing/org-lifecycle can gate on owner rather than the owner+admin `RequireOrgAdmin`.

### 3.4 Retiring ad-hoc checks

- Replace each `r.Use(middleware.RequireOrgAdmin)` block and each inline `isOrgManager`/`canManageChannel` with `r.Use(middleware.Require("<resource>", "<action>"))`.
- **Close the write holes first** (these have *no* gate today): billing → `Require("billing","write")` (Owner); plugins install/uninstall & agent-plugin enable/disable → `Require("plugin","install")`; sandbox-templates writes & sandboxes exec/delete → admin/`Require`; system tasks → admin-or-machine; connections create/revoke → `Require("connection","create")`.
- **Fix scope-gate leakage:** `ScopeOrJWT` must be paired with an inner human-role predicate on every mutating route it fronts; it should never be the *only* gate on a write.
- **API-key ceiling:** make each scope's allowed `(resource,action)` explicit in the matrix so `OrgAdminOrKey` bypass is intentional per scope, not blanket.

### 3.5 Frontend mirroring

Expose a single **`GET /v1/capabilities`** (or fold into `/orgs/current`) returning the resolved actor's booleans (`canBill`, `canManageOrg`, `teamMemberOf:[teamIDs]`, per-channel `canManage`). The UI renders from that — never from a client-side role guess — and the button-hiding is defense-in-depth over the authoritative matrix.

**Today the frontend barely gates at all.** There is no role hook or permission context; role lives only as `activeOrg.role` on the auth context (`apps/web/lib/auth/auth-context.tsx`). The **only** role-gated screen is Teams (`app/w/settings/teams/*`, `isAdmin = role ∈ {owner,admin}`). Every other admin surface — create/install agents, create knowledge sources, grant sources to channels, assign agents to channels, create connections/credentials, billing/subscription — renders for **any member**, so the backend is the *sole* real gate (which is why LAPSES 1-7 are exploitable, not merely cosmetic). The consolidated rule for the rebuild: **hide what the matrix denies, but the backend always re-checks.**

---

## 4. Cross-feature severity summary — top authorization lapses

Ranked by the `_MODEL.md` severity vocabulary. Each links to the feature file that details it; findings below are from a direct handler sweep (feature files corroborate).

| # | Severity | Lapse | Who can do what they shouldn't | File |
|---|---|---|---|---|
| 1 | **CRITICAL** | **Cross-org IDOR — `CreateReconnectSession`.** `internal/handler/connections_session.go:76` queries the connection by `id AND revoked_at IS NULL` with **no `org_id`** (unlike Get/Revoke/UpdateResources). Any authenticated user in **any** org can pass any connection UUID and receive a Nango reconnect OAuth token for it. | account takeover of another org's external integration (OAuth re-auth) | [connections-credentials.md](./connections-credentials.md) |
| 2 | **CRITICAL** | **Billing is ungated.** `mountBillingRoutes` adds no role gate; handlers do no role check. Any member/viewer — *and any scoped API key* — can start checkout, upgrade, **cancel**, or resume the subscription. | non-admin cancels/changes the paid plan (money + denial-of-service) | [billing.md](./billing.md) |
| 3 | **CRITICAL** | **Sandbox `Stop`/`Exec`/`Delete` ungated and unscoped.** `sandboxes_ops.go:27/68/128` filter by `org_id` only — no visibility/role gate (the *read* path `List`/`Get` does gate by visible-agent). Any member runs **arbitrary shell** in, or deletes, any sandbox in the org, including agent-less/other-team sandboxes. | cross-team remote code execution | [agents-plugins.md](./agents-plugins.md) |
| 4 | **HIGH** | **Connections create/revoke ungated.** `setupConnectRoutes` = `ResolveUser` only; no inline check. Any member can link, **revoke** (no creator check), or edit resources on org OAuth connections. | member revokes prod connections → org-wide agent outage; links attacker connection | [connections-credentials.md](./connections-credentials.md) |
| 5 | **HIGH** | **Plugin install/uninstall + sandbox-template writes ungated.** `POST/DELETE /plugins/{slug}/install` sit on the base group (no admin, no scope); all `/sandbox-templates` writes (incl. arbitrary `build_commands` that run as shell) have no role/visibility gate. Agent-plugin `enable/disable` is gated only to *visible-agent* members, not admins. | member mutates org-wide plugins, injects build commands, rewires agent capabilities | [agents-plugins.md](./agents-plugins.md) |
| 6 | **HIGH** | **Channel-agent assignment gated weaker than everything adjacent.** `AssignChannelAgent`/`Unassign` require only `CanUseChannel` (view), while sibling `SetChannelRAGSources` requires manage. Any channel *user* changes which agents run in the channel. | member adds/removes agents (incl. default-agent swap) in channels they merely use | [channels.md](./channels.md) |
| 7 | **MEDIUM** | **`system/tasks/{taskName}` run by any member.** Privileged server-side task runner (spends org credits) behind `ScopeOrJWT("agents")` only, no role check. | member triggers system tasks / burns credits | [automations.md](./automations.md) |
| 8 | **MEDIUM** | **Org-wide reads open to all members:** `/orgs/current/members` (roster+email), `/usage` `/reporting` `/dashboard` `/generations` (org-wide cost, per-request `user_id`) — while the adjacent `/audit` is deliberately admin-gated; plus `/api-keys` `/tokens` `/credentials` `/database-integrations` list metadata, and channel `environment-variables` **values** to any channel viewer. | cost/PII/roster/secret-metadata disclosure to non-admins | multiple |
| 9 | **MEDIUM** | **Channel composition mis-gated on the channel-owner shortcut, not team membership.** `canManageChannel` treats a channel *owner* as sufficient (and channel-agent assign is view-gated — see #6), so composition authority neither requires team membership nor is consistently gated. Fix: gate all team-channel composition on `CanManageTeamResource(team_id)`. *(Team management and knowledge-source grants correctly remain Org-Admin-only — not lapses.)* | inconsistent channel-compose authority; channel-owner ad-hoc principal | [channels.md](./channels.md), [knowledge-sources.md](./knowledge-sources.md) |
| 10 | **MEDIUM** | **Any member can create channels**, including team-scoped ones, becoming their `owner`. | member self-provisions channels / claims ownership | [channels.md](./channels.md) |
| 11 | **LOW** | **Frontend gates only the Teams screen** (`isAdmin`); every other admin action (create agent/source/connection, install catalog, assign agents, billing) renders for any member, so the backend is the *sole* gate — fatal wherever the backend is also permissive (LAPSES 1-7). | UI exposes admin actions to members | (frontend) |

> **Corroborated by direct handler sweep.** Sessions, sheets, apps, memory/directives, and canvas were verified to enforce channel/team scope *inline* (memory/directive **writes are admin-gated**, not member-writable), and no cross-org `org_id` gap exists in those subsystems — the IDOR in #1 is the single cross-org query gap found. Team-less (`team_id NULL`) and `external` channels are org-wide by design, which bounds member isolation to team-scoped channels.

---

## 5. Roadmap — from ad-hoc to unified

**Phase 0 — Stop the bleeding (days).** Add the missing gates on the CRITICAL/HIGH holes with today's primitives, before the matrix lands:
- [ ] **Cross-org IDOR (LAPSE-1):** add `AND org_id = ?` to `CreateReconnectSession` (`connections_session.go:76`) — this is a live vulnerability, patch first.
- [ ] Billing: wrap `mountBillingRoutes` writes in an Owner check (new `RequireOrgOwner`).
- [ ] Connections: wrap create/revoke/resources/reconnect in `RequireOrgAdmin`; add a creator/admin check on Revoke.
- [ ] Plugins install/uninstall + agent-plugin enable/disable → `RequireOrgAdmin`.
- [ ] Sandbox-templates writes + `/sandboxes/{id}/exec·stop·delete` → `RequireOrgAdmin` (or confirm inline session/channel scoping and gate accordingly).
- [ ] `system/tasks` → `RequireOrgAdmin` or machine-only.
- [ ] Channel-agent assign/unassign → `requireManage` (parity with rag-sources).

**Phase 1 — Build the team-resource predicate + the matrix (the core).**
- [ ] **No migration** — `CanManageTeamResource` reads the existing `team_members` (§3.3).
- [ ] `Actor.IsTeamMember(teamID)`, `Actor.CanManageTeamResource(teamID)`, `Actor.CanManageChannel(channel)`, `Actor.IsOrgOwner()` in `internal/access`.
- [ ] Capability matrix `(role,resource,action)` + `middleware.Require(resource,action)`.
- [ ] Point HTTP `canManageChannel` and the MCP path at the shared predicate (kill drift).

**Phase 2 — Rebalance channel compose to team membership.**
- [ ] Channel-agent assign, channel membership, channel create/settings/env/memory → **team-member-or-Org-Admin** via `CanManageTeamResource` in the matrix.
- [ ] `PUT /channels/{id}/rag-sources` and `PUT /rag/sources/{id}/channels` → **Org-Admin-only** (kill the channel-owner grant path; knowledge stays centralized).
- [ ] Agent create/edit/delete/model → **any member** (relax out of `RequireOrgAdmin`); note the shared-org-global-agent caveat.
- [ ] Team read/update/roster remain **Org Admin** (team management is an org-level act); optionally expose team-*read* to members of that team.

**Phase 3 — Tighten reads & API-key ceilings.**
- [ ] Scope org-wide reads (members/usage/reporting/generations/api-keys/tokens/credentials/db-integrations) to Org Admin; keep member-relevant reads member-visible.
- [ ] Channel env-var **values** → manage; masked metadata → view.
- [ ] Make each API-key scope's `(resource,action)` ceiling explicit in the matrix; audit every `OrgAdminOrKey`/`ScopeOrJWT` write.

**Phase 4 — Frontend + retirement.**
- [ ] `GET /v1/capabilities`; refactor UI gating to consume it.
- [ ] Delete ad-hoc `RequireOrgAdmin` blocks and inline `isOrgManager` reads superseded by `Require(...)`.
- [ ] Contract tests: for every route, assert the matrix outcome per principal (the enum-contract-test pattern already in `internal/agents/`).

---

## 6. Ground-truth primitives (reference)

- **Route gates:** `cmd/server/serve_routes_v1.go` (+ `serve_routes_{apps,billing,brand,memory,session,sheets,uploads,v1_rag,connect,aux}.go`). Gate impls: `internal/middleware/auth.go` (`RequireOrgAdmin`, `RequireOrgAdminOrAPIKey`, `ResolveUser`), `internal/middleware/apikeyauth.go` (`MultiAuth`, `ResolveOrgFlexible`, `RequireAPIKeyScopeOrJWT`), `internal/middleware/admin_secret.go`.
- **Access core:** `internal/access/access.go` — `Actor{UserID,OrgID,OrgRole}`, `Resolve`, `IsOrgManager` (`owner|admin`), `CanUseChannel[ID]` (external OR `team_id` NULL OR active team member). **Team membership already lives in `team_members`; the missing piece is wiring `CanManageTeamResource`/`IsOrgOwner` — not a new role column.**
- **HTTP mirror:** `internal/handler/channels_auth.go` (`currentUserOrgRole`, `isOrgManager`, `canManageChannel`, `authorizeChannel`); `internal/handler/channel_access.go` (`canUseChannel`, `visibleTeamSubquery`, `actorSeesOrgWide`); `channels.go` `isAPIKeyRequest`.
- **Membership:** `team_members{role}` + `teams` (enforced dimension); `channel_members{role}` (creator=owner, else member; compose-authority today); `org_memberships{role: owner|admin|member|viewer}`.
- **Channel-visibility helpers:** `internal/channelagents` `VisibleAgentIDsSubquery`, `VisibleChannelIDsSubquery`.

---

## 7. Open questions for the operator

1. **Team-less channels** (personal/external, `team_id NULL`): who is their admin? Recommend: creator (`channel_members.role=owner`) + Org Admin; external/provider channels → Org Admin only.
2. **API keys and money/capability:** should any API-key scope ever reach billing, connection-create, or plugin-install? Recommend: no — these are human-admin actions; keys get an explicit, narrow ceiling.
3. **`viewer` org role:** currently treated as a plain member by every predicate (`IsOrgManager` false). Is read-only `viewer` meant to be *more* restricted than `member` (no session create)? Define it or drop it.
4. **Members creating channels:** may a member create channels only *within teams they belong to*, or also team-less ones? Recommend: only within their teams.
5. **Sandbox exec:** is `/sandboxes/{id}/exec` meant for humans at all, or runtime-only? If human, it must be manage-gated and session/channel-scoped.

---

## 8. Table of contents — feature files

| File | Scope |
|---|---|
| [`_MODEL.md`](./_MODEL.md) | Shared vocabulary, role baseline, severity definitions, per-file template. |
| [`organization-and-members.md`](./organization-and-members.md) | Orgs, members, invites, teams, team roster & roles; org/team management (all Org Admin). |
| [`teams.md`](./teams.md) | Team resource, membership, and the inert (unused) `team_members.role` column. |
| [`channels.md`](./channels.md) | Channels, membership, env-vars, channel↔agent, channel↔source, sessions; the compose surface. |
| [`agents-plugins.md`](./agents-plugins.md) | Agents, catalog install, models, plugins (org + per-agent), sandbox templates, sandboxes. |
| [`knowledge-sources.md`](./knowledge-sources.md) | RAG sources create/sync/prune and source↔channel grants. |
| [`connections-credentials.md`](./connections-credentials.md) | Nango connections, credentials, tokens, database integrations, git/proxy machine surface. |
| [`billing.md`](./billing.md) | Checkout, subscription lifecycle, plans, credits — the Owner surface. |
| [`automations.md`](./automations.md) | Triggers, schedules, trigger deliveries, system tasks, the automation catalog. |
| [`memories-content.md`](./memories-content.md) | Memories, directives, observations, sheets, apps, canvas, uploads/assets. |

---

*Prepared as the lead-architect synthesis over a direct sweep of `cmd/server/serve_routes*.go`, `internal/middleware`, `internal/access`, and the `internal/handler` authorization paths. Route inventory and role model are first-hand; per-feature inline-check detail is corroborated in the linked files.*
