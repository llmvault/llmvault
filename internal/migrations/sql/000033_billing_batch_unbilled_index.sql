-- +goose Up
-- Supporting indexes for the billing batch tick (P2-44).
--
-- 1) Unbilled scan. selectUnbilledBatch does
--      WHERE billed_at IS NULL AND is_system = TRUE
--        AND (cost > 0 OR input_tokens > 0 OR output_tokens > 0)
--      ORDER BY created_at
--      LIMIT n FOR UPDATE OF g SKIP LOCKED
--    Without a supporting index this seq-scans the full generations table on
--    every tick and re-sorts by created_at. A partial index over only the
--    unbilled system rows, ordered by created_at, keeps the scan proportional to
--    the (small) backlog rather than total history, and serves the ORDER BY/LIMIT
--    directly. The set this index covers shrinks back toward empty as the batch
--    drains the queue.
CREATE INDEX IF NOT EXISTS idx_gen_unbilled_system_created
    ON generations (created_at)
    WHERE billed_at IS NULL AND is_system = TRUE;

-- 2) Per-org billed-cost sum. planCumulativeDebits recomputes
--      SUM(cost) WHERE org_id = ? AND is_system = TRUE
--        AND billed_at IS NOT NULL AND billing_error = '' AND cost > 0
--    per org each tick to keep split/concurrent ticks idempotent (the debit is
--    cumulative, not per-row). This is still O(billed history per org); a true
--    fix is an incremental per-org counter, deferred as too invasive for a P2
--    cleanup (it would change the cumulative-debit idempotency contract the
--    concurrent-tick tests rely on). This partial index at least bounds the sum
--    to the matching rows of one org via an index scan instead of a heap scan of
--    the whole table.
CREATE INDEX IF NOT EXISTS idx_gen_billed_org_cost
    ON generations (org_id, cost)
    WHERE is_system = TRUE AND billed_at IS NOT NULL AND billing_error = '' AND cost > 0;

-- +goose Down
DROP INDEX IF EXISTS idx_gen_unbilled_system_created;
DROP INDEX IF EXISTS idx_gen_billed_org_cost;
