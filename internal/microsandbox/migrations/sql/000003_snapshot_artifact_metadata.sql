-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_snapshots
    ADD COLUMN IF NOT EXISTS artifact_digest text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS artifact_size_bytes bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS artifact_media_type text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS snapshot_digest text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS image_manifest_digest text NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
