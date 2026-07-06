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
{ "query": "railway services inventory", "tags": ["service-discovery", "railway"] }
```

`search_memories` automatically searches the channel your session is running in (plus org-wide facts, when this channel is configured to expose them) — there are no scope arguments. Swap the provider tag (`railway` → `vercel` / `notion` / `slack`) per service. If the results cover what you need and the user hasn't asked for a refresh — use them and stop. Only discover what is missing or explicitly stale.

## The memory contract for discovery facts

**Scope is automatic — the channel you are in.** `retain_memory` always stores the fact in the current channel, and every future session in this channel auto-loads it. There are no owner/visibility/target arguments: an agent only ever reads its own channel's memories (plus org-wide facts the channel is set to expose), so persist an inventory in the channel where it will be used.

**Shape: one compact line per project or service group, under 300 characters.** Injected memory lines are truncated beyond that, so a bloated memory is a broken memory. Pack the durable identifiers — IDs, names, domains — and nothing else:

```json
{
  "content": "railway project acme-prod (id 9f3c2e1a): services api (id 4b7d9f21, domain api.acme.example.com), worker (id 8c1e5a37); environment production (id 2d6f8b44)",
  "tags": ["service-discovery", "railway", "project-acme-prod"]
}
```

**Tags are the index.** Always `service-discovery` + the provider (`railway`/`vercel`/`notion`/`slack`); add `project-<slug>` when a provider has several projects. Max 5 tags, lowercase kebab-case. Consistent tags are what make Step 1's search and the refresh loop work.

**What to persist:** project/service/environment/data-source/channel **IDs**, their **names**, and **domains** — the facts that are stable and expensive to re-enumerate.

**What must NEVER go into memory:** tokens, secrets, environment-variable *values*, connection strings, deployment statuses, message contents. Memory is re-injected into prompts; secrets in memory leak, and volatile data rots.

**Timing fact:** a just-retained memory is auto-loaded from your *next* session in this channel onward, and becomes searchable after a short embedding delay. If a search right after retaining misses it, that is normal — do not retain it again.

## Refreshing — there is no update tool

Memories are immutable to you: no update call exists, and retains do not deduplicate. Refresh is strictly **search → forget the stale entry → retain the fresh one**:

```json
{ "memory_id": "b82fd4c1-…", "reason": "stale service inventory replaced after re-discovery" }
```

`forget_memory` can only archive a memory in your own channel. Never retain a fresh inventory while the stale one still exists — you would duplicate, and both would compete for the injection budget. Forget first, retain second.

## Managing memories across channels (default Hivy agent only)

If you are the org's default agent you have one extra tool: **`manage_memories`** — read and curate memories across *every* channel plus org-wide facts (regular `search_memories` only ever sees the current channel). It also requires the human you are acting for to be an org admin or owner. Use it for:

**Search across everything** — before curating or when the user asks about another channel's inventory:

```json
{ "action": "search", "query": "railway services inventory", "tags": ["service-discovery", "railway"] }
```

Narrow with `channel_id: "<uuid>"` for one channel, or `channel_id: "org"` for org-wide facts. Results carry each memory's `channel_id`.

**Map memory** — totals, per-channel counts, top tags:

```json
{ "action": "overview" }
```

**Archive any memory** in the org:

```json
{ "action": "forget", "memory_id": "b82fd4c1-…", "reason": "duplicate railway inventory superseded by fresh discovery" }
```

Before archiving on the user's behalf, show them the memory's content and channel and get an explicit yes in this session — never batch-forget speculatively.

**Make an inventory available in every channel** — store it org-wide, which folds into any channel configured to expose org memories:

```json
{ "action": "retain", "channel_id": "org", "content": "railway project acme-prod (id 9f3c2e1a): services api (id 4b7d9f21, domain api.acme.example.com); environment production (id 2d6f8b44)", "tags": ["service-discovery", "railway"] }
```

Or pass a specific `channel_id` to seed a particular channel's memory. Only the default agent can write org-wide or cross-channel facts; every other agent is confined to its own channel.

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
- IDs come from tool results only, never from recollection.
- Memories are channel-scoped: retain/search/forget act on the current channel only. Org-wide or cross-channel facts are the default agent's `manage_memories` tool, and only when the user wants them shared beyond this channel.

## Final response checklist

When discovery finishes, your reply must state:

1. What was discovered, per provider, with counts (projects/services/domains/data sources/channels).
2. What was persisted: how many memories, under which tags — and whether org-wide, if you used `manage_memories`.
3. What was skipped and why (provider not connected, volatile data excluded).
4. Any cleanup performed — each memory forgotten and why.
5. That future sessions in this channel will know this automatically, and a refresh is one ask away ("re-discover my Railway services").
