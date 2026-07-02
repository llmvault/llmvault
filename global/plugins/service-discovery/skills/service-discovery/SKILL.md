---
name: service-discovery
description: Use when the user asks to discover, map, inventory, memorize, or refresh their services — Railway projects/services/domains, Vercel projects, Notion data sources, Slack channels — or when you are about to work with one of those services and your memory holds no inventory for it. Discover once, persist the durable IDs to memory, reuse them in every future session.
---

# Service Discovery

Without this skill, every session re-enumerates the same infrastructure: list the Railway projects again, find the service ID again, look up the Slack channel again. That is slow and wasteful. This skill's contract: **discover once, persist the durable identifiers to memory, and never enumerate again until asked to refresh.**

The loop is always the same four steps:

1. **Check memory first** — the inventory may already exist.
2. **Discover** what's missing via the service's proxy (using that service's own skill for exact API syntax).
3. **Persist** compact, durable facts to memory.
4. **Report** what you learned, what you stored, and what was skipped.

## Step 1 — always check memory first

Never discover before searching. Discovery inventories are tagged consistently, so one search tells you what you already know:

```json
{ "query": "railway services inventory", "target": { "owner": "org" }, "tags": ["service-discovery", "railway"] }
```

Swap the provider tag (`railway` → `vercel` / `notion` / `slack`) per service. If the results cover what you need and the user hasn't asked for a refresh — use them and stop. Only discover what is missing or explicitly stale.

## The memory contract for discovery facts

**Target.** Retain with `{"owner": "org", "visibility": "this_agent"}`. This is the only combination that is both private to you AND automatically loaded into your context at the start of every future session. Do not use `owner: "user"` for infrastructure facts (never auto-injected), and do not use `visibility: "all_agents"` unless the user explicitly wants every agent in the org to carry this inventory — that broadcasts into every agent's context.

**Shape: one compact line per project or service group, under 300 characters.** Injected memory lines are truncated beyond that, so a bloated memory is a broken memory. Pack the durable identifiers — IDs, names, domains — and nothing else:

```json
{
  "content": "railway project acme-prod (id 9f3c2e1a): services api (id 4b7d9f21, domain api.acme.example.com), worker (id 8c1e5a37); environment production (id 2d6f8b44)",
  "target": { "owner": "org", "visibility": "this_agent" },
  "tags": ["service-discovery", "railway", "project-acme-prod"]
}
```

**Tags are the index.** Always `service-discovery` + the provider (`railway`/`vercel`/`notion`/`slack`); add `project-<slug>` when a provider has several projects. Max 5 tags, lowercase kebab-case. Consistent tags are what make Step 1's search and the refresh loop work.

**What to persist:** project/service/environment/data-source/channel **IDs**, their **names**, and **domains** — the facts that are stable and expensive to re-enumerate.

**What must NEVER go into memory:** tokens, secrets, environment-variable *values*, connection strings, deployment statuses, message contents. Memory is re-injected into prompts; secrets in memory leak, and volatile data rots.

**Timing fact:** a just-retained memory is auto-loaded from your *next* session onward, and becomes searchable after a short embedding delay. If a search right after retaining misses it, that is normal — do not retain it again.

## Refreshing — there is no update tool

Memories are immutable to you: no update call exists, and retains do not deduplicate. Refresh is strictly **search → forget the stale entry → retain the fresh one**:

```json
{ "memory_id": "b82fd4c1-…", "reason": "stale service inventory replaced after re-discovery" }
```

Never retain a fresh inventory while the stale one still exists — you would duplicate, and both would compete for the injection budget. Forget first, retain second.

## Supervising org memory (default Hivy agent only)

If you are the org's default agent you have one extra tool: **`org_memories`** — supervisor-wide visibility across every agent's org memories (other agents' `search_memories` only ever sees their own plus shared). Use it for two things:

**Check any agent's inventory** before discovering or seeding — this replaces guessing:

```json
{ "action": "search", "query": "railway services inventory", "agent_id": "7c9e6679-…", "tags": ["service-discovery", "railway"] }
```

Drop `agent_id` to search across all agents at once. Results carry each memory's `agent_id`, `agent_name`, and `shared` flag so you can see exactly who knows what.

**Map the org's memory** — totals, shared count, per-agent counts, top tags:

```json
{ "action": "overview" }
```

Privacy boundary you must respect and can rely on: `org_memories` never returns user-scoped personal memories — they appear only as an aggregate `user_scoped_count`. Do not try to inspect them; the tool will not show them.

**Cleaning up another agent's memories — only with the user's explicit approval.** As the default agent you may archive a memory that belongs to another agent, but the flow is strict:

1. Find it (`org_memories` search) and show the user the memory's **content and owning agent**.
2. Get an explicit yes **in this session** — for that specific memory, not a blanket "clean things up".
3. Only then call `forget_memory` with `user_approved`:

```json
{ "memory_id": "b82fd4c1-…", "reason": "duplicate railway inventory superseded by fresh discovery", "user_approved": true }
```

Never set `user_approved` speculatively, never batch-forget without listing each memory to the user first, and note the flag does nothing for non-default agents — it is not an escalation path. User-scoped personal memories remain untouchable regardless of approval.

## Seeding another agent's memory

When the discovered IDs belong in a *different* agent's head — you are coordinating, and a deploy agent will do the work — bind the memory to that agent instead of yourself:

```json
{
  "content": "railway deploy target for acme-prod: service api (id 4b7d9f21, domain api.acme.example.com), environment production (id 2d6f8b44), project id 9f3c2e1a",
  "target": { "owner": "org", "agent_id": "7c9e6679-…" },
  "tags": ["service-discovery", "railway"]
}
```

**Resolve the target agent BEFORE you persist anything:**

1. If you have the `list_agents` tool (the org's default assistant typically does), call it first and match the intended agent by name from the response. If more than one agent could be the target, show the candidates and ask the user which one — never pick silently.
2. If you don't have `list_agents`, ask the user for the agent and use the ID they provide.
3. Never guess or reconstruct an agent ID from memory of past sessions — resolve it fresh, in this session, from a tool result or the user.

Hard rules for seeding:

- `agent_id` and `visibility` are **mutually exclusive** — sending both is rejected. Pick one.
- `agent_id` requires `owner: "org"` and must be an active agent in this org.
- **Only the receiving agent can forget a seeded memory.** You cannot retract it afterward — seed accurate, atomic facts or don't seed.
- **Check before you seed — or acknowledge you can't.** If you are the default agent, run `org_memories` with the target's `agent_id` first and seed only what's missing (stale entries: propose cleanup via the approved-forget flow above). If you are NOT the default agent, seeding is blind — you cannot see the target's memories — so seed only right after a fresh discovery or when the user explicitly asks, tell the user exactly what you seeded, and note that duplicates can only be cleaned up by the receiving agent, the default agent (with user approval), or the dashboard.

## Step 2 — per-service discovery

**Connection check comes first, and failure is loud:** the proxy env vars exist even when a provider isn't connected. An unconnected provider fails at call time with HTTP 404 `{"error":"no <provider> connection for org"}`. When you see that, stop discovering that provider, tell the user it needs connecting (the plugin's page is where they connect it), and continue with the providers that work. Never fabricate an inventory.

For exact API syntax, load the provider's own skill with `skill_view` (`railway`, `vercel`, `notion`, `slack`) — this skill tells you *which* calls to make and *what to keep*:

**Railway** (GraphQL via `$HIVY_RAILWAY_API_URL` + `$HIVY_RAILWAY_API_KEY`):
- `UserProjects` — every workspace and project (the top-level sweep).
- `Project` (per project id) — environments, services, latest deployments, domains.
- `Domains` (per service) — service + custom domains.
- Persist per project: project id+name; each service id+name+domain; environment ids+names. Skip: variables (secrets), deployment statuses (volatile).

**Vercel** (REST via `$HIVY_VERCEL_API_URL` + `$HIVY_VERCEL_API_KEY`):
- `GET /v2/user` and `GET /v2/teams` — identity and team ids.
- `GET /v10/projects` — projects (id, name, framework); paginate.
- `GET /v5/domains` and `GET /v10/projects/{id}/domains` — domains.
- Persist: team ids+slugs; project ids+names+frameworks; domains. Skip: env values (only keys are readable anyway — persist neither), deployment lists.

**Notion** (REST via `$HIVY_NOTION_API_URL` + `$HIVY_NOTION_TOKEN`, always with the `Notion-Version` header per the notion skill):
- `GET /v1/users/me` — the bot identity.
- `POST /v1/search` with `filter.object = "data_source"` (then `"page"` if needed) — data sources and key pages. Search is title-oriented and non-exhaustive; say so if coverage matters.
- `GET /v1/databases/{id}` — a database's data sources.
- Persist: data source ids+titles+urls (the `url`/`public_url` values, never constructed links). Skip: page contents, property values.

**Slack** (Web API via `$HIVY_SLACK_API_URL` + `$HIVY_SLACK_TOKEN`):
- `GET /auth.test` — workspace name, team id, bot identity.
- `GET /conversations.list?types=public_channel,private_channel&limit=100` — channels.
- Persist: team id+workspace name; the channels you are a member of (id+name, note private ones). Skip: message history, user lists (look users up when needed).

Discover one provider at a time, and only the providers the user asked about (or the one you actually need for the task at hand).

## Rules

- Memory first, discovery second — never re-enumerate what you already know.
- One memory per project/service group, under 300 characters, IDs + names + domains only.
- Tags always include `service-discovery` + the provider slug. Consistency is the index.
- Never persist secrets, env values, connection strings, statuses, or message contents.
- Refresh = forget stale, then retain fresh. Never both-alive.
- A 404 "no connection" means stop and tell the user — never invent services.
- IDs come from tool results only — and the seeding `agent_id` is resolved fresh via `list_agents` (or the user) in the current session, never from recollection.
- Don't broadcast (`all_agents`) unless the user explicitly wants org-wide memory.
- `user_approved` on forget_memory is set only after showing the user that exact memory and its owner and hearing yes in this session — never speculatively, never batched.

## Final response checklist

When discovery finishes, your reply must state:

1. What was discovered, per provider, with counts (projects/services/domains/data sources/channels).
2. What was persisted: how many memories, under which tags — and for which agent, if you seeded another agent.
3. What was skipped and why (provider not connected, volatile data excluded).
4. Any cleanup performed — each memory forgotten, its owner, and that the user approved it.
5. That future sessions will know this automatically, and a refresh is one ask away ("re-discover my Railway services").
