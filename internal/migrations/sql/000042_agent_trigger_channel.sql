-- +goose Up
ALTER TABLE agent_triggers
    ADD COLUMN channel_id uuid;

CREATE INDEX idx_agent_triggers_channel_id
    ON agent_triggers USING btree (channel_id);

ALTER TABLE ONLY agent_triggers
    ADD CONSTRAINT fk_agent_triggers_channel FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE ONLY agent_triggers
    DROP CONSTRAINT IF EXISTS fk_agent_triggers_channel;

DROP INDEX IF EXISTS idx_agent_triggers_channel_id;

ALTER TABLE agent_triggers
    DROP COLUMN IF EXISTS channel_id;
