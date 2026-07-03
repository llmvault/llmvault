-- +goose Up
-- Agents and sub-agents can declare skills the runtime preloads into a session
-- before the first model call (skill_view is invoked automatically). Stored as
-- the normalized object form [{"name": "<slug>", "files": ["<rel-path>", ...]}].
-- Empty '[]' means no auto-loaded skills. Sub-agent rows (type='subagent') use
-- the same agents.auto_load_skills column.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS auto_load_skills jsonb NOT NULL DEFAULT '[]';
ALTER TABLE agent_catalog ADD COLUMN IF NOT EXISTS auto_load_skills jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE agents DROP COLUMN IF EXISTS auto_load_skills;
ALTER TABLE agent_catalog DROP COLUMN IF EXISTS auto_load_skills;
