-- +goose Up
-- Enforce trigger-delivery idempotency at the database level. claimTriggerDelivery
-- inserts (trigger_id, delivery_id) with ON CONFLICT (trigger_id, delivery_id)
-- WHERE delivery_id <> '' DO NOTHING to dedup asynq retries and provider
-- redeliveries, but no unique index backed that predicate (the model's GORM
-- uniqueIndex tag is never applied — the project has no AutoMigrate), so the
-- claim errored instead of deduping. This partial unique index matches the
-- ON CONFLICT target exactly. Empty delivery_ids are excluded because the claim
-- code short-circuits them before insert.
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_trigger_deliveries_trigger_delivery
    ON agent_trigger_deliveries USING btree (trigger_id, delivery_id)
    WHERE delivery_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_agent_trigger_deliveries_trigger_delivery;
