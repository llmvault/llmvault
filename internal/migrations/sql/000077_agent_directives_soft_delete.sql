-- +goose Up
-- Rules (agent_directives) become soft-deleted: content is immutable and
-- deletes must preserve history so "which rules were active at time T" stays
-- answerable (as-of temporal reasoning / decision audit trails). Deleted rows
-- keep their content and timestamps; every read path filters them out.
ALTER TABLE agent_directives ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

-- +goose Down
ALTER TABLE agent_directives DROP COLUMN IF EXISTS deleted_at;
