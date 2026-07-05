-- +goose Up
-- Optional human label for a trigger. Nullable so existing triggers are
-- unaffected; the API populates it for all newly-created triggers.
ALTER TABLE agent_triggers ADD COLUMN name text;

-- +goose Down
ALTER TABLE agent_triggers DROP COLUMN IF EXISTS name;
