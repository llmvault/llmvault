# Company knowledge and connectors

Status: proposed
Build in: phases 1 to 4
Teams: Connections, RAG, Security, Platform, Web

## The problem

A green “connected” badge says almost nothing. It doesn't tell an employee which files an agent can read, whether permissions are stale, or which writes the agent can make.

Hivy must keep four objects separate:

1. **Connection:** a working login to a service.
2. **Knowledge grant:** permission to search named data.
3. **Action grant:** permission to request named operations.
4. **Policy:** rules that allow, deny, limit, redact, or ask for approval right now.

Enabling Salesforce for a team can't give every agent contact exports and record deletion.

## Which identity acts

**Delegated identity** uses the requesting employee's rights. Use it for personal email, calendars, files, and search where results must match the person.

**Service identity** gives an agent its own company account. Use it for schedules, shared queues, and maintenance work.

Every citation and action must name the identity mode. Admins can require delegated access for sensitive sources or block service accounts from exports and sharing.

## Permission-safe indexing

When a source connects, Hivy should:

1. Find the resources the connection can see.
2. Save stable source IDs, versions, links, owners, dates, types, regions, and labels.
3. Extract content without losing page, section, cell, message, or code location.
4. Sync user, group, folder, channel, site, and document permissions.
5. Tie each search chunk to its source version and access list.
6. Filter results by both agent rights and the person's current source rights.
7. Return the evidence with freshness data.
8. Remove deleted or revoked content within a published time limit.

Access checks happen before text reaches the model. A cached answer must not revive access that was removed later.

## Source health

Admins need a page that shows connection state, granted scopes, identity, token expiry, last good sync, current sync, and next sync. It should separate content freshness from permission freshness.

Show counts for found, indexed, changed, skipped, deleted, and failed objects. Also show webhook health, rate limits, stale-access warnings, retrieval by team or agent, data region, and classification.

A test tool should explain why one employee can find a file and another can't. This will save days of support work.

## Citations people can trust

A cited claim needs source system, title, link, exact location, source modification time, index time, and identity mode.

The location changes by source. Use page and section for documents, sheet and cell range for spreadsheets, channel and message for chat, repository/path/revision for code, and ticket/comment for support work.

Company evidence, public web evidence, uploaded files, and model guesses must look different. Exports should keep citations in notes, comments, footnotes, or a source section.

## Connector contract

Each connector publishes a versioned manifest. It lists resources, reads, writes, events, login scopes, identity modes, schemas, sensitive fields, preview support, idempotency, retries, undo behavior, rate limits, webhooks, permission rules, deletion behavior, and test-environment support.

Don't measure progress by provider count. A connector is ready when its supported resources behave well under expired login, denied access, duplicate events, limits, retries, and partial failure.

## Build order

Start with daily company work:

- Google Drive, Docs, Sheets, Gmail, and Calendar.
- SharePoint, OneDrive, Outlook, Teams, Word, Excel, and PowerPoint.
- Existing Slack, Notion, GitHub, Linear, website, and file sources.

Next: Confluence, Jira, Salesforce, HubSpot, Zendesk, Intercom, and Box. Add Stripe, accounting, procurement, and ERP resources only when a target workflow needs them.

## Private MCP servers

Admins register MCP servers with OAuth, bearer, header, or client credentials. Each registration names its owner, teams, environment, network route, version, expected tool schemas, and data types.

Tool discovery creates a review. Show added and removed tools, schema changes, new sensitive fields, and any change from read to write. New or changed write tools stay off until an admin approves them.

Private services should use an outbound customer gateway. The customer opens the connection, limits destinations, and avoids a public inbound endpoint.

## Search and research

One search box should cover permitted documents, messages, email, tickets, CRM, code, meetings, Hivy work, and the public web. Filters include source, person, team, date, type, project, label, and freshness.

Research mode first shows a plan. While it runs, users can change the question, add or remove sources, or stop. The final report links each important claim to evidence and shows source disagreements instead of hiding them.

## Writes

Search rights never imply write rights. The agent proposes an action against the connector manifest. Policy judges it. A preview shows the change. After approval, Hivy runs the exact payload with an idempotency key and checks the result.

Secrets stay encrypted and out of prompts, logs, citations, and normal audit fields. Provider errors map to clear types: login expired, denied, limited, conflict, bad request, or unavailable.

## Requirements

| ID | Hivy must |
|---|---|
| **KNOW-001** | Keep connections, knowledge grants, action grants, and policy separate. |
| **KNOW-002** | Support delegated and service identities with visible labels. |
| **KNOW-003** | Check current source access before adding content to model context. |
| **KNOW-004** | Remove revoked and deleted content within a stated limit. |
| **KNOW-005** | Attach exact location and freshness to citations. |
| **KNOW-006** | Show source, permission, webhook, and token health. |
| **KNOW-007** | Publish versioned connector manifests. |
| **KNOW-008** | Review new and changed MCP tools before writes run. |
| **KNOW-009** | Support private outbound gateways and separate environments. |
| **KNOW-010** | Keep company and public evidence separate in search. |
| **KNOW-011** | Send every write through policy, approval, idempotency, and checks. |
| **KNOW-012** | Write search and action events to durable audit. |

## Done when

- Two employees with different source rights get different, correct evidence.
- Revoking a file removes it from search and cache on time.
- Every citation opens the right place or explains that the source is gone.
- Turning on a provider doesn't turn on every write.
- An MCP schema change can't slip into production.
- Duplicate webhooks and retries don't create duplicate records.
- Admins can tell stale content from stale permissions.

Measure source freshness, permission freshness, stale-access incidents, citation use, unsupported claims, login failures, write checks, duplicates, repair time, and query cost.
