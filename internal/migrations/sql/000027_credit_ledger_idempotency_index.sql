-- +goose Up
-- Idempotency key for credit grants/spends. The application's check-then-insert
-- in GrantWithTx/SpendWithTx and the 23505 (unique_violation) handling rely on
-- this index to make concurrent writes with the same reference safe. The index
-- is partial so empty-ref entries (manual adjustments, expiry offsets without a
-- stable key) are not idempotency-scoped against each other.
CREATE UNIQUE INDEX idx_credit_ledger_entries_idem
    ON credit_ledger_entries (org_id, reason, ref_type, ref_id)
    WHERE ref_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_credit_ledger_entries_idem;
