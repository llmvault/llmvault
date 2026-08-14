# Business and enterprise controls

Status: proposed
Build in: phases 0, 1, and 5
Teams: Identity, Security, Platform, Admin, Legal, Finance

## What belongs in each plan

Business needs MFA, passkeys, SSO, verified domains, safe default roles, session and device control, policy, approvals, reliable audit, budgets, basic retention, deletion status, and working legal pages.

Enterprise adds SCIM, directory groups, custom roles, compliance and analytics APIs, SIEM export, legal hold, regional rules, customer keys, network and device policy, private execution, SLAs, onboarding, and named support.

Don't sell a control before it works in the customer's requested setup.

## Login and domains

Support TOTP, passkeys, one-use recovery codes, and recent-login checks for risky admin actions. Companies can require MFA by role and insist on phishing-resistant step-up.

Domain verification uses DNS. Admins can claim a domain, require SSO, decide how personal accounts behave, and move existing users through a visible process. Never move their data silently.

SAML and OIDC setup needs metadata, signed messages, certificate rotation, claim mapping, test mode, break-glass owners, and an enforcement date. A readiness test must stop admins from locking out every owner.

JIT provisioning gives new people a safe role and approved teams. Removing a user ends new access quickly and starts reassignment for owned agents, work, connections, and approvals.

## SCIM, groups, and roles

SCIM handles users, groups, activation, profile fields, and membership with idempotent updates. Directory groups can map to teams, roles, approval groups, and policy subjects.

Admins need sync health, errors, unmatched mappings, and a preview of changes. Hivy must protect the last active owner.

Keep owner, admin, and member, then add permission bundles for operator, approver, connection manager, agent publisher, billing admin, security auditor, and compliance exporter. Enterprise customers can combine stable resource/action permissions into custom roles.

An access inspector should answer why someone can perform an action and who else can do it.

## Sessions and devices

List web, mobile, desktop, API, and connector sessions with device, location estimate, login method, start, last use, and scopes. Users can revoke one or all.

Admins set idle and total duration, recent-login intervals, concurrent-session limits, device requirements, and token rules. Revocation must reach APIs, streams, desktop dispatch, mobile bindings, and cached access within a stated target.

## Budgets and billing

Budgets can apply to org, team, project, agent, workflow, and cost center. Support warnings, hard caps, forecast alerts, daily or monthly periods, and approved exceptions. Policy decides whether running approved work may finish after a cap.

Track model, compute, browser, storage, rendering, connector, and outside service costs. Report cost per accepted result as well as raw use.

Business billing needs invoices, tax details, payment method, contacts, and usage export. Enterprise adds purchase orders, payment terms, commitments, and procurement exports.

## Audit

The current in-memory request audit isn't enough. Each transaction that changes business or security state must write an audit outbox record in the same database transaction. A worker then delivers it and tracks sink acknowledgement.

Events store who acted, session/device, resource, action, result, reason, changed fields, policy, approval, trace, and safe metadata. Cover login, roles, sessions, connections, sources, agents, policies, approvals, actions, browsers, desktops, files, sharing, retention, exports, keys, billing, and admin settings.

Search and exports need stable schemas and cursors. SIEM delivery must retry, sign requests, resume from checkpoints, and expose failed records. Never drop evidence because a buffer filled.

## Retention, deletion, export, and legal hold

Set retention by data type: messages, work events, model input/output, tool data, audit, files, recordings, transcripts, browser data, desktop metadata, memory, embeddings, backups, and support records.

Deletion must cover databases, vectors, search, object storage, browser profiles, caches, analytics, and backups. Show requested, running, blocked, complete, and backup-expiry states. A job isn't complete while one registered store is unaccounted for.

Exports need a manifest, schema version, hashes, and relationships. Legal hold blocks normal deletion for matching data and logs each search and export.

## APIs, regions, and customer keys

Compliance APIs return audit, work metadata, policy decisions, approvals, files, and admin events. Content fields need separate rights. Analytics APIs return use, cost, outcomes, version results, approval time, source health, and denials with stable definitions.

Region policy covers databases, objects, vectors, search, backups, logs, browser workers, coordination, and support. Model-processing region is a separate rule. If no allowed region is available, fail; don't fall back across the boundary.

Customer-managed keys use a named customer KMS reference. Document covered stores, rotation, disable, recovery, and deletion. If the key fails, protected reads and writes stop. Support can't bypass it.

## Network and private execution

Support IP ranges, device/network conditions, private connector gateways, outbound destination rules, webhook signatures, and desktop MDM settings.

Encrypt and authenticate service traffic before claiming every internal hop is protected.

Enterprise deployment may use a dedicated data plane, customer-cloud workers, private browser/connector workers, or supported self-hosting. Document what metadata, credentials, model traffic, connector traffic, updates, and support access cross the boundary. Keep one release and migration path for every form.

## Trust and support

Publish Terms, Privacy Policy, DPA, subprocessors, security design, model-provider handling, deletion rules, vulnerability reporting, incident updates, status history, and backup promises.

Enterprise service needs onboarding, migration, named contacts, severity levels, response times, SLA, escalation, maintenance rules, and regular service reviews. Compliance claims need audit evidence and contracts, not a settings page.

## Requirements

| ID | Hivy must |
|---|---|
| **ENT-001** | Support TOTP, passkeys, recovery, and step-up rules. |
| **ENT-002** | Verify and claim domains without silent data movement. |
| **ENT-003** | Support SAML/OIDC, safe enforcement, JIT, and break-glass owners. |
| **ENT-004** | Sync SCIM users and groups with clear diagnostics. |
| **ENT-005** | Apply stable permissions at scoped resources and explain access. |
| **ENT-006** | List and revoke every session and device type on time. |
| **ENT-007** | Apply nested budgets, caps, cost assignment, and export. |
| **ENT-008** | Store and deliver audit events without silent loss. |
| **ENT-009** | Apply retention, proven deletion, export, and legal hold. |
| **ENT-010** | Expose versioned compliance and analytics APIs. |
| **ENT-011** | Enforce storage and model regions without unsafe fallback. |
| **ENT-012** | Operate customer keys and report their exact coverage. |
| **ENT-013** | Enforce network, endpoint, MDM, and private-connection rules. |
| **ENT-014** | Keep one platform contract across shared and private execution. |
| **ENT-015** | Publish legal, security, status, and support promises before selling them. |

## Done when

| Test | Expected result |
|---|---|
| SSO lockout drill | At least one break-glass owner retains access. |
| User removal | New access stops and owned work appears for reassignment. |
| Access simulation | Each allow or denial has a clear reason. |
| Old audit review | Risky actions remain provable after prompt content expires. |
| Deletion drill | Every store and backup date appears in the result. |
| SIEM restart | Delivery resumes from the accepted cursor without gaps. |
| Region outage | Work doesn't cross the allowed boundary. |
| Customer key disabled | Protected access stops for users and support. |

Measure login success, provisioning and revocation time, privileged roles, budget accuracy, audit delay, SIEM lag, deletion time, legal-hold health, region denials, key health, and support response.
