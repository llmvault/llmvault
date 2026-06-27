-- +goose Up
ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS agents_sandbox_size_check;

ALTER TABLE agents
    ADD CONSTRAINT agents_sandbox_size_check
    CHECK (sandbox_size IN ('nano', 'small', 'medium', 'large', 'xlarge'));

UPDATE agents
SET sandbox_size = 'nano'
WHERE is_default = true
  AND status <> 'archived';

ALTER TABLE agents
    DROP COLUMN IF EXISTS sandbox_strategy;

ALTER TABLE agent_catalog
    DROP COLUMN IF EXISTS sandbox_strategy;

-- +goose Down
UPDATE agents
SET sandbox_size = 'small'
WHERE is_default = true
  AND sandbox_size = 'nano';

UPDATE agents
SET sandbox_size = 'small'
WHERE sandbox_size = 'nano';

ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS agents_sandbox_size_check;

ALTER TABLE agents
    ADD CONSTRAINT agents_sandbox_size_check
    CHECK (sandbox_size IN ('small', 'medium', 'large', 'xlarge'));
