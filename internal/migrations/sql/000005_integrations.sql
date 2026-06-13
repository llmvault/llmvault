-- +goose Up
-- Integration catalog, user connections, and database connections

-- Integration catalog and user connection tables

CREATE TABLE connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    user_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    nango_connection_id text NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb,
    webhook_configured boolean DEFAULT true NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE integrations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    unique_key text NOT NULL,
    provider text NOT NULL,
    display_name text NOT NULL,
    org_id uuid,
    agent_id uuid,
    custom_app boolean DEFAULT false NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb,
    nango_config jsonb DEFAULT '{}'::jsonb,
    managed_by text DEFAULT ''::text NOT NULL,
    managed_id text DEFAULT ''::text NOT NULL,
    managed_hash text DEFAULT ''::text NOT NULL,
    required boolean DEFAULT false NOT NULL,
    supports_rag_source boolean DEFAULT false NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY connections
    ADD CONSTRAINT connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY integrations
    ADD CONSTRAINT integrations_pkey PRIMARY KEY (id);

CREATE INDEX idx_connections_integration_id ON connections USING btree (integration_id);

CREATE INDEX idx_connections_org_id ON connections USING btree (org_id);

CREATE INDEX idx_connections_user_id ON connections USING btree (user_id);

CREATE INDEX idx_integrations_custom_app ON integrations USING btree (custom_app);

CREATE INDEX idx_integrations_deleted_at ON integrations USING btree (deleted_at);

CREATE INDEX idx_integrations_agent_id ON integrations USING btree (agent_id);

CREATE INDEX idx_integrations_managed_by ON integrations USING btree (managed_by);

CREATE INDEX idx_integrations_managed_id ON integrations USING btree (managed_id);

CREATE INDEX idx_integrations_org_id ON integrations USING btree (org_id);

CREATE INDEX idx_integrations_provider ON integrations USING btree (provider);

CREATE UNIQUE INDEX idx_integrations_unique_key ON integrations USING btree (unique_key);

-- First-class database integrations for agent runtime database proxy access.

CREATE TABLE database_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid,
    provider varchar(32) NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    encrypted_dsn bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    schema_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    access_policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY database_connections
    ADD CONSTRAINT database_connections_pkey PRIMARY KEY (id);


CREATE INDEX idx_database_connections_org_provider ON database_connections USING btree (org_id, provider);
CREATE INDEX idx_database_connections_agent_id ON database_connections USING btree (agent_id);
CREATE INDEX idx_database_connections_active ON database_connections USING btree (org_id, provider) WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX idx_database_connections_one_active_provider
    ON database_connections (org_id, provider)
    WHERE revoked_at IS NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
