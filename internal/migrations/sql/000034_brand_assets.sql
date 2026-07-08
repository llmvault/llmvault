-- +goose Up
CREATE TABLE public.brand_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    kind text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    key text NOT NULL,
    public_url text NOT NULL,
    content_type text NOT NULL,
    bytes bigint NOT NULL,
    width integer,
    height integer,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    CONSTRAINT brand_assets_bytes_check CHECK ((bytes > 0)),
    CONSTRAINT brand_assets_height_check CHECK (((height IS NULL) OR (height > 0))),
    CONSTRAINT brand_assets_kind_check CHECK ((kind = ANY (ARRAY['logo'::text, 'mark'::text, 'icon'::text, 'image'::text, 'font'::text, 'document'::text, 'other'::text]))),
    CONSTRAINT brand_assets_width_check CHECK (((width IS NULL) OR (width > 0)))
);

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT brand_assets_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_brand_assets_key ON public.brand_assets USING btree (key);

CREATE INDEX idx_brand_assets_kind ON public.brand_assets USING btree (kind);

CREATE INDEX idx_brand_assets_org_brand ON public.brand_assets USING btree (org_id, brand_id);

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT fk_brand_assets_brand FOREIGN KEY (brand_id) REFERENCES public.brands(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT fk_brand_assets_created_by FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT fk_brand_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
