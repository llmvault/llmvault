-- +goose Up
-- User-created agents can own sub-agents stored as rows in the agents table.
-- Catalog agents keep their sub-agents in agent_catalog.sub_agents (unchanged).

ALTER TABLE agents
    ADD COLUMN type text DEFAULT 'agent'::text NOT NULL;

ALTER TABLE agents
    ADD CONSTRAINT agents_type_check
    CHECK (type IN ('agent', 'subagent'));

ALTER TABLE agents
    ADD COLUMN parent_agent_id uuid;

ALTER TABLE ONLY agents
    ADD CONSTRAINT fk_agents_parent_agent
    FOREIGN KEY (parent_agent_id) REFERENCES agents(id) ON DELETE CASCADE;

CREATE INDEX idx_agents_parent_agent_id ON agents USING btree (parent_agent_id);

-- Name uniqueness becomes two-tier: top-level agent names stay unique per org,
-- while sub-agent names only need to be unique within their parent (so two
-- different parents may each own a "Worker").
DROP INDEX IF EXISTS idx_agents_org_name;

CREATE UNIQUE INDEX idx_agents_org_name ON agents USING btree (org_id, name)
    WHERE parent_agent_id IS NULL;

CREATE UNIQUE INDEX idx_agents_parent_name ON agents USING btree (parent_agent_id, name)
    WHERE parent_agent_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_agents_parent_name;
DROP INDEX IF EXISTS idx_agents_org_name;
CREATE UNIQUE INDEX idx_agents_org_name ON agents USING btree (org_id, name);

DROP INDEX IF EXISTS idx_agents_parent_agent_id;

ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS fk_agents_parent_agent;

ALTER TABLE agents
    DROP COLUMN IF EXISTS parent_agent_id;

ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS agents_type_check;

ALTER TABLE agents
    DROP COLUMN IF EXISTS type;
