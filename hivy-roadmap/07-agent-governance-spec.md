# Agent rules, testing, and releases

Status: proposed
Build in: phases 0 to 4
Teams: Agent Platform, Security, Product, Web

## The rule

The model can propose work. It can't grant itself power.

Hivy needs one trusted control system for connectors, MCP tools, browsers, desktop apps, terminals, sandboxes, generated apps, and subagents.

## Start with a job, not a blank prompt

The agent builder should ask what result the agent owns, which team owns it, who is accountable, what can start work, what evidence it can read, what it may change, when it must ask for help, what it must never do, and how success is measured.

Then configure triggers, knowledge, actions, policies, models, memory, skills, subagents, compute, environments, tests, and release plan.

A generated map can explain triggers, decisions, tools, approvals, handoffs, and outputs. Most users should edit forms and rule tables, not draw a maze of boxes.

## Versions and environments

Publishing creates an immutable version. It contains the job, instructions, model rules, skills, tool and knowledge grants, connections, triggers, policies, memory settings, sandbox, limits, test result, publisher, and hashes.

Development, staging, and production use different connection instances and secrets. Promotion points an environment to an approved version; it doesn't edit that version. Active work stays on its pinned version unless an operator chooses to restart it.

Release review must call out new writes, wider data access, lower approval limits, external sharing, new model providers, region changes, bigger budgets, new desktop apps, MCP changes, and worse test results.

## Action manifests

Every action needs a stable ID and version, plus:

- Effects such as read, internal write, external message, money, deletion, admin, authentication, code, or data transfer.
- Target and input/output schemas.
- Sensitive fields and preview support.
- Idempotency, retry, result check, and undo behavior.
- Default risk and required identity.

Companies may raise risk. They shouldn't lower Hivy's safety floor for permanent deletion, money, credentials, access, or production code.

## Policy

Policy runs outside the model prompt. It sees the org, team, agent version, requester, identity, device, environment, connection, action, target, clean payload summary, data labels, destination, amount, cost, time, and run history.

It can allow, deny, ask for one or two approvals, enforce a limit, remove fields, require a test system, require a managed device, choose a region, or send work to a group.

Rules need IDs, owners, scope, dates, tests, and change history. When two rules conflict, the stricter one wins unless an explicit priority says otherwise.

Examples make this concrete:

- Refunds below $100 can run. Up to $1,000 needs support approval. Larger refunds also need finance.
- External email stays approval-only until an agent proves a low correction rate over enough accepted drafts.
- Production deploys need tests, the approved window, a managed computer, and on-call approval.
- Contact export stays blocked.

Every decision becomes a permanent record linked to the action.

## Approvals

An approval shows the agent, work item, action, target, before/after change, reason, evidence, rule, value at risk, and expiry. The exact payload remains available for inspection.

Approvers can accept once, deny with a reason, edit allowed fields, ask for evidence, delegate, or create a narrow temporary exception if policy allows. Any reviewed-field change cancels the old approval.

Two-person approval needs two different eligible people. Policies can stop self-approval. High-risk phone approval uses the device biometric prompt.

## Execution

Hivy runs only the payload whose hash was approved. It adds an idempotency key, records the provider request ID, follows manifest retry rules, and checks the result.

If the target changed after review, Hivy asks again. If an action has a safe undo, expose it through policy. Broad “always allow” rules shouldn't cover money, permanent deletion, credentials, access changes, or production code unless an org admin writes a narrow rule on purpose.

## Hostile content

Emails, websites, files, tickets, MCP output, and screenshots may contain instructions written to fool the agent. Label every context source and trust level. Keep tool rules and company policy outside that content.

Check destinations, sensitive fields, secrets, downloads, file types, domains, and instruction conflicts before high-risk work. If a web page asks the agent to upload customer data somewhere unrelated, stop and show the user what happened.

## Tests and releases

Each production agent needs a reviewed test set with normal work, missing data, conflicting facts, forbidden records, hostile instructions, provider failure, duplicates, and vague requests.

Score result correctness, rule compliance, tool choice, citation accuracy, human edits, wasted actions, time, cost, and unsafe behavior. A zero-tolerance failure blocks release.

Canaries route a chosen share of safe work to the new version. Compare it with the current one, then promote, pause, or roll back. Rollback changes future work; it doesn't rewrite history.

## Catalog, subagents, memory, and models

Catalog entries need an owner, job, connections, permissions, cost range, test result, versions, and support status. Templates create customer-owned drafts. Managed updates show a diff and require a choice.

A parent gives each subagent a bounded task, context, permission ceiling, budget, deadline, and output. Children can't gain rights their parent lacks. Their actions and costs remain visible.

Keep personal, agent, project, and org memory separate. Users can inspect, edit, and delete it. Passwords, card data, and tokens never belong in free-form memory.

Model routing follows company rules for provider, model, region, data type, team, agent, action, price, quality, and availability. Record which model received data and why. Switching models can't change action rights.

## Requirements

| ID | Hivy must |
|---|---|
| **GOV-001** | Build agents around a named job, owner, and result measure. |
| **GOV-002** | Publish immutable versions with separate environments. |
| **GOV-003** | Show security changes in release diffs. |
| **GOV-004** | Require a versioned manifest for every action. |
| **GOV-005** | Judge every meaningful action outside the model prompt. |
| **GOV-006** | Bind approval to the exact payload and expire changed requests. |
| **GOV-007** | Support two-person approval and separation of duties. |
| **GOV-008** | Check writes and offer safe undo where defined. |
| **GOV-009** | Label trust and scan risky outbound work. |
| **GOV-010** | Block production releases that fail required tests. |
| **GOV-011** | Support canary, pause, promote, and roll back. |
| **GOV-012** | Limit subagent rights, cost, depth, and concurrency. |
| **GOV-013** | Keep memory scoped, visible, and deletable. |
| **GOV-014** | Route models within data, region, and cost rules. |

## Done when

- A prompt can't override a denial.
- Editing an approved field forces a new review.
- A subagent can't call a tool its parent can't call.
- Production uses immutable versions that passed required tests.
- A rollback leaves old run history untouched.
- An operator can trace an action through manifest, rule, approval, execution, check, and result.
- Revoked rights stop new work on time.

Measure policy results, prevented violations, approval time, expired requests, result checks, undo success, release failures, canary rollback, edits by version, and subagent cost.
