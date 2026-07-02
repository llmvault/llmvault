---
name: agent-builder
description: Use this skill whenever the user wants to create, build, set up, configure, edit, improve, or manage an agent (or its sub-agents) in this Hivy organization — including choosing a model, picking tools or plugins, writing an agent's instructions, or reviewing what agents already exist. This is the execution skill for the agent-builder MCP tools, with exact payloads for create_agent and update_agent.
---

# Building agents in Hivy

You help the user create and improve the other agents in their Hivy organization. Run it like a short consulting engagement: understand the job, propose a design, confirm it, build it, verify it, and hand it off. Work in small, verified steps — never rush ahead.

Your five tools:

- **`list_agents`** — the top-level agents that already exist (id, name, model, status). Check it before creating (avoid duplicates) and whenever the user says "my agent."
- **`get_agent`** — one agent's full setup: instructions, model, plugins, skills, tools, sub-agents, plus its page `url`. Always read an agent with this **before** you change it.
- **`list_org_plugins`** — the org's capabilities, split into `installed` and `available`, with each plugin's `skills`, `required_connections`, an `install_url`, and (for available ones) `missing_requirements`.
- **`create_agent`** — create a new agent, optionally with sub-agents.
- **`update_agent`** — patch an existing agent: only the fields you send change, but **any array you send replaces that whole list**.

## Tool reference — exact payloads

**`list_agents`** takes no input:

```json
{}
```

Response shape: `{ "agents": [ { "id", "name", "description", "model", "status", "is_default" } ] }`. Sub-agents and archived agents are not listed.

**`list_org_plugins`** takes no input:

```json
{}
```

Response shape: `{ "installed": [...], "available": [...] }` where each plugin is `{ "id", "slug", "name", "description", "category", "skills": [{ "slug", "name", "description" }], "required_connections": [{ "provider", "kind", "required" }], "install_url" }` — and `available` entries also carry `missing_requirements`. The `slug` values here are what `plugin_slugs` accepts; the skill `slug` values are what `skills` accepts. Only ever use slugs you saw in this response.

**`get_agent`**:

```json
{ "agent_id": "7c9e6679-…" }
```

Response shape: `{ "agent": { "id", "name", "description", "instructions", "model", "status", "is_default", "plugins": ["slug"], "skills": ["slug"], "tools": ["id"], "sub_agents": [{ "id", "name", "description", "instructions", "model", "skills", "tools" }] }, "url": "…" }`. The `plugins`/`skills`/`tools` arrays are exactly what you re-send (plus or minus your change) when updating — see the worked example below.

**`create_agent`** — full worked example (a support triage agent with one sub-agent):

```json
{
  "name": "Support Triage",
  "description": "Triages incoming support requests, drafts replies, and escalates edge cases to a human.",
  "instructions": "You are Support Triage for the team's support inbox.\n\nYour job: classify each incoming request (bug, billing, how-to, feature request), answer the ones covered by known solutions, and escalate anything ambiguous, angry, or contractual to a human — never guess on those.\n\nHow to work: read the full request first. Search the web only when the answer likely changed recently. When you draft a reply, delegate to your Responder sub-agent and review its draft before sending.\n\nBoundaries: never promise refunds, legal terms, or timelines. Never reply to legal threats — escalate.\n\nVoice: warm, direct, under 8 sentences.",
  "plugin_slugs": ["github"],
  "skills": ["github-triage"],
  "tools": ["web_search", "web_fetch"],
  "sub_agents": [
    {
      "name": "Responder",
      "description": "Drafts the customer-facing reply for requests Triage has classified.",
      "instructions": "Draft a reply for the classified support request you are given. Match the team voice: warm, direct, no filler. Return only the draft.",
      "tools": ["web_fetch"]
    }
  ]
}
```

Note what the example does **not** include: `model` is omitted (org default — see Model selection), the sub-agent has no `model` field (sub-agents always inherit the parent's model; the schema has no such field, so do not send one), and `tools` lists **only optional capabilities** — every agent you create automatically gets the baseline sandbox tools (shell, file read/write/search, planning) and the read-only floor (`skills_list`, `skill_view`, `list_channels`). Never list baseline tools; they are not in the enum.

The response is `{ "agent": { …same shape as get_agent… }, "url" }`. Verify it: confirm the `plugins`, `skills`, `tools`, and `sub_agents` in the response match what you intended before telling the user anything succeeded.

**`update_agent`** — a true patch. Only provided fields change; `agent_id` is required:

```json
{ "agent_id": "7c9e6679-…", "description": "Triages support requests for the EU team." }
```

Archive an agent (removes it from list_agents; it stops running):

```json
{ "agent_id": "7c9e6679-…", "status": "archived" }
```

## Field reference

| Field | Where | Required | Constraints |
|---|---|---|---|
| `name` | create; update (optional) | create: **yes** | Unique per org among top-level agents. The org's default agent **cannot be renamed**. |
| `description` | both | no | Short, one sentence. |
| `instructions` | both | no | The agent's system prompt (see Writing instructions). |
| `model` | both | no | **Strict enum in the schema — pick only from it.** Omit on create → org default; omit on update → unchanged. |
| `status` | update only | no | `active` or `archived`. |
| `plugin_slugs` | both | no | Must be **installed for the org and active** — from `list_org_plugins.installed`. Replaces the set on update. Auto-installed org plugins stay attached no matter what you send. |
| `skills` | both | no | Skill slugs from **installed** plugins only (shown under each plugin in `list_org_plugins`). Replaces the set on update. |
| `tools` | both | no | Strict enum of **optional capabilities only** (see Tools). Replaces the optional set on update; the automatic baseline is never affected. |
| `sub_agents` | both | no | Array of `{ name (required), description, instructions, skills, tools }`. Replaces the **entire** set on update — sub-agents are deleted and recreated. Names must be unique within one parent. |

**NOT settable with these tools** — do not promise them or try to fake them via fields that don't exist: channels, schedules, sandbox image/size, permission toggles, per-sub-agent models, MCP servers. The user configures those on the agent's page — share the `url` from the response and tell them where to look.

### Tools — baseline is automatic, the enum is the extras

**Every agent you create automatically gets** (do not list these; they are not in the enum):
- The baseline sandbox tools: `bash`, `check_bash_status`, `read_file`, `write_file`, `apply_patch`, `file_search`, `glob`, `grep`, `multi_grep`, `update_plan`, `search_sessions`, `request_user_input`.
- The read-only MCP floor: `skills_list`, `skill_view` (how it reads its skills — granting `skills` alone is enough), and `list_channels`.
- `subagent_task`, added automatically whenever the agent has `sub_agents`.

**Optional capabilities — the only valid values for a parent's `tools`:** `lsp`, `web_search`, `web_fetch`, `generate_image`, `generate_vector_image`, `search_memories`, `retain_memory`, `forget_memory`, `search_knowledge_base`, `cron`, `create_http_trigger`. The schema's `tools` enum is the authoritative list. Grant only what the job needs.

Sub-agents' `tools` accept the full tool set (baseline included), so a deliberately narrow read-only sub-agent is expressible; a sub-agent with no `tools` defaults to read-only file tools.

**`create_agent`, `update_agent`, and `list_org_plugins` are not grantable.** You cannot build another builder; don't try, and don't promise it.

## Worked example — add one tool without wiping the rest

Lists **replace**, they never merge. To add `generate_image` to the Support Triage agent, first `get_agent`:

```json
{ "agent_id": "7c9e6679-…" }
```

The response shows `"tools": ["web_search", "web_fetch"]` — the echo lists **only the optional capabilities**; baseline tools, the read-only floor, and the auto-granted `subagent_task` are on the agent but never appear here. Send back the **full current list plus the new tool**:

```json
{
  "agent_id": "7c9e6679-…",
  "tools": ["web_search", "web_fetch", "generate_image"]
}
```

Sending `"tools": ["generate_image"]` instead would strip `web_search` and `web_fetch`. The baseline and floor are never affected by what you send — only the optional set replaces. The same rule applies to `plugin_slugs`, `skills`, and `sub_agents` — for `sub_agents`, re-send every sub-agent you want to keep (from `get_agent`'s `sub_agents`, minus their `id`/`model` fields), or they are deleted.

## Model selection

Decide by the kind of work — but by **rule**, never from a memorized list of model names:

- **Default: omit `model`.** The org default is fast, inexpensive, and right for most agents. Only override when you can say why in one sentence.
- **The schema's `model` enum is the only source of truth.** It is generated from the live model catalog for this deployment. Never type a model id from memory — pick one you can see in the enum.
- Rough guide for overriding: complex multi-step reasoning, high-stakes judgment, or hard coding → one of the enum's top reasoning models; high-volume routing/classification/short replies → one of its fast, cheap models.
- **Being in the enum does not guarantee the org can use it.** Model access is credential-gated per org; if create/update rejects a model for credentials, tell the user which model needs credentials, and either omit `model` or pick another.

Changing a model with `update_agent`:

```json
{ "agent_id": "7c9e6679-…", "model": "MODEL_ID_FROM_ENUM" }
```

(Replace `MODEL_ID_FROM_ENUM` with an id you selected from the schema enum — never a name the user or you remembered.)

## The engagement: interview → design → confirm → build

**Interview first.** Don't turn a one-line request into an agent. Learn: the purpose and 2–3 concrete example requests it will handle; who talks to it and where; what systems/data it must touch (drives plugins/tools); tone; what a great vs. bad response looks like; hard boundaries; volume (informs model). If the user is vague, propose a concrete draft ("Here's what I'd build: …") and let them react. Stop interviewing once you can state the agent's job in two sentences.

**Name it well.** Short, Title Case, describing the job: "Support Triage", "Release Notes Writer". Unique in the org (check `list_agents`; qualify if taken — "Support Triage – EU"). Avoid generic names ("Assistant", "Bot"), emojis, and sentences-as-names.

**Choose capabilities with least privilege.** Call `list_org_plugins`, map each need to a capability ("read the web" → `web_search`/`web_fetch`; "work with GitHub" → the GitHub plugin), and grant only what the job needs — a focused agent behaves far better than one with everything switched on. If a needed plugin is under `available` or shows `missing_requirements`, you cannot install or connect it yourself: share its `install_url` and continue with what's installed.

**Write instructions that are specific and testable.** Second person. Structure: role & mission (1–2 sentences) → main tasks and how to handle each → how to work (approach, when to use which tool, when to ask) → boundaries and human-handoff cases → voice & format → one or two short "when asked X, respond like Y" examples. Prefer checkable rules ("reply in under 5 sentences") over adjectives ("be concise"). Reference only capabilities the agent actually has. No filler, no contradictions, no walls of text.

**Sub-agents only when they earn their place.** Add them when the job splits into distinct specialties or phases that each benefit from their own focused instructions and tools (Researcher + Writer; Reviewer + Fixer; Triage + Responder). Don't add them for a single-purpose agent, when they'd share the parent's tools and instructions (that's overhead, not specialization), or to look thorough. Each sub-agent gets a tight single-responsibility brief and only the tools/skills it needs. Remember: they inherit the parent's model, and `sub_agents` on update replaces the whole set.

**Confirm before you create or change.** Summarize the plan — name, purpose, model choice (or "org default"), plugins/tools/skills, sub-agents — and get a yes.

## Verify every action

After **every** tool call, read the result before moving on:

- After `list_org_plugins`: the plugin/skill you plan to use is present, installed, and has no `missing_requirements`.
- After `create_agent`/`update_agent`: the response's `plugins`, `skills`, `tools`, `sub_agents` match your intent. Fix discrepancies before reporting success.
- Before `update_agent`: `get_agent` first — right agent, current lists in hand.
- On any error: stop, read it, correct the input, retry. Never continue as if it worked, and never tell the user something succeeded until a tool result proves it.

## Errors and recovery

The tools return precise errors — match on these and act:

| Error contains | Meaning → what to do |
|---|---|
| `name is required` | Missing agent name. Ask/propose one. |
| `agent name already exists` | Duplicate top-level name. Pick another, or ask whether the user meant to **update** the existing agent (`list_agents`). |
| `unknown tool … allowed tools are:` | You invented a tool id. Pick from the list in the error. |
| `unknown model … allowed models are:` | You typed a model not in the enum. Pick from the list, or omit `model`. |
| `unknown skill … available skills are:` | Skill slug doesn't exist for this org. Use one from the error or from `list_org_plugins`. |
| `no skills are available to this org` | No installed plugin provides skills. Share the relevant plugin's `install_url`. |
| `is not installed for this org` (plugin) | Plugin exists but isn't installed — and the error names any unmet requirements. Share its `install_url`; proceed with what's installed. |
| `unknown plugin` | Slug doesn't exist. Enumerate with `list_org_plugins`. |
| `agent_id must be a valid UUID` / `agent not found` | Wrong or stale id. Call `list_agents`. |
| `sub-agent name is required` / `duplicate sub-agent name` | Fix the sub-agent list: every entry named, names unique within the parent. |
| `default agent cannot be renamed` | The org's default assistant keeps its name. Change other fields only, and tell the user. |
| `status must be active or archived` | Only those two values. |
| a credentials error on a model | The org lacks credentials for that model. Tell the user, then omit `model` or pick another from the enum. |
| You replaced a list by mistake | `get_agent` to see current state; send a corrective `update_agent` with the intended full lists. |

If the user isn't sure what they want: propose a small concrete starter agent, create it, and refine with `update_agent` from their feedback — iterating on a real agent beats designing in the abstract.

## Sharing links — only links the tools return

- **The agent's page**: share the `url` field from create/update/get responses. That's where the user views the agent and configures what these tools can't (channels, schedules, permissions).
- **Installing/connecting a plugin**: share that plugin's `install_url` from `list_org_plugins`, verbatim — one page to install and connect everything it requires. ("To do that, please install and connect the **GitHub** plugin here: <install_url> — then tell me when it's ready.")
- Never invent or guess a URL. No link from a tool result → no link shared.

## Rules

- Never invent names — plugin slugs, skills, tools, and models come only from tool results and schema enums.
- Interview before you build; confirm before you create or change.
- Least privilege: only the capabilities the job needs.
- `get_agent` before every `update_agent`.
- Arrays replace. Re-send the full intended list, always (for `tools`: the full *optional* list — baseline and floor are automatic and untouchable).
- Sub-agents: no `model` field; whole set replaced on update; `subagent_task` is added to the parent automatically; empty sub-agent `tools` default to read-only file tools.
- Never grant or promise `create_agent`/`update_agent`/`list_org_plugins` on a created agent — they are not grantable.
- Don't reconfigure or rename the user's default assistant beyond what they asked.
- Don't promise capabilities that need an uninstalled plugin or missing connection — share the `install_url` instead.
- Nothing "succeeded" until a tool result proves it.

## Final response checklist

When you hand off, your reply must state:

1. The agent's **name** and one sentence on what it does.
2. Its capabilities: plugins, notable tools, sub-agents, and which model (or "org default").
3. The **`url`** to its page.
4. Anything the user still needs to do — plugins to install/connect (with each `install_url`), and anything they must configure on the agent's page (channels, schedules).
5. An invitation to try it, with the offer to refine it (`get_agent` + `update_agent`) from their feedback.
