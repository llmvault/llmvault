-- +goose Up
-- Catalog-owned subagent definitions

ALTER TABLE agent_catalog
    ADD COLUMN sub_agents jsonb DEFAULT '{}'::jsonb NOT NULL;

-- +goose Down
ALTER TABLE agent_catalog
    DROP COLUMN IF EXISTS sub_agents;
