-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS sleep_after_at timestamptz;
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS route_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS activity_token text NOT NULL DEFAULT '';

UPDATE microsandbox_sandboxes
SET sleep_after_at = GREATEST(
    COALESCE(last_gateway_activity_at, '-infinity'::timestamptz),
    COALESCE(last_runtime_activity_at, '-infinity'::timestamptz),
    COALESCE(last_wake_at, '-infinity'::timestamptz),
    COALESCE(created_at, '-infinity'::timestamptz)
) + (auto_sleep_after_seconds || ' seconds')::interval
WHERE status = 'running'
  AND auto_sleep_after_seconds > 0
  AND sleep_after_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_microsandbox_sandboxes_due_sleep
    ON microsandbox_sandboxes (sleep_after_at)
    WHERE status = 'running' AND auto_sleep_after_seconds > 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
