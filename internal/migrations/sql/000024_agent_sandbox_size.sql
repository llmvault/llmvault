-- +goose Up
ALTER TABLE agents
    ADD COLUMN sandbox_size text DEFAULT 'small'::text NOT NULL;

ALTER TABLE agents
    ADD CONSTRAINT agents_sandbox_size_check
    CHECK (sandbox_size IN ('small', 'medium', 'large', 'xlarge'));

-- +goose Down
ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS agents_sandbox_size_check;

ALTER TABLE agents
    DROP COLUMN IF EXISTS sandbox_size;
