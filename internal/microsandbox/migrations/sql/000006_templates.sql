-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS microsandbox_templates (
    id text PRIMARY KEY,
    org_id text NOT NULL,
    runner_id text NOT NULL,
    name text NOT NULL,
    base_image_ref text NOT NULL,
    status text NOT NULL,
    image_ref text NOT NULL DEFAULT '',
    image_digest text NOT NULL DEFAULT '',
    commands_json text NOT NULL DEFAULT '[]',
    logs text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    validation_sandbox_id text NOT NULL DEFAULT '',
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_microsandbox_templates_org_id ON microsandbox_templates (org_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_templates_runner_id ON microsandbox_templates (runner_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_templates_status ON microsandbox_templates (status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
