# Team-Resolved Agent Plugins

Remove per-agent plugin installs (`model.AgentPluginInstall`) and resolve every agent's
plugins from its team. Agents ALWAYS belong to a team (`agents.team_id` becomes NOT NULL —
in scope, D4). The `agent_plugin_installs` table stays in Postgres untouched (orphaned);
all Go code that reads or writes it is removed.

Trigger: production incident 2026-07-08 — sandbox repo clone silently skipped because the
clone query gates on `agent_plugin_installs`, which no agent had, while `team_plugins` had
the GitHub grant. See memory `sandbox-repo-clone-gating`.

## Target resolution semantics

One resolver, one definition, used everywhere:

```
EffectivePluginIDs(agent) =
    autoInstallPlugins()                                  // global active, manifest auto_install
  ∪ defaultAgentPlugins()            if agent.IsDefault   // manifest default_agent_install
  ∪ (team_plugins[agent.team_id] ∩ orgInstalledActive(agent.org_id))

  then apply the GitHub-pair rule (D1): if the set contains BOTH github and
  github-code-reviews, keep exactly one — the one in the agent's catalog
  required_plugins, else github — and drop the other.
```

- `agents.team_id` is never NULL (enforced by D4); the resolver takes it unconditionally.
- New home: `internal/plugins/resolve.go` (`EffectivePluginIDs`, `AgentHasPluginSlug`,
  `EffectivePluginSlugs`). `internal/plugins` must not import `teamprovision`
  (cycle — teamprovision imports plugins); it queries `model.TeamPlugin` directly.
- Query sites that today JOIN `agent_plugin_installs` switch to
  `plugin_integrations.plugin_id IN (?)` with the resolved ID set (one extra small query;
  plugin sets are tiny). No duplicated SQL across gates.

## Decisions (FINAL — signed off 2026-07-08)

**D1 — GitHub identity: exactly one per agent, picked explicitly.** An agent's effective
set NEVER contains both `github` and `github-code-reviews`. When a team grants both, the
resolver keeps the one named in the agent's `agent_catalog.required_plugins` (e.g. Zuko →
`github-code-reviews`), otherwise `github`. This is enforced at the effective-set level —
credentials, MCP tools, and skills all see one identity — not just at connection
resolution. Delete `internal/plugins/github_exclusivity.go` and all call sites; the pair
rule in the resolver replaces it. `connectionaccess.ResolveAgentProviderAny` /
`git_credentials` then resolve naturally from the single surviving plugin.

**D2 — Team admins cannot disable a plugin that active team agents require.**
`teamprovision.DisablePlugin` gains a guard: error (409-style, named error → HTTP 409)
if any non-archived agent on the team has the plugin's slug in its catalog's
`required_plugins`. Same guard on org-level plugin uninstall (`handler/plugins.go`
uninstall path) for teams in that org. Catalog install keeps its existing hard-block
(`teamMissingRequiredPlugins`). No catalog-required union in the resolver — the guard
makes team grants authoritative and sufficient.

**D3 — Agent-builder MCP loses plugin management entirely.** `plugin_slugs` is removed
from the `create_agent`/`update_agent` tool schemas. Any agent created via the
agent-builder automatically has all its team's plugins: the MCP detects the team from the
session's channel (`channel.team_id`) and creates the agent with that `team_id` — nothing
else to wire. `attachPlugins`, `replacePlugins`, `protectedAgentPluginIDs` in
`internal/agents/service_subagents.go` are deleted. Tool descriptions updated to say
plugins are team-managed.

**D4 — `agents.team_id` becomes NOT NULL. IN SCOPE.**
- Migration `000085_agents_team_id_not_null.sql`: backfill any NULL `team_id` to the org's
  oldest team (create one if an org somehow has none — prod has zero NULL-team agents and
  every org has a forced first team); then `ALTER COLUMN team_id SET NOT NULL` and change
  the FK from `ON DELETE SET NULL` to `ON DELETE RESTRICT` (teams are archive-only, never
  deleted — locked launch decision).
- `internal/model/agent.go`: `TeamID *uuid.UUID` → `uuid.UUID`.
- Rework `ensureHivyAgent` (`agents_create.go:37`, via `agents_provision_runner.go`): the
  teamless org-level fallback is removed; it resolves the org's first team (provisioning
  it via the existing first-team flow if absent) and ensures that team's Hivy instead.
  `createHivyAgentWithDefaultsTx` takes a non-pointer `teamID`.
- Ripple (~6 non-test agent sites lose nil-handling): `agents_response.go:153`,
  `agents_mutation_helpers.go:97-104`, `agentschedule/channel.go:48`,
  `channelagents/acts.go:59`, `channels_data.go:71`, plus `plugins_agent.go` (deleted
  anyway). `channel.TeamID` stays nullable — untouched.
- `internal/testdb/migrations.go`: `latestMigrationVersion` 84 → 85.

**D5 — ALL per-agent plugin routes removed, including GET.** Agents have all of their
team's plugins — there is no per-agent plugin state to serve. Remove
`GET /v1/agents/{id}/plugins`, `POST`/`DELETE /v1/agents/{id}/plugins/{slug}` and the
whole `internal/handler/plugins_agent.go`. The frontend reads the team's plugins
(`GET /v1/orgs/current/teams/{teamID}/plugins`) where it needs to show what an agent has.
`enabled_agent_ids` on plugin list responses stays, derived (agents of granting teams;
all agents for auto-install; default agents for default-agent plugins) — it still powers
example-prompt filtering and plugin-page display.

**D6 — `skills.install_count`.** Was `COUNT(*) FROM agent_plugin_installs WHERE plugin_id=?`.
Becomes: count of non-archived agents whose effective set contains the plugin (all agents
for auto-install; agents in granting teams otherwise). Computed in Go at the existing
refresh points.

**D7 — Org uninstall hygiene.** `disablePluginForOrg` (looped per-agent deletes) is
replaced by deleting the plugin's `team_plugins` rows for that org when an org plugin is
uninstalled — after the D2 guard passes. The resolver's `∩ orgInstalledActive`
intersection already makes stale rows inert; the delete keeps data clean.

## Phase 1 — D4 migration + resolver

- Migration `000085` + model `TeamID` value type + `ensureHivyAgent` rework + testdb bump
  (do this first: the resolver's unconditional `agent.team_id` depends on it).
- `internal/plugins/resolve.go`: `EffectivePluginIDs`, `EffectivePluginSlugs`,
  `AgentHasPluginSlug`, plus `EffectiveAgentIDsForPlugin(orgID, pluginID)` (the reverse
  mapping for `enabled_agent_ids`).
- Unit tests: auto-install, default-agent, team grant, org-uninstalled exclusion, inactive
  plugin exclusion, GitHub-pair rule (catalog-required wins; plain agent → github; never
  both; single-plugin teams unaffected).

## Phase 2 — Repoint the runtime gates (the substantive behavior change)

| Site | Today | Change |
|---|---|---|
| `internal/connectionaccess/access.go:42,98` | JOIN agent_plugin_installs | filter by effective IDs (pair rule already applied by resolver) |
| `internal/agentruntime/repositories.go:81` | same JOIN (the incident) | filter by effective IDs |
| `internal/handler/git_credentials.go:167` | via connectionaccess | inherits |
| `internal/handler/database_proxy.go:143-153` | JOIN | effective IDs |
| `internal/sheets/mcptools.go:83-91`, `internal/apps/mcptools.go:83-91`, `internal/handler/apps_preview_env.go:68` | COUNT join | `AgentHasPluginSlug` |
| `internal/skills/mcp_resolve.go:23-48` | Pluck plugin_ids | effective IDs |
| `internal/agents/mcptools_result.go:47-65` | Pluck → slugs | `EffectivePluginSlugs` |
| `internal/handler/agents_response_helpers.go:111-137` | Find installs → skills | effective IDs |
| `internal/handler/triggers_helpers.go:232` + provider proxies (`provider_raw_proxy.go`, `linear_proxy.go`, `notion_proxy.go`) | via connectionaccess | inherit |

## Phase 3 — API surface + write-path removal

- Routes (`cmd/server/serve_routes_v1.go:249-251`): remove all three agent-plugin routes (D5).
- Delete `internal/handler/plugins_agent.go` entirely.
- `internal/handler/plugins_install.go`: delete `enablePluginForAgent`,
  `disablePluginForAgent`; replace `disablePluginForOrg` per D7 (+ D2 guard);
  `refreshPluginSkillInstallCounts` per D6 (also its duplicates in
  `autoinstall_default_agent.go:141`, `skills/mcptools_manage_helpers.go:267-273`).
- `internal/handler/plugins_list_data.go`: `enabled_agent_ids` from
  `EffectiveAgentIDsForPlugin`, still filtered through `VisibleAgentIDsSubquery`.
- `internal/teamprovision/plugins.go`: D2 guard in `DisablePlugin`.
- `internal/plugins/autoinstall.go` + `autoinstall_default_agent.go`: strip every
  `agent_plugin_installs` INSERT. What survives: manifest readers
  (`PluginAutoInstall`/`PluginLocked`/`PluginDefaultAgentInstall`) and org-install ensuring
  at plugin sync / org install. Callers in `agents_create.go:131,136`, `agents_crud.go:198`,
  `agents/service.go:187`, `agents_catalog_install_helpers.go:159` drop the per-agent step.
- Catalog install (`agents_catalog.go`, `agents_catalog_install_helpers.go`): keep the
  team hard-block; delete `enableRequiredCatalogPlugins`.
- `internal/agents/service_subagents.go`: per D3 — delete plugin sync; `create_agent`
  resolves `team_id` from the session's channel.
- Delete `internal/plugins/github_exclusivity.go` (D1) and `internal/plugins/detach_lock.go`
  (its only remaining consumer, the per-agent disable path, is gone).
- Delete `model.AgentPluginInstall` (`internal/model/plugin.go:71-81`). Table untouched;
  `internal/testdb/migrations.go:185` entry stays.

## Phase 4 — Tests

- Rewrite seeds to `team_plugins` using the existing `grantPluginToTeam` pattern
  (`plugins_agent_team_test.go:75`). Affected helper/fixture files:
  `visibility_contract_fixture_test.go`, `authorization_contract_seed_test.go`,
  `plugin_access_test.go`, `triggers_github_mention_test.go`,
  `database_integrations_scope_test.go`, `connections_resources_test.go`,
  `apps_preview_env_harness_test.go`, `sheets/apps mcptools_helpers_test.go`,
  `connectionaccess/access_test.go`, `agentruntime/repositories_test.go`,
  `compile_integration_test.go`, `skills/mcptools_manage_test.go`.
- Delete tests of removed API/semantics: `plugins_agent_team_test.go`,
  `plugins_default_agent_disable_test.go`, `github_exclusivity_test.go`,
  `plugins_auto_install_test.go` row-count assertions (`assertAgentPluginInstalled` is a
  shared helper — replace with an effective-resolution assertion),
  `agents/service_plugin_protection_db_test.go`, `autoinstall_test.go` row counts.
- Rewrite contract tests to derived/team semantics: `plugins_visibility_test.go`
  (`enabled_agent_ids` actor-scoping), `agents_catalog_install_team_test.go`,
  `auth_signup_test.go:141` (assert Hivy's effective set),
  `route_gating_contract_test.go` / `authorization_contract_*` (removed routes),
  `kara_runtime_flow_test.go`, `service_db_test.go:187`.
- New tests: D1 pair rule end-to-end (Zuko + coder on one team → each resolves its own
  identity in git_credentials); D2 guard (team disable blocked while catalog agent active;
  allowed after archive); D4 (agent creation without team fails; ensureHivyAgent
  provisions first team); repo-clone regression (team grant alone ⇒ repos in workspace
  config — the incident as an acceptance test).
- e2e: `agent_sessions_sheets_e2e_test.go:83` (HTTP enable call → team grant),
  `agent_sessions_{skills,plugin,isolation}_e2e_test.go` seed/cleanup swaps.

## Phase 5 — OpenAPI + frontend

- Regenerate first (hard rule: generated `$api` hooks only): `make openapi` →
  `docs/{swagger,openapi}.json` → `npm run generate` → `apps/web/lib/api/schema.d.ts`.
- `agents/[slug]/page.tsx` + `_agent-plugin-sections.tsx`: per-agent toggle grid replaced
  by a read-only "plugins from team" display sourced from
  `GET /v1/orgs/current/teams/{teamID}/plugins` (+ auto-install), with a "manage in team
  settings" link for admins. Remove both mutations, `handleAgentPluginToggle`, and the
  `queryKeys.agentPlugins()` key.
- `agents/edit/[id]/page.tsx`: remove the enable/disable diff-sync loop (`:64-86`) and
  plugin form section. `agents/new/page.tsx` + `_agent-form.tsx` + `_plugins-field.tsx`:
  remove the per-agent plugin picker; show a read-only preview of the selected team's
  plugins instead.
- `agents/_lib.ts:208 pluginEnabledForAgent` and `plugin-example-prompts.tsx:44-46`:
  unchanged — `enabled_agent_ids` keeps its meaning, now derived.
- Team settings page (`w/settings/teams/[teamId]/team-provisioning.tsx`): copy tweak —
  enabling a plugin grants it to all team agents; disable can 409 per D2 (surface the error).
- `agents/_lib.test.ts:241` update.

## Phase 6 — Verify + ship

- Pre-deploy prod checks (all must return zero rows):
  ```sql
  -- agents that would LOSE a non-system plugin
  SELECT api.agent_id, p.slug FROM agent_plugin_installs api
  JOIN plugins p ON p.id = api.plugin_id
  JOIN agents a ON a.id = api.agent_id
  WHERE COALESCE((p.manifest->>'auto_install')::boolean, false) IS NOT TRUE
    AND COALESCE((p.manifest->>'default_agent_install')::boolean, false) IS NOT TRUE
    AND NOT EXISTS (SELECT 1 FROM team_plugins tp
                    WHERE tp.team_id = a.team_id AND tp.plugin_id = api.plugin_id);
  -- teamless agents (must be zero before migration 000085's SET NOT NULL)
  SELECT id, name FROM agents WHERE team_id IS NULL;
  ```
- End-to-end: new session on Anna's team → `usehivy/hivy` clones (the original incident
  becomes the acceptance test).
- `/verify` + full test suite; watch the local test-stack gotcha (stray `hivy-test-pg`
  on :5433 with stale schema).

## Blast radius

**Backend, non-test: ~30 files.** Substantive behavior: 9 runtime gates (Phase 2) + the
D4 type change (~6 nil-check sites + ensureHivyAgent). Mechanical removal: ~12 files.
Routes: 3 removed. **One migration** (000085, agents.team_id NOT NULL; testdb → 85).
**Tests: ~30 Go test files** (5 shared fixtures do most of the damage), 5 e2e files.
**Frontend: ~8 files** + regenerated `schema.d.ts`.

Behavior changes shipped on purpose:
1. Granting a plugin to a team instantly grants it to **all** the team's agents — including
   credentials-bearing plugins (GitHub, Slack). The team is the privilege boundary.
2. Team plugin disable instantly strips all team agents, and is refused (409) while an
   active catalog agent requires the plugin (D2). Running sandboxes keep their last pushed
   config until the next config push — `enqueueConnectionResourceReconcile` is still a
   stub (pre-existing, unchanged here).
3. GitHub identity is exactly one per agent via the D1 pair rule (catalog-required wins,
   else `github`); the hard exclusivity guard is gone and teams may grant both.
4. Agents can never be teamless; agent-builder-created agents inherit the session
   channel's team and therefore its plugins.
5. Fixes the incident class: repo cloning, connection access, DB proxy, sheets/apps tools,
   and skills all light up from a single team grant.

Risk hot spots, in order: `connectionaccess` (every provider proxy, git credentials,
triggers flow through it), D1 pair-rule correctness (Zuko + coder on one team),
D4 migration on prod (`SET NOT NULL` — verify zero NULLs first), `enabled_agent_ids`
visibility scoping in `plugins_list_data.go`, the shared test fixtures.
