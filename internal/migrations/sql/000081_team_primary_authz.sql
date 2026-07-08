-- +goose Up
-- Team-primary authorization foundation. Teams are the provisioning unit:
-- agents belong to a team, and plugins / knowledge sources are provisioned
-- per-team. "Manual, no backfill": existing rows get NO team assignment and are
-- unusable until an admin assigns them. There are intentionally zero backfill
-- INSERTs/UPDATEs in this migration.

-- agents.team_id: the owning team. NULLABLE and intentionally NOT backfilled — a
-- NULL team_id means the agent is unassigned and unusable until an admin sets it.
-- ON DELETE SET NULL so archiving/deleting a team unassigns its agents rather
-- than cascading a delete.
ALTER TABLE agents ADD COLUMN team_id uuid REFERENCES teams(id) ON DELETE SET NULL;
CREATE INDEX idx_agents_team_id ON agents USING btree (team_id);

-- team_plugins: plugins provisioned to a team. A team's agents may install only
-- the plugins granted to the team here.
CREATE TABLE team_plugins (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    team_id uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    plugin_id uuid NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    enabled_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT team_plugins_team_plugin_unique UNIQUE (team_id, plugin_id)
);
CREATE INDEX idx_team_plugins_org_team ON team_plugins USING btree (org_id, team_id);

-- team_rag_sources: knowledge (RAG) sources provisioned to a team. FK to
-- rag_sources(id) — a real table (see 000012_rag.sql) — mirroring
-- channel_rag_sources' ON DELETE CASCADE.
CREATE TABLE team_rag_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    team_id uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    rag_source_id uuid NOT NULL REFERENCES rag_sources(id) ON DELETE CASCADE,
    granted_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT team_rag_sources_team_source_unique UNIQUE (team_id, rag_source_id)
);
CREATE INDEX idx_team_rag_sources_org_team ON team_rag_sources USING btree (org_id, team_id);

-- INVARIANT (application-enforced, NOT a DB constraint): every channel_agents
-- row must reference an agent whose team_id equals the channel's team_id.
-- Enforcing this cross-table equality would require a trigger; per the design it
-- is enforced in Go (channel/agent assignment handlers + internal/channelagents),
-- deliberately NOT in the schema.

-- +goose Down
DROP TABLE IF EXISTS team_rag_sources;
DROP TABLE IF EXISTS team_plugins;
DROP INDEX IF EXISTS idx_agents_team_id;
ALTER TABLE agents DROP COLUMN IF EXISTS team_id;
