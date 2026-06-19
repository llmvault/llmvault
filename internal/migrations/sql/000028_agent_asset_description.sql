-- +goose Up
ALTER TABLE agent_assets ADD COLUMN IF NOT EXISTS description jsonb;

-- +goose Down
ALTER TABLE agent_assets DROP COLUMN IF EXISTS description;
