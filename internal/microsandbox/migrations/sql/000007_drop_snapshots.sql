-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_microsandbox_sandboxes_snapshot_id;
ALTER TABLE microsandbox_sandboxes DROP COLUMN IF EXISTS snapshot_id;
DROP TABLE IF EXISTS microsandbox_snapshots;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
