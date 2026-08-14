# What Hivy has today

## Short answer

Hivy already has more agent infrastructure than its current product surfaces suggest. The hard part isn't starting over. It's turning several strong pieces into one dependable work system, then adding the controls business buyers expect.

## How this review was done

The review covered the Go API, Next.js web app, Rust sandbox runtime, Tauri desktop code, Expo mobile app, Kubernetes files, pricing copy, and product docs. Claims below come from code or checked-in documentation, not screenshots or future plans.

## What already works

### Teams and agents

Hivy has orgs, teams, and owner/admin/member roles. Agents belong to teams and can carry models, reasoning settings, skills, tools, MCP servers, resources, permissions, memory, and sandbox settings. Agents can also start subagents.

That gives Hivy a real base for shared company agents. Most assistants still organize work around one person and one conversation.

### Runs and local work

Sessions persist. They record participants, execution source, model, sandbox, creator, and usage. Hosted sandboxes run cloud work.

The desktop app has a local runtime, keychain-backed access, SQLite history, and cloud links for login, model data, MCP data, metadata, and backup. The local runner is a serious advantage because it can reach repositories and files that a cloud agent can't see.

### Connections and triggers

Hivy uses Nango for OAuth connections and has checked-in action catalogs for roughly 41 providers. Teams can add MCP servers and grant tools to teams or agents.

Schedules, webhooks, Slack, GitHub, and other events can start agent work. This is the beginning of event-driven agents, not only chat-triggered agents.

### Knowledge and outputs

The RAG system currently registers GitHub, Linear, Notion, Slack, and website sources. Hivy also has files, Drive, uploads, transcription, Canvas, brands, Sheets, and team apps.

Usage reports, audit routes, and bring-your-own provider credentials also exist.

## Where the product breaks down

### No shared work object

Sessions exist, but the product doesn't yet have one work item with an owner, deadline, state, actions, approvals, cost, and accepted result. That makes cross-device work, operations, retries, and reporting harder than they need to be.

### Authority sits too close to execution

Hivy can grant tools, but it lacks one policy service that judges every connector, MCP, browser, desktop, terminal, and app action. Approval isn't yet a product-wide object tied to the exact payload that will run.

General computer control would be dangerous before this is fixed.

Hivy also has no teaching mode that watches a bounded demonstration and turns it into a reviewed routine. Existing skills, agent settings, and triggers are useful pieces, but they don't yet provide demonstration capture, inferred steps, correction history, tests, or routine release controls.

### Company knowledge isn't permission-complete

Five RAG sources are registered. Google Workspace, Microsoft 365, Confluence, Jira, and deeper business-system search are still missing or incomplete. More important, company search needs current per-user source permissions, clear sync health, and exact citations.

### Business identity is thin

The current auth system has passwords, OTP flows, resets, and social login, but no business MFA policy, SAML/OIDC SSO, SCIM, verified-domain capture, or custom roles.

### Audit can lose events

The generic request audit writer uses a memory buffer. It drops events when the buffer fills and may leave records behind during a timed-out shutdown. That is fine for best-effort telemetry. It isn't acceptable evidence for money movement, access changes, or production work.

### Data controls are missing

There is no full retention system, deletion status across every store, legal hold, compliance API, residency policy, or customer-managed key option.

### Mobile isn't a product yet

The Expo app is still the starter experience. Mobile needs a narrow first job: ask by voice, capture evidence, watch work, approve actions, and dispatch to desktop.

### Marketing outruns the code

Current copy refers to more than 1,000 integrations and a visual builder. The repository shows roughly 41 checked-in action catalogs and no general visual node editor. Legal footer links also need working destination pages.

These gaps matter in sales. A buyer will test the claims.

### Platform debt raises the risk

Some runtime modules are very large, and authority crosses several code paths. Kubernetes security notes also describe plaintext internal hops with no workload mTLS. Hivy should split trusted boundaries and encrypt service traffic before claiming end-to-end service protection.

## What Hivy can own

Hivy's best position isn't “another strong model.” Model access changes too fast and every competitor has it.

The defensible product is a governed agent workforce:

- Company agents belong to teams, not one employee.
- Work can run in Hivy's cloud, on a company computer, or in a customer environment.
- One rule system controls every action, no matter where it runs.
- Every job has evidence, cost, approvals, and an outcome.
- Unlimited collaborators can review work without adding another seat fee.

The base exists. The next job is turning it into one product.

## Code references

| Area | Files checked |
|---|---|
| Agents and runs | `internal/model/agent.go`, `internal/model/session.go` |
| Login and org routes | `cmd/server/serve_routes.go`, `cmd/server/serve_routes_v1.go` |
| Audit and search | `internal/middleware/audit.go`, `internal/rag/connectors/all.go` |
| Device apps | `apps/desktop/README.md`, `apps/mobile/App.tsx` |
| Product claims | `apps/web/app/pricing/_components/pricing-page.tsx`, `apps/web/app/home/_components/landing-shared.tsx` |
| Cluster security | `kubernetes/docs/security.md` |
