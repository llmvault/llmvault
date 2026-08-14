# Hivy roadmap

Research date: 14 August 2026
For: founders, product, design, engineering, security, operations, and sales

Hivy shouldn't become another chat app with a longer tool list. It should run company work: take a request, find the right facts, act within clear limits, ask when it gets stuck, and leave behind a result people can check.

That idea drives every file in this folder.

## Start here

Read the first five files to understand the market and Hivy's starting point:

1. [What Hivy has today](00-current-state.md)
2. [ChatGPT for business](01-chatgpt-business-research.md)
3. [Claude for business](02-claude-business-research.md)
4. [Grok Business and Grok Bot](03-grok-business-research.md)
5. [What Hivy must build](04-competitive-requirements.md)

Then use the specs when it's time to design or build:

| Product | Platform and planning |
|---|---|
| [Work system](05-core-work-system-spec.md) | [Business and enterprise controls](12-enterprise-admin-security-spec.md) |
| [Company knowledge and connectors](06-knowledge-connectors-spec.md) | [Platform design](13-platform-architecture-spec.md) |
| [Agent rules, testing, and releases](07-agent-governance-spec.md) | [Delivery plan](14-delivery-roadmap.md) |
| [Desktop app](08-desktop-spec.md) | [All 91 features](15-feature-inventory.md) |
| [Mobile app](09-mobile-spec.md) | |
| [Web app](10-web-spec.md) | |
| [Files, reports, and media](11-artifacts-multimodal-spec.md) | |

## One product model

Chat is how someone asks for work. It isn't the work itself.

The main object should be a **work item**. A work item has a requester, owner, status, deadline, evidence, costs, approvals, actions, and final result. It stays alive when someone closes the app or switches devices.

```text
Org
  -> Team
    -> Agent
      -> Work item
        -> Run
          -> Action
            -> Result
```

Policies, approvals, files, devices, connections, and audit events attach to that chain. Someone with the right access should always be able to answer: who asked, which agent ran, what it read, what it changed, who approved it, how much it cost, and whether the result was accepted.

## Product plans

The roadmap assumes three offers. Prices can wait.

**Usage** serves individuals and small teams. They get agents, cloud and local work, basic connections, schedules, files, and normal support.

**Business** adds what a company needs to trust daily agent work: MFA, SSO, verified domains, action rules, approvals, budgets, company search, reliable audit records, and better support.

**Enterprise** adds SCIM, custom roles, retention rules, compliance exports, regional controls, customer keys, private execution, SLAs, and named support.

Hivy can keep unlimited collaborators as its pricing difference. Charge for work done and for the real cost of running business controls, not for every person who needs to review an invoice or approve a deployment.

## What “shipped” means

A feature isn't done because a happy-path demo works. It needs correct access checks, useful errors, recovery after failure, audit records, cost tracking, retention behavior, user docs, and tests for its acceptance criteria.
