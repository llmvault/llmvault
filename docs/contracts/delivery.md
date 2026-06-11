# Message-delivery contract (ack ⇒ durable)

This document captures the delivery guarantees of Hivy's message paths and the
invariants the integration tests enforce. The overriding rule is **ack ⇒
durable**: a `2xx`/`nil` ack is only returned once the message has reached a
state from which it will not be lost. Anything that may still be lost returns a
non-2xx (or non-nil error) so the upstream layer (provider, runtime outbox, or
asynq) redelivers, and every redelivery must be deduped to the same effect as a
single delivery.

Derived from the code at:
`internal/gateway`, `internal/handler/nango_webhooks.go`,
`internal/handler/employee_outbound_webhooks.go`,
`internal/handler/employee_event_writer.go`,
`internal/tasks/employee_trigger_dispatch.go`,
`sandboxes/runtime/crates/outbound`.

---

## 1. Inbound (provider webhook → gateway → runtime)

Flow: provider → `nango_webhooks.go` HTTP handler → `gateway.Service.Receive*` →
`RuntimeMessenger.Send` (POST to the sandbox runtime).

### When `200` is returned to the provider

`200` is returned only when one of:

- The runtime accepted the message (`Send` succeeded and the inbound event row
  was marked `delivered`), **or**
- The event was intentionally not delivered — adapter-ignored (bot message,
  unparseable), connection inbound rejected (`allowConnectionInbound`), or a
  **duplicate** of an already-`received`/`delivered` event.

Both are durable outcomes: the message was either handed to the runtime or
deliberately dropped.

### What is retried (non-2xx → provider redelivers)

If `gatewayService.ReceiveWebhookFromConnection`/`Receive` returns an error
(sandbox waking, runtime restart, post timeout), the inbound event row is marked
`status="failed"` (`markEventFailed`) and the handler returns `500`
(`nango_webhooks.go:149-159`). The provider redelivers the webhook.

### Dedupe semantics (incl. failed-row retry)

- Inbound events are keyed by `(route_id, dedupe_key)` (or
  `(route_id IS NULL, org_id, dedupe_key)` for the connection path). The insert
  uses `ON CONFLICT DO NOTHING` (`store.go insertInboundEvent`).
- A conflicting row with status `received`/`delivered` is a **duplicate**:
  return it, do not re-`Send`.
- A conflicting row with status **`failed`** means the prior `Send` never
  reached the runtime. It is **reclaimed**: the row is conditionally moved back
  to `received` (`WHERE id = ? AND status = 'failed'`) and `Send` is re-run.
  This is the key fix for P0-12 — a transient runtime failure no longer
  tombstones the message forever.
- Exactly one inbound event row exists per `dedupe_key` across the
  fail→retry→deliver lifecycle (no duplicate rows, no duplicate runtime sends).

Tested by: `internal/gateway/service_inbound_retry_test.go`.

---

## 2. Outbound (runtime outbox → Go: `employee_outbound_webhooks`)

Flow: runtime SQLite outbox (`crates/outbound`) → signed HTTP POST →
`EmployeeOutboundWebhookHandler.Handle`. The outbox retries any non-2xx with
backoff, so the Go handler's ack policy defines durability.

### Ack rules: `5xx` until durable

`Handle` returns `500` (so the outbox retries) when a **revenue- or
reply-bearing** record could not be durably persisted:

- `agent.run.model.usage` (the sole billing record for runtime LLM spend): a
  token/credential lookup error or a generation insert failure returns `500`
  (P0-10). The generation primary key is derived deterministically from
  `(sandbox_id, session_id, sequence)` (`runtimeGenerationID`), so the outbox
  retry is **idempotent** — a duplicate insert collides on the PK and is treated
  as already-recorded, never double-billed.
- `session.created` / session-event persistence in the **synchronous** writer
  path (no `EmployeeEventWriter` configured): the `tx.Create` runs inside the
  request and a failure returns `500`.

`Handle` returns `200` for best-effort side effects that are captured to Sentry
but must not block the ack (memory-retain enqueue, gateway runtime-final
delivery, schedule sync, `last_active_at` update).

Signature verification **fails closed**: with no encryption key, or an invalid
HMAC, the handler returns `401` and never persists (P1-2).

### Event-writer flush guarantees (`EmployeeEventWriter`)

Production wires the **buffered** writer (`cmd/server/serve.go`). Its durability
does not come from the HTTP ack (which returns immediately); it comes from the
drain:

- `flushBatch` inserts each batch inside a transaction and **retries transient
  failures** with capped exponential backoff
  (`employeeEventFlushRetries=5`, `250ms → 5s`). A transient Postgres blip no
  longer silently drops up to 100 buffered events (notably
  `agent.message.sent`, the only durable record of a reply) — P0-15.
- A unique-violation on retry is treated as success (a prior attempt already
  committed), so retries are idempotent.
- On **shutdown** the channel is closed and the final flush runs on
  `context.WithoutCancel(ctx)` with a bounded timeout, so the buffered backlog
  is persisted instead of being discarded on the already-cancelled root signal
  context — P0-15.
- Buffer-full falls back to a synchronous, transactional direct write rather
  than dropping the entry.

Tested by: `internal/handler/employee_event_writer_retry_test.go`,
`employee_event_writer_shutdown_test.go`,
`employee_outbound_generation_idempotency_test.go`,
`employee_event_writer_redelivery_test.go` (this change).

### Outbound agent replies (gateway runtime-final)

When an `agent.message.sent` event drives a gateway reply,
`gateway.Service.HandleRuntimeFinal` sends the provider reply keyed by an
outbound dedupe key on `(route_id, dedupe_key)`:

- A `sent` (or in-flight) delivery row short-circuits — no duplicate reply.
- A **`failed`** delivery row is **retried in place** (`upsertDelivery` updates
  the existing row rather than inserting a duplicate that would violate the
  unique index). A transient Slack/provider error no longer permanently drops
  the reply under an occupied dedupe key — P0-13.

Tested by: `internal/gateway/service_delivery_retry_test.go`.

---

## 3. Trigger dispatch idempotency (`employee_trigger_dispatch.go`)

Flow: provider/event → `EmployeeTriggerDispatch` asynq task →
`Handle` loops over all matched triggers → `deliver` → `PostHTTPMessage` to the
runtime. asynq retries the **whole task** on any returned error.

The irreversible step is `PostHTTPMessage` (it makes the agent act — open PRs,
send messages). Idempotency is enforced by a **claim before post**:

1. `claimTriggerDelivery` inserts an `employee_trigger_deliveries` row keyed by
   `(trigger_id, delivery_id)` with `ON CONFLICT DO NOTHING` *before* posting.
   - Won the claim (`RowsAffected > 0`) ⇒ this attempt posts.
   - Lost the claim ⇒ a prior attempt already posted; **skip** (return nil).
2. `PostHTTPMessage`.
3. If the post **fails**, the claim is released
   (`releaseTriggerDeliveryClaim`, only for rows that never recorded a runtime
   session id) so the asynq retry can re-deliver this trigger. The handler then
   returns the error.

Consequence (P0-14): when the batch fails at trigger N, triggers `1..N-1` keep
their claims and are **not re-posted** on retry; only `N..end` are delivered.
The agent never re-receives an already-delivered trigger. Exactly one delivery
row exists per `(trigger_id, delivery_id)`.

A blank `delivery_id` disables dedupe (always-deliver fallback).

Tested by: `internal/tasks/employee_trigger_idempotency_test.go` (unit) and
`internal/tasks/employee_trigger_dispatch_retry_test.go` (full-`Handle`
batch-fails-at-N integration, this change).
