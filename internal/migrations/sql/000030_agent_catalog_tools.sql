-- +goose Up
-- Catalog-owned runtime tool allowlists

ALTER TABLE agent_catalog
    ADD COLUMN tools jsonb DEFAULT '{}'::jsonb NOT NULL;

-- +goose Down
ALTER TABLE agent_catalog
    DROP COLUMN IF EXISTS tools;
