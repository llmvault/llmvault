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
CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_gateway_events_null_route_dedupe
    ON employee_gateway_events (org_id, dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_gateway_deliveries_null_route_dedupe
    ON employee_gateway_deliveries (dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_employee_gateway_deliveries_null_route_dedupe;
DROP INDEX IF EXISTS idx_employee_gateway_events_null_route_dedupe;
