-- +goose Up
ALTER TABLE agent_triggers
    ADD COLUMN trigger_key text DEFAULT ''::text NOT NULL,
    ADD COLUMN trigger_value text DEFAULT ''::text NOT NULL;

CREATE INDEX idx_agent_triggers_trigger_key
    ON agent_triggers USING btree (trigger_key);

CREATE UNIQUE INDEX idx_agent_triggers_enabled_key_value
    ON agent_triggers USING btree (org_id, connection_id, channel_id, trigger_key, trigger_value)
    WHERE enabled = true AND trigger_key <> '' AND trigger_value <> '';

ALTER TABLE slack_thread_events
    ADD COLUMN trigger_id uuid;

CREATE INDEX idx_slack_thread_events_trigger_id
    ON slack_thread_events USING btree (trigger_id);

ALTER TABLE ONLY slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_trigger FOREIGN KEY (trigger_id) REFERENCES agent_triggers(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE ONLY slack_thread_events
    DROP CONSTRAINT IF EXISTS fk_slack_thread_events_trigger;

DROP INDEX IF EXISTS idx_slack_thread_events_trigger_id;

ALTER TABLE slack_thread_events
    DROP COLUMN IF EXISTS trigger_id;

DROP INDEX IF EXISTS idx_agent_triggers_enabled_key_value;
DROP INDEX IF EXISTS idx_agent_triggers_trigger_key;

ALTER TABLE agent_triggers
    DROP COLUMN IF EXISTS trigger_value,
    DROP COLUMN IF EXISTS trigger_key;
