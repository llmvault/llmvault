-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_snapshots
    ADD COLUMN IF NOT EXISTS alias text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS global boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX IF NOT EXISTS idx_microsandbox_snapshots_alias_unique
    ON microsandbox_snapshots (alias)
    WHERE alias <> '';

CREATE INDEX IF NOT EXISTS idx_microsandbox_snapshots_global
    ON microsandbox_snapshots (global);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
