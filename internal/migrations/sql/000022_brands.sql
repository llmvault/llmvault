-- +goose Up
CREATE TABLE public.brands (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    logos jsonb DEFAULT '{}'::jsonb NOT NULL,
    colors jsonb DEFAULT '{}'::jsonb NOT NULL,
    typography jsonb DEFAULT '{}'::jsonb NOT NULL,
    voice jsonb DEFAULT '{}'::jsonb NOT NULL,
    source jsonb DEFAULT '{"origin": "manual", "version": 1}'::jsonb NOT NULL,
    raw_import jsonb,
    archived_at timestamp with time zone,
    created_by uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT brands_pkey PRIMARY KEY (id);

CREATE INDEX idx_brands_archived_at ON public.brands USING btree (archived_at);

CREATE INDEX idx_brands_org_created ON public.brands USING btree (org_id, created_at DESC) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_brands_org_default_active ON public.brands USING btree (org_id) WHERE (is_default AND (archived_at IS NULL));

CREATE UNIQUE INDEX idx_brands_org_slug_active ON public.brands USING btree (org_id, slug) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT fk_brands_created_by FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT fk_brands_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.brands CASCADE;
