-- +goose Up
ALTER TABLE agent_catalog ADD COLUMN IF NOT EXISTS sandbox_image text NOT NULL DEFAULT 'default';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS sandbox_image text NOT NULL DEFAULT 'default';

UPDATE agent_catalog SET sandbox_image = 'default' WHERE sandbox_image IS NULL OR sandbox_image = '';
UPDATE agents SET sandbox_image = 'default' WHERE sandbox_image IS NULL OR sandbox_image = '';

ALTER TABLE agent_catalog ADD CONSTRAINT agent_catalog_sandbox_image_valid CHECK (sandbox_image IN ('default', 'developer'));
ALTER TABLE agents ADD CONSTRAINT agents_sandbox_image_valid CHECK (sandbox_image IN ('default', 'developer'));

-- +goose Down
ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_sandbox_image_valid;
ALTER TABLE agent_catalog DROP CONSTRAINT IF EXISTS agent_catalog_sandbox_image_valid;
ALTER TABLE agents DROP COLUMN IF EXISTS sandbox_image;
ALTER TABLE agent_catalog DROP COLUMN IF EXISTS sandbox_image;
