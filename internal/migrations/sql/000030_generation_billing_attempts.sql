-- +goose Up
-- Track how many times the billing batch has attempted (and failed) to debit a
-- generation row. Rows that fail with insufficient_credits are left with
-- billed_at NULL so the existing sweep retries them after a top-up; this counter
-- caps the retries so a permanently underfunded org cannot hot-loop the batch.
ALTER TABLE generations
    ADD COLUMN IF NOT EXISTS billing_attempts integer DEFAULT 0 NOT NULL;

-- +goose Down
ALTER TABLE generations
    DROP COLUMN IF EXISTS billing_attempts;
