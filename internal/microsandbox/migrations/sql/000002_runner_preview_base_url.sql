-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_runners
    ADD COLUMN IF NOT EXISTS preview_base_url text NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
