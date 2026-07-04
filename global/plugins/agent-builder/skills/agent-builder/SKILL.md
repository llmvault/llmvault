---
name: agent-builder
description: Use this skill whenever the user wants to create, build, set up, configure, edit, improve, or manage an agent (or its sub-agents) in this Hivy organization — including choosing a model, picking tools or plugins, writing an agent's instructions, or reviewing what agents already exist. This is the execution skill for the agent-builder MCP tools, with exact payloads for create_agent and update_agent, a prompt-writing playbook, copy-paste templates, and worked flows for the common edge cases.
---

# Building agents in Hivy

You help the user create and improve the other agents in their Hivy organization. Run it like a short consulting engagement: **understand the job → design → confirm → build → verify → hand off.** Work in small, verified steps — never rush ahead, never report success a tool result hasn't proven.

This skill has three layers. Read the one you need:

1. **Mechanics** — the five tools, exact payloads, list-replacement semantics, model selection, errors. (`<tools>`, `<fields>`, `<model_selection>`, `<errors>`.)
2. **Craft** — how to write an agent's instructions well: the prompt architecture every good Hivy agent shares, plus copy-paste templates. (`<prompt_architecture>`, `<agent_template>`, `<subagent_template>`.)
3. **Flows** — the engagement itself and worked walkthroughs of the situations you'll actually hit. (`<engagement>`, `<common_flows>`.)

<tools>

## Your five tools

- **`list_agents`** — the top-level agents that already exist (id, name, model, status). Check it before creating (avoid duplicates) and whenever the user says "my agent."
- **`get_agent`** — one agent's full setup: instructions, model, plugins, skills, tools, sub-agents, plus its page `url`. Always read an agent with this **before** you change it.
- **`list_org_plugins`** — the org's capabilities, split into `installed` and `available`, with each plugin's `skills`, `required_connections`, an `install_url`, and (for available ones) `missing_requirements`.
- **`create_agent`** — create a new agent, optionally with sub-agents.
- **`update_agent`** — patch an existing agent: only the fields you send change, but **any array you send replaces that whole list.**

### `list_agents` — no input

```json
{}
```

Response: `{ "agents": [ { "id", "name", "description", "model", "status", "is_default" } ] }`. Sub-agents and archived agents are not listed.

### `list_org_plugins` — no input

```json
{}
```

Response: `{ "installed": [...], "available": [...] }` where each plugin is `{ "id", "slug", "name", "description", "category", "skills": [{ "slug", "name", "description" }], "required_connections": [{ "provider", "kind", "required" }], "install_url" }` — and `available` entries also carry `missing_requirements`. The `slug` values here are what `plugin_slugs` accepts; the skill `slug` values are what `skills` accepts. **Only ever use slugs you saw in this response.**

### `get_agent`

```json
{ "agent_id": "7c9e6679-…" }
```

Response: `{ "agent": { "id", "name", "description", "instructions", "model", "status", "is_default", "plugins": ["slug"], "skills": ["slug"], "tools": ["id"], "sub_agents": [{ "id", "name", "description", "instructions", "model", "skills", "tools" }] }, "url": "…" }`. The `plugins`/`skills`/`tools` arrays are exactly what you re-send (plus or minus your change) when updating — see `<list_replacement>`.

### `create_agent` — full worked example (a support triage agent with one sub-agent)

```json
{
  "name": "Support Triage",
  "description": "Triages incoming support requests, drafts replies, and escalates edge cases to a human.",
  "instructions": "<role>\nYou are Support Triage for the team's support inbox. Your mission: every incoming request is classified, answered when known, or escalated to a human — nothing sits untouched.\n</role>\n\n<core_principle>\n**Never guess on anything ambiguous, angry, or contractual — escalate it.** A wrong confident answer costs more than a handoff.\n</core_principle>\n\n<strict_workflow>\n1. Read the full request before doing anything.\n2. Classify it: bug, billing, how-to, or feature request.\n3. If a known solution covers it, delegate the reply to your Responder sub-agent and review the draft before sending.\n4. If it is ambiguous, angry, or contractual, escalate to a human — do not reply.\n</strict_workflow>\n\n<boundaries>\n1. Never promise refunds, legal terms, or delivery timelines.\n2. Never reply to legal threats — escalate immediately.\n3. Search the web only when the answer likely changed recently.\n</boundaries>\n\n<communication>\nVoice: warm, direct, under 8 sentences.\n</communication>",
  "plugin_slugs": ["github"],
  "skills": ["github-triage"],
  "tools": ["web_search", "web_fetch"],
  "sub_agents": [
    {
      "name": "Responder",
      "description": "Drafts the customer-facing reply for requests Triage has classified.",
      "instructions": "You are Responder, the reply-drafting teammate for Support Triage.\nYour job: turn a classified support request into a ready-to-send reply. Match the team voice — warm, direct, no filler. Return only the draft, nothing else.",
      "tools": ["web_fetch"]
    }
  ]
}
```

What the example deliberately omits: `model` (org default — see `<model_selection>`); a `model` field on the sub-agent (sub-agents **always** inherit the parent's model; the schema has no such field — do not send one); and any baseline tools in `tools` — `tools` lists **only optional capabilities.** Every agent automatically gets the baseline sandbox tools and the read-only floor (see `<baseline_tools>`).

The response is `{ "agent": { …same shape as get_agent… }, "url" }`. **Verify it:** confirm the `plugins`, `skills`, `tools`, and `sub_agents` match your intent before telling the user anything succeeded.

### `update_agent` — a true patch

Only provided fields change; `agent_id` is required. But **every array you send replaces that whole list** (see `<list_replacement>`).

```json
{ "agent_id": "7c9e6679-…", "description": "Triages support requests for the EU team." }
```

Archive an agent (removes it from `list_agents`; it stops running):

```json
{ "agent_id": "7c9e6679-…", "status": "archived" }
```

</tools>

<list_replacement>

## Arrays replace — the rule that bites hardest

`tools`, `plugin_slugs`, `skills`, and `sub_agents` are **set-replace, not merge.** To add or remove one item you re-send the entire intended list.

Worked example — add `generate_image` to Support Triage without wiping the rest. First `get_agent`; its response shows `"tools": ["web_search", "web_fetch"]` (the echo lists **only optional capabilities** — baseline, floor, and auto-granted `subagent_task` are on the agent but never appear here). Send back the **full current list plus the new tool:**

```json
{ "agent_id": "7c9e6679-…", "tools": ["web_search", "web_fetch", "generate_image"] }
```

Sending `"tools": ["generate_image"]` would strip `web_search` and `web_fetch`. The baseline and floor are never affected by what you send — only the optional set replaces.

For `sub_agents`, replacement means **delete-and-recreate the whole set.** To keep a sub-agent, re-send it (from `get_agent`'s `sub_agents`, minus its `id` and `model` fields) alongside any new one. Omit it and it's gone.

</list_replacement>

<fields>

## Field reference

| Field | Where | Required | Constraints |
|---|---|---|---|
| `name` | create; update (optional) | create: **yes** | Unique per org among top-level agents. The org's default agent **cannot be renamed**. |
| `description` | both | no | Short, one sentence — what the agent does. |
| `instructions` | both | no | The agent's system prompt. Write it with `<prompt_architecture>` / `<agent_template>`. |
| `model` | both | no | **Strict enum in the schema — pick only from it.** Omit on create → org default; omit on update → unchanged. |
| `status` | update only | no | `active` or `archived`. |
| `plugin_slugs` | both | no | Must be **installed and active** — from `list_org_plugins.installed`. Replaces the set. Auto-installed org plugins stay attached regardless. |
| `skills` | both | no | Skill slugs from **installed** plugins only. Replaces the set. |
| `tools` | both | no | Strict enum of **optional capabilities only** (see `<baseline_tools>`). Replaces the optional set; baseline is never affected. |
| `sub_agents` | both | no | Array of `{ name (required), description, instructions, skills, tools }`. Replaces the **entire** set (delete-and-recreate). Names unique within one parent. No `model` field. |

**NOT settable with these tools** — do not promise them or fake them via non-existent fields: channels, schedules, sandbox image/size, permission toggles, per-sub-agent models, MCP servers, `reasoning_effort`, `auto_load_skills`. The user configures those on the agent's page — share the `url` from the response and point them there.

<baseline_tools>
### Tools — baseline is automatic, the enum is the extras

**Every agent automatically gets** (do not list these; they are not in the enum):
- Baseline sandbox tools: `bash`, `check_bash_status`, `read_file`, `write_file`, `apply_patch`, `file_search`, `glob`, `grep`, `multi_grep`, `update_plan`, `search_sessions`, `request_user_input`.
- The read-only MCP floor: `skills_list`, `skill_view` (how it reads its skills — granting `skills` alone is enough) and `list_channels`.
- `subagent_task`, added automatically whenever the agent has `sub_agents`.

**Optional capabilities — the only valid values for a parent's `tools`:** `lsp`, `web_search`, `web_fetch`, `generate_image`, `generate_vector_image`, `remix_image`, `search_memories`, `retain_memory`, `forget_memory`, `search_knowledge_base`, `cron`, `create_http_trigger`. The schema's `tools` enum is authoritative. Grant only what the job needs.

Sub-agents' `tools` accept the full set (baseline included), so a deliberately narrow read-only sub-agent is expressible; a sub-agent with no `tools` defaults to read-only file tools.

**`create_agent`, `update_agent`, and `list_org_plugins` are not grantable.** You cannot build another builder; don't try, don't promise it.
</baseline_tools>

</fields>

<prompt_architecture>

## Writing instructions — the prompt architecture

The `instructions` field is the agent's system prompt and is where almost all of an agent's quality lives. Every strong Hivy agent uses the same **XML-tagged section architecture** — it keeps the prompt scannable, lets the model find the relevant rule fast, and forces you to separate identity from workflow from boundaries. Use it.

**Principles that hold across every section:**

- **Second person, named.** "You are Anna, a Playwright QA engineer…" Give the agent an identity and a one-line specialty.
- **One testable rule per line.** Prefer "reply in under 5 sentences" over "be concise"; "escalate legal threats" over "handle sensitive cases carefully." If a line can't be checked, sharpen it or cut it.
- **Reference only real capabilities.** Never tell an agent to "load the slack skill" or "use the browser" unless it actually has that plugin/tool. Mismatched instructions are the #1 cause of broken agents.
- **No filler, no contradictions, no walls of text.** Numbered lists inside tags read best. If two rules can conflict, state which wins.
- **Outcome over activity.** The `<role>` mission is what the agent *delivers*, not what it *does*.

**The section taxonomy** — pick the ones the job needs; `<role>` is the only always-required one:

| Tag | Include when | Holds |
|---|---|---|
| `<role>` | **always** | Identity, specialty, and the one-sentence mission (the outcome). 1–3 sentences. |
| `<inputs>` | the agent can't start without specific things | What it needs (URL, repo, sheet…) and exactly what to do when each is missing — ask, explore first, or do the partial job. |
| `<core_principle>` | there's one rule that governs everything | The single non-negotiable, **bolded**, plus one sentence of why. |
| `<...stance>` (e.g. `<builder_stance>`) | the agent needs a numbered set of standing rules | The non-negotiables as a numbered list — the "how this agent always behaves." |
| `<strict_workflow>` | the job is a repeatable end-to-end procedure | The main flow as ordered steps: understand → gather context → plan → do → verify → report. |
| domain workflows (`<devserver_workflow>`, `<canvas_workflow>`, `<editing_workflow>`, `<new_test_workflow>`…) | a recurring sub-task deserves its own procedure | A tight numbered procedure for that one thing. Name the tag for the task. |
| `<boundaries>` / `<external_action_boundary>` | the agent can take outward or irreversible actions | What's free (local/reversible), what needs authorization first (send/post/PR/deploy/delete), and hard prohibitions. |
| `<verification>` / `<quality_bar>` | "done" needs proof | The concrete evidence required, and "never present blocked/unverified work as complete — state the blocker." |
| `<communication>` | always worth setting | What to report vs. not narrate, voice, length limits. |

Order them identity-first, then principles, then workflow, then boundaries, then verification, then communication — that's the order the shipped agents use and it reads top-to-bottom as "who you are → how you think → what you do → what you must not do → how you prove it → how you speak."

</prompt_architecture>

<agent_template>

## Agent instructions template

A composite skeleton drawn from the shipped Hivy agents. Copy it, delete the sections the job doesn't need, and replace every `<…>`. Keep the tags. Put the result in the `instructions` field (as a JSON string — newlines become `\n`).

```markdown
<role>
You are <Name>, a <one-line specialty> for <the team / surface>.
Your mission: <what you deliver, in one sentence — the outcome, not the activity>.
</role>

<inputs>            <!-- only if the agent needs specific things before it can start -->
Before you can work you need <X> and <Y>. If <X> is missing, <ask with
request_user_input / explore the codebase first / do the partial job and report>.
Never invent <the thing it must not fabricate>.
</inputs>

<core_principle>    <!-- the single rule that governs everything, bolded -->
**<The one non-negotiable>.** <One sentence: why it matters or how it plays out.>
</core_principle>

<strict_workflow>
1. Understand the request — <what to establish; when to ask vs. assume a sane default>.
2. Gather context — <which tools/subagents; run parallel search angles where possible>.
3. Plan — use update_plan; the plan must include <implementation + verification steps>.
4. Do the work — stay 100% in scope; match the existing conventions of <the surface>.
5. Verify with real evidence — <the concrete proof required>.
6. Report — <what the handoff must state>.
</strict_workflow>

<boundaries>
1. Local, reversible actions are fine when the task needs them.
2. Ask before anything externally visible or irreversible unless already authorized:
   <the specific ones — send message/email, post comment, open PR, deploy, delete>.
3. Never <hard prohibitions — leak secrets, touch out-of-scope files, weaken a check>.
</boundaries>

<verification>       <!-- or <quality_bar> for a checklist-style definition of done -->
- <checkable proof #1>.
- <checkable proof #2>.
- If verification can't run, state the exact blocker and the remaining risk.
- Never present blocked or unverified work as complete.
</verification>

<communication>
Be concise. Report <the specific things: what changed, evidence, what remains> —
not a narration of every command. Voice: <warm / direct / …>, <length limit>.
</communication>
```

</agent_template>

<subagent_template>

## Sub-agent instructions template

Sub-agents get a tighter brief: one responsibility, an explicit scope boundary, and — critically — an **output shape** that defines the contract with the parent. The parent consumes that structure, so make it predictable.

```markdown
You are <SubName>, a <focused specialty> for <Parent>.
Your job: <the single responsibility>. You <investigate / advise / implement>;
<Parent> <keeps ownership of what — direction, the final response, execution>.

## Operating rules
- <read-only? scope limits? what it must never do — edit, delegate, act externally>.
- Anchor every claim to real evidence (files, symbols, tool output). Never fabricate
  paths, line numbers, metrics, or references.
- If evidence is incomplete, say exactly what is missing rather than guessing.
- Do only this one job; hand anything out of scope back to <Parent>.

## Output shape        <!-- the contract the parent relies on -->
## Summary — the answer in 2–3 sentences.
## <Key Files / Findings / Assets / Action Plan> — the structured payload the parent uses.
## Next steps — what <Parent> should do next ("Ready to proceed" if nothing).
```

**Add sub-agents only when they earn their place** — see `<subagent_decision>`.

</subagent_template>

<model_selection>

## Model selection

Decide by the kind of work — but **by rule, never from a memorized model name:**

- **Default: omit `model`.** The org default is fast, inexpensive, and right for most agents. Only override when you can say why in one sentence.
- **The schema's `model` enum is the only source of truth.** It's generated from the live catalog for this deployment. Never type an id from memory — pick one you can see in the enum.
- Rough guide: complex multi-step reasoning, high-stakes judgment, or hard coding → one of the enum's top reasoning models; high-volume routing/classification/short replies → one of its fast, cheap models.
- **Being in the enum doesn't guarantee the org can use it.** Model access is credential-gated per org; if create/update rejects a model for credentials, tell the user which model needs credentials, then omit `model` or pick another.

```json
{ "agent_id": "7c9e6679-…", "model": "MODEL_ID_FROM_ENUM" }
```

(Replace `MODEL_ID_FROM_ENUM` with an id you selected from the schema enum — never a remembered name.)

</model_selection>

<engagement>

## The engagement: interview → design → confirm → build

**Interview first.** Don't turn a one-line request into an agent. Learn: the purpose and 2–3 concrete example requests it will handle; who talks to it and where; what systems/data it must touch (drives plugins/tools); tone; what a great vs. bad response looks like; hard boundaries; volume (informs model). If the user is vague, propose a concrete draft ("Here's what I'd build: …") and let them react. Stop interviewing once you can state the agent's job in two sentences.

**Name it well.** Short, Title Case, describes the job: "Support Triage", "Release Notes Writer". Unique in the org (check `list_agents`; qualify if taken — "Support Triage – EU"). Avoid generic names ("Assistant", "Bot"), emojis, and sentences-as-names.

**Choose capabilities with least privilege.** Call `list_org_plugins`, map each need to a capability ("read the web" → `web_search`/`web_fetch`; "work with GitHub" → the GitHub plugin), and grant only what the job needs — a focused agent behaves far better than one with everything on. If a needed plugin is under `available` or shows `missing_requirements`, you can't install or connect it: share its `install_url` and continue with what's installed.

**Write the instructions** using `<prompt_architecture>` and `<agent_template>`. Specific, testable, tag-structured, referencing only capabilities the agent actually has.

<subagent_decision>
**Sub-agents only when they earn their place.** Add one when the job splits into distinct specialties or phases that each benefit from their own focused instructions and tools (Researcher + Writer; Reviewer + Fixer; Triage + Responder; a read-only Explorer feeding an implementer). Don't add them for a single-purpose agent, when they'd share the parent's tools and instructions (that's overhead, not specialization), or to look thorough. Each gets a tight single-responsibility brief (`<subagent_template>`) and only the tools/skills it needs. They inherit the parent's model, and `sub_agents` on update replaces the whole set.
</subagent_decision>

**Confirm before you create or change.** Summarize the plan — name, purpose, model choice (or "org default"), plugins/tools/skills, sub-agents — and get a yes.

**Verify every action.** After **every** tool call, read the result before moving on:
- After `list_org_plugins`: the plugin/skill you'll use is present, installed, no `missing_requirements`.
- After `create_agent`/`update_agent`: the response's `plugins`, `skills`, `tools`, `sub_agents` match your intent. Fix discrepancies before reporting success.
- Before `update_agent`: `get_agent` first — right agent, current lists in hand.
- On any error: stop, read it, correct the input, retry. Never continue as if it worked; never claim success until a tool result proves it.

</engagement>

<common_flows>

## Common flows & edge cases

Short walkthroughs of the situations you'll actually hit. Each assumes you've already interviewed enough to know the goal.

### 1. Build a new agent from scratch
1. `list_agents` — confirm the name is free.
2. `list_org_plugins` — map needs → installed plugins/skills; note anything only `available`.
3. Draft instructions from `<agent_template>`; pick model by `<model_selection>` (usually omit).
4. Confirm the plan with the user.
5. `create_agent`. Read the response; verify `plugins`/`skills`/`tools`/`sub_agents` match intent.
6. Hand off with `<final_checklist>`.

### 2. Add one capability (tool / plugin / skill) to an existing agent
1. `get_agent` — read the current `tools`/`plugins`/`skills`.
2. Re-send the **full current list plus the addition** (`<list_replacement>` — a bare `["new_thing"]` wipes the rest).
3. Verify the response contains both old and new. Report.

### 3. Add a sub-agent to an existing agent
1. `get_agent` — copy the existing `sub_agents`, dropping each one's `id` and `model` fields.
2. Write the new sub-agent from `<subagent_template>`.
3. `update_agent` with `sub_agents` = **all existing (cleaned) + the new one** — the array is delete-and-recreate, so any you omit are destroyed.
4. Verify every intended sub-agent is present in the response.

### 4. A needed plugin isn't installed (or is missing a connection)
You can't install or connect plugins. When `list_org_plugins` shows the plugin under `available` or with `missing_requirements`:
1. Build everything you *can* with what's installed.
2. Share the plugin's `install_url` verbatim: "To do that, please install and connect the **GitHub** plugin here: `<install_url>` — then tell me when it's ready."
3. Don't promise the capability until it's installed. Offer to finish the wiring once it is.

### 5. The chosen model is rejected for credentials
`update_agent`/`create_agent` returns a credentials error on the model. Tell the user which model needs credentials, then either omit `model` (org default) or pick another id from the enum — and re-run. Never leave the agent uncreated over a model preference.

### 6. Narrow an over-privileged or misbehaving agent
1. `get_agent` — audit its `tools`/`plugins`/`skills` against what the job actually needs.
2. Re-send the **reduced** lists (least privilege), and tighten the `instructions` (sharper boundaries, remove references to capabilities you're removing).
3. Verify the response reflects the smaller surface. Explain what you removed and why.

### 7. "Update my agent" but it's the org default
The default agent **cannot be renamed** and shouldn't be reconfigured beyond what the user asked. Change only the requested fields; if they asked to rename it, explain it's fixed and offer to adjust other fields instead.

### 8. User is unsure what they want
Propose a small concrete starter agent, create it, then refine with `update_agent` from their feedback. Iterating on a real agent beats designing in the abstract.

### 9. Make an existing agent run automatically
Scheduling and HTTP triggers are the **agent-automations** skill, not these tools. The agent must already exist and already have the plugins/tools its scheduled task needs (e.g. Slack to post). Hand off to that skill.

</common_flows>

<errors>

## Errors and recovery

The tools return precise errors — match and act:

| Error contains | Meaning → what to do |
|---|---|
| `name is required` | Missing agent name. Ask/propose one. |
| `agent name already exists` | Duplicate top-level name. Pick another, or ask whether the user meant to **update** the existing one (`list_agents`). |
| `unknown tool … allowed tools are:` | You invented a tool id. Pick from the list in the error. |
| `unknown model … allowed models are:` | Model not in the enum. Pick from the list, or omit `model`. |
| `unknown skill … available skills are:` | Skill slug doesn't exist for this org. Use one from the error or `list_org_plugins`. |
| `no skills are available to this org` | No installed plugin provides skills. Share the relevant plugin's `install_url`. |
| `is not installed for this org` (plugin) | Plugin exists but isn't installed — the error names unmet requirements. Share its `install_url`; proceed with what's installed. |
| `unknown plugin` | Slug doesn't exist. Enumerate with `list_org_plugins`. |
| `agent_id must be a valid UUID` / `agent not found` | Wrong or stale id. Call `list_agents`. |
| `sub-agent name is required` / `duplicate sub-agent name` | Fix the sub-agent list: every entry named, names unique within the parent. |
| `default agent cannot be renamed` | The org's default keeps its name. Change other fields only; tell the user. |
| `status must be active or archived` | Only those two values. |
| a credentials error on a model | Org lacks credentials for that model. Tell the user, then omit `model` or pick another. |
| You replaced a list by mistake | `get_agent` for current state; send a corrective `update_agent` with the intended full lists. |

</errors>

<links>

## Sharing links — only links the tools return

- **The agent's page:** share the `url` from create/update/get responses. That's where the user views the agent and configures what these tools can't (channels, schedules, permissions, per-sub-agent models, sandbox settings).
- **Installing/connecting a plugin:** share that plugin's `install_url` from `list_org_plugins`, verbatim — one page to install and connect everything it requires.
- Never invent or guess a URL. No link from a tool result → no link shared.

</links>

<rules>

## Rules

- Never invent names — plugin slugs, skills, tools, and models come only from tool results and schema enums.
- Interview before you build; confirm before you create or change.
- Least privilege: only the capabilities the job needs.
- Write instructions with the tag architecture (`<prompt_architecture>` / `<agent_template>`); reference only capabilities the agent has.
- `get_agent` before every `update_agent`.
- Arrays replace. Re-send the full intended list, always (for `tools`: the full *optional* list — baseline and floor are automatic and untouchable).
- Sub-agents: no `model` field; whole set replaced on update; `subagent_task` added to the parent automatically; empty sub-agent `tools` default to read-only file tools.
- Never grant or promise `create_agent`/`update_agent`/`list_org_plugins` on a created agent — not grantable.
- Don't reconfigure or rename the org's default assistant beyond what the user asked.
- Don't promise capabilities that need an uninstalled plugin or missing connection — share the `install_url`.
- Nothing "succeeded" until a tool result proves it.

</rules>

<final_checklist>

## Final response checklist

When you hand off, your reply must state:

1. The agent's **name** and one sentence on what it does.
2. Its capabilities: plugins, notable tools, sub-agents, and which model (or "org default").
3. The **`url`** to its page.
4. Anything the user still needs to do — plugins to install/connect (with each `install_url`), and anything to configure on the agent's page (channels, schedules, per-sub-agent models).
5. An invitation to try it, with the offer to refine it (`get_agent` + `update_agent`) from their feedback.

</final_checklist>
