# Authorization — Memories & Agent Content

> Applies the shared model in `_MODEL.md`. Scope: agent memory/learning surfaces
> (memories, observations, directives, channel memory settings) and agent-produced
> content (sheets, apps, canvas artifacts, org skills/plugins via the
> skill-manager). READ fixes on memory *reads* are already shipped and are not
> re-flagged here — this doc audits **writes** and the actors that perform them.

## 1. Overview

Two families of resources, and they sit on opposite ends of the role model:

- **Config that shapes agent behavior** (admin-shaped): **memories**, **observations**
  (the consolidated human-feedback layer), **directives** (hard rules injected
  verbatim into every prompt in scope), **channel memory settings** (mission /
  `expose_org_memories`), and **org skills/plugins** authored via the
  skill-manager MCP tools. These change what agents *say and do* across sessions
  and, for org-wide scope, across the whole org.
- **Work product** (member-shaped): **sheets** (channel-scoped tabular data),
  **apps** (SPAs bound to one sheet in one channel), and **canvas artifacts**
  (agent-generated preview bundles tied to a session). These are outputs created
  inside a channel, like sessions.

Principals that touch these: human org members (by org role), channel members
(by channel/team membership), and — importantly — the **automated actor** (the
agent runtime itself, via agent-proxy MCP tokens, app secrets, and runtime
secrets). The single most important finding in this doc is on the automated-actor
axis: the skill-manager lets an agent perform **org-level** mutations with no
human role gate.

## 2. Backend endpoint inventory

### Memories / observations / directives — all under `RequireAPIKeyScopeOrJWT("memories")`

Write gate helpers: `canManageOrgMemory` (memories) and `canManageMemoryResources`
(directives/observations) — both resolve to **`isAPIKeyRequest || org role owner|admin`**.

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | /v1/memories | memories.go:130 `List` | Reads | scope-gate + `listVisibility` (member sees global + usable channels) | ✅ |
| GET | /v1/memories/grouped | memories_grouped.go:40 `Grouped` | Reads | visibility-scoped | ✅ |
| GET | /v1/memories/channels/{channelId} | memories_grouped.go:91 `ListChannel` | Reads | `canReadChannelMemories` (403 on unusable channel) | ✅ |
| POST | /v1/memories | memories.go:72 `Create` | Mutates (org-wide OR channel) | `canManageOrgMemory` = admin/owner or API key | ✅ (see §4.2 for channel scope) |
| PATCH | /v1/memories/{id} | memories.go:180 `Update` | Mutates | `authorizeMemoryMutation` → `canManageOrgMemory` | ✅ |
| DELETE | /v1/memories/{id} | memories.go:243 `Archive` | Mutates | same | ✅ |
| GET | /v1/directives | directives.go:84 `List` | Reads | `applyChannelVisibility` | ✅ |
| POST | /v1/directives | directives.go:135 `Create` | Mutates (org-wide OR channel) | `canManageMemoryResources` = admin/owner or API key | ✅ (org-wide); ⚠️ channel scope §4.2 |
| PATCH | /v1/directives/{id} | directives.go:200 `Update` | Mutates (active flag) | `authorizeDirectiveMutation` → same | ✅ |
| DELETE | /v1/directives/{id} | directives.go:240 `Delete` | Mutates (soft-delete) | same | ✅ |
| GET | /v1/observations | observations.go:63 `List` | Reads | `applyChannelVisibility` | ✅ |
| POST | /v1/observations/{id}/confirm | observations.go:130 `Confirm` | Mutates | `authorizeObservationMutation` → `canManageMemoryResources` | ✅ |
| POST | /v1/observations/{id}/correct | observations.go:170 `Correct` | Mutates (new row) | same | ✅ |
| POST | /v1/observations/{id}/pin | observations_actions.go:70 `Pin` | Mutates (**creates a directive**) | same | ✅ |
| DELETE | /v1/observations/{id} | observations_actions.go:33 `Delete` | Mutates (+suppression) | same | ✅ |

**Verdict:** the memory-control surface is uniformly gated to **org admin/owner** (or scoped API key). This is the correct *upper bound* per the model (directives = org-wide prompt rules → org admin). The only nuance is that **channel-scoped** memory content is also forced to org-admin, where a member of the channel's team should suffice (§4.2) — over-restrictive, not a leak.

### Channel memory settings (cross-references the Channels doc)

| Method | Path | Handler | Mutates | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| PATCH | /v1/channels/{id} (`memory_mission`, `expose_org_memories`) | channels_mutation.go:163 `Update` → `authorizeChannel(...,true)` → `canManageChannel` | Mutates channel memory config | **API key OR org manager OR channel_member role `owner`** (channels_auth.go:58) | ✅ but **inconsistent** with above (§4.2) |

Note the divergence: channel memory *mission* is manageable by a **channel owner** (the one place `channel_members.role` is used — an ad-hoc channel-owner axis that should collapse into team-membership gating via `CanManageTeamResource`), yet channel-scoped *directives/observations* on the very same channel require **org admin**.

### Sheets — `RequireAPIKeyScopeOrJWT("sheets")` + `ResolveUser`; per-sheet `RequireChannelAccess`

Write gate: `canUseSheetChannel` = org manager, OR API key limited to external/team-less channels, OR channel member via `canUseChannel`. **Member-of-channel = correct (work product).**

| Method | Path | Handler | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | /v1/sheets | sheets.go:95 `ListSheets` | Reads | org-scoped, channel-filtered | ✅ |
| POST | /v1/sheets | sheets.go:135 `CreateSheet` | Mutates | `canUseSheetChannel` (member) | ✅ |
| GET | /v1/sheets/imports/{jobID} | GetImportJob | Reads | org-scoped | ✅ |
| GET/PATCH/DELETE | /v1/sheets/{sheetID} | Get/Update/ArchiveSheet | Reads/Mutates | `RequireChannelAccess` (member) | ✅ |
| POST | .../live-token | LiveToken | Mutates (token) | member | ✅ |
| POST/PATCH/DELETE | .../pages, .../fields, .../rows, .../views | page/field/row/view CRUD | Mutates | member (RequireChannelAccess) | ✅ |
| POST | .../imports, .../operations/{id}/revert | CreateImport / RevertOperation | Mutates | member | ✅ |
| GET | .../export.csv, .../operations | ExportCSV / ListOperations | Reads | member | ✅ |
| GET | /v1/sheets/{sheetID}/live (outside /v1 auth) | Live | Reads (SSE) | short-lived live token, self-validated | ✅ |

### Apps — `ResolveUser`; per-app `canUseAppChannel` (mirrors sheets)

| Method | Path | Handler | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| POST | /v1/apps | apps_rest.go:118 `Create` | Mutates | `canUseAppChannel` (member) | ✅ |
| GET | /v1/apps | apps_rest.go:172 `List` | Reads | member | ✅ |
| GET | /v1/apps/{appID} | apps_rest_read.go:27 `Get` | Reads | `requireApp` → member | ✅ |
| DELETE | /v1/apps/{appID} | apps_rest_read.go:53 `Archive` | Mutates | member (any channel member) | ⚠️ §4.4 |
| GET | /v1/apps/{appID}/launch | Launch | Mutates (mints launch JWT) | member | ✅ |
| POST | /v1/apps/{appID}/versions | Versions | Mutates | member | ✅ |

### Apps internal API — app-secret bearer (automated actor; sandbox-facing)

| Method | Path | Handler | Gate | Correct? |
|---|---|---|---|---|
| GET/POST/PATCH/DELETE | /internal/apps/{appID}/v1/* (sheet structure, live, activity, row CRUD, attachment URL) | apps_internal.go | per-request app-secret bearer (`authApp`), scoped to the app's one bound sheet | ✅ (automated actor, correctly scoped) |

### Canvas artifacts

| Method | Path | Handler | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | /v1/canvas/projects | canvas_projects.go:23 `ListProjects` | Reads | `visibleSessionScope` (member sees only visible sessions) | ✅ |
| GET | /v1/canvas/artifacts | canvas_artifacts.go:33 `ListArtifacts` | Reads | visible sessions | ✅ |
| GET | /v1/canvas/artifacts/{artifactID} | canvas_artifacts.go:57 `GetArtifact` | Reads | visible sessions | ✅ |
| POST | /v1/canvas/artifacts/{artifactID}/preview-url | canvas_artifacts.go:85 `PreviewArtifactURL` | Mutates (mints sandbox token) | visible sessions (member) | ✅ |
| GET/POST | /internal/agents/{agentID}/canvas/{projects,artifacts/sync,brands} | canvas.go / canvas_artifacts.go / canvas_brands.go | Mutates | **runtime secret** (`authorizeRuntimeRequest`) — agent actor | ✅ (automated, agent-scoped) |

**All human-facing canvas routes are reads/preview**; every *write* is on the agent-runtime actor axis and stays within the agent's own project/artifact scope. No cross-scope escape → fine.

### Skill-manager — agent-proxy MCP tools (`internal/skills`)

Registration gate: `NewToolsFunc` (mcptools.go:44) registers these **only when `skillManagerEnabled(agent)`** = `agent.IsDefault` **OR** the agent's `McpToolFilter.Allow` names one of the tools. Scope is `token.OrgID`. **There is no human-role check and no `_hivy_session_id`/actor-user resolution** (contrast sheets MCP tools, which plumb `_hivy_session_id` and resolve the acting channel).

| Tool | File | Mutates | CURRENT gate | Correct? |
|---|---|---|---|---|
| create_org_plugin | mcptools_manage.go:87 | Creates an **org-owned plugin** and **installs it org-wide** | `skillManagerEnabled(agent)` only | ❌ **HIGH** |
| create_skill | mcptools_manage_skills.go:30 | Publishes an **org skill** to every agent with the plugin | same | ❌ **HIGH** |
| update_skill | mcptools_manage_skills.go:177 | Rewrites an org skill's instructions org-wide | same | ❌ **HIGH** |
| archive_skill | mcptools_manage_skills.go (registerArchiveSkillTool) | Archives an org skill org-wide | same | ❌ **HIGH** |

### Memory MCP tools — agent-proxy (for completeness)

| Tool | File | Gate | Correct? |
|---|---|---|---|
| search_memories | memory/mcptools.go:51 | any agent proxy, auto-scoped to session channel — **read-only** | ✅ |
| manage_memories | memory/mcptools_org.go:87 | `agent.IsDefault` only — **read-only** (search + overview) | ✅ |

Agents are deliberately **read-only** on memory (writes flow through background reflection/consolidation, and human confirm/correct/delete happens via the admin-gated REST routes above). This is exactly the discipline the skill-manager lacks.

## 3. Frontend screens & actions

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---|---|---|---|---|
| `app/w/settings/memories/page.tsx` | List/inspect directives & observations | GET /v1/directives, /v1/observations | No | Read: any member (already visibility-scoped server-side) |
| ↳ `_memory-dialogs.tsx` / `_memory-rows.tsx` | Create/toggle/delete directive; confirm/correct/delete/pin observation | POST/PATCH/DELETE directives, POST observations/* | **No** — buttons shown to everyone; backend 403s | Hide/disable mutation controls unless org manager |
| `app/w/settings/_components/nav.tsx` | "Memories" workspace nav item | — | No (shown to all) | At minimum keep visible (read is fine); mutation surface must reflect role |
| `app/w/(chat)/sheets/*` | Create/edit/delete sheet, pages, fields, rows | /v1/sheets/* | No | Member of channel — **correct as-is** (work product) |
| `app/w/(chat)/apps/*` | Create/launch/archive app | /v1/apps/* | No | Member of channel — correct (but archive §4.4) |
| Canvas (in chat views) | View artifact / open preview | /v1/canvas/* | Implicit (session visibility) | Member — correct |
| Skill-manager | (no dedicated human UI) — org skills are authored by the agent through chat | MCP tools | **N/A — that's the problem (§4.1)** | Needs an admin gate on the tool, not a UI |

The frontend has **no role-gating primitives at all** in these screens (no `isOrgManager`/role hooks found). The pattern is "show everything, let the backend 403." For work product (sheets/apps) that's fine because the member *can* act. For the memories admin surface it's a UX/defense-in-depth gap (§4.3). For skills there is no human UI to gate — the gate must live on the tool.

## 4. Ambiguities & lapses (ranked)

### 4.1 — HIGH: skill-manager lets any member's agent write ORG-WIDE skills/plugins with no role gate
`create_org_plugin`/`create_skill`/`update_skill`/`archive_skill` are gated **only** by
`skillManagerEnabled(agent)` (agent is default, or allow-lists the tool) and scoped
by `token.OrgID`. Because the **org's default agent gets these tools automatically**,
**any member who can start a session with the default agent can, by prompting it,
create/update/archive org-wide skills and create+auto-install org plugins.** Skills
are instructions loaded into *every* agent that has the plugin attached, so this is
simultaneously (a) an org-level config mutation performed by a non-admin, and (b) a
standing prompt-injection surface (a member can plant instructions that later run
inside other users' agent sessions). Blast radius: **org-wide agent behavior**. The
tool descriptions say "get the user's explicit approval," but nothing *enforces* a
role or even a human in the loop. This is the crux finding of this doc.

**Related (belongs to the Plugins/Catalog doc, compounds this one):**
`POST/DELETE /v1/plugins/{slug}/install` (plugins.go:110 `Install`) has **no role
gate** — any org member can install/uninstall catalog plugins org-wide.
`create_org_plugin` piggybacks on the same ungated install path.

### 4.2 — MEDIUM: channel-scoped memory content is org-admin-only while channel memory *settings* are channel-owner-manageable
On one channel: editing the **memory mission** needs only `canManageChannel`
(org manager **or** channel_member `owner`), but creating a **channel-scoped
directive/observation/memory** needs full **org admin**. Per `_MODEL.md`,
channel-scoped memory content is channel composition and should be manageable by
a **member of the channel's team** (via `CanManageTeamResource`). So directives/
observations are **over-gated** (org admin where team membership should do) and the
two channel-scoped memory surfaces **use different predicates**. Not a leak — an
inconsistency and a friction point — but it should be unified onto the
team-membership predicate (no new role or migration required).

### 4.3 — LOW: memories settings UI shows admin-only mutations to all members
The Memories settings screen renders create-directive, confirm/correct/delete/pin
controls for everyone; the backend returns 403. Cosmetic + missing defense-in-depth.
Hide/disable when the user is not an org manager.

### 4.4 — LOW: any channel member can archive any app/sheet in the channel
`Archive` (apps) and `ArchiveSheet` only check channel membership, not authorship
or channel-management. Consistent with "work product = member," but a member can
delete another member's app/sheet. Acceptable if the operator treats channel
content as shared; flag if per-creator ownership is desired.

### 4.5 — Note (by design, call out): scoped API keys bypass the human role on memory writes
An API key with the `memories` scope satisfies `canManageOrgMemory`/
`canManageMemoryResources` and can create **org-wide directives** (org-wide prompt
rules). This is the automated-actor axis working as designed, but because directives
are high-impact, the operator should be aware that "memories" scope == "can rewrite
org-wide agent rules."

## 5. Recommendation

**5.1 Skill-manager (fix first).** Add a human-role gate to the four mutating tools.
Two buildable options, in order of preference:
- **Plumb the actor and require org-manager.** Mirror the sheets MCP tools: accept
  `_hivy_session_id`, resolve the session's initiating user, and gate with
  `internal/access` — `access.Resolve(ctx, db, orgID, actorUserID).IsOrgManager()`.
  Register the tools (or accept the call) only when the acting human is owner/admin.
  This keeps `agent.IsDefault` as a *necessary* condition and adds the *human role*
  as the second. (The `_hivy_actor_user_id` plumbing already exists per the
  agent-user-scoping work; skills just don't consume it.)
- If autonomous (human-less) skill authoring must remain possible (schedules/
  triggers), gate instead on an **explicit per-agent capability** that only an org
  admin can grant, and drop the automatic `IsDefault` grant — so the org must
  deliberately designate which agent may author org skills. Do **not** leave it
  implicitly on for the default agent.
- Also gate `POST/DELETE /v1/plugins/{slug}/install` with `RequireOrgAdmin`
  (cross-ref Plugins doc); `create_org_plugin` should route through the same
  admin-checked install path rather than inserting `OrgPluginInstall` directly.

**5.2 Memory content (memories/observations/directives).** Current org-admin gate is
correct for **org-wide** scope — keep it. For **channel-scoped** rows, gate on
`CanManageTeamResource(teamID) = IsOrgManager() || IsTeamMember(teamID)` — i.e. allow
any **member of the channel's team** (or an org manager) to manage channel-scoped
directives/observations/memories, and **unify** it with the `canManageChannel`
predicate that already governs channel memory mission. No new role, column, or
migration is needed — `team_members` membership is the grant. Enforcement:
extend `canManageMemoryResources`/`canManageOrgMemory` to accept a channel scope and,
when set, defer to the team-membership predicate instead of org-admin.

**5.3 Sheets / apps / canvas.** No change to the authorization *level* — member-of-
channel is correct (work product). Optionally scope app/sheet *deletion* to the
creator or a channel manager (§4.4) if the operator wants per-creator ownership.

**5.4 Frontend.** Introduce a shared `isOrgManager` gate (the codebase lacks one on
these screens) and hide/disable memory mutation controls for non-managers. Keep the
backend as the authoritative gate — the UI change is defense-in-depth + UX, never the
only check.

## 6. Deviations from the baseline model

- **Sheets, apps, and canvas artifacts have no natural org-admin gate and
  should not.** They are *work product created in a channel* — the model's "using a
  resource in a channel → Member." They correctly use the `canUseChannel` predicate
  family. This is a deliberate, correct deviation from "creating a resource type →
  Org Admin": a sheet/app is not a catalog resource type, it's an instance of member
  output. No change recommended.
- **Canvas/apps-internal/skill writes live on the automated-actor axis**, not the
  human-role axis, so the org/team/member ladder doesn't map directly. The
  distinguishing test is *scope of the mutation*: canvas writes stay within the
  agent's own artifacts and apps-internal writes stay within the one bound sheet
  (both correctly scoped), whereas skill-manager writes **escape to org scope** — so
  only skills need a human-role gate layered on top of the automated actor.
- **There is no team-scoped role tier** (per `_MODEL.md` — a team-admin tier was
  considered and rejected). This feature currently substitutes **org admin** for
  channel-scoped memory content and **channel_member `owner`** for channel memory
  settings; the inconsistency in §4.2 resolves by gating both on team membership
  (`CanManageTeamResource`), not by adding a role.

## 7. Open questions for the operator

1. **Should an agent be able to author org-wide skills at all**, or only *draft*
   skills that an org admin approves before they go live? The tool copy implies human
   approval but nothing enforces it.
2. **Who is the intended principal for skill/plugin authoring** — org admin only, or
   any member acting through their agent? (Determines whether §5.1 option 1 or 2 is
   right.) And should **autonomous** (schedule/trigger-driven, human-less) sessions be
   allowed to author org skills?
3. **Channel-scoped memory content**: should team members (of the channel's team)
   manage channel-scoped directives & observations (unifying with the memory-mission
   gate), or is org-admin-only the deliberate policy for anything that injects into
   prompts?
4. **App/sheet deletion**: shared channel content (any member may delete), or
   per-creator/channel-manager ownership?
5. **`memories`-scoped API keys can rewrite org-wide directives.** Is that acceptable,
   or should org-wide directive writes require a stricter scope than channel/memory
   reads?
