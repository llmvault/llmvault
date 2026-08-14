# Grok for business

## What xAI sells

[Grok Business](https://x.ai/news/grok-business) launched at $30 per person each month for small and medium teams. It includes team management, one bill, user analytics, high model limits, no training on team data, shared conversations, shared projects, and company knowledge.

Enterprise adds SSO and SCIM. xAI also describes Vault, a dedicated data plane with app-level encryption and customer-managed keys.

[The current business page](https://x.ai/grok/business) lists domain claims, roles, custom retention, audit and security controls, custom roles, support, voice, image and video work, connectors, documents, spreadsheets, slides, and Grok Build.

Grok's broad product range matters. It puts business search, document work, media creation, coding, and app building in one subscription.

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

## Where Hivy can win

Grok covers a lot of ground. Hivy can be much clearer about business ownership and control: each job belongs to a team, each action has a policy decision, and each result carries cost and acceptance data.
