-- +goose Up
-- Agent catalog and agent schema cleanup

CREATE TABLE agent_catalog (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category text DEFAULT ''::text NOT NULL,
    avatar_url text DEFAULT ''::text NOT NULL,
    developer text DEFAULT 'Hivy'::text NOT NULL,
    official boolean DEFAULT false NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    multimodal_model text DEFAULT ''::text NOT NULL,
    sandbox_strategy text DEFAULT 'per_session'::text NOT NULL,
    instructions text DEFAULT ''::text NOT NULL,
    required_plugins text[] DEFAULT '{}'::text[] NOT NULL,
    recommended_plugins text[] DEFAULT '{}'::text[] NOT NULL,
    manifest jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_hash text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY agent_catalog
    ADD CONSTRAINT agent_catalog_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_agent_catalog_slug ON agent_catalog USING btree (slug);
CREATE INDEX idx_agent_catalog_status ON agent_catalog USING btree (status);
CREATE INDEX idx_agent_catalog_default ON agent_catalog USING btree (is_default);

ALTER TABLE agents
    ADD COLUMN agent_catalog_id uuid;

CREATE INDEX idx_agents_agent_catalog_id ON agents USING btree (agent_catalog_id);
CREATE UNIQUE INDEX idx_agents_org_agent_catalog_active ON agents USING btree (org_id, agent_catalog_id)
WHERE agent_catalog_id IS NOT NULL AND status <> 'archived';

ALTER TABLE ONLY agents
    ADD CONSTRAINT fk_agents_agent_catalog
    FOREIGN KEY (agent_catalog_id) REFERENCES agent_catalog(id) ON DELETE SET NULL;

ALTER TABLE agents
    DROP COLUMN IF EXISTS shared_memory,
    DROP COLUMN IF EXISTS harness;

-- +goose Down
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS shared_memory boolean DEFAULT true NOT NULL,
    ADD COLUMN IF NOT EXISTS harness character varying(32) DEFAULT ''::character varying NOT NULL;

ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS fk_agents_agent_catalog;

DROP INDEX IF EXISTS idx_agents_org_agent_catalog_active;
DROP INDEX IF EXISTS idx_agents_agent_catalog_id;

ALTER TABLE agents
    DROP COLUMN IF EXISTS agent_catalog_id;

DROP TABLE IF EXISTS agent_catalog;
