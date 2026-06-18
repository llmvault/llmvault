-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_runners
    ADD COLUMN IF NOT EXISTS memory_overcommit double precision NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS disk_overcommit double precision NOT NULL DEFAULT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
