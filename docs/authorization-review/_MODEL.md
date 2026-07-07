# Authorization Review — Shared Model & Conventions

> This is the shared baseline every per-feature doc in this folder applies. It exists so the recommendations cohere into ONE predictable system instead of each feature inventing its own rules. Per-feature docs must (a) apply this vocabulary, (b) map their feature's reality against it, and (c) explicitly flag where the feature can't/shouldn't follow it (with reasoning). The README owns the final synthesis and may refine this model — if you disagree with a baseline here, state it in your file's "Deviations from the baseline model" section rather than silently diverging.

## Why this review exists

Authorization today is ad-hoc: some routes are gated by `RequireOrgAdmin`, some by API-key scope, many reads are ungated for any org member, and channel/team/org admin boundaries are blurred. The operator wants a single predictable model answering "who can do what" across every API endpoint AND every frontend screen, with special attention to: who can add knowledge sources, who can grant them to channels, who can install/uninstall catalog agents, who touches billing, and where **team administration** ends and **organization administration** begins.

## The role model (FINAL — operator-confirmed 2026-07-07)

There is **no Team Admin role**. Authorization is NOT tiered inside a team: any member of a team has full rights over that team's resources. Three human principals plus one non-human axis.

- **Org Owner** — billing/subscription, transfer ownership, delete org. The only role that can touch money and org lifecycle. Superset of Org Admin.
- **Org Admin** — owns the **org boundary and shared catalog**: team management (create/rename/delete teams, add/remove team members), knowledge sources (create + grant to channels), connections/credentials/database-integrations, members & invites, API keys & tokens, org settings. NOT billing.
- **Member** — scoped to the teams they belong to. Within ANY team they are a member of: **full CRUD on that team's channels** (create, configure, assign agents, archive) **and agents** (create, edit, delete, assign to the team's channels), plus normal usage (sessions, chat, invoke). A member has NO access to org-level catalog/secrets, team management, billing, or anything in teams they don't belong to.
- **Automated actor / API key** — non-human. Scoped by API-key scopes and the runtime actor plumbing (`_hivy_actor_user_id`), NOT by human role. Distinct axis; call it out separately wherever it applies.

### The org-vs-team principle (the crux, resolved)

**Org Admins define the org structure (teams, membership, shared secrets & knowledge); team members freely operate their teams' channels and agents; the owner pays.**
- **Team management** (create/delete teams, add/remove members) → **Org Admin**.
- **Channels & agents of a team** (create/edit/delete/configure/assign) → **any Member of that team** (or Org Admin).
- **Org-level catalog & secrets** (knowledge sources, connections, credentials, database-integrations) and **members/invites/API-keys** → **Org Admin**.
- **Billing & org lifecycle** → **Owner**.
- **Using** a resource in a channel → **Member of that channel's team**.

Enforcement collapses to one predicate for team resources: `CanManageTeamResource(teamID) = IsOrgManager() || IsTeamMember(teamID)`. No new role column is needed — `team_members` membership alone is the grant; the inert `team_members.role` can stay unused or be dropped.

**Open data-model decision (agents):** agents are **org-global** today (`agents.org_id`, no `team_id`); channels carry `team_id` and agents attach via `channel_agents`. "Team members CRUD their team's agents" is realized WITHOUT a new migration by: agent create/edit/delete allowed for any org member, and the just-shipped read-visibility model already limits which agents a member SEES to those assigned to channels in their teams. Caveat to flag per feature: an org-global agent shared across teams can be edited/deleted by a member of any team it's assigned to. If strict per-team agent isolation is required later, scope agents to teams (add ownership) — noted as a future option, not built now.

## Enforcement recommendation (baseline)

The end state should be a **single authorization layer**, not per-handler ad-hoc checks:
- A capability/permission matrix: `(principal-role, resource, action) → allow/deny`, evaluated by shared middleware/predicates — extend `internal/access` (`Actor`, `IsOrgManager`, `CanUseChannel`) rather than scattering `RequireOrgAdmin` and inline role reads.
- **No new role is needed.** Team resources gate on `CanManageTeamResource(teamID) = IsOrgManager() || IsTeamMember(teamID)` (team membership already lives in `team_members`). Org-level resources gate on `IsOrgManager()`. Billing gates on a new `IsOrgOwner()` predicate (none exists today — `RequireOrgAdmin` collapses owner+admin).
- Reads follow the visibility rules just implemented (member sees only usable channels + assigned agents; admins see all). Writes follow the role model above.
- The recurring escalation root to kill: `canManageChannel` treats any channel-*owner* as sufficient, and channel creation has no team-membership check — so a member creates a channel in any team and self-grants rights. Replace with team-membership gating.
- Frontend must mirror the backend gate (hide/disable actions the role can't perform) AND never be the only gate — backend is authoritative.

## Current authorization primitives (ground truth to reference)

- Route gates in `cmd/server/serve_routes_v1.go`: `middleware.RequireOrgAdmin` (org settings/invites/teams; agents/triggers/schedules WRITE block; database-integrations write), `middleware.RequireOrgAdminOrAPIKey` (api-keys, credentials write, tokens write), `middleware.RequireAPIKeyScopeOrJWT("channels"|"agents"|"credentials"|"tokens"|"sheets")` (scope-gates API keys, lets any JWT/org-member through — this is where member reads leak).
- `internal/access/access.go`: `Actor{UserID,OrgID,OrgRole}`, `Resolve()`, `IsOrgManager()` = role `owner|admin`, `CanUseChannel`/`CanUseChannelID` (team-based: external OR team_id NULL OR active team member).
- HTTP mirror: `internal/handler/channels_auth.go` `currentUserOrgRole`, `isOrgManager`; `internal/handler/channel_access.go` `canUseChannel`, `visibleTeamSubquery`; `isAPIKeyRequest` (`channels.go:146`).
- Membership: `team_members`+`teams` (the enforced dimension); `channel_members` (roles only, largely unused); `org_memberships` (org role).
- Just-shipped visibility helpers: `internal/channelagents` `VisibleAgentIDsSubquery`, `VisibleChannelIDsSubquery`.

## Per-feature file template (use these exact section headings)

```
# Authorization — <Feature>

## 1. Overview
What the feature is; its resources; which principals interact with it.

## 2. Backend endpoint inventory
Table: | Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
Cover EVERY route for this feature. "CURRENT gate" = the actual middleware/inline check today. "Correct?" = ✅ / ⚠️ ambiguous / ❌ wrong, vs the role model.

## 3. Frontend screens & actions
Table: | Screen (path) | Action | Calls | UI gated by role today? | Should be |
Every settings/dashboard screen and mutating action for this feature.

## 4. Ambiguities & lapses (ranked)
The concrete authorization gaps, worst first. Each: what's wrong, who can do what they shouldn't (or can't do what they should), blast radius.

## 5. Recommendation
For each action: the target principal per the role model, the enforcement mechanism (which layer/predicate), and any new role/column/migration needed. Be specific and buildable.

## 6. Deviations from the baseline model
Where this feature can't/shouldn't follow _MODEL.md, and why.

## 7. Open questions for the operator
Decisions only the operator can make.
```

## Severity vocabulary (use consistently)

- **CRITICAL** — a non-admin can perform an org-level mutation or read cross-team data that leaks secrets/PII/money.
- **HIGH** — a principal can perform config outside their scope (a member acting org-admin, or a non-member managing another team's channels/agents; create/delete/assign of org-level resources by a member).
- **MEDIUM** — inconsistent gating that's exploitable but low blast radius, or org-admin-gated where a team member should suffice.
- **LOW** — UI/UX gating mismatch, cosmetic, or defense-in-depth.
