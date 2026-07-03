-- +goose Up
-- Agents can declare a default reasoning effort (low|medium|high) that seeds
-- new sessions when the caller does not pass an explicit reasoning_effort.
-- Nullable / empty means "unset" (the session falls back to its own default).
ALTER TABLE agents ADD COLUMN IF NOT EXISTS default_reasoning_effort text;
ALTER TABLE agent_catalog ADD COLUMN IF NOT EXISTS default_reasoning_effort text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agents DROP COLUMN IF EXISTS default_reasoning_effort;
ALTER TABLE agent_catalog DROP COLUMN IF EXISTS default_reasoning_effort;
