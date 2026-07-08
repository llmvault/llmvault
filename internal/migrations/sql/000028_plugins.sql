-- +goose Up
CREATE TABLE public.plugins (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category character varying(64) DEFAULT ''::character varying NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    icon_color text DEFAULT ''::text NOT NULL,
    developer text DEFAULT 'Hivy'::text NOT NULL,
    version text DEFAULT '1'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    source_hash text DEFAULT ''::text NOT NULL,
    manifest jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    org_id uuid
);

ALTER TABLE ONLY public.plugins
    ADD CONSTRAINT plugins_pkey PRIMARY KEY (id);

CREATE INDEX idx_plugins_category ON public.plugins USING btree (category);

CREATE INDEX idx_plugins_org_id ON public.plugins USING btree (org_id);

CREATE UNIQUE INDEX idx_plugins_org_slug ON public.plugins USING btree (org_id, slug) WHERE (org_id IS NOT NULL);

CREATE UNIQUE INDEX idx_plugins_slug_global ON public.plugins USING btree (slug) WHERE (org_id IS NULL);

CREATE INDEX idx_plugins_status ON public.plugins USING btree (status);

ALTER TABLE ONLY public.plugins
    ADD CONSTRAINT fk_plugins_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.plugins CASCADE;
