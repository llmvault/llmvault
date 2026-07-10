---
name: service-discovery
description: Use when the user asks to discover, map, inventory, or refresh their services — Railway projects/services/domains, Vercel projects, Notion data sources, Slack channels — or when you are about to work with one of those services and your memory holds no inventory for it. Discover once, report the durable IDs clearly so memory captures them, reuse them in every future session.
---

# Service Discovery

Without this skill, every session re-enumerates the same infrastructure: list the Railway projects again, find the service ID again, look up the Slack channel again. That is slow and wasteful. This skill's contract: **discover once, report the durable identifiers clearly so they land in memory, and never enumerate again until asked to refresh.**

The loop is always the same four steps:

1. **Use the session's injected memory first** — the inventory may already exist.
2. **Discover** what's missing via the service's proxy (using that service's own skill for exact API syntax).
3. **Report** the durable facts compactly and explicitly — your stated inventory is what memory captures.
4. **Reuse** remembered IDs in every future session instead of re-enumerating.

## Step 1 — use injected memory first

Never re-enumerate facts already present in the session context. If the injected context covers what you need and the user has not asked for a refresh, use it and stop. Otherwise discover only what is missing or explicitly stale. Treat the current context as channel-scoped unless you use the default agent's org-wide memory view below.

## How discovery facts become memory

**You do not write memory.** Memory is written automatically by background reflection over your sessions: after the session, reflection extracts the durable facts you stated and consolidation folds them into this channel's observations, which future sessions auto-load. Corrections and deletions happen in the memories UI, not through tools.

That makes your final report the write path. State the inventory explicitly and compactly:

**Shape: one clear line per project or service group.** Pack the durable identifiers — IDs, names, domains — and nothing else:

> railway project acme-prod (id 9f3c2e1a): services api (id 4b7d9f21, domain api.acme.example.com), worker (id 8c1e5a37); environment production (id 2d6f8b44)

**What to state for memory:** project/service/environment/data-source/channel **IDs**, their **names**, and **domains** — the facts that are stable and expensive to re-enumerate.

**What must NEVER be stated as a durable fact:** tokens, secrets, environment-variable *values*, connection strings, deployment statuses, message contents. Memory is re-injected into prompts; secrets in memory leak, and volatile data rots.

**Timing fact:** facts from this session become auto-loaded only after background reflection and consolidation run — from a later session onward, not immediately.

## Refreshing

When the user asks for a refresh (or a remembered ID turns out wrong), re-discover and state the corrected inventory plainly, including what changed — e.g. "service api now has domain api2.acme.example.com; the old api.acme.example.com is gone". Consolidation merges the correction into the existing observation rather than duplicating it. If a memory is outright wrong and causing harm, tell the user to delete it in the memories UI — you have no forget tool.

## Viewing memories across channels (default Hivy agent only)

If you are the org's default agent you have one extra tool: **`manage_memories`** — a read-only view over memories across *every* channel plus org-wide facts. It also requires the human you are acting for to be an org admin or owner. Use it for:

**Search across everything** — when the user asks about another channel's inventory:

```json
{ "action": "search", "query": "railway services inventory" }
```

Narrow with `channel_id: "<uuid>"` for one channel, or `channel_id: "org"` for org-wide facts. Results carry each memory's `channel_id`.

**Map memory** — totals, per-channel counts, top tags:

```json
{ "action": "overview" }
```

It has no write actions: seeding or cleaning up another channel's memories happens in the memories UI.

## Step 2 — per-service discovery

**Connection check comes first, and failure is loud:** the proxy env vars exist even when a provider isn't connected. An unconnected provider fails at call time with HTTP 404 `{"error":"no <provider> connection for org"}`. When you see that, stop discovering that provider, tell the user it needs connecting (the plugin's page is where they connect it), and continue with the providers that work. Never fabricate an inventory.

For exact API syntax, load the provider's own skill with `skill_view` (`railway`, `vercel`, `notion`, `slack`) — this skill tells you *which* calls to make and *what to keep*:

**Railway** (GraphQL via `$HIVY_RAILWAY_API_URL` + `$HIVY_RAILWAY_API_KEY`):
- `UserProjects` — every workspace and project (the top-level sweep).
- `Project` (per project id) — environments, services, latest deployments, domains.
- `Domains` (per service) — service + custom domains.
- Report per project: project id+name; each service id+name+domain; environment ids+names. Skip: variables (secrets), deployment statuses (volatile).

**Vercel** (REST via `$HIVY_VERCEL_API_URL` + `$HIVY_VERCEL_API_KEY`):
- `GET /v2/user` and `GET /v2/teams` — identity and team ids.
- `GET /v10/projects` — projects (id, name, framework); paginate.
- `GET /v5/domains` and `GET /v10/projects/{id}/domains` — domains.
- Report: team ids+slugs; project ids+names+frameworks; domains. Skip: env values (only keys are readable anyway — report neither), deployment lists.

**Notion** (REST via `$HIVY_NOTION_API_URL` + `$HIVY_NOTION_TOKEN`, always with the `Notion-Version` header per the notion skill):
- `GET /v1/users/me` — the bot identity.
- `POST /v1/search` with `filter.object = "data_source"` (then `"page"` if needed) — data sources and key pages. Search is title-oriented and non-exhaustive; say so if coverage matters.
- `GET /v1/databases/{id}` — a database's data sources.
- Report: data source ids+titles+urls (the `url`/`public_url` values, never constructed links). Skip: page contents, property values.

**Slack** (Web API via `$HIVY_SLACK_API_URL` + `$HIVY_SLACK_TOKEN`):
- `GET /auth.test` — workspace name, team id, bot identity.
- `GET /conversations.list?types=public_channel,private_channel&limit=100` — channels.
- Report: team id+workspace name; the channels you are a member of (id+name, note private ones). Skip: message history, user lists (look users up when needed).

Discover one provider at a time, and only the providers the user asked about (or the one you actually need for the task at hand).

## Rules

- Injected memory first, discovery second — never re-enumerate what you already know.
- One clear statement per project/service group: IDs + names + domains only.
- Never state secrets, env values, connection strings, statuses, or message contents as durable facts.
- Memory is read-only to you: reflection writes it from what you report; the memories UI is where humans correct or delete it.
- A 404 "no connection" means stop and tell the user — never invent services.
- IDs come from tool results only, never from recollection.
- Memories are channel-scoped: what you report here becomes this channel's memory. The default agent's `manage_memories` tool is a read-only cross-channel view, nothing more.

## Final response checklist

When discovery finishes, your reply must state:

1. What was discovered, per provider, with counts (projects/services/domains/data sources/channels).
2. The durable inventory itself, one compact line per project/service group — this is what memory captures, so make it explicit.
3. What was skipped and why (provider not connected, volatile data excluded).
4. Any corrections to previously remembered facts, stated plainly (old value → new value).
5. That future sessions in this channel will know this automatically once reflection runs, and a refresh is one ask away ("re-discover my Railway services").
