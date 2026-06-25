-- +goose Up
ALTER TABLE agent_catalog DROP COLUMN IF EXISTS multimodal_model;

-- +goose Down
SELECT 1;
