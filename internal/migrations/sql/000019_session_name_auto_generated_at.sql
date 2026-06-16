-- +goose Up
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS session_name_auto_generated_at timestamptz;

-- +goose Down
ALTER TABLE sessions
    DROP COLUMN IF EXISTS session_name_auto_generated_at;
