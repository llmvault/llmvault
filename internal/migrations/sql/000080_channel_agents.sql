-- +goose Up
-- channel_agents: the set of agents assigned to a channel. A channel may have
-- MANY assigned agents; channels.default_agent_id is the primary/fallback and
-- is always one of them. Assignment is the hard gate for invoking an agent in a
-- channel (see internal/channelagents) — the ONLY exemption is system-kind
-- channels, which are handled in code, not here.
CREATE TABLE channel_agents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    agent_id uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    added_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT channel_agents_channel_agent_unique UNIQUE (channel_id, agent_id)
);

CREATE INDEX idx_channel_agents_org_agent ON channel_agents USING btree (org_id, agent_id);

-- +goose Down
DROP TABLE IF EXISTS channel_agents;
