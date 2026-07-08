-- +goose Up
CREATE TABLE public.drive_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size bigint NOT NULL,
    s3_key text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT drive_assets_pkey PRIMARY KEY (id);

CREATE INDEX idx_drive_asset_agent ON public.drive_assets USING btree (agent_id);

CREATE INDEX idx_drive_asset_org ON public.drive_assets USING btree (org_id);

CREATE UNIQUE INDEX idx_drive_assets_s3_key ON public.drive_assets USING btree (s3_key);

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT fk_drive_assets_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT fk_drive_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
