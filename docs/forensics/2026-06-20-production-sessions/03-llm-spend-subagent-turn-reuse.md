# High LLM Spend And Subagent Turn Reuse

## Summary

Session `7b3367b3-d81d-431a-b4dc-1c2e1a2af105` completed successfully but spent about `$3.02` in model costs. This was real provider-reported usage, not billing duplication.

The spending came from:

- 92 main-session generation rows totaling `$2.97980742`
- 12 linked subagent generation rows totaling `$0.04308193`
- 104 total generation rows totaling `$3.02288935`
- 3023 debited credits

The primary cost driver was repeated model calls with very large context. The final PR-writing turn alone cost `$1.88038030`. Several late calls had 100k-111k input tokens and zero cached tokens, producing individual calls around `$0.15`.

There is also a concrete runtime/session-state bug: one turn emitted `final` and `turn_completed`, then continued doing model work under the same `turn_id` after a subagent result was merged back into the parent session. That post-terminal work added `$0.63853896`.

## Impact

- The session completed and created PR 191.
- The user-visible workflow appeared successful.
- The account was charged for real LLM usage.
- Part of the spend is hidden from canonical session joins because subagent generation tags use the raw runtime session key instead of the canonical DB session UUID.
- Runtime event ordering/state is misleading: one turn has two `final` events and two `turn_completed` events.

## Primary Evidence

- [Latest web sessions and linked runtime sessions snapshot](/tmp/hivy-prod-session-forensics-20260620/web_and_linked_sessions_snapshot.json)
- [Session 7b event table](/tmp/hivy-prod-session-forensics-20260620/session_7b_events.tsv)
- [Main DB snapshot](/tmp/hivy-prod-session-forensics-20260620/main_db_snapshot.json)
- [API logs](/tmp/hivy-prod-session-forensics-20260620/api_logs_4h.jsonl)
- [Runner 1 evidence](/tmp/hivy-prod-session-forensics-20260620/runner1_forensics.txt)

Relevant code paths:

- Runtime turn loop and queued follow-ups: `sandboxes/runtime/crates/runtime/src/handler.rs`
- Queued inbound merge preserving `turn_id`: `sandboxes/runtime/crates/runtime/src/handler.rs`
- Runtime generation recording: `internal/handler/agent_outbound_generation.go`
- Credit gate: `internal/middleware/require_credits.go`
- Billing cost selection: `internal/tasks/billing_batch_process.go`
- Runtime output budget warning: `sandboxes/runtime/crates/agent/src/runner.rs`

## User Timeline

All times are UTC.

| Time | User message |
| --- | --- |
| 2026-06-20 10:56:32 | "hey hakaree. do you see our codebase locally ? what's the latest commit ?" |
| 2026-06-20 10:57:13 | "what specs are on your computer ?" |
| 2026-06-20 10:57:54 | "did you not have this information in your system information? why did you have you check?" |
| 2026-06-20 10:58:30 | "okay good. but i need to understand the disk. it says 40 GB, but is that 40GB really usable?" |
| 2026-06-20 11:00:16 | User asked for a deep dive into the codebase to understand the disk provisioning issue. |
| 2026-06-20 11:10:00 | "can you create a quick pr addressing these issues ?" |

## Cost Timeline By Turn

| Turn | Model calls | Cost | Input tokens | Cached tokens | Output tokens | Window |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `turn-1781953803882-5-e8m6UMq8UdjpVquY` | 36 | `$1.88038030` | 3,652,291 | 2,865,135 | 7,597 | 11:10:06-11:22:09 |
| `turn-1781953219890-4-eXDCcLAoWHdiflF6` | 51 | `$1.11230286` | 2,948,260 | 2,701,961 | 15,092 | 11:01:06-11:07:45 |
| `turn-1781952995858-0-08f7TRwhT6g-flNy` | 3 | `$0.02457678` | 35,913 | 23,703 | 300 | 10:56:44-10:56:56 |
| Other early turns | 5 | `$0.02423256` | 65,180 | 63,656 | 1,261 | 10:57:20-10:58:44 |

The expensive rows were all `openrouter` / `glm-5.2`.

Top generation examples:

| Time | Cost | Input | Cached | Output | Sequence |
| --- | ---: | ---: | ---: | ---: | --- |
| 2026-06-20 11:22:05 | `$0.15630960` | 111,150 | 0 | 159 | 1321 |
| 2026-06-20 11:21:39 | `$0.15289900` | 109,091 | 0 | 39 | 1312 |
| 2026-06-20 11:17:49 | `$0.14880820` | 106,213 | 0 | 25 | 1292 |
| 2026-06-20 11:17:33 | `$0.14872460` | 105,949 | 0 | 90 | 1288 |
| 2026-06-20 11:17:09 | `$0.14802800` | 105,640 | 0 | 30 | 1274 |
| 2026-06-20 11:16:14 | `$0.14791720` | 105,476 | 0 | 57 | 1270 |

## Tooling Activity

The session performed substantial local work:

| Tool | Count |
| --- | ---: |
| `bash` | 63 |
| `read_file` | 22 |
| `edit_file` | 7 |
| `update_plan` | 6 |
| `grep` | 4 |
| `check_subagent_task_status` | 3 |
| `subagent_task` | 1 |
| `skill_view` | 1 |

This is enough tool output to make the prompt grow quickly. The high cost is consistent with a long tool-heavy coding session, not with one simple PR-summary request.

## Subagent Attribution Issue

The linked subagent session is:

- Canonical DB session ID: `deba8afc-c8a4-57c7-b4ff-4068787cd405`
- Runtime session key: `subagent-subagent-task-1781953265979-1`
- Source resource key: `subagent-subagent-task-1781953265979-1`

Generation tags use:

```text
session:subagent-subagent-task-1781953265979-1
```

instead of:

```text
session:deba8afc-c8a4-57c7-b4ff-4068787cd405
```

The generation recorder builds tags from the raw runtime payload:

```go
sessionID := stringValue(payload, "session_id")
tags := pq.StringArray{"agent-runtime", "source:" + source, "sandbox:" + sb.ID.String()}
if sessionID != "" {
    tags = append(tags, "session:"+sessionID)
}
```

This means normal cost joins by `sessions.id` miss the subagent spend unless they also join `sessions.source_resource_key`.

## Terminal Event Reuse Bug

Turn `turn-1781953219890-4-eXDCcLAoWHdiflF6` emitted a terminal pair, then continued:

| Time | Event |
| --- | --- |
| 2026-06-20 11:04:48.486 | `final` |
| 2026-06-20 11:04:48.672 | `turn_completed` |
| 2026-06-20 11:04:54.771 | More `thinking` under same `turn_id` |
| 2026-06-20 11:04:56.753 | More `model_usage` under same `turn_id` |
| 2026-06-20 11:07:45.666 | Second `final` |
| 2026-06-20 11:07:45.847 | Second `turn_completed` |

The post-first-terminal work:

- 25 model calls
- `$0.63853896`
- 1,816,913 input tokens
- 1,696,386 cached tokens

The runtime code explicitly drains queued follow-ups after a turn:

```rust
let follow_ups = coordinator.drain_queued(&current_inbound.session_id);
if let Some(next) = next_queued_inbound(&current_inbound, follow_ups, &mut queued_backlog) {
    current_inbound = next;
    continue 'turns;
}
```

The merge function preserves the current `turn_id`:

```rust
if let Some(turn_id) = turn_id(current) {
    map.insert("turn_id".to_string(), Value::String(turn_id));
}
```

So synthetic follow-up work can run as a new model turn but remain stamped with an already-completed `turn_id`.

## Root Causes

1. Real high-volume usage:
   - Multiple user turns.
   - Heavy tool use.
   - Large prompt context.
   - Expensive uncached calls late in the session.

2. Missing hard spend controls:
   - `RequireCredits` only checks `balance > 0`.
   - Runtime has output budget warnings, but not hard dollar/token/model-call cutoffs.

3. Subagent follow-up turn identity reuse:
   - Follow-up work after subagent completion can be merged into the parent with the original `turn_id`.
   - This creates duplicate terminal events and hides additional work under a completed turn.

4. Incomplete attribution:
   - Subagent generation rows are tagged with raw runtime session keys, not canonical DB session IDs.

## Recommended Fix Areas

1. Hard spend limits:
   - Per-session max dollars.
   - Per-turn max dollars.
   - Per-turn max model calls.
   - Max prompt tokens per call.
   - Stop after repeated large uncached prompts.

2. Runtime context management:
   - Summarize/compact after tool-heavy phases.
   - Avoid re-sending full tool history when prompt exceeds a threshold.
   - Treat zero-cache 100k+ prompt calls as a danger signal.

3. Fix subagent follow-up identity:
   - A subagent result follow-up should use a fresh `turn_id`.
   - Or it should be recorded as a distinct internal turn with explicit parent-child linkage.
   - A completed `turn_id` must never receive later `model_usage`, `final`, or `turn_completed`.

4. Improve generation tags:
   - Add `db_session:<uuid>`.
   - Add `runtime_session:<raw>`.
   - Add `parent_session:<uuid>` for subagent rows.
   - Keep raw session key for traceability, but do not use it as the only session tag.

5. UI and billing visibility:
   - Show live cost per session.
   - Include subagent costs in parent session totals.
   - Warn before expensive follow-up turns.

## Acceptance Criteria For A Fix

- No single session can exceed a configured dollar cap without explicit user approval.
- No completed `turn_id` can receive new model usage.
- Parent session cost totals include linked subagent costs.
- Cost dashboard/session joins use canonical session IDs.
- Tool-heavy sessions compact context before repeated 100k+ token calls.

## Suggested Owner Brief

Own runtime cost control and turn identity. Start with `sandboxes/runtime/crates/runtime/src/handler.rs`, `internal/handler/agent_outbound_generation.go`, `internal/middleware/require_credits.go`, and `internal/tasks/billing_batch_process.go`. The fix should make spend bounded and make terminal events semantically true.

