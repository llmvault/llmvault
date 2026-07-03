-- +goose Up
-- Per-agent model allowlists are gone: any credential-backed model in the
-- registry can be assigned to any agent, so the allowlist columns are dead.
ALTER TABLE agents DROP COLUMN IF EXISTS available_models;
ALTER TABLE agent_catalog DROP COLUMN IF EXISTS available_models;

-- +goose Down
ALTER TABLE agents ADD COLUMN IF NOT EXISTS available_models text[] NOT NULL DEFAULT '{}';
ALTER TABLE agent_catalog ADD COLUMN IF NOT EXISTS available_models text[] NOT NULL DEFAULT '{}';

UPDATE agents SET available_models = ARRAY[model] WHERE model <> '';
UPDATE agent_catalog SET available_models = ARRAY[model] WHERE model <> '';
