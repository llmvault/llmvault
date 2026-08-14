# Work system

Status: proposed
Build in: phases 0 to 3
Teams: Product, Agent Runtime, Platform, Web, Desktop, Mobile

## Why this exists

Chats are poor work records. They don't tell a manager what is late, what needs approval, who owns the result, or whether a retry sent the same refund twice.

Hivy needs a durable **work item**. Every request becomes one, whether it starts in chat, email, Slack, GitHub, a schedule, an API, a file, a saved routine, or another agent.

## What a work item stores

Each item needs an org, team, agent, pinned agent version, requester, owner, source, priority, due date, and optional SLA. It also stores:

- Current state and the reason for it.
- Messages, files, source records, and citations.
- Runs, checkpoints, retries, and handoffs.
- Proposed actions, policy decisions, approvals, and results.
- Created files and external records.
- Model, compute, tool, and service costs.
- Human edits, final result, and acceptance status.

One item may have several runs. Retrying a failed step or moving work to another agent shouldn't erase the original request.

## States

Use eight states: `queued`, `working`, `waiting_for_person`, `waiting_for_system`, `paused`, `failed`, `cancelled`, and `completed`.

Completed work also records whether the user accepted it, changed it, rejected it, or didn't rate it. Each state change must include the previous state, new state, actor, time, reason, and trace ID. The server owns this state machine; clients can't invent transitions.

Examples:

- An approval moves work from `working` to `waiting_for_person`.
- An offline desktop moves its step to `waiting_for_system`.
- A safe retry moves `failed` back to `queued`.
- Required checks must pass before `working` becomes `completed`.

## One inbox for work

The inbox replaces a loose list of chats. Users can filter by state, team, project, agent, owner, source, due date, risk, approval, cost, and result. Saved views can be private or shared.

Each row should answer three things quickly: what is happening, who must act next, and when it is due.

The detail page has a readable timeline, current plan, blockers, evidence, proposed actions, approvals, files, and final result. Operators can open a technical view with payload hashes, tool results, policy records, cost, and trace IDs. Ordinary users shouldn't have to read model logs to understand the job.

## Starting work safely

Every source adapter produces the same request shape. External events also send an idempotency key based on org, source, source event ID, and trigger. If GitHub or Slack sends the same event five times, Hivy returns the existing item.

A user can clone work on purpose. The copy gets a new ID and links back to the original.

## Moving between devices

Web, mobile, and desktop read the same server state. A user can start on web, let desktop read a local file, approve on mobile, then review the result on web.

Each step says where it may run: Hivy cloud, enrolled desktop, customer worker, or local-only mode. Before data moves, Hivy shows what will move and where it will go.

If a desktop disconnects, local steps wait. Cloud-safe steps may continue. Hivy must never move a local-only job into the cloud because a device went offline.

## Asking a person for help

Agents should stop and ask when data is missing, sources disagree, confidence is too low, an action exceeds authority, a service fails, or the deadline is at risk.

The request for help needs completed work, evidence, the exact question, a recommended next move, any prepared draft, and what happens if nobody answers.

The person can answer, edit, approve, deny, take ownership, delegate, or return the item to the agent. Hivy resumes from a saved checkpoint with the same agent version.

## Following up without becoming annoying

An agent may watch a handoff, inbox, deadline, or outside record and create follow-up work when a written rule allows it. The rule needs a source, condition, quiet period, retry limit, owner, stop condition, and budget.

Each follow-up is a normal work item or event, never hidden background behavior. If the user closes the issue, removes access, or tells the agent to stop, pending follow-ups end. A routine learned from a person uses the same rules.

## Projects and sharing

Projects group work items, agents, files, instructions, people, and results. They support comments, mentions, assignment rules, watchers, and saved views.

Sharing levels are private, named people, team, org, and external link. Org policy sets the widest allowed level. External links may expire, require a passcode, block downloads, add a watermark, and be revoked. An agent can't widen sharing on its own.

## Notifications

Alert people for completion, questions, approvals, expiry, failure, deadline risk, device handoff, mentions, assignment, and budget thresholds. Users choose in-app, push, email, Slack, or desktop delivery within company rules.

Each alert opens the exact item. Retries shouldn't send the same alert again.

## API rules

Typed endpoints should create, list, read, comment, assign, pause, resume, cancel, retry, attach files, record source events, and record outcomes. Progress uses SSE with resumable event IDs.

Mutations send an idempotency key and expected item version. If two people edit at once, the second client gets a typed conflict instead of overwriting newer work.

## Access and audit

Team access must use `internal/access`. Someone outside the team gets `404`; missing login gets `401`.

Requesting, owning, operating, approving, and auditing are different permissions. Seeing a work item doesn't automatically expose secrets or raw connector payloads.

State changes, assignments, comments, sharing, takeover, retry, actions, approvals, and downloads all create durable audit events.

## Failure rules

- Worker crash: resume at the last saved checkpoint.
- Duplicate event: return the existing item.
- Connector outage: show the retry plan and waiting state.
- Approval race: run the reviewed payload once.
- Client disconnect: work continues if policy allows; reconnect resumes events.
- Cancel during a write: determine whether the write happened before doing anything else.
- Audit failure: block high-risk execution.

## Requirements

| ID | Hivy must |
|---|---|
| **WORK-001** | Keep work after every client and worker closes. |
| **WORK-002** | Enforce state changes and edit versions on the server. |
| **WORK-003** | Normalize every approved source into one request shape. |
| **WORK-004** | Deduplicate external events. |
| **WORK-005** | Pin each run to an agent version and execution place. |
| **WORK-006** | Build user and operator timelines from the same events. |
| **WORK-007** | Support comments, mentions, assignment, watchers, views, and safe sharing. |
| **WORK-008** | Pause and resume from saved checkpoints. |
| **WORK-009** | Never change data location just because a device failed. |
| **WORK-010** | Record cost and acceptance for completed work. |
| **WORK-011** | Send policy-aware alerts that open the right item. |
| **WORK-012** | Publish typed APIs and resumable progress streams. |
| **WORK-013** | Track routine runs and proactive follow-ups as normal, stoppable work. |

## Done when

- A worker can restart without losing the job.
- One item can cross web, desktop, and mobile without splitting its history.
- Five copies of one webhook create one item.
- A retry can't repeat a verified write.
- An unauthorized person can't tell the item exists.
- An operator can reconstruct the job without reading raw model traffic.

Measure accepted completion, time to result, correction rate, waiting time, cross-device use, recovery rate, overdue work, and cost per accepted result.

First release does not need BPMN, customer-written state machines, public anonymous editing, or automatic mid-run upgrades to a new agent version.
