-- +goose Up
-- First-class database integrations for employee runtime database proxy access.

CREATE TABLE database_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid,
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

ALTER TABLE ONLY database_connections
    ADD CONSTRAINT fk_database_connections_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY database_connections
    ADD CONSTRAINT fk_database_connections_employee FOREIGN KEY (employee_id) REFERENCES employees(id) ON DELETE SET NULL;

CREATE INDEX idx_database_connections_org_provider ON database_connections USING btree (org_id, provider);
CREATE INDEX idx_database_connections_employee_id ON database_connections USING btree (employee_id);
CREATE INDEX idx_database_connections_active ON database_connections USING btree (org_id, provider) WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX idx_database_connections_one_active_provider
    ON database_connections (org_id, provider)
    WHERE revoked_at IS NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'database connections down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
