# Plugin Removal and Flat Skills Refactor

Status: implemented on the refactor branch

Branch: `codex/remove-plugins-flat-skills`

## Purpose

Hivy will remove plugins as a product concept, persistence model, API resource,
runtime permission boundary, and source-code package. Connections already expose
their tools through generated MCP servers, so a plugin no longer provides a
useful runtime abstraction. Skills and connections will become independent,
first-class resources.

This is a pre-launch rewrite. We will reset the database and migration history as
needed. We will not preserve plugin data, plugin endpoints, plugin identifiers,
or compatibility adapters.

## Non-negotiable rules

1. Remove `plugin` and `plugins` from the entire active codebase: backend,
   frontend, routes, models, database schema, manifests, runtime configuration,
   onboarding, agent catalog, tests, fixtures, product copy, and documentation.
2. Delete `global/plugins` and the bundled global skill catalog. Connection
   capabilities are exposed directly as tools by generated MCP servers.
3. Do not add compatibility routes, compatibility columns, aliases, translation
   layers, dual reads, dual writes, or staged plugin-to-skill behavior.
4. Do not add tests whose only assertion is that plugins, plugin routes, plugin
   tables, or plugin UI no longer exist. Tests must cover the behavior that
   replaces them.
5. Connection and skill access remain independent. A connection grant exposes
   generated MCP tools; it never creates or grants a skill.
6. Preserve tenant and team isolation. A connection owned by one org or granted
   to one team must never become visible or executable outside that scope.

## Product model

Hivy has two separate capability types after this rewrite:

- A **connection** gives an agent an MCP server backed by one external account or
  database. Multiple instances of the same provider are allowed.
- A **skill** gives an agent instructions and bundled files. Skills come from an
  org admin or a member of a team.

Standalone plugins have no automatic replacement in this rewrite. Runtime is
the explicit example: delete the Runtime plugin and do not migrate its browser
or drive skills into a catalog. Provider capabilities are represented by their
generated MCP tools. We will design a separate mechanism for other standalone
capabilities later; this refactor must not invent one.

## Skill scopes

### Org skills

An org admin can create an org-owned skill. Org ownership does not mean every
agent can use it. The admin grants the skill to one or more teams through an
explicit team-skill join table.

Admins can list, create, update, and archive org skills. Archiving removes the
skill from effective resolution without destroying its audit history.

### Team skills

A user can create a skill for any team they belong to. The server derives the
org and team from the authenticated actor and request; clients cannot assign a
skill to a foreign org or a team they do not belong to.

The skill belongs directly to the team. There is no containing plugin or other
group resource. Once published, it becomes available to every active agent on
that team.

Team members can list, create, update, and archive skills
for their teams. Org admins retain management access across teams. All handler
authorization must go through `internal/access`.

## Connection scopes and grants

Connections remain org-owned instances. A team gains access to a specific
connection instance, not merely a provider name. This matters when an org has
two GitHub installations, two Slack workspaces, or several Postgres databases.

The replacement for team plugin provisioning is team connection provisioning:

```text
team_connection_grants
  org_id
  team_id
  connection_id
  database_connection_id
  granted_by
  created_at
```

Exactly one of `connection_id` and `database_connection_id` is set. The database
enforces that invariant plus org and team consistency with composite foreign
keys.

An agent inherits every active connection granted to its team. Runtime MCP
compilation loads those concrete connection instances, checks that they are
active and org-scoped, then emits one generated MCP server per connection.

### Nango configuration authority

Nango is the source of truth for configured provider integrations. Operators
configure integrations in Nango, not through Hivy. At startup Hivy fetches
Nango's provider templates and `GET /integrations`, then transactionally
reconciles the configured integrations into its local `integrations` table.
The local table is a projection used for stable foreign keys, connection
sessions, webhooks, resource discovery, and generated MCP routing; Hivy never
creates, updates, or deletes Nango integration configurations.

Startup logs include every discovered provider configuration key, its Nango
provider, its Hivy provider identity, display name, and reconciliation state,
followed by created, updated, unchanged, and unavailable totals. Integrations
removed from Nango are marked unavailable locally instead of cascading through
existing connections and historical records.

The Nango configuration key `github-app-code-reviews` is imported explicitly.
Its underlying Nango provider template remains `github-app`, while Hivy keeps
`github-app-code-reviews` as the local provider identity required by code-review
trigger routing and uses the `usehivy-reviews` bot handle.

Runtime config push publishes the new authorization snapshot without waiting
for MCP startup. It immediately revokes the previous live/activated registry,
then connects to every configured MCP server concurrently in the background,
discovers and caches its tool catalog, and closes every discovery transport.
The dynamic system prompt lists the discovered tool names without their full
schemas. An MCP server remains dormant until the agent activates one of its
tools with `get_tool_details`; the runtime then reconnects that server and keeps
the transport live for subsequent calls. Other servers remain dormant.

Per-agent connection removal is not part of the initial model. Team grants stay
the permission boundary. If the product later needs agent exceptions, add them
as connection-specific overrides rather than reviving a bundle concept.

## Effective skill resolution

Create one resolver as the source of truth for the runtime, skill MCP tools,
agent responses, and authorization checks:

```text
effective skills for an agent =
    published skills owned by the agent's team
  union org skills granted directly to the team
```

The resolver returns the skill and its access source.

Suggested source values:

```text
team_owned
team_grant
```

Skill install counts, if the product still displays them, count distinct active
agents produced by this resolver. They do not count connection rows or team
grants.

## Catalog agents

Delete `required_plugins` and `recommended_plugins` from agent manifests, the
agent catalog model, API responses, install checks, and frontend copy.

Catalog agents declare concrete requirements instead:

```json
{
  "requirements": {
    "connections": ["github-app"]
  }
}
```

Connection requirements identify provider definitions, while installation
checks use the target team's granted connection instances. A catalog agent that
requires `github-app` can install only into a team with an active GitHub App
connection grant.

Agent manifests do not duplicate provider instructions as skill requirements;
the required connection supplies generated MCP tools directly.

## Backend rewrite

Delete these concepts and their callers:

- `model.Plugin`, `PluginIntegration`, `OrgPluginInstall`, `TeamPlugin`, and
  `AgentPluginOverride`;
- `internal/plugins` and `internal/pluginresolve`;
- plugin HTTP handlers, response types, error mappers, and route registration;
- team plugin provisioning and per-agent plugin overrides;
- plugin-keyed MCP tool deny configuration;
- onboarding side effects that install a matching plugin;
- plugin gates around first-party MCP tools such as Agent Builder and Skill
  Manager.

Replace the gates around first-party MCP tools with explicit managed MCP tool
permissions. Connection MCP tool restrictions, if retained, must key by concrete
connection ID so two instances of the same provider can have different settings.

Do not add substitute packages, hidden catalog entries, default skills, or
special-case runtime grants for deleted standalone plugins. Runtime and similar
bundles leave the product for now.

New backend services should have separate packages for skill access and
connection grants, sentinel errors, and one HTTP error mapper per domain. Every
query must remain org-scoped and context-carrying.

## Database and migrations

There is no compatibility period. Rewrite the schema and reset development
databases rather than writing a production-grade plugin data migration.

The final schema must contain no plugin tables, columns, indexes, constraints,
or JSON fields. At minimum:

- remove the plugin tables and `skills.plugin_id`;
- add direct skill scope and ownership fields;
- add team skill grants;
- add team connection grants;
- replace plugin-keyed agent MCP settings;
- rename agent catalog requirement columns.

Because this project has not launched, squash or rewrite the relevant goose
migrations so a new database has never heard of plugins. Update
`internal/testdb/migrations.go` to match the rewritten baseline. Every developer
and test environment must recreate its database after the merge.

## API contract

Remove all plugin routes. Do not leave redirects or deprecated operations in
OpenAPI.

The replacement surface should include:

```text
GET    /v1/skills
POST   /v1/skills
PATCH  /v1/skills/{id}
DELETE /v1/skills/{id}

GET    /v1/orgs/current/teams/{teamID}/skills
POST   /v1/orgs/current/teams/{teamID}/skills
DELETE /v1/orgs/current/teams/{teamID}/skills/{skillID}

GET    /v1/orgs/current/teams/{teamID}/connections
POST   /v1/orgs/current/teams/{teamID}/connections
DELETE /v1/orgs/current/teams/{teamID}/connections/{connectionID}
```

The exact create route can split org skills and team skills if that produces a
clearer authorization contract. Every changed route needs a `@Router`
annotation, regenerated OpenAPI, and regenerated `$api` types in the same
change.

Skill responses include scope and owning team when applicable. Team skill
responses include effective access sources.

## Frontend rewrite

`/w/plugins` becomes `/w/connections`. The page lists provider definitions and
every active connection instance, with controls to add another instance,
reconnect, rename, configure resources, and revoke it.

Settings gets a first-class Skills area:

- admins manage org-owned skills, then grant skills to one or more teams;
- members manage team-owned skills for teams they belong to;
- skill forms edit the actual skill bundle without asking for a plugin slug.

Team settings replace the plugin grant section with connection grants and skill
grants. Agent details show effective skills and connections. Remove plugin logos, plugin helpers, plugin query keys,
plugin copy, and plugin-specific page components instead of renaming them and
leaving the old model underneath.

Onboarding connects one or more accounts, grants the selected connection
instances to the initial team. It never installs a bundle in the background.

## Global catalog layout

The target repository layout is:

```text
global/
  agents/
```

Delete every bundled skill with its plugin, including provider-specific,
Runtime browser, and drive skills. Generated MCP tools now represent provider
capabilities. Delete plugin and integration manifests rather than converting
them; configured provider integrations are discovered from Nango.

## Testing policy

Delete plugin tests and replace only the behavior that still matters. Do not add
negative archaeology tests such as `TestPluginRoutesDoNotExist`, filesystem
checks for a deleted `global/plugins` directory, or schema assertions whose only
purpose is proving a plugin table disappeared.

Required replacement coverage includes:

- a team member can create, update, archive, and delete a skill for their team;
- a member cannot manage a skill for another team or org;
- an admin can create an org skill and grant it to several teams;
- a connection grant exposes that exact MCP connection only to agents on the
  granted team;
- two instances of one provider remain distinct in grants and runtime MCP
  configuration;
- connection grants and skill grants remain independent;
- catalog agent installation validates required connection grants;
- org switching and query invalidation do not leak skill or connection data;
- onboarding creates and grants connections without bundle side effects.

Tests should exercise service and HTTP behavior with real database constraints.
Keep focused unit tests for pure resolution logic, but do not mock away the
authorization joins that protect org and team boundaries.

## Definition of done

The refactor is complete only when:

- active source, generated contracts, frontend copy, fixtures, and docs contain
  no plugin concept;
- a clean database starts from the rewritten migration baseline;
- neither `global/plugins` nor bundled skills under `global/skills` exist;
- standalone bundles such as Runtime have no replacement, alias, or hidden
  auto-grant in the rewritten system;
- external-service MCP servers come only from connection grants; native Hivy
  MCP tools remain controlled by managed first-party MCP configuration;
- all skill access comes through the flat effective-skill resolver;
- catalog agents use required connections;
- `/w/connections` supports multiple instances and `/w/settings` exposes skill
  management;
- OpenAPI and the generated web client match the new routes;
- backend build, vet, lint, frontend type checking, frontend lint, and affected
  end-to-end suites pass.

## Decisions applied

1. Connections expose provider tools directly through their generated MCP
   servers; they do not install skills.
2. Should a required connection on a catalog agent name a provider
   (`github-app`) or a provider capability that can accept several providers?
   This document assumes an exact provider key.
3. Should database and managed integration connections share one table and API
   resource during this rewrite? This document keeps their current storage
   separate but gives them one product surface and one grant abstraction.
4. When a member creates a team skill, may every member of that team edit and
   delete it, or only the creator plus org admins? This document assumes every
   team member can manage it, matching the stated team-scoped CRUD rule.
