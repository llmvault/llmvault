-- +goose Up
DROP TABLE IF EXISTS hindsight_banks;

ALTER TABLE agents
    DROP COLUMN IF EXISTS last_memory_refreshed_at,
    DROP COLUMN IF EXISTS memory_refresh_status,
    DROP COLUMN IF EXISTS memory_refresh_error;

-- +goose Down
-- Irreversible removal: legacy memory service schema is intentionally not recreated.
