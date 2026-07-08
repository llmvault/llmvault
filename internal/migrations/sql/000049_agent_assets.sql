-- +goose Up
CREATE TABLE public.agent_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    path text NOT NULL,
    filename text NOT NULL,
    key text NOT NULL,
    public_url text NOT NULL,
    content_type text NOT NULL,
    bytes bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    description jsonb
);

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT agent_assets_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_agent_assets_key ON public.agent_assets USING btree (key);

CREATE INDEX idx_agent_assets_org_id ON public.agent_assets USING btree (org_id);

CREATE INDEX idx_emp_asset_agent_created ON public.agent_assets USING btree (agent_id, created_at DESC);

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT fk_agent_assets_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT fk_agent_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT fk_agent_assets_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;
