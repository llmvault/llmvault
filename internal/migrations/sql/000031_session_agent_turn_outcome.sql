-- +goose Up
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_turn_last_outcome text DEFAULT ''::text NOT NULL;

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS agent_turn_last_outcome;
