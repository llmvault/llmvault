# Platform design

Status: proposed
Build in: phases 0 to 5
Teams: Platform, Runtime, Security, Data, SRE

## Why the platform must change

Long-running jobs, approvals, desktop control, browser work, files, and compliance can't live safely inside one large model loop.

The model should plan and propose. Trusted code should decide access, apply policy, collect approval, run the action, check the result, and save evidence.

This design describes boundaries. It doesn't require a separate network service for every box on day one.

## Rules for every component

| Rule | Why |
|---|---|
| Save work in durable storage | Worker failure must not erase progress. |
| Give every outside write an idempotency key and result check | Retries can't repeat business effects. |
| Stop risky work when proof is missing | Identity, policy, approval, and audit aren't optional. |
| Carry org scope and request context on every query | Tenant access and cancellation remain correct. |
| Keep secrets out of prompts, normal logs, and client errors | Models and users shouldn't see credentials. |
| Use one action contract on every execution surface | Rules don't change between connector, browser, desktop, terminal, or app. |
| Show and control execution place | Data can't move silently. |
| Make events replayable | Consumers can recover and handle repeated delivery. |

## Main parts

### Work service

Owns work items, states, assignments, checkpoints, comments, timelines, results, and duplicate-source detection. Clients request changes; this service decides whether they're valid.

### Workflow engine

Runs saved steps. It can retry, wait, pause, resume, cancel, time out, recover, or send a failed job to a dead-letter queue. A step may call the model, ask policy, wait for approval, use a device, create a file, or check an outside result.

### Agent runtime

Loads one immutable agent version, assembles permitted context, calls a model, checks structured output, and proposes a plan or action. It cannot grant access or execute a write by itself.

### Capability compiler

Combines org, team, agent, person, identity, device, environment, and resource grants. It returns a short-lived scoped capability or a denial.

### Action registry

Stores action manifests for connectors, MCP, browser, desktop, terminal, and apps. Each manifest describes effects, schemas, risk, preview, retries, idempotency, undo, and result checks.

### Policy service

Takes clean facts about the request and returns allow, deny, limits, field changes, or an approval rule. It doesn't trust prose pulled from a website or email.

### Approval service

Creates a request for one exact payload, checks who may approve, enforces expiry and login strength, and returns a signed execution right.

### Action executor

Checks manifest, capability, policy result, approval, payload hash, and idempotency key. Then it calls the provider or device and checks what happened.

### Device bridge

Keeps signed connections to desktop and mobile, stores posture, queues remote work, and resumes status after reconnect. A device can't create rights the server didn't grant.

### Routine registry and workspace manager

The routine registry stores demonstrations, structured draft steps, immutable routine versions, tests, trust level, owners, and releases. It sends each step through the normal workflow and action path; it never replays clicks with inherited authority.

The workspace manager owns persistent cloud computers, agent membership, apps, browser profiles, credential references, region, network, storage, lifecycle, and suspension. Sharing a computer doesn't create a shared identity or union of permissions.

### Knowledge, artifact, audit, and cost services

Knowledge owns sync, source access, search, citations, and freshness. Artifact owns file versions, previews, checks, comments, sharing, and export.

Audit consumes committed events and sends them to search, exports, and SIEM. Cost records model, compute, browser, storage, connector, rendering, and outside service use against the exact work and action.

## Core records

```text
WorkItem
  id, org, team, project, agent, agent_version, requester, owner,
  source, source_key, state, reason, priority, due_at, risk,
  sensitivity, location_policy, version, timestamps

Run
  id, work_item, agent_version, location, state, model_policy,
  start, end, cost

Checkpoint
  id, run, step, sequence, saved_state_ref, completed_effects, time

ActionIntent
  id, run, step, manifest_version, target, payload_ref, payload_hash,
  identity, capability, risk, idempotency_key

PolicyDecision
  id, action, policy_version, result, limits, approval_rule,
  fact_hash, reason, time, trace

Approval
  id, action, payload_hash, policy_decision, required_count,
  eligible_group, state, expiry, responses

Execution
  id, action, authorization, payload_hash, state, attempt,
  provider_request, result_ref, check_state, undo_ref, times

ArtifactVersion
  id, artifact, parent, content_hash, native_ref, preview_ref,
  lineage_ref, checks, review_state, time

RoutineVersion
  id, routine, parent, owner, source_demonstration, structured_steps,
  inputs, outputs, required_capabilities, tests, trust_level,
  environment, review_state, content_hash, time

CloudWorkspace
  id, org, owner, region, environment, allowed_agent_versions,
  app_profile, browser_profile, credential_refs, network_policy,
  storage_limit, idle_policy, retention, state, expiry
```

Large or secret data stays in encrypted storage. Queue messages and database rows keep references.

## Events

Every domain event uses the same envelope:

```json
{
  "event_id": "evt_...",
  "event_type": "work.state_changed",
  "schema_version": 1,
  "org_id": "org_...",
  "occurred_at": "RFC3339",
  "actor": {"type": "user|agent|service|device", "id": "..."},
  "resource": {"type": "work_item", "id": "..."},
  "trace_id": "...",
  "causation_id": "...",
  "correlation_id": "...",
  "data": {}
}
```

Families cover work state, steps, actions, policy, approval, execution, result checks, files, source sync, permission changes, devices, security, budgets, and admin settings.

Routine events cover teaching start, pause, resume, discard, draft generation, sensitive-data removal, review, test, publish, correction, trust change, and rollback. Workspace events cover create, agent join/leave, credential attach, file transfer, human takeover, suspend, expire, and delete.

Schema updates may add fields. Breaking changes get a new version and a migration period where both versions publish.

## Reliable audit with an outbox

When a database transaction changes business or security state, it also inserts an outbox row. One transaction either writes both or neither.

A publisher locks pending rows, sends them to the event log, and marks them done. Event ID makes retries safe. Audit search, notifications, analytics, and SIEM each keep their own cursor, so a slow destination can't cause data loss.

Outside writes need extra care. Record `started`, call the provider with an idempotency key, then save its answer and result check. If the worker dies between those steps, ask the provider what happened before trying again.

## Saved workflows

Each step has an input reference, attempt count, retry class, timeout, cancellation rule, and optional undo. Save workflow state before acknowledging queue work.

Use clear failure types: policy denied, approval expired, login expired, rate limited, provider down, bad input, bad model output, device offline, runtime crash, and unknown write result.

Pausing saves progress. Resuming checks access again. Cancelling stops future steps but still settles any write already running.

A routine version is a workflow template, not a capability bundle. Each run resolves current inputs and recompiles rights before the first step and again before risky actions. If the application screen no longer matches the taught step, the routine stops for recovery instead of guessing through the change.

## Capability tokens

Tokens are short-lived and bound to org, agent version, action, resource, audience, and execution place. A desktop token can't run in the cloud. A read token can't write. Risky actions check recent revocations again just before execution.

## Device messages

Desktop and mobile use mutual authentication and rotated session keys. Each command includes command ID, work and step IDs, capability, payload hash, expiry, and server signature.

Results include device sequence, command ID, status, output references, and local evidence. Reconnect starts after the last accepted sequence. Duplicate commands don't run twice. “Delivered” never means “executed.”

## Data lifecycle registry

Every customer-data store must register its data types, org field, region, retention, deletion method, export method, backup policy, and owner. New stores can't go live without this record.

Deletion calls each adapter and reports status by store. Search, vectors, objects, caches, analytics, and backups all count. Legal hold runs before deletion.

## Security and operations

Trace IDs connect request, work, model, policy, approval, action, provider/device, file, audit, and cost. Track work-start delay, step success, recovery, action checks, approval wait, audit delay, source age, device uptime, file rendering, alerts, and cost-ledger delay.

Use `internal/logging` and redact values. Encrypt and authenticate service traffic. Isolate browsers, code, renderers, and untrusted connectors. Give them short-lived secrets and limited network routes.

All routes remain contract-first. Generate OpenAPI and `$api` clients with route changes. Lists use named envelopes, SSE uses event IDs, and client writes include expected versions.

## Migration

1. Add new records and events behind flags.
2. Write audit outbox rows beside the current audit path and compare results.
3. Wrap current sessions and runs in work items.
4. Send connector and MCP calls through the action path in observe-only mode.
5. Turn on policy, then approval wait and resume for selected actions.
6. Add device and artifact consumers.
7. Remove lossy audit and direct execution paths after replay and failure tests pass.

## Requirements

| ID | Hivy must |
|---|---|
| **ARCH-001** | Save work and checkpoints outside runtime processes. |
| **ARCH-002** | Separate model proposals from access, policy, approval, and action. |
| **ARCH-003** | Link action, policy, approval, execution, and result records. |
| **ARCH-004** | Write audit outbox rows in the same transaction as state changes. |
| **ARCH-005** | Replay events and checkpoint each consumer. |
| **ARCH-006** | Check unknown outside writes before retry. |
| **ARCH-007** | Bind short-lived capabilities to org, scope, version, audience, and place. |
| **ARCH-008** | Sign device commands and deduplicate command IDs. |
| **ARCH-009** | Register every data store for retention, deletion, export, region, and backup. |
| **ARCH-010** | Trace each request through result and cost. |
| **ARCH-011** | Isolate untrusted work and issue short-lived secrets. |
| **ARCH-012** | Keep generated client contracts and typed errors. |
| **ARCH-013** | Store reviewed routine versions and persistent workspace policy without merging agent rights. |

## Done when

| Test | Expected result |
|---|---|
| Kill a worker at each step boundary | Saved work remains and checked writes don't repeat. |
| Call the executor without proof | Runtime receives a denial. |
| Fill or stop a memory audit buffer | Committed events still publish from the outbox. |
| Replay every event twice | Read models remain correct. |
| Resume an old checkpoint after revocation | Risky access stays denied. |
| Reconnect a device and repeat commands | Each command runs once. |
| Delete data with one store missing | The job stays incomplete and names the store. |
| Follow one trace | It reaches source, agent, model, actions, approvals, files, result, and cost. |

Test state machines, outbox transactions, crashes around outside calls, duplicate delivery, access intersections, policy cases, approval races, device reconnect, sandbox escapes, cross-org access, deletion adapters, and recovery drills.
