# Authorization — Teams

## 1. Overview

A **team** groups org members and scopes **private channel visibility**. Its resources:

- `teams` — an org-owned grouping (`name`, `description`, `created_by`, `archived_at`). Unique active name per org.
- `team_members` — `(org_id, team_id, user_id, role)`. `role` is `owner` | `member`, defaulting to `member`.
- `org_invite_teams` — join rows attaching a pending org invite to one or more teams, so an invited user lands in those teams on accept.

**What teams actually do (the enforced dimension):** channel visibility. A channel with `team_id = NULL` (or `origin = "external"`) is visible/usable by every org member; a channel with a `team_id` is visible/usable only by **active members of that team** (plus org managers, plus API keys). This is enforced identically in three places that must never drift: `internal/access/access.go` (`userIsActiveTeamMember`, agent-facing MCP path), `internal/handler/channel_access.go` (`userIsActiveTeamMember` / `visibleTeamSubquery`, HTTP path), and `internal/channelagents/visibility.go` (a deliberate copy). So teams are the substrate for the entire member-facing channel/agent/RAG visibility model.

**Principals who interact:** Org Owner/Admin (full team CRUD + membership today), Member (team membership is *assigned to* them and silently governs what channels/agents they see; under the confirmed model a member also gains full CRUD on their team's channels and agents), Automated actor / API key (bypasses team scoping entirely — `apiKey` short-circuits every `canUseChannel`/`canManageChannel` check).

**Central finding up front:** there is **no per-team admin tier** in data or code — and per the operator-confirmed model, none is wanted. The `team_members.role` column exists and is write-validated to `owner`|`member`, but **no authorization path ever reads it** (grep across `internal/` finds zero reads of `team_members` role for authz; `userIsActiveTeamMember` only checks row existence). Every team-management endpoint is gated by `RequireOrgAdmin`. "Team owner" is dead data — a latent hook that should stay inert or be dropped, not promoted.

## 2. Backend endpoint inventory

All routes below are mounted inside the `RequireOrgAdmin(database)` group in `cmd/server/serve_routes_v1.go` (lines ~78–92), itself nested under `ResolveOrgFlexible` + `RequireEmailConfirmed`.

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | `/v1/orgs/current/teams` | `teams.go:59` `List` | Reads | RequireOrgAdmin | ⚠️ over-restrictive: a member should be able to read teams they belong to |
| POST | `/v1/orgs/current/teams` | `teams.go:105` `Create` | Mutates (create resource) | RequireOrgAdmin | ✅ creating a *team* is an org-level (team-management) act |
| GET | `/v1/orgs/current/teams/{id}` | `teams.go:151` `Get` (team + members) | Reads | RequireOrgAdmin | ⚠️ over-restrictive: a member should be able to read their own team's roster |
| PATCH | `/v1/orgs/current/teams/{id}` | `teams.go:177` `Update` (name/description) | Mutates | RequireOrgAdmin | ✅ renaming a team is team management → Org Admin (current gate correct) |
| DELETE | `/v1/orgs/current/teams/{id}` | `teams.go:231` `Archive` (blocks if channels attached) | Mutates (delete resource) | RequireOrgAdmin | ✅ deleting a *team* is org-level |
| PUT | `/v1/orgs/current/teams/{id}/members/{userID}` | `team_members.go:44` `PutMember` (add/update role) | Mutates | RequireOrgAdmin | ✅ composing team membership is team management → Org Admin (current gate correct) |
| DELETE | `/v1/orgs/current/teams/{id}/members/{userID}` | `team_members.go:93` `DeleteMember` | Mutates | RequireOrgAdmin | ✅ same — Org Admin (current gate correct) |

**Adjacent route that lives elsewhere but is the crux of the team boundary** — channel↔team assignment is *surfaced on the team detail screen* but is **not** a team route:

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| PATCH | `/v1/channels/{id}` with `body.team_id` | `channels_mutation.go:163` `Update` → `channels_update.go:58` (`applyChannelUpdates`) | Mutates (assign/detach team) | `RequireAPIKeyScopeOrJWT("channels")` route gate **+ inline `canManageChannel`** (`channels_auth.go:58` = apiKey OR org-manager OR **channel-owner `member_role=="owner"`**) | ❌ a non-admin channel owner can attach/detach any team |
| POST | `/v1/channels` with `body.team_id` | `channels_mutation.go:30` `Create` → `resolveTeamID` (`channels_data.go:42`) | Mutates | `RequireAPIKeyScopeOrJWT("channels")` (any JWT/member passes) | ❌ any member can create a channel directly into any team |

`resolveTeamID` (`channels_data.go:42`) validates only that `team_id` belongs to an active team **in the same org** — it never checks the caller is a member of that team. So the team boundary is not enforced on the channel side at all.

Note the asymmetry: `team_members.PutMember` requires the target user already belong to the org (`userBelongsToOrg`, `team_members.go:143`) and validates `role ∈ {owner, member}`, but that role is inert.

## 3. Frontend screens & actions

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---|---|---|---|---|
| `settings/teams/page.tsx` | View teams list | GET `/teams` (`enabled: isAdmin`) | Yes — `isAdmin = role∈{owner,admin}`; non-admins see a static "workspace admins manage teams" placeholder | Members should also be able to see (at least) their own teams |
| ″ | Invite member (opens `InviteMemberModal`, can pre-assign teams via `org_invite_teams`) | POST `/orgs/current/invites` | Yes (`isAdmin`) | Org admin (inviting to the org is org-level; assigning the new hire to teams is team management → also Org Admin) |
| ″ | New team (`TeamFormModal`) | POST `/teams` | Yes (`isAdmin`) | Org admin ✅ |
| ″ | View org members / pending invites | GET `/orgs/current/members`, `/invites` | Members list ungated; invites `isAdmin` | (members feature) |
| `settings/teams/[teamId]/page.tsx` | View team detail | GET `/teams/{id}`, `/teams`, `/members`, `/channels` (all `enabled: isAdmin`) | Yes — non-admin gets "Team management" placeholder | A member of this team should be able to view it (read-only) |
| ″ | Edit team (`TeamFormModal`) | PATCH `/teams/{id}` | Yes (`isAdmin`) | Org admin ✅ (renaming is team management) |
| ″ | Archive team | DELETE `/teams/{id}` (with `window.confirm`) | Yes (`isAdmin`) | Org admin ✅ |
| `[teamId]/team-sections.tsx` `TeamMembersSection` | Add existing member | PUT `/teams/{id}/members/{userID}` (`body.role:"member"`) | Rendered only inside `isAdmin` page | Org admin ✅ (roster is team management) |
| ″ | Remove member | DELETE `/teams/{id}/members/{userID}` | ″ | Org admin ✅ |
| ″ | Invite to team | POST `/orgs/current/invites` | ″ | Org admin (invite + team assignment are both org-level) |
| `TeamChannelsSection` | Assign public channel to team | PATCH `/channels/{id}` `{team_id}` | ″ | Member of the relevant team (or Org Admin) via `CanManageTeamResource` — but backend gate is `canManageChannel`, **weaker than the UI** |
| ″ | Remove channel from team | PATCH `/channels/{id}` `{team_id:""}` | ″ | Member of the relevant team (or Org Admin) — same weaker-backend note |

The frontend is uniformly gated on `activeOrg.role ∈ {owner, admin}` (a single `isAdmin` boolean derived in each page from `useAuth`). There is no per-team membership set in the auth context, so the UI cannot express "member of team X" for channel/agent actions or "let a member read their own team."

## 4. Ambiguities & lapses (ranked)

1. **HIGH — channel↔team assignment is gated by `canManageChannel`, not by team/org membership, so a plain member can move a private team channel in or out of any team.** The team-detail UI hides "Assign / Remove channel" behind `isAdmin`, but the underlying `PATCH /v1/channels/{id}` only requires `apiKey || org-manager || channel-owner`. A member who **created** a team-scoped channel is its `channel_members` owner (`channels_mutation.go:120`), so they can `PATCH team_id:""` to **detach it from the team**, instantly making a formerly team-private channel visible/usable by the *entire org* (`canUseChannel`: `team_id == NULL` → open to all). Conversely they can attach any channel they own into a team they don't belong to. `resolveTeamID` never checks team membership. Blast radius: leak of team-scoped channel content (sessions, memory, assigned RAG sources) org-wide, driven by a non-admin. This is the single concrete exploit in the teams surface.

2. **MEDIUM — team members can't manage their own team's channels/agents; those acts require channel-ownership or org-admin.** Under the confirmed model, any member of a team has full CRUD on that team's channels **and** agents via `CanManageTeamResource(teamID)`. Today, channel management gates on `canManageChannel` (channel-*owner* or org-manager) and agent writes gate on `RequireOrgAdmin`, so a plain team member who didn't create a channel can't configure it, and can't touch agents at all, without being made an org admin. Consequence: to let a department lead operate their own team you over-grant org admin — which also hands them knowledge-source/connection/invite/org-settings powers — and that over-grant is what *becomes* a leak. Note this is scoped to a team's channels/agents: team *management* (roster, rename, create/delete) correctly stays org-admin. The `team_members.role` column plays no part in the fix — team **membership** alone is the grant.

3. **MEDIUM — `team_members.role` is dead authorization data.** `PutMember` accepts and stores `owner`/`member`, and the API returns it (`teamMemberResponse.Role`), and the UI renders it as a chip (`roleLabel`), implying a capability that does not exist. Anyone reading the model or the UI would reasonably assume a team owner has elevated rights; they have none. This is a correctness/expectation trap that will mislead the next engineer and any customer. Under the confirmed model the column stays inert (membership alone grants rights) and may simply be dropped; either way the API/UI should stop surfacing `owner`/`member` as if it were a capability.

4. **LOW — team reads are admin-only, so members have zero visibility into their own team membership.** A member's channel visibility is silently governed by teams they can't see. Not a security issue (fail-closed), but it makes team membership unauditable by the affected user and complicates any future "members read their own team" story. This is a real finding; its fix is optionally exposing team-read to members — not a new role.

5. **LOW — invite-time team assignment (`org_invite_teams`) rides the org-admin invite gate.** Correct today **and** under the confirmed model: choosing which teams a new hire joins is team *management*, which stays Org Admin, fused into the org-level invite action. No change needed.

## 5. Recommendation

Adopt the baseline's org-vs-team split explicitly for teams. The model needs **no team-admin tier and no new role column** — we considered and rejected a per-team admin role. Concretely:

**Team management stays Org Admin; team members get full rights on their team's resources via membership.** There is no tier inside a team. Team *management* — create/delete/rename teams, add/remove team members — remains an Org Admin act, and the current `RequireOrgAdmin` gate on those routes is already correct. A team's *channels and agents* (create/edit/delete/configure/assign) are open to **any member of that team**, enforced by one predicate:

`CanManageTeamResource(teamID) = IsOrgManager() || IsTeamMember(teamID)`

- **No schema migration and no new role column.** Team membership already lives in `team_members`; membership alone is the grant. The inert `team_members.role` can stay unused or be dropped — do **not** promote it.
- Add `Actor.CanManageTeamResource(ctx, db, teamID) bool` to `internal/access/access.go` (reuse the existing `userIsActiveTeamMember` row-existence check `OR IsOrgManager()`), and add the HTTP-side twin next to `canUseChannel` in `channel_access.go` so the two paths can't drift (the codebase already enforces this mirroring discipline).
- Optionally expose team **read** to members (see lapse #4): let a member `GET` the teams they belong to, without granting any management right.

**Then re-gate per the role model** (target principal → enforcement layer):

| Action | Target principal | Enforcement change |
|---|---|---|
| Create team (`POST /teams`) | Org Admin | keep `RequireOrgAdmin` |
| Delete/archive team (`DELETE /teams/{id}`) | Org Admin | keep `RequireOrgAdmin` |
| Rename team (`PATCH /teams/{id}`) | Org Admin | keep `RequireOrgAdmin` (current gate is correct — team management is org-level) |
| Read team + roster (`GET /teams`, `GET /teams/{id}`) | Org Admin (management); optionally also a **member** of that team (read-only) | keep `RequireOrgAdmin` for the management view; optionally scope `List`/`Get` to also expose a member their own teams |
| Add/remove/role team member (`PUT`/`DELETE /teams/{id}/members/...`) | Org Admin | keep `RequireOrgAdmin` |
| Assign/detach channel↔team (`PATCH /channels/{id}` `team_id`) | **Member of the relevant team (or Org Admin)** via `CanManageTeamResource` | **fix now:** in `applyChannelUpdates`, when `team_id` changes, require `CanManageTeamResource(teamID)` of the target team (and, to detach, of the current team) — do **not** let bare `channel-owner` set `team_id`. This closes lapse #1. |
| Create channel into a team (`POST /channels` `team_id`) | Member of that team or Org Admin via `CanManageTeamResource` | in `resolveTeamID`, when a non-manager sets `team_id`, verify `CanManageTeamResource(team_id)`; otherwise reject |

**Enforcement mechanism, generally:** the channel↔team decisions are per-resource (per-team, per-channel), so they belong in the shared predicate `Actor.CanManageTeamResource` invoked inline in handlers, **not** in the route-level `RequireOrgAdmin` middleware (which can't see the `{id}`). Team *management* routes stay on `RequireOrgAdmin` — those are genuinely org-level. This is the "extend `internal/access` rather than scatter `RequireOrgAdmin`" direction the baseline calls for.

**Frontend:** thread a `myTeamIds` set (the teams the current user belongs to) into the auth context, and derive `CanManageTeamResource(teamId) = isOrgAdmin || myTeamIds.has(teamId)` to gate channel/agent actions on a team. Keep the team-*management* screens (New Team, Edit/Rename, Archive, add/remove member) gated on `isOrgAdmin`. Additionally, let a team member **read** their own team detail (mirroring the optional backend read exposure). Backend stays authoritative regardless.

## 6. Deviations from the baseline model

- **None in principle** — teams are precisely where the baseline's org-vs-team boundary is meant to live, and this doc adopts it wholesale: no per-team admin tier, team management on Org Admin, team members full on their team's channels/agents via `CanManageTeamResource`. The only deviation from *current reality* is that team members can't yet manage their team's channels/agents without org-admin (lapse #2); every recommendation above closes that gap **without any migration or new role**.
- One nuance worth flagging against the baseline table: the baseline lists "channel membership" and "assign existing agent/RAG to channel" as member-of-team acts, but in this codebase **those live on the channels feature's routes** (`/v1/channels/*`, gated by `canManageChannel`), not on team routes. The teams feature only owns *team membership* and *team CRUD*. The channel↔team edge is the one shared seam, and it is currently the weakest link (lapse #1). Whoever owns the channels doc must land the same `CanManageTeamResource` predicate; the two docs converge here.
- API keys bypass all team scoping by design (`apiKey` short-circuits `canUseChannel`/`canManageChannel`). Consistent with the baseline's "automated actor is a separate axis," but it means an org-scoped `channels`-scoped key can reassign any channel's team with no team check — acceptable for a trusted server credential, worth noting.

## 7. Open questions for the operator

1. **Should a team member be able to *create* channels within their own team, or only assign existing ones into it?** The baseline leans compose-not-create for org-level catalog, but a channel is arguably a team-local resource (unlike agents/knowledge-sources). The decision drives whether `POST /channels` with `team_id` is open to any team member (via `CanManageTeamResource`) or restricted further.
2. **Should a team's roster be visible to the members of that team, not just org admins?** Exposing the roster to members is convenient but leaks who-is-in-what to non-admins. Ties into the read-visibility decision below.
3. **Who may detach a channel from a team (making it org-public)?** This is a privacy-downgrade. Recommend requiring `CanManageTeamResource` of the *current* team at minimum (member of it, or Org Admin); confirm whether detaching should be Org-Admin-only.
4. **Do members get any read visibility into their own team memberships?** Currently none; decide whether that's a feature gap or intentional. Its fix is exposing team-read to members — not any new role.
