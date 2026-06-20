-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS infra_probe_token text NOT NULL DEFAULT '';
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS auto_sleep_after_seconds bigint NOT NULL DEFAULT 0;
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS runtime_busy boolean NOT NULL DEFAULT false;
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS last_gateway_activity_at timestamptz;
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS last_runtime_activity_at timestamptz;
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS last_wake_at timestamptz;
ALTER TABLE microsandbox_sandboxes ADD COLUMN IF NOT EXISTS last_wake_error text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_microsandbox_sandboxes_auto_sleep
    ON microsandbox_sandboxes (auto_sleep_after_seconds)
    WHERE auto_sleep_after_seconds > 0;
CREATE INDEX IF NOT EXISTS idx_microsandbox_sandboxes_runtime_busy
    ON microsandbox_sandboxes (runtime_busy)
    WHERE runtime_busy = true;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
