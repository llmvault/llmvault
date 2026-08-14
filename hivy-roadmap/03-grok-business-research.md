# Grok Business and Grok Bot

## What xAI sells

[Grok Business](https://x.ai/news/grok-business) launched at $30 per person each month for small and medium teams. It includes team management, one bill, user analytics, high model limits, no training on team data, shared conversations, shared projects, and company knowledge.

Enterprise adds SSO and SCIM. xAI also describes Vault, a dedicated data plane with app-level encryption and customer-managed keys.

[The current business page](https://x.ai/grok/business) lists domain claims, roles, custom retention, audit and security controls, custom roles, support, voice, image and video work, connectors, documents, spreadsheets, slides, and Grok Build.

Grok's broad product range matters. It puts business search, document work, media creation, coding, and app building in one subscription.

## Grok Bot

xAI [launched Grok Bot](https://x.ai/news/introducing-grok-bot) on 11 August 2026 as an early beta. Access currently comes with SuperGrok Heavy, Cursor Ultra, and Cursor Teams Premium on macOS and iOS. Enterprise access still uses a waitlist.

Grok Bot is different from the `@grok` account on X and from xAI's Voice Agent Builder. It is a team of persistent work agents. Bots share a cloud computer where they can sign in to apps, inboxes, tools, and websites, including services with no API or MCP server. Work keeps running after the user steps away.

A user talks to a Bot from phone or desktop and can continue the same thread on either device. The setup avoids forcing someone to draw a workflow before they can delegate a task.

### Bots working together

Several Bots can run at once. One may manage the others while specialists handle inbox work, expenses, recruiting, bugs, or operations. Bots can message each other, share project context, hand off work, assign ownership, and coordinate in a group chat. They ask the user when a judgment call blocks progress.

This maps to Hivy's existing subagent, work-item, assignment, approval, and command-center plans. It doesn't need a second “bot team” subsystem.

### Teach by showing

Grok Bot's most distinct feature is routine capture. A user asks a Bot to watch while they complete a job. The Bot remembers the steps, saves them as a routine, accepts later corrections, and runs the routine again without another long explanation.

Hivy doesn't yet have this in the roadmap. It should. But recording clicks alone would produce brittle and risky automation, so Hivy needs to turn the demonstration into a reviewed routine with named inputs, expected outputs, app and data rights, action manifests, tests, version history, and a release state.

### Memory and growing trust

xAI says Bots learn a person's style, edge cases, and preference for when to ask versus continue. They can return to dropped conversations, follow up on stalled handoffs, and grow more proactive over time.

Hivy already plans scoped memory, schedules, triggers, escalation rules, and test-based release. Extend those objects rather than adding an opaque learning system. A correction may propose a memory or routine change; it must never change production behavior silently.

### What xAI hasn't documented yet

The launch page doesn't explain credential storage, browser-session isolation, admin controls, audit coverage, retention, model selection, routine review, rollback, reliability targets, or how proactive behavior is limited. It also doesn't claim general Enterprise availability yet.

Those gaps matter because a cloud computer logged into several company tools holds more practical authority than a normal chat session.

### Deduplicated Hivy map

| Grok Bot behavior | Hivy roadmap treatment |
|---|---|
| Always-on cloud computer and signed-in apps | Expand F51 and F52; don't create another cloud runtime item. |
| One thread on phone and desktop | Already F02, F42, and F46. |
| Several Bots, manager Bot, group chat, and handoffs | Expand F05, F32, and F55. |
| Ask only for judgment calls | Already F04 and F20. |
| Remember preferences and apply corrections | Expand F33 under visible, approved memory. |
| Follow dropped work and stalled handoffs | Expand F03 and F04. |
| Learn a reusable job by watching | Add F91: reviewed teach-by-demonstration routines. |

## Connectors

[Grok connectors](https://x.ai/news/grok-connectors) work on web, iOS, and Android. The published set includes SharePoint, OneDrive, Outlook, Google Drive, Gmail, Calendar, Notion, GitHub, Linear, and custom MCP servers. Connectors can read and write where the provider supports it.

[Connector management](https://docs.x.ai/grok/connector-management) makes an admin enable a provider before members can connect accounts. This gives companies a catalog allowlist.

Grok also describes permission-aware Google Drive search with citations. Hivy needs that same result, but across every supported source: source access should decide retrieval before content reaches the model.

## Skills and business files

[Grok Skills](https://x.ai/news/grok-skills) run on web, iOS, and Android. Built-in skills create and edit Word files, presentations, spreadsheets, and PDFs. Users can make reusable skills by describing them, uploading a file, or writing one.

This is a good model for repeatable company methods. Hivy should add ownership, tests, versions, required connections, permissions, cost range, and support status before a skill reaches production.

## Build mode and coding agents

[Grok Build Mode](https://x.ai/news/grok-build-mode) creates sites, apps, games, and interactive dashboards inside web and mobile chat. Users can preview, revise, and publish to a Grok URL or custom domain. Dashboards can use connected data and live filters.

[Grok Build](https://x.ai/news/grok-build-cli) is a coding agent with plan review, inline comments, diffs, repository instructions, plugins, hooks, skills, MCP, subagents, worktrees, headless mode, and Agent Client Protocol support.

Hivy already has sandboxes, team apps, agents, subagents, and local runtime pieces. The missing product layer is safe publication: versioned code, isolated previews, named data grants, action policy, scans, and rollback.

## Agent operations

[The agent dashboard](https://x.ai/news/agent-dashboard) groups sessions into waiting, working, and idle. Operators can inspect output, answer questions, approve work, start tasks, and take over.

[Grok workflows](https://x.ai/news/workflows) break a job into phases, delegate to several agents, save progress, show per-agent use, pause and resume, and turn good workflows into reusable commands. Its research workflow runs source work in parallel and checks claims.

Hivy's subagents need this kind of operating view. A parent should set each child's task, permission ceiling, budget, deadline, and expected output.

## Sharing and admin rules

[Grok management](https://docs.x.ai/grok/management) lets admins set the widest sharing level separately for conversations, projects, and skills: private, team, org, or public where available. Tightening the org rule also narrows existing shares.

[The business user guide](https://docs.x.ai/grok/user-guide) separates personal and team workspaces and limits team conversation links to active licensed members.

Hivy should use the same ceiling idea for every resource. A project or agent can narrow org policy but can't widen it.

## Voice and media

Grok Business includes voice, image generation, and video generation. xAI also offers a [Voice Agent Builder](https://x.ai/news/grok-voice-agent-builder) with phone calls, knowledge, tools, guardrails, MCP, human handoff, and live operator alerts.

Hivy should build two-way voice because it helps mobile and field work. Image support fits business files and brands. Video can wait until documents, spreadsheets, slides, PDFs, and internal apps work well.

## What Hivy must match

- Workplace connectors on web and mobile.
- Permission-aware company search with citations.
- Reusable skills and first-class business files.
- Coding agents, subagents, plans, diffs, and worktrees.
- An agent dashboard with waiting, working, approval, and takeover states.
- Resource sharing ceilings and admin connector controls.
- SSO, SCIM, retention, audit, regional options, and customer keys for large customers.

Grok Bot adds four expectations to that list: a persistent signed-in cloud workspace, direct task assignment without setup work, agent-to-agent coordination with owned handoffs, and teach-by-demonstration routines. Hivy already covers the first three under existing specs. F91 covers the missing fourth.

## Where Hivy can win

Grok covers a lot of ground. Hivy can be much clearer about business ownership and control: each job belongs to a team, each action has a policy decision, and each result carries cost and acceptance data.
