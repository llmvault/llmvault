# Authorization — Channels

## 1. Overview

A **channel** is the primary team-scoped workspace surface: a place where members open sessions, chat, and invoke agents. It is also the unit of **composition** — an org's catalog resources (agents, knowledge/RAG sources) and channel-scoped config (environment variables, memory category/mission, default agent, image models) are attached to a channel, and access is granted through it.

Resources owned/controlled by this feature:
- `channels` (`internal/model/channel.go`) — `kind` (`standard`/`personal`), `visibility` (`public`/`private`), `team_id` (nullable), `default_agent_id` (not null), `origin` (`native`/`external`), `is_default`, plus external-provider (Slack) mirror fields, `category`, `memory_mission`, `image_model`, `created_by`.
- `channel_members` (`ChannelMember`) — `(channel_id, user_id, role)` where role ∈ `{owner, member}`.
- `channel_rag_sources`, `channel_env_vars`, `channel_agents` (join tables) — channel-scoped composition/config.

Principals that interact:
- **Members** — open/use channels, read channel-scoped content.
- **Channel "owner"** (a `channel_members.role`) — the closest thing to a per-channel admin today; the channel creator is auto-assigned this role.
- **Org Admin/Owner** — bypass all channel gates.
- **Team member** — channels are team-scoped via `team_id`, and authority over a channel is conferred by **membership of that channel's team** (`CanManageTeamResource(teamID)`). There is no separate team-level administrative role — any member of the team may manage the team's channels; there is no tier inside a team.
- **API key / automated actor** — API keys with the `channels` scope pass every gate; the agent runtime path (`_hivy_actor_user_id` → `access.Actor`) mirrors the HTTP predicates.

The central architectural fact: **two disjoint membership models coexist.** `canUseChannel` (access) is *team-based* (`team_members`), while `channel_members` is a *separate* ownership/display dimension. They do not align (see §4).

## 2. Backend endpoint inventory

Route registration: all live under `cmd/server/serve_routes_v1.go:148` inside `middleware.RequireAPIKeyScopeOrJWT("channels")` — i.e. **any authenticated JWT (any org member) OR an API key carrying the `channels` scope** passes the route gate; the real authorization is the per-handler predicate below.

Three handler predicates do the work:
- `canUseChannel` (`channel_access.go:18`) — apiKey OR org owner/admin OR (member AND (`origin=external` OR `team_id IS NULL` OR active member of `team_id`)). Used as `authorizeChannel(requireManage=false)` and by the agent path's `access.Actor.CanUseChannel`.
- `canManageChannel` (`channels_auth.go:58`) — apiKey OR org owner/admin OR `channel_members.role == "owner"`. Used as `authorizeChannel(requireManage=true)`.
- `channelForAgentMutation` (`channel_agents.go:156`) — apiKey OR (member AND `Actor.CanUseChannel`). **This is the anomaly.**

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | /v1/channels | `channels_read.go:34` List | Reads | Any member; non-managers filtered to team-visible channels (`visibleTeamSubquery`) | ✅ |
| POST | /v1/channels | `channels_mutation.go:30` Create | Mutates | **None beyond org context** — any member; `resolveTeamID` only checks the team *exists* in the org, not that the caller belongs to it; creator auto-becomes `owner` | ❌ HIGH |
| GET | /v1/channels/{id} | `channels_read.go:99` Get | Reads | `canUseChannel` | ⚠️ (visibility ignored, §4.4) |
| PATCH | /v1/channels/{id} | `channels_mutation.go:163` Update | Mutates | `canManageChannel` (org admin OR channel owner). Covers name, visibility, `team_id`, `default_agent_id`, category, mission, image models | ⚠️ (channel-owner ≠ team member) |
| DELETE | /v1/channels/{id} | `channels_mutation.go:221` Archive | Mutates | `canManageChannel`; blocked for `is_default`/`personal` | ⚠️ |
| POST | /v1/channels/{id}/join | `channels_members.go:28` Join | Mutates | `canUseChannel` + `visibility == public` | ✅ |
| PUT | /v1/channels/{id}/members/{userID} | `channels_members.go:70` PutMember | Mutates | `canManageChannel`; target must be an org member | ⚠️ |
| DELETE | /v1/channels/{id}/members/{userID} | `channels_members.go:121` DeleteMember | Mutates | self-remove OR `canManageChannel`; guards last owner | ✅ |
| GET | /v1/channels/{id}/environment-variables | `channel_environment_variables.go:47` List | Reads | `canUseChannel` (names+descriptions to all channel-usable members; values never returned) | ⚠️ (§4.7) |
| POST | /v1/channels/{id}/environment-variables | `channel_environment_variables.go:87` Create | Mutates | `canManageChannel` (writes an encrypted secret) | ⚠️ |
| PATCH | /v1/channels/{id}/environment-variables/{name} | `channel_environment_variables.go:159` Update | Mutates | `canManageChannel` | ⚠️ |
| DELETE | /v1/channels/{id}/environment-variables/{name} | `channel_environment_variables.go:266` Delete | Mutates | `canManageChannel` | ⚠️ |
| GET | /v1/channels/{id}/rag-sources | `channels_rag_sources.go:42` List | Reads | `canUseChannel` | ✅ |
| PUT | /v1/channels/{id}/rag-sources | `channels_rag_sources.go:77` Set | Mutates | `canManageChannel`; each source must be in-org | ⚠️ |
| GET | /v1/channels/{id}/agents | `channel_agents.go:40` ListChannelAgents | Reads | `canUseChannel` | ✅ |
| POST | /v1/channels/{id}/agents | `channel_agents.go:72` AssignChannelAgent | Mutates | **`CanUseChannel` — ANY channel member** (any in-org, non-archived agent) | ❌ HIGH |
| DELETE | /v1/channels/{id}/agents/{agentID} | `channel_agents.go:130` UnassignChannelAgent | Mutates | **`CanUseChannel` — ANY channel member** (except the default) | ❌ HIGH |
| GET | /v1/channels/{id}/sessions | `sessions_read.go:55` ListChannelSessions | Reads | `canViewChannel` (= `canUseChannel`) | ✅ |

Adjacent (Slack provisioning, same scope group, out of core scope): `GET /v1/slack/channels`, `POST /v1/slack/channels/join` (`serve_routes_v1.go:143-144`) — connection-scoped, drive external-channel creation.

## 3. Frontend screens & actions

Mechanism note: the **only** role signal available to the UI is `useAuth().activeOrg?.role` (`owner`/`admin`/member) from `apps/web/lib/auth/auth-context.tsx` (fed by `GET /auth/me`). **Channel-level role/membership is not exposed to the client at all** — it is known only server-side and surfaces as a 403-to-toast after a failed request. Role-based UI gating exists *only* in the Teams settings pages (`isAdmin = activeOrg.role === "owner"||"admin"`), never in any channel screen.

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---|---|---|---|---|
| `apps/web/app/w/settings/channels/page.tsx` | List/search all channels | `GET /v1/channels` | No gate (read) | Fine (read) |
| `apps/web/app/w/settings/channels/[id]/page.tsx` (Channel tab) | Update name/visibility/category/mission | `PATCH /v1/channels/{id}` | No — only form-validity (`canSave`) | Member of channel's team / Org Admin |
| …(Channel tab) | Delete channel | `DELETE /v1/channels/{id}` | No — hidden only when `is_default`/`personal` | Member of channel's team / Org Admin |
| `apps/web/app/w/settings/channels/[id]/_agents-tab.tsx` (new) | Assign agent | `POST /v1/channels/{id}/agents` | No — controls shown to all; 403 rewritten to friendly copy in `_agents-tab-lib.ts` | Member of channel's team / Org Admin |
| …(Agents tab) | Unassign agent | `DELETE /v1/channels/{id}/agents/{agentID}` | No — only "can't remove default" data guard | Member of channel's team / Org Admin |
| …(Agents tab) | Change default agent | `PATCH /v1/channels/{id}` `{default_agent_id}` | No | Member of channel's team / Org Admin |
| `apps/web/app/w/settings/channels/[id]/page.tsx` (Members tab) | View members + roles | (from `GET /v1/channels/{id}`) | Read-only; **no add/remove/role UI exists** | — |
| `apps/web/app/w/settings/channels/[id]/_knowledge-sources-tab.tsx` | Toggle RAG sources | `PUT /v1/channels/{id}/rag-sources` | No — switch disabled only if source has no id | Member of channel's team / Org Admin |
| `apps/web/app/w/(chat)/_components/channel-environment-variables.tsx` (+ `channel-env-var-forms.tsx`) | Create/update/delete env var (secrets) | `POST/PATCH/DELETE …/environment-variables` | No — explicit comment "rely on the server for authorization" | Member of channel's team / Org Admin |
| `apps/web/app/w/(chat)/_components/sidebar.tsx` + `channel-create-modal.tsx` | Create channel (Slack-bound) | `POST /v1/channels` | No — "+" button + Create button unconditional (form-validity only) | Member of channel's team / Org Admin |
| `apps/web/app/w/(chat)/_components/channel-actions-menu.tsx` | Rename / details | routes to settings page (no inline mutation) | No | (inherits detail-page gate) |
| Settings nav `apps/web/app/w/settings/_components/nav.tsx:27` | Reach Channels section | — | No — nav shows Channels to everyone | Visible to all; actions gated |

Bottom line: **no channel screen or action is role-gated in the UI.** Every control renders and fires for any authenticated user who can reach it; enforcement is 100% server-side.

## 4. Ambiguities & lapses (ranked)

**4.1 — HIGH — Agent assign/unassign is gated by `CanUseChannel` (any member), not `canManageChannel`.**
`channelForAgentMutation` (`channel_agents.go:156-193`) authorizes `POST`/`DELETE …/agents` with `Actor.CanUseChannel` — *any* member who can use the channel. So any plain member of a team can wire **any active org agent** into that channel, or remove agents from it. This is channel *composition* (per the baseline model, team-membership / Org-Admin territory) but is gated at the *use* level. Blast radius: a member can attach a powerful/over-privileged agent (with its tool set) into a channel to exfiltrate or act, or strip the agents a team relies on. It is also **internally inconsistent**: changing the *default* agent goes through `PATCH` → `canManageChannel` (admin/owner), yet adding/removing the agent *pool* needs only `CanUseChannel`. The error copy ("join the channel before managing its agents") shows the weaker intent was deliberate but wrong vs. the role model.

**4.2 — HIGH — `POST /v1/channels` (create) has no role gate, and does not verify team membership.**
`Create` (`channels_mutation.go:30`) checks only that an org context exists. Any member can create a channel, set `team_id` to **any** team in the org (`resolveTeamID` verifies only existence, not that the caller belongs to that team — `channels_data.go:42`), pick **any** org agent as default, and is auto-inserted as `channel_members.role="owner"` (line 120-124), which then grants them full `canManageChannel` rights (env-var secrets, RAG grants, archive, membership) over the channel they just made. Net effect: **any member can self-promote to de-facto channel admin** and can plant a channel inside a team they aren't part of. Per the model, creating a channel in a team requires being a member of that team (or Org Admin).

**4.3 — MEDIUM — "Channel owner" is an ad-hoc per-channel admin unmoored from the role model.**
`canManageChannel` treats `channel_members.role == "owner"` as equivalent to org admin *for that channel* — covering env-var secret writes, RAG grants, channel update, archive, and member management. Ownership is conferred by *creating* the channel (4.2) or by an existing owner via `PutMember`. This is a fourth, ad-hoc principal that the baseline model does not have; it should collapse into **team-membership gating** (`CanManageTeamResource`). Sensitive because channel env vars are encrypted secrets injected into sandboxes.

**4.4 — MEDIUM — `visibility` (`private`) is not enforced by the access predicates.**
`canUseChannel`/`canViewChannel` never consult `visibility` (grep-confirmed: only `Join` and update-validation read it). A `private` channel with `team_id IS NULL` returns `true` from `canUseChannel` for **every org member** — so "private" delivers no real access restriction unless the channel is team-scoped; it only prevents self-`Join`. A user who marks a team-less channel private reasonably expects it hidden; it is not.

**4.5 — MEDIUM — Two disjoint membership models (`team_members` vs `channel_members`).**
Access to a team-scoped channel is *purely* team-based; `channel_members` is ignored by `canUseChannel`. Consequences: (a) `Join` and `PutMember` write `channel_members` rows that **do not grant use** of a team-scoped channel; (b) `channel_members` meaningfully affects only the *owner*-manage gate and the members display; (c) a channel owner can `PutMember` any org user into `channel_members` without granting them access. The model has two "membership" concepts that don't compose — a source of confusion and of the 4.3 ad-hoc-admin problem.

**4.6 — MEDIUM — `canManageChannel` never checks team membership, so team members can't manage their team's channels.**
`canManageChannel` recognizes org role OR channel-owner only — it never consults `team_members`. A member of a channel's team, who by the role model should have full CRUD on that team's channels, can't manage them unless they're an org admin or happen to hold the per-channel `owner` role. There is no `Actor.IsTeamMember(teamID)` / `CanManageTeamResource(teamID)` gating. The fix is team-membership gating (`CanManageTeamResource = IsOrgManager() || IsTeamMember(teamID)`), reading the existing `team_members` rows — **not** a new role.

**4.7 — LOW — Env-var names/descriptions readable by all channel-usable members.**
`ListChannelEnvironmentVariables` is `canUseChannel`-gated, so every member who can use the channel sees the *names/descriptions* of its secrets (values are never returned). Low blast radius but leaks secret naming to non-admins.

**4.8 — LOW — Frontend performs zero role gating.**
All channel controls render for everyone and rely entirely on the backend 403. Backend-authoritative is correct, but the UI should hide/disable actions the role can't perform (defense-in-depth + UX). Today the only feedback is a post-failure toast.

## 5. Recommendation

Target end-state per the baseline: **Org Admins own the catalog (agents, RAG sources); any member of a channel's team composes them into that team's channels; Members consume.** Concretely:

**Enforcement primitive (prerequisite for most of the below):**
- **No new column or migration.** Team membership already lives in `team_members`, and the gate reads it directly — the inert `team_members.role` stays unused. (We deliberately rejected a team-admin tier: there is no authority tier inside a team; any member has full rights over the team's channels.)
- Add `Actor.IsTeamMember(ctx, db, teamID) bool` and a composite `Actor.CanManageChannel(ctx, db, channel) bool` = `IsOrgManager() || (channel.TeamID != nil && IsTeamMember(channel.TeamID))`. For team-less/external channels with no natural team — hence no team-membership principal — `CanManageChannel` falls back to Org-Admin only (see §6). Mirror it in a handler-side `canManageChannel` so HTTP and agent paths can't drift.

**Per action:**

| Action | Target principal | Mechanism |
|---|---|---|
| Create channel (`POST /v1/channels`) | Member of the target team, or Org Admin | Add `canManageChannel` check at top of `Create`; make `resolveTeamID` require the caller be a member of that team (or org manager). Stop auto-granting channel `owner` to arbitrary members. |
| Update / Archive (`PATCH`/`DELETE /v1/channels/{id}`) | Member of channel's team, or Org Admin | Replace the org-role-OR-owner `canManageChannel` body with `CanManageTeamResource(channel.TeamID)` in `authorizeChannel(requireManage=true)`. |
| Assign/unassign agent (`POST`/`DELETE …/agents`) | Member of channel's team, or Org Admin | **Fix 4.1:** change `channelForAgentMutation` from `CanUseChannel` to `canManageChannel`. Brings it in line with default-agent change and RAG grants. |
| Set default agent (`PATCH default_agent_id`) | Member of channel's team, or Org Admin | Already `canManageChannel`; moves with the Update change above. |
| Grant/revoke RAG sources (`PUT …/rag-sources`) | Member of channel's team, or Org Admin | Retarget `canManageChannel` to team-membership gating. (Creating the source itself stays Org-Admin — out of this feature.) |
| Env var CRUD (`…/environment-variables`) | Member of channel's team, or Org Admin | Retarget `canManageChannel` to team-membership gating. |
| Channel membership (`PUT`/`DELETE …/members`) | Member of channel's team, or Org Admin (self-remove always allowed) | Retarget `canManageChannel` to team-membership gating; keep self-remove. Resolve 4.5: either make `channel_members` the real access dimension for team-less channels, or drop the concept and derive membership from team + explicit invite. |
| Use / read (list, get, sessions, list agents/rag/env-names) | Member of channel | Keep `canUseChannel`, but **fix 4.4**: fold `visibility` into `canUseChannel` so a `private` team-less channel is restricted (e.g. to `channel_members` / creator). Consider hiding env-var *names* from non-managers (4.7). |

**Enforcement layer:** do all of this in `internal/access` (`CanManageChannel` / `CanManageTeamResource`) + the two handler mirrors, not new inline `RequireOrgAdmin` sprinkles. Retire the `channel-owner` special-case inside `canManageChannel` once team-membership gating lands.

**Frontend:** expose the caller's effective channel capability (manage vs. use) — either a `can_manage` boolean on the channel DTO or derive from `activeOrg.role` + team membership — and hide/disable the create button, the Agents/Knowledge/Env tabs' mutating controls, Update, and Delete when the user can't manage. Keep backend authoritative.

## 6. Deviations from the baseline model

- **Channels with no team (`team_id IS NULL`) and external (Slack) channels have no natural team — hence no team-membership principal.** The model's "team members manage" cannot bind them to any team. Recommendation: for these, `CanManageChannel` falls back to **Org-Admin only**. External channels are intentionally usable by all members (they mirror a Slack channel), but their *composition* (agents/RAG/env) should still require Org-Admin. Flag for the operator: whether team-less channels should even be creatable by non-admins.
- **`channel_members` roles are, contrary to the baseline note, *actively used*** — as the `owner`-manage gate, not merely display. The recommendation is to retire that role in favor of team-membership gating, so the baseline's "barely used" framing becomes true only after this change.
- **The baseline assumes reads follow team-visibility.** They do here, except `visibility=private` is not honored (4.4) — a genuine gap, not an intentional deviation.

## 7. Open questions for the operator

1. **Who may create a channel?** Members of the target team (or Org Admin) only (recommended), or should any member be able to spin up a personal/team-less channel? Today anyone can.
2. **Retire the per-channel "owner" role** in favor of team-membership gating, or keep both? If kept, what governs who becomes owner (today it's "whoever created the channel")?
3. **Should assigning/removing agents and RAG sources require membership of the channel's team rather than mere channel-use?** (We recommend yes — this is the headline 4.1 fix.)
4. **Should `private` + team-less channels be truly access-restricted** (fix 4.4), and if so, to whom — creator only, or `channel_members`?
5. **Reconcile the two membership models (4.5):** should `channel_members` grant access to team-less channels, or should channel access always derive from `team_members` + visibility, with `channel_members` dropped?
6. **May a non-member set a channel's `team_id` to a team they don't belong to?** Today `resolveTeamID` permits it; recommend requiring membership of the target team (or Org Admin).
