-- +goose Up
ALTER TABLE sessions DROP COLUMN IF EXISTS access_mode;

-- +goose Down
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS access_mode text DEFAULT 'full'::text NOT NULL;
