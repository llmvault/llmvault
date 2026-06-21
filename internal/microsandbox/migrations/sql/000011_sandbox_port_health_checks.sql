-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_sandbox_ports ADD COLUMN IF NOT EXISTS health_check_type text NOT NULL DEFAULT '';
ALTER TABLE microsandbox_sandbox_ports ADD COLUMN IF NOT EXISTS health_check_method text NOT NULL DEFAULT '';
ALTER TABLE microsandbox_sandbox_ports ADD COLUMN IF NOT EXISTS health_check_path text NOT NULL DEFAULT '';
ALTER TABLE microsandbox_sandbox_ports ADD COLUMN IF NOT EXISTS health_check_expected_status bigint NOT NULL DEFAULT 0;
ALTER TABLE microsandbox_sandbox_ports ADD COLUMN IF NOT EXISTS health_check_timeout_seconds bigint NOT NULL DEFAULT 0;
ALTER TABLE microsandbox_sandbox_ports ADD COLUMN IF NOT EXISTS health_check_interval_ms bigint NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
