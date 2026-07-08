-- +goose Up
CREATE TABLE public.connections (
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

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT connections_pkey PRIMARY KEY (id);

CREATE INDEX idx_connections_integration_id ON public.connections USING btree (integration_id);

CREATE INDEX idx_connections_org_id ON public.connections USING btree (org_id);

CREATE INDEX idx_connections_user_id ON public.connections USING btree (user_id);

-- At most one live connection may own a given (integration_id, nango_connection_id)
-- so a forwarded webhook can never resolve ambiguously across orgs.
CREATE UNIQUE INDEX idx_connections_active_nango_id ON public.connections USING btree (integration_id, nango_connection_id) WHERE (revoked_at IS NULL);

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_integration FOREIGN KEY (integration_id) REFERENCES public.integrations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
