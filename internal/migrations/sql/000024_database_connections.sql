-- +goose Up
CREATE TABLE public.database_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    provider character varying(32) NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    encrypted_dsn bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    schema_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    access_policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.database_connections
    ADD CONSTRAINT database_connections_pkey PRIMARY KEY (id);

CREATE INDEX idx_database_connections_active ON public.database_connections USING btree (org_id, provider) WHERE (revoked_at IS NULL);

CREATE UNIQUE INDEX idx_database_connections_one_active_provider ON public.database_connections USING btree (org_id, provider) WHERE (revoked_at IS NULL);

CREATE INDEX idx_database_connections_org_provider ON public.database_connections USING btree (org_id, provider);

ALTER TABLE ONLY public.database_connections
    ADD CONSTRAINT fk_database_connections_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
