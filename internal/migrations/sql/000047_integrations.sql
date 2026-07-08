-- +goose Up
CREATE TABLE public.integrations (
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
    updated_at timestamp with time zone,
    bot_handle text DEFAULT ''::text NOT NULL
);

ALTER TABLE ONLY public.integrations
    ADD CONSTRAINT integrations_pkey PRIMARY KEY (id);

CREATE INDEX idx_integrations_agent_id ON public.integrations USING btree (agent_id);

CREATE INDEX idx_integrations_custom_app ON public.integrations USING btree (custom_app);

CREATE INDEX idx_integrations_deleted_at ON public.integrations USING btree (deleted_at);

CREATE INDEX idx_integrations_managed_by ON public.integrations USING btree (managed_by);

CREATE INDEX idx_integrations_managed_id ON public.integrations USING btree (managed_id);

CREATE INDEX idx_integrations_org_id ON public.integrations USING btree (org_id);

CREATE INDEX idx_integrations_provider ON public.integrations USING btree (provider);

CREATE UNIQUE INDEX idx_integrations_unique_key ON public.integrations USING btree (unique_key);

ALTER TABLE ONLY public.integrations
    ADD CONSTRAINT fk_integrations_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.integrations
    ADD CONSTRAINT fk_integrations_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.integrations CASCADE;
