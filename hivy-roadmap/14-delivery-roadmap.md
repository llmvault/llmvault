# Delivery plan

Planning window: 15 months, then later expansion

## How to use this plan

These phases follow dependency order. Several teams can work at once, but a later feature doesn't get to skip an earlier safety gate.

Each phase ends with evidence, not a date. If the exit checks fail, the phase isn't done.

## Phase 0: get the truth and foundation right

Weeks 0 to 4

### Ship

- Fix unsupported marketing claims and dead legal links.
- Publish Terms, Privacy Policy, DPA, subprocessors, security contact, vulnerability reporting, and a status page.
- Finalize work, checkpoint, action, policy, approval, execution, file, audit, and cost records.
- Add a transactional audit outbox and consumer cursors.
- Split access, policy, approval, action, and result checks away from the model loop.
- Register every customer-data store for retention, deletion, export, backup, and region.
- Add TOTP MFA, recovery codes, recent-login checks, sessions, and budget records.

### Prove

- Database changes can't commit without their audit record.
- Crashes around a test write don't duplicate it.
- Risky actions pass through the new authority path in observe-only mode.
- Every data store appears in the lifecycle registry.
- Public claims and links match the product.

## Phase 1: business controls

Weeks 5 to 12

### Ship

- SAML/OIDC, verified domains, safe enforcement, JIT, passkeys, and break-glass owners.
- Permission bundles for approvers, operators, connection managers, auditors, and billing admins.
- Action manifests for supported connectors and MCP tools.
- Policy pages with limits, destination rules, simulations, device conditions, and history.

Then connect those controls to daily work:

- Web approvals bound to one payload, plus expiry, delegation, edits, and two-person rules.
- Idempotent action execution, provider IDs, result checks, and unknown-write recovery.
- Basic hostile-content and outbound-data checks.
- Budgets, cost assignment, audit search, and signed export.

### Prove

- SSO setup can't lock out every owner.
- Hostile text can't override a denial.
- Changing an approved field forces another review.
- Session and grant revocation meet the published time.
- Cost reports match provider and infrastructure records within the chosen tolerance.

Business can go on sale after these checks pass.

## Phase 2: one task across devices

Months 3 to 5

### Ship

- Work inbox, timeline, state machine, duplicate detection, comments, assignment, escalation, pause, retry, cancel, and result tracking.
- Projects, saved views, cross-device events, and alerts.
- Mobile v1: voice, camera, scan, share sheet, push, monitoring, questions, approvals, biometrics, and encrypted offline drafts.
- Desktop v1: enrollment, folder rights, quick capture, local-only mode, remote dispatch, repository and terminal profiles.
- One artifact model with DOCX, PPTX, XLSX, PDF, previews, comments, versions, and checks.

### Prove

- A job starts on web, waits for a desktop file, gets phone approval, and finishes with one history.
- Device disconnect and client restart don't lose work.
- Folder escape tests fail safely.
- Old phone approvals can't run.
- Every accepted file links to its source work and exact version.

## Phase 3: company knowledge and agent releases

Months 5 to 8

### Ship

- Google Workspace and Microsoft 365 search with delegated identity, source access, citations, and health pages.
- Confluence and Jira, followed by workflow-specific CRM and support resources.
- Company search, planned research, MCP schema review, and private outbound gateway.
- Job-first builder, immutable versions, separate environments, release diff, tests, gates, canary, rollback, and certified catalog.
- Service identities, memory controls, model rules, governed subagents, and concurrency limits.
- Operations pages for outcomes, edits, cost, source health, approvals, and emergency pause.

### Prove

- Search results match each employee's current source rights.
- Revoked content disappears on time.
- Every production agent has an owner, version, tests, rollback, manifests, and policy.
- A canary can stop and roll back a bad release.
- Changed MCP writes remain off until review.

## Phase 4: browser and computer work

Months 8 to 11

### Ship

- Visible desktop app control with per-app rights, emergency stop, takeover, clipboard rules, and sensitive-action pauses.
- Shared desktop browser and isolated cloud browser with site, file, login, region, and profile rules.
- Hostile-instruction warnings, destination checks, download quarantine, and window masking.
- Better terminal work, worktrees, and bounded long-running commands.
- Meeting consent, recording, transcript, decisions, tasks, and approved follow-up drafts.
- Two-way voice on desktop, mobile, and web.

### Prove

- The user can see and stop every control session at once.
- Hostile page text can't escape app, site, clipboard, or destination rules.
- Unknown browser and desktop writes enter recovery, not blind retry.
- Audio, transcript, and follow-up actions obey their own retention and approval rules.

## Phase 5: enterprise

Months 10 to 15

### Ship

- SCIM, directory groups, custom roles, access inspector, IP rules, device conditions, and MDM settings.
- Per-data-type retention, deletion proof, exports, legal hold, compliance and analytics APIs, and SIEM delivery.
- Supported storage and model regions.
- Customer-managed keys with documented coverage.
- Authenticated encrypted internal traffic.
- Dedicated data plane, customer-cloud workers, and private network routes.
- Enterprise onboarding, migration, SLA, support, service reviews, and incident exercises.

### Prove

- SCIM removal and session revocation meet the target.
- Deletion drills touch every store and backup schedule.
- SIEM restarts from its accepted cursor without gaps.
- Provider failure doesn't cross a region rule.
- Customer-key disable and recovery work in a drill.
- Real service data supports the promised SLA.

Enterprise can go on sale for a given setup only after its requested controls pass.

## Later work

After the control system and main surfaces settle, add richer apps and dashboards, deeper office editing, brand-controlled images, video where customers will pay for it, more connector resources, regulated-industry packages, and added hosting forms.

Don't chase a huge connector count, broad computer control, video, or a general workflow canvas before daily business work is reliable.

## Packaging

**Usage:** agents, hosted and local runs, basic connections, schedules, files, and normal support for individuals and small teams.

**Business:** an org fee plus use. Include MFA, SSO, domains, policy, approvals, budgets, audit, company search as it ships, team admin, and business support. Keep unlimited collaborators if the economics work.

**Enterprise:** annual contract. Add SCIM, custom roles, retention, compliance APIs, regions, customer keys, private execution, networking, SLA, onboarding, and named support.

## Teams that run through every phase

The control team owns work, workflow, access, action, policy, approval, audit, and cost. Trust owns identity, data rules, legal, and security operations. Surface teams own web, mobile, and desktop. Knowledge owns connectors and search. Agent quality owns builder, tests, releases, and operations. Artifacts owns native files, previews, comments, and apps.

Desktop control and enterprise data work need dedicated staff. They aren't spare-time features.

## Measure the program

Product: accepted completion, time to result, waiting time, edits, takeover, retry, rollback, cross-device use, and file acceptance.

Safety: blocked violations, complete approval evidence, audit delay, permission age, access revocation, deletion, key disable, and reviewed data-loss alerts.

Economics: cost per accepted result, assigned spend, budget forecast error, prevented overage, margin by execution place, and Business conversion caused by control features.
