# Authorization — Agents, Plugins & Catalog

## 1. Overview

This feature covers three org-level resource types and their lifecycles:

- **Agents** — org-scoped bots (`agents` table, `org_id` set, `parent_agent_id IS NULL`). Created from scratch, installed from the catalog, or seeded as the default "Hivy" agent. Editing touches instructions/tools/model/sandbox/connection-resources. Sub-agents (`type='subagent'`, `parent_agent_id` set) are children created *inside* an agent create/update payload — there is no standalone sub-agent endpoint.
- **Agent catalog** — a global menu of installable agent templates (`agent_catalog` table). Install = materialize a `model.Agent` for the org (+ auto-enable the catalog's required plugins). Uninstall = archive the org's agent(s) for that catalog entry.
- **Plugins** — capability bundles (skills + required connections). Two scopes: **org install** (`org_plugin_installs`) makes a plugin available to the org, and **per-agent enablement** (`agent_plugin_installs`) turns it on for one shared agent. The just-added GitHub-exclusivity guard (`internal/plugins/github_exclusivity.go`) is a data-integrity invariant preventing an agent from holding two GitHub-App identities; it fires at every plugin-write path but is orthogonal to role authz.

Per the baseline model these resources split by whether they bear org connections/credentials. **Agents are org-global**, and per the revised model **agent CRUD (create/edit/delete, model change) is a Member action** — any org member may create/edit/delete agents. **Plugins**, and plain **catalog agent install** that carries no connection, differ from connection-bearing installs (see §5/§7 for the flagged decision): plugin install/uninstall and per-agent plugin enable/disable wire org credentials/connections and stay **Org Admin**; a plain catalog *agent* install (no connection/credential) is a **Member** action, consistent with agent CRUD. Assigning an existing agent to a channel is a team-member concern and lives in `channels.md` (cross-referenced, not duplicated here). Members merely *use* agents in channels. Automated actors reach these routes via API keys scoped `agents`.

**Headline finding:** agent *creation/editing/model-change* is `RequireOrgAdmin`-gated today, but under the revised model this should **RELAX to any org member** (agent CRUD is a Member action) — see §5. Separately, the entire **plugin surface is not gated at all** — org plugin install/uninstall has no role check, and per-agent plugin enable/disable is JWT-open to any member who can see the agent (both should be Org Admin, being connection-bearing). The **frontend has zero role gating** across every agent/plugin screen.

## 2. Backend endpoint inventory

Route file: `cmd/server/serve_routes_v1.go`. All rows sit inside the `/v1` group (`MultiAuth` + `RequireEmailConfirmed` + `ResolveOrgFlexible` + `RateLimit` + `Audit`). "CURRENT gate" lists what is added *on top* of that baseline.

### Agent lifecycle & catalog

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|--------|------|--------------------|---------------|--------------|----------|
| GET | `/v1/agents` | `agents_list.go` `List` (serve:233) | Reads | `RequireAPIKeyScopeOrJWT("agents")` + `VisibleAgentIDsSubquery` | ✅ (reads, visibility-scoped) |
| GET | `/v1/agents/{id}` | `agents_response.go` `Get` (serve:237) | Reads | scope/JWT + visibility | ✅ |
| GET | `/v1/agents/models` | `ListModels` (serve:236) | Reads | scope/JWT | ✅ |
| GET | `/v1/agents/catalog` | `agents_catalog.go:52` `ListCatalog` (serve:234) | Reads | scope/JWT | ✅ |
| GET | `/v1/agents/catalog/{slug}` | `agents_catalog.go:90` `GetCatalog` (serve:235) | Reads | scope/JWT | ✅ |
| POST | `/v1/agents` | `agents_crud.go` `Create` (serve:251) | Mutates (agent **+ sub-agents**) | **`RequireOrgAdmin`** | ✅ |
| POST | `/v1/agents/catalog/{slug}/install` | `agents_catalog.go:120` `InstallCatalog` (serve:252) | Mutates (creates org agent) | **`RequireOrgAdmin`** | ✅ |
| DELETE | `/v1/agents/catalog/{slug}/install` | `agents_catalog.go:186` `UninstallCatalog` (serve:253) | Mutates (archives agents) | **`RequireOrgAdmin`** | ✅ |
| PATCH | `/v1/agents/{id}` | `agents_update.go` `Update` (serve:254) | Mutates (instructions/tools/sub-agents) | **`RequireOrgAdmin`** | ✅ |
| DELETE | `/v1/agents/{id}` | `agents_archive.go` `Archive` (serve:255) | Mutates | **`RequireOrgAdmin`** | ✅ |
| PATCH | `/v1/agents/{id}/model` | `agents_model.go` `UpdateModel` (serve:256) | Mutates | **`RequireOrgAdmin`** | ✅ |
| PUT | `/v1/agents/{id}/connections/{connectionID}/resources` | `agents_connection_resources.go` `UpdateConnectionResources` (serve:257) | Mutates | **`RequireOrgAdmin`** | ✅ |

The eight write routes above are gated purely by the `RequireOrgAdmin` sub-group (serve:249–264); the handlers carry **no inline role check** and rely entirely on the middleware. That is fine as long as the route stays inside the group — but it's brittle (see §4.4).

### Plugins

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|--------|------|--------------------|---------------|--------------|----------|
| GET | `/v1/plugins` | `plugins.go:36` `List` (serve:137) | Reads | **none** (baseline `/v1` only) | ⚠️ read leak, low |
| GET | `/v1/plugins/{slug}` | `plugins.go:75` `Get` (serve:138) | Reads | **none** | ⚠️ read leak, low |
| POST | `/v1/plugins/{slug}/install` | `plugins.go:110` `Install` (serve:139) | Mutates (org-wide install) | **none** | ❌ **HIGH** |
| DELETE | `/v1/plugins/{slug}/install` | `plugins.go:187` `Uninstall` (serve:140) | Mutates (org-wide uninstall) | **none** | ❌ **HIGH** |
| GET | `/v1/agents/{id}/plugins` | `plugins_agent.go:26` `ListAgentPlugins` (serve:239) | Reads | scope/JWT + `loadAgentFromRoute` visibility | ✅ |
| POST | `/v1/agents/{id}/plugins/{slug}` | `plugins_agent.go:80` `EnableForAgent` (serve:240) | Mutates (enable on shared agent) | `RequireAPIKeyScopeOrJWT("agents")` only — **no `RequireOrgAdmin`** | ❌ **HIGH** |
| DELETE | `/v1/agents/{id}/plugins/{slug}` | `plugins_agent.go:137` `DisableForAgent` (serve:241) | Mutates (disable on shared agent) | scope/JWT only — **no `RequireOrgAdmin`** | ❌ **HIGH** |

Note the placement: the plugin org routes (serve:136–141) sit in the *outer* authenticated group — outside even `RequireAPIKeyScopeOrJWT` — so any authenticated, email-confirmed principal (any org member's JWT, or an API key of **any** scope) can hit them. The per-agent enable/disable routes (serve:238–242) are registered **before** the `RequireOrgAdmin` sub-group opens at serve:249, so they inherit only the `agents` scope gate; any JWT (member) passes.

## 3. Frontend screens & actions

Role is available everywhere via `useAuth().activeOrg?.role` (`lib/auth/auth-context.tsx`); the teams screens already gate on `role === "owner" || role === "admin"`. None of the agent/plugin screens use it.

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---------------|--------|-------|-------------------------|-----------|
| `/w/settings/agents` (`page.tsx`) | "Create agent" button; browse catalog | navigates | **No** | Member (any org member); list visible to all |
| `/w/settings/agents/new` (`new/_agent-form.tsx`, `_sub-agents-field.tsx`, `_plugins-field.tsx`, `_tools-field.tsx`) | Create agent + sub-agents + tools + plugin selection | `POST /v1/agents` | **No** | Member screen (create is a member action); plugin selection within it stays admin-only |
| `/w/settings/agents/edit/[id]` (`edit/[id]/page.tsx`) | Edit instructions/tools/sub-agents | `PATCH /v1/agents/{id}` | **No** | Member (agent edit is a member action) |
| `/w/settings/agents/[slug]` (`[slug]/page.tsx`) | Install / Uninstall catalog agent; change model; change sandbox image/size; **toggle per-agent plugins** | `POST`/`DELETE /v1/agents/catalog/{slug}/install`, `PATCH /v1/agents/{id}/model`, `PATCH /v1/agents/{id}`, `POST`/`DELETE /v1/agents/{id}/plugins/{slug}` | **No** | Member for plain catalog-agent install / model / sandbox / edit; **Admin-only for per-agent plugin toggle** (connection-bearing) |
| `/w/(chat)/plugins` (`page.tsx`) | Browse plugin catalog | reads | **No** | List visible to all; actions admin |
| `/w/(chat)/plugins/[slug]` (`plugin-install-action.tsx`, `page.tsx`) | Add / Remove org plugin | `POST`/`DELETE /v1/plugins/{slug}/install` | **No** (`canInstall` only reflects requirement-connectedness, not role) | Admin-only actions |

Every mutating agent/plugin action is rendered for members. The backend agent-CRUD/catalog routes *are* admin-gated today, so a member clicking "Create agent" / "Install" merely gets a 403 toast (confusing UX, LOW) — but under the revised model those routes should relax to any member, so the correct end state is that the buttons render *and* the backend allows them. For the connection-bearing plugin actions the backend is instead **open** when it should be admin-gated, so the UI and API agree on the wrong answer (HIGH).

## 4. Ambiguities & lapses (ranked)

**4.1 — HIGH — Org plugin install/uninstall is completely ungated.**
`POST`/`DELETE /v1/plugins/{slug}/install` (`plugins.go` Install/Uninstall) carry no `RequireOrgAdmin` and aren't even under a `RequireAPIKeyScopeOrJWT` scope. Any org member (or any API key of any scope) can install a plugin org-wide — which, via `EnsureInstalledPluginForEligibleAgents` (`plugins.go:157`), auto-enables it on every eligible non-default agent — or uninstall one, which cascades `disablePluginForOrg` and strips the capability from every agent that had it. Blast radius: a member can grant every shared agent a new capability (e.g. a connector that reaches external systems using org credentials) or knock a capability out from under all channels. This is an org-level catalog mutation performed by a member — a textbook HIGH per the model, arguably CRITICAL given it can wire org connections into agents.

**4.2 — HIGH — Per-agent plugin enable/disable is JWT-open to members.**
`POST`/`DELETE /v1/agents/{id}/plugins/{slug}` (`plugins_agent.go` EnableForAgent/DisableForAgent) sit outside the `RequireOrgAdmin` block; the only gate is `RequireAPIKeyScopeOrJWT("agents")`, which lets every JWT through. `loadAgentFromRoute` (`plugins_response.go:247`) only checks *visibility* (`VisibleAgentIDsSubquery`), not management rights — so a plain member of a channel the agent is assigned to can toggle plugins on that **shared, org-wide** agent, changing its behavior for every other channel and member. Enabling/disabling a capability on a shared resource is agent-editing, which the model puts at Org Admin. The `PluginDetachLock`/required-plugin checks inside the handler are integrity guards, not authz.

**4.3 — LOW/MEDIUM — Plugin reads (`GET /v1/plugins`, `/v1/plugins/{slug}`) have no scope/role gate.**
They live in the bare `/v1` group. Members legitimately may need to see the catalog, so member-readability is defensible, but unlike every sibling read these aren't even behind `RequireAPIKeyScopeOrJWT` — an API key with no relevant scope can enumerate org plugin install state. Low blast radius (install-state + presentation metadata, no secrets), but inconsistent with the credentials/agents/channels read pattern.

**4.4 — LOW — Admin-gated agent writes rely solely on route grouping, no defense-in-depth.**
The eight agent write handlers have no inline `isOrgManager` assertion; they trust the `RequireOrgAdmin` sub-group. Correct today, but a future refactor that moves a route out of the group (exactly what happened to the plugin routes) silently de-gates it. The model recommends predicate-based checks, not position-in-file.

**4.5 — LOW (UI) — No frontend role gating on any agent/plugin screen.**
Create/edit/install/model/sandbox controls render for members. For the correctly-gated backend routes this is a 403-toast UX papercut; for the plugin routes it compounds 4.1/4.2. Frontend must mirror the backend gate and never be the only gate.

## 5. Recommendation

Target principals split by whether the action bears org connections/credentials:

- **Agent CRUD** — create, edit, delete, model change, connection-resource update — is a **Member** action (`CanManageTeamResource`-style: any org member). This is a **RELAXATION** from today's `RequireOrgAdmin` gate on the eight agent write routes (serve:249–264).
- **Plain catalog-agent install/uninstall** (`POST`/`DELETE /v1/agents/catalog/{slug}/install`) — no connection/credential borne — is likewise a **Member** action, consistent with agent CRUD (also a relaxation from today's `RequireOrgAdmin`).
- **Plugin install/uninstall** (`POST`/`DELETE /v1/plugins/{slug}/install`) and **per-agent plugin enable/disable** (`POST`/`DELETE /v1/agents/{id}/plugins/{slug}`) — these **wire org credentials/connections into agents** — stay/become **Org Admin** (`access.Actor.IsOrgManager()` = role `owner|admin`).

> **Flagged decision (connection-bearing vs plain install):** the operator must confirm the split — *connection-bearing* installs (plugins, agent-plugin enablement) = **Org Admin**; *plain agent* install and all agent CRUD = **Member**. The reasoning: plugins carry required connections/credentials, so a member installing one can wire org secrets into shared agents; a plain catalog-agent install materializes only an agent (which a member may already create from scratch). Note the catalog-install route today *auto-enables the catalog entry's required plugins* — if a given catalog entry requires plugins, that specific install is connection-bearing and should be treated as Org Admin. See §7.

> **Shared-org-global-agent caveat (per baseline):** because agents are **org-global** (`agents.org_id`, no `team_id`) and shared across teams, relaxing edit/delete to members means a member of **any** team an agent is assigned to can edit or delete that shared agent — affecting every other team/channel using it. The read-visibility model (`VisibleAgentIDsSubquery`) scopes *which* agents a member SEES, but does **not** prevent a member who can see a shared agent from editing/deleting it. Strict per-team agent isolation (scoping agents to teams via ownership) is a **future option, not built now**.

Concrete fixes:

1. **Relax the agent CRUD + plain-catalog-install routes to any member.** The eight agent write routes and the catalog install/uninstall routes currently sit inside the `RequireOrgAdmin` sub-group (serve:249–264). Move them out so any authenticated org member (JWT or API key scoped `agents`) may create/edit/delete/model-change agents and install a plain catalog agent — gate them only on org membership + `agents` scope, not `RequireOrgAdmin`. (Exception: if a catalog entry's install auto-enables required plugins, treat that path as connection-bearing per the flagged decision.)

2. **Move the four plugin org routes under `RequireOrgAdmin`.** In `serve_routes_v1.go`, keep `GET /v1/plugins` + `GET /v1/plugins/{slug}` readable (ideally behind `RequireAPIKeyScopeOrJWT("agents")` for consistency with agent reads), but wrap `POST`/`DELETE /v1/plugins/{slug}/install` in a `RequireOrgAdmin(database)` sub-group. (serve:136–141)

3. **Move `EnableForAgent`/`DisableForAgent` into the `RequireOrgAdmin` block.** They currently sit at serve:238–242, above the admin group at 249. Relocate the two write routes (keep `GET /v1/agents/{id}/plugins` where it is, as a visibility-scoped read) into the `RequireOrgAdmin` sub-group. Per-agent plugin enablement is connection-bearing, so it stays Org Admin even though the agent it targets is member-editable. `loadAgentFromRoute`'s visibility filter can stay as a second layer.

4. **Add defense-in-depth predicates.** Rather than relying on route position, have the mutating handlers assert the right predicate via a shared helper (extend `internal/access` / the `internal/handler/channels_auth.go` `isOrgManager` mirror): agent-CRUD/plain-install handlers assert org membership; plugin/agent-plugin handlers assert `actor.IsOrgManager()`. This makes 4.4 and any future route move safe. No new column/migration is required — org role and `team_members` membership already exist; this is purely plumbing existing predicates into these handlers/routes.

5. **Frontend: gate the screens on `activeOrg.role` where connection-bearing.** Render the "Create agent"/edit/model/sandbox controls and the plain catalog-agent install for **any member** (they should now succeed backend-side). Reuse the teams-page pattern (`role === "owner" || role === "admin"`) only for the **plugin Add/Remove and per-agent plugin-toggle** controls, hiding them for non-admins (render read-only or a "contact an admin" state). This is UX only — the backend gates in 1–4 remain authoritative.

6. **Sub-agents need no separate authz.** They're created/updated only within `POST /v1/agents` and `PATCH /v1/agents/{id}`, which follow the agent-CRUD gate (now member). Confirm no future standalone sub-agent route escapes that gate; if one is added, it must follow the same member gate. Note the pre-existing runtime caveat (memory: empty subagent `mcp_tool_filter` inherits ALL parent MCP tools) is a capability-scoping issue, not an HTTP-authz one — out of scope here but worth a cross-link.

## 6. Deviations from the baseline model

- **No per-team ownership dimension applies to agents.** Agents are org-global catalog resources with no `team_id`; per the baseline this is realized *without* a migration by letting agent CRUD be a member action and relying on the shipped read-visibility model to scope which agents a member sees. The consequence — a member of any team an agent is assigned to can edit/delete that shared agent — is flagged in §5 as a deliberate deviation from strict per-team isolation (a future option, not built now).
- **The feature is split, not uniformly Org Admin.** Unlike a pure org-catalog resource, agent CRUD and plain catalog-agent install are **member** actions here, while only the connection-bearing plugin surface stays Org Admin. This split follows the "who bears org secrets" line rather than a single "all writes = admin" rule.
- **Reads are intentionally member-visible** and already handled by the just-shipped `VisibleAgentIDsSubquery` plumbing; per instructions I did not re-flag agent reads. The only read nit raised (4.3) is the *plugin* read routes lacking even a scope gate, which is a consistency issue, not a visibility leak.

## 7. Open questions for the operator

1. **Catalog-agent install vs plugin install — confirm the target-role split.** The recommendation treats **plain catalog-agent install/uninstall** (no connection borne) as a **Member** action (consistent with agent CRUD being a member action), while **plugin install/uninstall + per-agent plugin enable/disable** (connection-bearing) stay **Org Admin**. Confirm this split. Edge case to decide: a catalog entry whose install *auto-enables required plugins* is effectively connection-bearing — should that specific install be forced to Org Admin, or should the member install be allowed to auto-wire the required plugins?
2. **Should any non-admin ever install a *plugin* or enable one on an agent?** The recommendation says no (plugins bear org connections/credentials). Confirm there's no product intent to let members self-serve connection-bearing capabilities on shared agents — if there were, it would need per-team agent isolation, not the current member-open state.
3. **Shared-agent edit exposure.** Because agents are org-global and agent edit/delete relaxes to members, a member of any team an agent is assigned to can edit/delete that shared agent for everyone. Is org-global sharing the intended model, or should agents eventually be scoped/owned per team (the future isolation option)? This also affects whether per-agent plugin config should become channel/team-scoped.
4. **Plugin catalog readability for API keys:** should `GET /v1/plugins*` require the `agents` scope (matching agent reads), or stay readable to any authenticated principal? Recommend scoping it for consistency; confirm no integration relies on reading plugins without the `agents` scope.
