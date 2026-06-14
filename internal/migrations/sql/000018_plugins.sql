-- +goose Up
-- Plugin catalog and plugin-owned skills

CREATE TABLE plugins (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category varchar(64) DEFAULT ''::varchar NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    icon_color text DEFAULT ''::text NOT NULL,
    developer text DEFAULT 'Hivy'::text NOT NULL,
    version text DEFAULT '1'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    source_hash text DEFAULT ''::text NOT NULL,
    manifest jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY plugins
    ADD CONSTRAINT plugins_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_plugins_slug ON plugins USING btree (slug);
CREATE INDEX idx_plugins_category ON plugins USING btree (category);
CREATE INDEX idx_plugins_status ON plugins USING btree (status);

CREATE TABLE plugin_integrations (
    plugin_id uuid NOT NULL,
    provider text NOT NULL,
    kind text DEFAULT 'integration'::text NOT NULL,
    required boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY plugin_integrations
    ADD CONSTRAINT plugin_integrations_pkey PRIMARY KEY (plugin_id, provider, kind);

CREATE TABLE org_plugin_installs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    plugin_id uuid NOT NULL,
    created_by_user_id uuid,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY org_plugin_installs
    ADD CONSTRAINT org_plugin_installs_pkey PRIMARY KEY (id);

CREATE INDEX idx_org_plugin_installs_org_id ON org_plugin_installs USING btree (org_id);
CREATE INDEX idx_org_plugin_installs_plugin_id ON org_plugin_installs USING btree (plugin_id);
CREATE INDEX idx_org_plugin_installs_created_by_user_id ON org_plugin_installs USING btree (created_by_user_id);
CREATE INDEX idx_org_plugin_installs_revoked_at ON org_plugin_installs USING btree (revoked_at);
CREATE UNIQUE INDEX idx_org_plugin_installs_one_active
    ON org_plugin_installs (org_id, plugin_id)
    WHERE revoked_at IS NULL;

CREATE TABLE agent_plugin_installs (
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    plugin_id uuid NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY agent_plugin_installs
    ADD CONSTRAINT agent_plugin_installs_pkey PRIMARY KEY (agent_id, plugin_id);

CREATE INDEX idx_agent_plugin_installs_org_id ON agent_plugin_installs USING btree (org_id);
CREATE INDEX idx_agent_plugin_installs_plugin_id ON agent_plugin_installs USING btree (plugin_id);

ALTER TABLE skills
    ADD COLUMN plugin_id uuid;

CREATE INDEX idx_skills_plugin_id ON skills USING btree (plugin_id);

ALTER TABLE ONLY plugin_integrations
    ADD CONSTRAINT fk_plugin_integrations_plugin FOREIGN KEY (plugin_id) REFERENCES plugins(id) ON DELETE CASCADE;
ALTER TABLE ONLY org_plugin_installs
    ADD CONSTRAINT fk_org_plugin_installs_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY org_plugin_installs
    ADD CONSTRAINT fk_org_plugin_installs_plugin FOREIGN KEY (plugin_id) REFERENCES plugins(id) ON DELETE CASCADE;
ALTER TABLE ONLY org_plugin_installs
    ADD CONSTRAINT fk_org_plugin_installs_created_by FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE ONLY agent_plugin_installs
    ADD CONSTRAINT fk_agent_plugin_installs_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_plugin_installs
    ADD CONSTRAINT fk_agent_plugin_installs_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_plugin_installs
    ADD CONSTRAINT fk_agent_plugin_installs_plugin FOREIGN KEY (plugin_id) REFERENCES plugins(id) ON DELETE CASCADE;
ALTER TABLE ONLY skills
    ADD CONSTRAINT fk_skills_plugin FOREIGN KEY (plugin_id) REFERENCES plugins(id) ON DELETE CASCADE;

-- The app no longer creates standalone skills. Keep this nullable during
-- migration so existing development databases can upgrade; the application
-- enforces plugin ownership for new synced skills and removes standalone
-- skill routes.

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
