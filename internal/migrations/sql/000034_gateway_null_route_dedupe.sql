-- +goose Up
-- Idempotency for gateway events/deliveries whose route_id is NULL.
--
-- Routes are resolved by connection at runtime and several paths create an
-- ephemeral route with ID == uuid.Nil (service.go / service_connection.go),
-- which persists as route_id IS NULL. The existing dedupe indexes
-- idx_employee_gateway_events_route_dedupe / _deliveries_route_dedupe are keyed
-- on (route_id, dedupe_key); Postgres treats NULLs as distinct, so the
-- ON CONFLICT DO NOTHING insert in gateway/store.go never fires for NULL-route
-- rows and a provider webhook redelivery (or an asynq retry of the stream
-- delivery) inserts a second row, re-driving the agent and double-posting the
-- reply. The recovery reads in store.go already key the NULL-route case on
-- (org_id, dedupe_key) (insertInboundEvent) / (dedupe_key) (loadDeliveryByDedupe),
-- so these partial indexes give that the unique enforcement it assumes.
--
-- The predicate mirrors each table's NULL-route read scope: events dedupe on
-- (org_id, dedupe_key) to match insertInboundEvent's recovery WHERE; deliveries
-- dedupe on dedupe_key alone to match loadDeliveryByDedupe / the pre-send dedupe
-- read in gateway_stream_delivery (both key NULL-route deliveries on dedupe_key
-- without org). dedupe_key <> '' keeps legacy/keyless rows out of the
-- idempotency scope, consistent with the existing route-scoped indexes.
--
-- PRE-EXISTING DUPLICATES: production carries historical NULL-route double-
-- deliveries (provider redeliveries from before this fix) that violate the new
-- unique indexes. We must collapse each dedupe group to the one row the running
-- code already treats as canonical before creating the indexes, otherwise the
-- CREATE fails with SQLSTATE 23505.
--
-- SURVIVOR RULE = lowest id (UUID) per dedupe group. The recovery reads use
-- GORM .First() with no explicit ORDER BY (store.go insertInboundEvent /
-- loadDeliveryByDedupe), which compiles to "ORDER BY <primary key> ASC LIMIT 1".
-- The primary key on both tables is the random-UUID `id` column
-- (gen_random_uuid()), so the row the code resolves to on a dedupe hit is the
-- one with the smallest id, NOT the earliest created_at. We delete every other
-- row in the group so that exact survivor is the one the unique index keeps.
--
-- This migration is wrapped in goose's default transaction (it has no
-- NO-TRANSACTION annotation), so on the production failure the whole
-- migration rolled back: neither index nor the version row for 34 persisted
-- ("partial migration" = migrations 1-33 committed, 34 failed and reverted).
-- It is nonetheless made fully re-runnable: the DELETEs no-op once each group
-- has a single row, and CREATE UNIQUE INDEX IF NOT EXISTS tolerates an index
-- that a partial/manual prior attempt may have already left behind.

-- Collapse NULL-route event duplicates to the lowest-id survivor per
-- (org_id, dedupe_key). Scoped to the exact predicate of the index below.
-- A window function is used because Postgres has no MIN()/aggregate for the uuid
-- type; ROW_NUMBER() ... ORDER BY id uses uuid's native btree ordering, the same
-- ordering GORM .First() relies on, so the surviving row matches the code.
DELETE FROM employee_gateway_events e
USING (
    SELECT id
    FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY org_id, dedupe_key ORDER BY id ASC) AS rn
        FROM employee_gateway_events
        WHERE route_id IS NULL AND dedupe_key <> ''
    ) ranked
    WHERE ranked.rn > 1
) dup
WHERE e.id = dup.id;

-- Collapse NULL-route delivery duplicates to the lowest-id survivor per
-- (dedupe_key). Scoped to the exact predicate of the index below.
DELETE FROM employee_gateway_deliveries d
USING (
    SELECT id
    FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY dedupe_key ORDER BY id ASC) AS rn
        FROM employee_gateway_deliveries
        WHERE route_id IS NULL AND dedupe_key <> ''
    ) ranked
    WHERE ranked.rn > 1
) dup
WHERE d.id = dup.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_gateway_events_null_route_dedupe
    ON employee_gateway_events (org_id, dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_gateway_deliveries_null_route_dedupe
    ON employee_gateway_deliveries (dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_employee_gateway_deliveries_null_route_dedupe;
DROP INDEX IF EXISTS idx_employee_gateway_events_null_route_dedupe;
