-- +goose Up
ALTER TABLE agent_triggers
    ADD COLUMN source_slug text DEFAULT ''::text NOT NULL;

ALTER TABLE agent_schedules
    ADD COLUMN source_slug text DEFAULT ''::text NOT NULL;

CREATE UNIQUE INDEX idx_agent_triggers_agent_source_active
    ON agent_triggers USING btree (agent_id, source_slug)
    WHERE source_slug <> ''::text AND enabled = true;

CREATE UNIQUE INDEX idx_agent_schedules_agent_source_active
    ON agent_schedules USING btree (agent_id, source_slug)
    WHERE source_slug <> ''::text AND cancelled_at IS NULL AND status <> 'cancelled';

-- +goose Down
DROP INDEX IF EXISTS idx_agent_schedules_agent_source_active;
DROP INDEX IF EXISTS idx_agent_triggers_agent_source_active;

ALTER TABLE agent_schedules
    DROP COLUMN IF EXISTS source_slug;

ALTER TABLE agent_triggers
    DROP COLUMN IF EXISTS source_slug;
