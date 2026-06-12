-- +goose Up
-- Idempotency key for trigger deliveries. EmployeeTriggerDispatch claims a row
-- per (trigger_id, delivery_id) before posting the (irreversible) message to the
-- runtime, and the store-delivery task upserts runtime correlation fields onto
-- the same row. The unique index makes both the pre-post claim (ON CONFLICT DO
-- NOTHING) and the store upsert safe under asynq retries so a retried batch does
-- not re-deliver triggers the agent already acted on.
--
-- delivery_id is NOT NULL DEFAULT '' on this table; the partial predicate keeps
-- legacy rows that never carried a delivery_id out of the idempotency scope.
CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_trigger_deliveries_trigger_delivery
    ON employee_trigger_deliveries (trigger_id, delivery_id)
    WHERE delivery_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_employee_trigger_deliveries_trigger_delivery;
