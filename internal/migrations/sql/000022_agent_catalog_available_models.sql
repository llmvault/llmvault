-- +goose Up
-- Catalog-owned model allowlists

ALTER TABLE agent_catalog
    ADD COLUMN available_models text[] DEFAULT '{}'::text[] NOT NULL;

UPDATE agent_catalog
SET available_models = ARRAY[model]
WHERE model IS NOT NULL
    AND model <> ''
    AND cardinality(available_models) = 0;

-- +goose Down
ALTER TABLE agent_catalog
    DROP COLUMN IF EXISTS available_models;
