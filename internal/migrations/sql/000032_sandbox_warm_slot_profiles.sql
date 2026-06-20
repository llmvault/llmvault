-- +goose Up
ALTER TABLE sandbox_warm_slots
    ADD COLUMN IF NOT EXISTS image_kind text DEFAULT 'default'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS sandbox_size text DEFAULT 'small'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS cpu integer DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS memory integer DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS disk integer DEFAULT 0 NOT NULL;

UPDATE sandbox_warm_slots
SET image_kind = 'developer'
WHERE runtime_image LIKE '%/hivy-sandboxes-runtime-developers:%'
   OR runtime_image LIKE '%/hivy-sandboxes-runtime-developers@%';

UPDATE sandbox_warm_slots
SET cpu = 1, memory = 2, disk = 10
WHERE sandbox_size = 'small' AND cpu = 0 AND memory = 0 AND disk = 0;

CREATE INDEX IF NOT EXISTS idx_sandbox_warm_slots_pool_profile_status
    ON sandbox_warm_slots (
        provider_id,
        mode,
        image_kind,
        runtime_image,
        sandbox_size,
        cpu,
        memory,
        disk,
        status,
        created_at
    );

-- +goose Down
DROP INDEX IF EXISTS idx_sandbox_warm_slots_pool_profile_status;

ALTER TABLE sandbox_warm_slots
    DROP COLUMN IF EXISTS disk,
    DROP COLUMN IF EXISTS memory,
    DROP COLUMN IF EXISTS cpu,
    DROP COLUMN IF EXISTS sandbox_size,
    DROP COLUMN IF EXISTS image_kind;
