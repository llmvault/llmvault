-- +goose Up
UPDATE database_connections
SET agent_id = NULL
WHERE agent_id IS NOT NULL;

ALTER TABLE ONLY database_connections
    DROP CONSTRAINT IF EXISTS fk_database_connections_agent;

DROP INDEX IF EXISTS idx_database_connections_agent_id;

ALTER TABLE database_connections
    DROP COLUMN IF EXISTS agent_id;

-- +goose Down
ALTER TABLE database_connections
    ADD COLUMN agent_id uuid;

CREATE INDEX idx_database_connections_agent_id
    ON database_connections USING btree (agent_id);

ALTER TABLE ONLY database_connections
    ADD CONSTRAINT fk_database_connections_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;
