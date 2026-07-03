-- +goose Up
-- Per-channel environment variables. Replaces the org-wide env var store that
-- lived on agents.encrypted_env_vars. Each var is scoped to exactly one channel
-- and holds a per-channel value; a var assigned to multiple channels is one row
-- per channel. Values are AES-256-GCM encrypted with the sandbox env key.
CREATE TABLE channel_env_vars (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL,
    channel_id      uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name            text NOT NULL,
    encrypted_value bytea NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_channel_env_vars_channel_name ON channel_env_vars (channel_id, name);
CREATE INDEX idx_channel_env_vars_channel ON channel_env_vars (channel_id);
CREATE INDEX idx_channel_env_vars_org ON channel_env_vars (org_id);

-- Drop the removed org-wide store.
ALTER TABLE agents DROP COLUMN IF EXISTS encrypted_env_vars;

-- +goose Down
ALTER TABLE agents ADD COLUMN encrypted_env_vars bytea;
DROP TABLE IF EXISTS channel_env_vars;
