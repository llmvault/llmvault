# Web app

Status: proposed
Build in: phases 1 to 4
Teams: Web, Product, Agent Platform, Data Platform

## Its job

Web is where employees start cloud work, operators supervise agents, builders test releases, and admins run the org. Those people need different pages and different rights.

Main sections: Work, Search, Agents, Projects, Artifacts, Apps, and Admin.

## Work

The composer can look like chat, but sending creates a work item. A request may include files, connected records, project, agent, deadline, expected output, and execution place.

The page shows plan, progress, questions, actions, approvals, evidence, files, cost, and result. Work survives logout and navigation. Users with permission can pause, redirect, answer, cancel, retry, or take over.

Schedules and triggers appear beside manual jobs. Editing a schedule creates a new trigger version so old runs keep their real history.

## Cloud browser

Run each browser session in an isolated org worker with an empty profile by default. Users can watch, take over for login or consent, then return control.

Persistent profiles need explicit company and user permission, encrypted cookies, expiry, and revocation. Rules cover sites, downloads, uploads, clipboard, login takeover, region, duration, and data labels.

Always show where the browser runs, which profile it uses, which identity is signed in, and what files moved. It isn't the user's local browser.

## Search and research

Search supports plain words and questions. Filters cover source, person, team, date, type, project, and freshness. Results use current source access and exact citations.

Research mode shows its plan, active sources, collected evidence, disagreements, and draft. The user can change direction or stop it. Company facts and public facts stay visually separate.

## Agent builder and releases

Use the job-first setup in [agent rules](07-agent-governance-spec.md). A checklist covers owner, result, inputs, knowledge, actions, policy, runtime, tests, and release.

The generated execution map explains the setup, while forms remain the source of truth for rights and limits. Before publish, show effective permissions and every security change.

The release console holds versions, environments, tests, canaries, live results, rollback, connection health, denials, and cost. Draft edits use optimistic locking so two builders don't erase each other's work.

## Operations

Show work volume, age, deadlines, completion, corrections, cost, approvals, agent versions, connection health, source freshness, device status, and incidents.

Every number must open the records behind it. Cost breaks down model, compute, browser, storage, rendering, connector, and outside service charges, then groups them by team, project, agent, customer, and cost center.

Operators can pause a version, connection, trigger, tool, device class, agent, or all agent work. Emergency controls should be quick, narrow, reversible, and audited.

## Many agents at once

Show parent and child agents as a hierarchy with task, rights, budget, state, cost, and blocker. Operators can inspect, message, pause, stop, or reassign a child.

With dozens of agents, state and waiting reasons matter more than animation. Lead with jobs that need a person, risky actions, failures, and budget pressure.

## Analysis and internal apps

Data work should keep source versions, profiles, queries or code, transformations, charts, and written findings. Users need to inspect formulas, SQL, Python, filters, and sample sizes. A refresh creates a new result version.

Agents can make dashboards, forms, calculators, trackers, and small internal apps. Preview them in isolation. Publishing records owner, code version, data rights, action rights, audience, and policy.

An app never inherits its creator's personal access. Public publishing stays off unless org policy allows it.

## Admin

Admin covers people, groups, roles, domains, SSO, SCIM, sessions, connections, MCP, source health, policies, approval groups, agents, models, devices, retention, region, exports, deletion, keys, network, audit, budgets, invoices, and tax.

Risky changes need an impact preview and clear confirmation. Every settings change keeps its version and audit trail.

## Frontend rules

All Hivy API calls use generated `$api` hooks. Query keys come from the shared module. Document SSE routes in OpenAPI. Org switching goes through auth context and clears tenant-scoped cache before new data renders.

Primary flows must meet WCAG 2.2 AA, including keyboard use, focus, screen-reader status, captions, reduced motion, contrast, and non-color status cues.

## Requirements

| ID | Hivy must |
|---|---|
| **WEB-001** | Make durable work the default web experience. |
| **WEB-002** | Keep long jobs alive across navigation and logout. |
| **WEB-003** | Provide an isolated, visible, rule-controlled cloud browser. |
| **WEB-004** | Search company sources and run cited research with current access. |
| **WEB-005** | Build, test, compare, release, canary, and roll back agents. |
| **WEB-006** | Link every operations number to its records. |
| **WEB-007** | Supervise parent and child agents at useful scale. |
| **WEB-008** | Keep data-analysis source and code history. |
| **WEB-009** | Publish internal apps with explicit data and action rights. |
| **WEB-010** | Administer identity, policy, data, security, cost, connections, and devices. |
| **WEB-011** | Use generated API contracts and tenant-safe caching. |
| **WEB-012** | Meet WCAG 2.2 AA for main flows. |

## Done when

- A user can leave a job and return to the exact state.
- Browser location and profile type never disappear from view.
- Search can't return a source the user has lost access to.
- Release can't bypass required tests or policy review.
- Dashboard totals drill into matching records.
- Internal apps can't borrow creator access.
- Switching orgs can't flash old-org data.

Measure inbox use, long-job returns, citation opens, browser completion, release and rollback rate, dashboard drilldown, admin success, app publishing, accessibility defects, and tenant-cache incidents.
