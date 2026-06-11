-- +goose Up
-- Idempotency for gateway events/deliveries whose route_id is NULL.
--
-- Several paths create an ephemeral route with ID == uuid.Nil, persisted as
-- route_id IS NULL. The existing dedupe indexes are keyed on
-- (route_id, dedupe_key); Postgres treats NULLs as distinct, so the
-- ON CONFLICT DO NOTHING insert in gateway/store.go never fires for
-- NULL-route rows and a provider redelivery inserts a second row. The
-- recovery reads already key the NULL-route case on (org_id, dedupe_key)
-- (insertInboundEvent) / (dedupe_key) (loadDeliveryByDedupe); these partial
-- indexes give that the unique enforcement it assumes. dedupe_key <> ''
-- keeps legacy/keyless rows out of the idempotency scope, consistent with
-- the route-scoped indexes.
--
-- Requires duplicate-free data: any environment carrying NULL-route
-- duplicates from before this invariant must be deduplicated (keep the
-- lowest-id row per group, matching GORM .First()) before migrating.

CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_gateway_events_null_route_dedupe
    ON employee_gateway_events (org_id, dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_gateway_deliveries_null_route_dedupe
    ON employee_gateway_deliveries (dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_employee_gateway_deliveries_null_route_dedupe;
DROP INDEX IF EXISTS idx_employee_gateway_events_null_route_dedupe;
