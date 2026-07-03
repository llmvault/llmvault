-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS microsandbox_aliases (
    alias text PRIMARY KEY,
    sandbox_id text NOT NULL,
    port bigint NOT NULL,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_microsandbox_aliases_sandbox_id
    ON microsandbox_aliases (sandbox_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
